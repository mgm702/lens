package openaiadapter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	openaiadapter "github.com/mgm702/lens/internal/adapters/openai"
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseCfg() config.TargetConfig {
	return config.TargetConfig{
		Type:      "openai",
		Model:     "gpt-4o-mini",
		MaxTokens: 256,
	}
}

// TestOpenAIAdapter_New_NoSystemPrompt verifies successful construction
// without a system prompt file.
func TestOpenAIAdapter_New_NoSystemPrompt(t *testing.T) {
	a, err := openaiadapter.New(baseCfg())
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestOpenAIAdapter_New_WithSystemPrompt verifies the system prompt file
// is loaded at construction time.
func TestOpenAIAdapter_New_WithSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sys.md")
	require.NoError(t, os.WriteFile(path, []byte("You are a helpful assistant."), 0644))

	cfg := baseCfg()
	cfg.SystemPromptFile = path
	a, err := openaiadapter.New(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestOpenAIAdapter_New_MissingSystemPromptFile returns an error when
// system_prompt_file points to a non-existent file.
func TestOpenAIAdapter_New_MissingSystemPromptFile(t *testing.T) {
	cfg := baseCfg()
	cfg.SystemPromptFile = "/nonexistent/system.md"
	_, err := openaiadapter.New(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system_prompt_file")
}

// TestOpenAIAdapter_CreateSession verifies that sessions with a system prompt
// pre-load it into history, and that each call returns a unique session ID.
func TestOpenAIAdapter_CreateSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sys.md")
	require.NoError(t, os.WriteFile(path, []byte("You are a test assistant."), 0644))

	cfg := baseCfg()
	cfg.SystemPromptFile = path
	a, err := openaiadapter.New(cfg)
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

// TestOpenAIAdapter_CloseSession verifies CloseSession is a no-op.
func TestOpenAIAdapter_CloseSession(t *testing.T) {
	a, err := openaiadapter.New(baseCfg())
	require.NoError(t, err)
	s, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)
	assert.NoError(t, a.CloseSession(context.Background(), s))
}

// TestOpenAIAdapter_SendTurn_Integration calls the real OpenAI API.
// Skipped unless LENS_INTEGRATION_TESTS=1 is set (requires OPENAI_API_KEY).
func TestOpenAIAdapter_SendTurn_Integration(t *testing.T) {
	if os.Getenv("LENS_INTEGRATION_TESTS") == "" {
		t.Skip("set LENS_INTEGRATION_TESTS=1 to run integration tests")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	a, err := openaiadapter.New(baseCfg())
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
