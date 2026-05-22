package bedrockadapter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bedrockadapter "github.com/mgm702/lens/internal/adapters/bedrock"
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseCfg() config.TargetConfig {
	return config.TargetConfig{
		Type:      "bedrock",
		Model:     "amazon.titan-text-lite-v1",
		MaxTokens: 256,
	}
}

// TestBedrockAdapter_New_NoSystemPrompt verifies successful construction
// without a system prompt file (no AWS API call is made at this point).
func TestBedrockAdapter_New_NoSystemPrompt(t *testing.T) {
	a, err := bedrockadapter.New(baseCfg())
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestBedrockAdapter_New_WithSystemPrompt verifies the system prompt file
// is loaded at construction time.
func TestBedrockAdapter_New_WithSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sys.md")
	require.NoError(t, os.WriteFile(path, []byte("You are a helpful assistant."), 0644))

	cfg := baseCfg()
	cfg.SystemPromptFile = path
	a, err := bedrockadapter.New(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestBedrockAdapter_New_MissingSystemPromptFile returns an error when
// system_prompt_file points to a non-existent file.
func TestBedrockAdapter_New_MissingSystemPromptFile(t *testing.T) {
	cfg := baseCfg()
	cfg.SystemPromptFile = "/nonexistent/system.md"
	_, err := bedrockadapter.New(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system_prompt_file")
}

// TestBedrockAdapter_CreateSession verifies each call returns a session with a
// unique non-empty ID.
func TestBedrockAdapter_CreateSession(t *testing.T) {
	a, err := bedrockadapter.New(baseCfg())
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

// TestBedrockAdapter_CloseSession verifies CloseSession is a no-op.
func TestBedrockAdapter_CloseSession(t *testing.T) {
	a, err := bedrockadapter.New(baseCfg())
	require.NoError(t, err)
	s, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)
	assert.NoError(t, a.CloseSession(context.Background(), s))
}

// TestBedrockAdapter_SendTurn_Integration calls the real Bedrock Converse API.
// Skipped unless LENS_INTEGRATION_TESTS=1 and AWS credentials are configured.
func TestBedrockAdapter_SendTurn_Integration(t *testing.T) {
	if os.Getenv("LENS_INTEGRATION_TESTS") == "" {
		t.Skip("set LENS_INTEGRATION_TESTS=1 to run integration tests")
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		t.Skip("no AWS credentials configured")
	}

	a, err := bedrockadapter.New(baseCfg())
	require.NoError(t, err)

	s, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)

	reply, err := a.SendTurn(context.Background(), s, "Say hello in one word.")
	require.NoError(t, err)
	assert.NotEmpty(t, reply)
	t.Logf("reply: %s", reply)
}
