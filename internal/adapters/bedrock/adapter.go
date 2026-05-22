// Package bedrockadapter wraps the AWS Bedrock Converse API as a Lens Adapter.
// Use it when the system under test is a foundation model hosted on Bedrock
// (Llama, Mistral, Amazon Titan, etc.) or a fine-tuned model via Bedrock.
package bedrockadapter

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
)

var sessionCounter atomic.Int64

// Adapter calls the AWS Bedrock Converse API, maintaining per-session
// conversation history for multi-turn evaluations.
type Adapter struct {
	client *bedrockruntime.Client
	cfg    config.TargetConfig
	system string // contents of system_prompt_file, empty if not configured
}

// session holds the conversation history for a single eval case.
type session struct {
	id      string
	history []bedrocktypes.Message
}

func (s *session) ID() string { return s.id }

// New constructs a Bedrock Adapter. AWS credentials are resolved using the
// standard AWS credential chain (env vars, ~/.aws/credentials, IAM role, etc.).
// If system_prompt_file is set in cfg, it is loaded once at construction time.
func New(cfg config.TargetConfig) (*Adapter, error) {
	var system string
	if cfg.SystemPromptFile != "" {
		data, err := os.ReadFile(cfg.SystemPromptFile)
		if err != nil {
			return nil, fmt.Errorf("bedrock adapter: reading system_prompt_file %q: %w", cfg.SystemPromptFile, err)
		}
		system = string(data)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("bedrock adapter: loading AWS config: %w", err)
	}
	client := bedrockruntime.NewFromConfig(awsCfg)

	return &Adapter{
		client: client,
		cfg:    cfg,
		system: system,
	}, nil
}

// CreateSession starts a new conversation with an empty history.
func (a *Adapter) CreateSession(_ context.Context, _ types.SessionConfig) (types.Session, error) {
	return &session{
		id: fmt.Sprintf("bedrock-%d", sessionCounter.Add(1)),
	}, nil
}

// SendTurn appends the user message to history, calls the Bedrock Converse
// API, appends the assistant reply, and returns the reply text.
func (a *Adapter) SendTurn(ctx context.Context, s types.Session, message string) (string, error) {
	sess := s.(*session)
	sess.history = append(sess.history, bedrocktypes.Message{
		Role: bedrocktypes.ConversationRoleUser,
		Content: []bedrocktypes.ContentBlock{
			&bedrocktypes.ContentBlockMemberText{Value: message},
		},
	})

	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(a.cfg.Model),
		Messages: sess.history,
	}

	if a.system != "" {
		input.System = []bedrocktypes.SystemContentBlock{
			&bedrocktypes.SystemContentBlockMemberText{Value: a.system},
		}
	}

	if a.cfg.MaxTokens > 0 {
		maxT := int32(a.cfg.MaxTokens)
		input.InferenceConfig = &bedrocktypes.InferenceConfiguration{
			MaxTokens: &maxT,
		}
	}

	output, err := a.client.Converse(ctx, input)
	if err != nil {
		return "", fmt.Errorf("bedrock adapter: converse: %w", err)
	}

	reply, err := extractReply(output.Output)
	if err != nil {
		return "", err
	}

	sess.history = append(sess.history, bedrocktypes.Message{
		Role: bedrocktypes.ConversationRoleAssistant,
		Content: []bedrocktypes.ContentBlock{
			&bedrocktypes.ContentBlockMemberText{Value: reply},
		},
	})
	return reply, nil
}

// CloseSession is a no-op — Bedrock is stateless and holds no server-side
// resources to release.
func (a *Adapter) CloseSession(_ context.Context, _ types.Session) error {
	return nil
}

// extractReply pulls the first text block from a ConverseOutput.
func extractReply(out bedrocktypes.ConverseOutput) (string, error) {
	msg, ok := out.(*bedrocktypes.ConverseOutputMemberMessage)
	if !ok {
		return "", fmt.Errorf("bedrock adapter: unexpected output type %T", out)
	}
	if len(msg.Value.Content) == 0 {
		return "", fmt.Errorf("bedrock adapter: empty response content")
	}
	textBlock, ok := msg.Value.Content[0].(*bedrocktypes.ContentBlockMemberText)
	if !ok {
		return "", fmt.Errorf("bedrock adapter: expected text content block, got %T", msg.Value.Content[0])
	}
	return textBlock.Value, nil
}
