// Package openaiadapter wraps the OpenAI Chat Completions API as a Lens Adapter.
// Use it when the system under test is a GPT model, or when you want to use
// any OpenAI-compatible endpoint (local models, Azure OpenAI, etc.).
package openaiadapter

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
)

var sessionCounter atomic.Int64

// Adapter calls the OpenAI Chat Completions API, maintaining per-session
// conversation history so each turn sees the full prior context.
type Adapter struct {
	client openai.Client
	cfg    config.TargetConfig
	system string // contents of system_prompt_file, empty if not configured
}

// session holds the conversation history for a single eval case.
type session struct {
	id      string
	history []openai.ChatCompletionMessageParamUnion
}

func (s *session) ID() string { return s.id }

// New constructs an OpenAI Adapter. The API key is read from the
// OPENAI_API_KEY environment variable (standard SDK default).
// If system_prompt_file is set in cfg, it is loaded once at construction time.
// For OpenAI-compatible endpoints (Azure, local), set the base URL via
// OPENAI_BASE_URL or pass option.WithBaseURL in a future config extension.
func New(cfg config.TargetConfig) (*Adapter, error) {
	var system string
	if cfg.SystemPromptFile != "" {
		data, err := os.ReadFile(cfg.SystemPromptFile)
		if err != nil {
			return nil, fmt.Errorf("openai adapter: reading system_prompt_file %q: %w", cfg.SystemPromptFile, err)
		}
		system = string(data)
	}

	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
	)
	return &Adapter{
		client: client,
		cfg:    cfg,
		system: system,
	}, nil
}

// CreateSession starts a new conversation. If a system prompt is configured
// it is pre-loaded as the first message in history so every turn sees it.
func (a *Adapter) CreateSession(_ context.Context, _ types.SessionConfig) (types.Session, error) {
	var history []openai.ChatCompletionMessageParamUnion
	if a.system != "" {
		history = append(history, openai.SystemMessage(a.system))
	}
	return &session{
		id:      fmt.Sprintf("openai-%d", sessionCounter.Add(1)),
		history: history,
	}, nil
}

// SendTurn appends the user message to the conversation history, calls the
// OpenAI Chat Completions API, appends the assistant reply, and returns the
// reply text.
func (a *Adapter) SendTurn(ctx context.Context, s types.Session, message string) (string, error) {
	sess := s.(*session)
	sess.history = append(sess.history, openai.UserMessage(message))

	maxTokens := int64(a.cfg.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 2048
	}

	resp, err := a.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:     a.cfg.Model,
		Messages:  sess.history,
		MaxTokens: openai.Int(maxTokens),
	})
	if err != nil {
		return "", fmt.Errorf("openai adapter: chat.completions.new: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai adapter: empty choices in response")
	}

	reply := resp.Choices[0].Message.Content
	sess.history = append(sess.history, openai.AssistantMessage(reply))
	return reply, nil
}

// CloseSession is a no-op — the OpenAI API is stateless and holds no
// server-side resources to release.
func (a *Adapter) CloseSession(_ context.Context, _ types.Session) error {
	return nil
}
