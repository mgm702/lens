package anthropicadapter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	anthropicadapter "github.com/mgm702/lens/internal/adapters/anthropic"
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseCfg() config.TargetConfig {
	return config.TargetConfig{
		Type:      "anthropic",
		Model:     "claude-haiku-4-5",
		MaxTokens: 256,
	}
}

// TestAnthropicAdapter_New_NoSystemPrompt verifies successful construction
// without a system prompt file.
func TestAnthropicAdapter_New_NoSystemPrompt(t *testing.T) {
	a, err := anthropicadapter.New(baseCfg())
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestAnthropicAdapter_New_WithSystemPrompt verifies the system prompt file
// is loaded at construction time.
func TestAnthropicAdapter_New_WithSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sys.md")
	require.NoError(t, os.WriteFile(path, []byte("You are a helpful assistant."), 0644))

	cfg := baseCfg()
	cfg.SystemPromptFile = path
	a, err := anthropicadapter.New(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestAnthropicAdapter_New_MissingSystemPromptFile returns an error when
// system_prompt_file points to a non-existent file.
func TestAnthropicAdapter_New_MissingSystemPromptFile(t *testing.T) {
	cfg := baseCfg()
	cfg.SystemPromptFile = "/nonexistent/system.md"
	_, err := anthropicadapter.New(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system_prompt_file")
}

// TestAnthropicAdapter_CreateSession verifies that each call returns a session
// with a unique, non-empty ID.
func TestAnthropicAdapter_CreateSession(t *testing.T) {
	a, err := anthropicadapter.New(baseCfg())
	require.NoError(t, err)

	seen := make(map[string]bool)
	for range 5 {
		s, err := a.CreateSession(context.Background(), types.SessionConfig{})
		require.NoError(t, err)
		require.NotEmpty(t, s.ID())
		assert.False(t, seen[s.ID()], "duplicate session ID: %s", s.ID())
		seen[s.ID()] = true
	}
}

// TestAnthropicAdapter_CloseSession verifies CloseSession is a no-op.
func TestAnthropicAdapter_CloseSession(t *testing.T) {
	a, err := anthropicadapter.New(baseCfg())
	require.NoError(t, err)
	s, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)
	assert.NoError(t, a.CloseSession(context.Background(), s))
}

// TestAnthropicAdapter_SendTurn_Integration calls the real Anthropic API.
// Skipped unless LENS_INTEGRATION_TESTS=1 is set (requires ANTHROPIC_API_KEY).
func TestAnthropicAdapter_SendTurn_Integration(t *testing.T) {
	if os.Getenv("LENS_INTEGRATION_TESTS") == "" {
		t.Skip("set LENS_INTEGRATION_TESTS=1 to run integration tests")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	a, err := anthropicadapter.New(baseCfg())
	require.NoError(t, err)

	s, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)

	reply, err := a.SendTurn(context.Background(), s, "Say exactly: hello")
	require.NoError(t, err)
	assert.NotEmpty(t, reply)
	t.Logf("reply: %s", reply)

	// Second turn should see the history.
	reply2, err := a.SendTurn(context.Background(), s, "What did I just ask you to say?")
	require.NoError(t, err)
	assert.NotEmpty(t, reply2)
	t.Logf("reply2: %s", reply2)
}
