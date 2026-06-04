// Package llm provides a uniform text-in / text-out interface for calling
// any supported LLM provider, along with a factory function that selects the
// right implementation from a config.LLMConfig.
//
// Both the Simulator and the Judge use this package so that provider-specific
// SDK code lives in exactly one place.
package llm

import (
	"context"
	"fmt"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/openai/openai-go"
	openaiopt "github.com/openai/openai-go/option"

	"github.com/mgm702/lens/internal/config"
)

// Caller is the single interface used to call an LLM for text generation.
// system is the system/instruction prompt; userMessage is the user turn content.
// The caller returns the model's text reply.
type Caller interface {
	Call(ctx context.Context, system, userMessage string) (string, error)
}

// New constructs the appropriate Caller for the provider named in cfg.
func New(cfg config.LLMConfig) (Caller, error) {
	switch cfg.Provider {
	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("llm: ANTHROPIC_API_KEY is not set")
		}
		client := anthropic.NewClient(option.WithAPIKey(apiKey))
		return &anthropicCaller{client: client, cfg: cfg}, nil
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("llm: OPENAI_API_KEY is not set")
		}
		client := openai.NewClient(openaiopt.WithAPIKey(apiKey))
		return &openaiCaller{client: client, cfg: cfg}, nil
	case "bedrock":
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, fmt.Errorf("llm: loading AWS config: %w", err)
		}
		return &bedrockCaller{client: bedrockruntime.NewFromConfig(awsCfg), cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q; must be one of: anthropic, openai, bedrock", cfg.Provider)
	}
}

// maxTokens returns n if positive, otherwise the given fallback.
func safeMax(n, fallback int) int64 {
	if n > 0 {
		return int64(n)
	}
	return int64(fallback)
}

// ── Anthropic ─────────────────────────────────────────────────────────────────

type anthropicCaller struct {
	client anthropic.Client
	cfg    config.LLMConfig
}

func (c *anthropicCaller) Call(ctx context.Context, system, userMessage string) (string, error) {
	params := anthropic.MessageNewParams{
		Model:     c.cfg.Model,
		MaxTokens: safeMax(c.cfg.MaxTokens, 1024),
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage))},
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	msg, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("llm (anthropic): %w", err)
	}
	if len(msg.Content) == 0 {
		return "", fmt.Errorf("llm (anthropic): empty response")
	}
	return msg.Content[0].Text, nil
}

// ── OpenAI ────────────────────────────────────────────────────────────────────

type openaiCaller struct {
	client openai.Client
	cfg    config.LLMConfig
}

func (c *openaiCaller) Call(ctx context.Context, system, userMessage string) (string, error) {
	msgs := []openai.ChatCompletionMessageParamUnion{}
	if system != "" {
		msgs = append(msgs, openai.SystemMessage(system))
	}
	msgs = append(msgs, openai.UserMessage(userMessage))

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:     c.cfg.Model,
		Messages:  msgs,
		MaxTokens: openai.Int(safeMax(c.cfg.MaxTokens, 1024)),
	})
	if err != nil {
		return "", fmt.Errorf("llm (openai): %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("llm (openai): empty choices")
	}
	return resp.Choices[0].Message.Content, nil
}

// ── Bedrock ───────────────────────────────────────────────────────────────────

type bedrockCaller struct {
	client *bedrockruntime.Client
	cfg    config.LLMConfig
}

func (c *bedrockCaller) Call(ctx context.Context, system, userMessage string) (string, error) {
	maxT := int32(safeMax(c.cfg.MaxTokens, 1024))
	input := &bedrockruntime.ConverseInput{
		ModelId: aws.String(c.cfg.Model),
		Messages: []bedrocktypes.Message{{
			Role: bedrocktypes.ConversationRoleUser,
			Content: []bedrocktypes.ContentBlock{
				&bedrocktypes.ContentBlockMemberText{Value: userMessage},
			},
		}},
		InferenceConfig: &bedrocktypes.InferenceConfiguration{MaxTokens: &maxT},
	}
	if system != "" {
		input.System = []bedrocktypes.SystemContentBlock{
			&bedrocktypes.SystemContentBlockMemberText{Value: system},
		}
	}
	output, err := c.client.Converse(ctx, input)
	if err != nil {
		return "", fmt.Errorf("llm (bedrock): %w", err)
	}
	msg, ok := output.Output.(*bedrocktypes.ConverseOutputMemberMessage)
	if !ok || len(msg.Value.Content) == 0 {
		return "", fmt.Errorf("llm (bedrock): unexpected or empty output")
	}
	tb, ok := msg.Value.Content[0].(*bedrocktypes.ContentBlockMemberText)
	if !ok {
		return "", fmt.Errorf("llm (bedrock): expected text block")
	}
	return tb.Value, nil
}
