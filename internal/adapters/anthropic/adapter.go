// Package anthropicadapter wraps the Anthropic Messages API as a Lens Adapter.
// Use it when the system under test is a Claude model — e.g. to evaluate
// prompt variations or system prompt changes.
package anthropicadapter

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
)

var sessionCounter atomic.Int64

// Adapter calls the Anthropic Messages API, maintaining per-session
// conversation history so each turn sees the full prior context.
type Adapter struct {
	client anthropic.Client
	cfg    config.TargetConfig
	system string // contents of system_prompt_file, empty if not configured
}

// session holds the conversation history for a single eval case.
type session struct {
	id      string
	history []anthropic.MessageParam
}

func (s *session) ID() string { return s.id }

// New constructs an Anthropic Adapter. The API key is read from the
// ANTHROPIC_API_KEY environment variable (standard SDK default).
// If system_prompt_file is set in cfg, it is loaded once at construction time.
func New(cfg config.TargetConfig) (*Adapter, error) {
	var system string
	if cfg.SystemPromptFile != "" {
		data, err := os.ReadFile(cfg.SystemPromptFile)
		if err != nil {
			return nil, fmt.Errorf("anthropic adapter: reading system_prompt_file %q: %w", cfg.SystemPromptFile, err)
		}
		system = string(data)
	}

	client := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)
	return &Adapter{
		client: client,
		cfg:    cfg,
		system: system,
	}, nil
}

// CreateSession starts a new conversation. The SessionConfig is recorded in
// case future adapters need persona context; the Anthropic adapter ignores it
// (persona context is injected by the simulator prompt, not the target).
func (a *Adapter) CreateSession(_ context.Context, _ types.SessionConfig) (types.Session, error) {
	return &session{
		id: fmt.Sprintf("anthropic-%d", sessionCounter.Add(1)),
	}, nil
}

// SendTurn appends the user message to the conversation history, calls the
// Anthropic Messages API, appends the assistant reply, and returns the reply text.
func (a *Adapter) SendTurn(ctx context.Context, s types.Session, message string) (string, error) {
	sess := s.(*session)
	sess.history = append(sess.history, anthropic.NewUserMessage(anthropic.NewTextBlock(message)))

	maxTokens := int64(a.cfg.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 2048
	}

	params := anthropic.MessageNewParams{
		Model:     a.cfg.Model,
		MaxTokens: maxTokens,
		Messages:  sess.history,
	}
	if a.system != "" {
		params.System = []anthropic.TextBlockParam{{Text: a.system}}
	}

	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("anthropic adapter: messages.new: %w", err)
	}
	if len(msg.Content) == 0 {
		return "", fmt.Errorf("anthropic adapter: empty response content")
	}

	reply := msg.Content[0].Text
	sess.history = append(sess.history, anthropic.NewAssistantMessage(anthropic.NewTextBlock(reply)))
	return reply, nil
}

// CloseSession is a no-op — the Anthropic API is stateless and holds no
// server-side resources to release.
func (a *Adapter) CloseSession(_ context.Context, _ types.Session) error {
	return nil
}
