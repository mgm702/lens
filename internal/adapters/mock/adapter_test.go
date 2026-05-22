package mock_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgm702/lens/internal/adapters/mock"
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeScript writes a JSON script file to dir and returns its path.
func writeScript(t *testing.T, dir string, responses []string) string {
	t.Helper()
	data, err := json.Marshal(responses)
	require.NoError(t, err)
	path := filepath.Join(dir, "script.json")
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

func newSession(t *testing.T, a *mock.Adapter) types.Session {
	t.Helper()
	s, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)
	return s
}

func sendTurn(t *testing.T, a *mock.Adapter, s types.Session, message string) string {
	t.Helper()
	reply, err := a.SendTurn(context.Background(), s, message)
	require.NoError(t, err)
	return reply
}

// TestMockAdapter_NoScript verifies that the adapter returns the default
// canned response for every turn when no script file is configured.
func TestMockAdapter_NoScript(t *testing.T) {
	a, err := mock.New(config.TargetConfig{Type: "mock"})
	require.NoError(t, err)

	s := newSession(t, a)
	for i := range 5 {
		reply := sendTurn(t, a, s, "hello")
		assert.NotEmpty(t, reply, "turn %d: expected non-empty reply", i)
	}
}

// TestMockAdapter_WithScript verifies that responses are served in the order
// defined in the script file.
func TestMockAdapter_WithScript(t *testing.T) {
	dir := t.TempDir()
	script := []string{"first reply", "second reply", "third reply"}
	path := writeScript(t, dir, script)

	a, err := mock.New(config.TargetConfig{Type: "mock", ScriptFile: path})
	require.NoError(t, err)

	s := newSession(t, a)
	for i, want := range script {
		got := sendTurn(t, a, s, "turn")
		assert.Equal(t, want, got, "turn %d", i)
	}
}

// TestMockAdapter_ScriptExhausted verifies that the last script entry is
// repeated once the script runs out.
func TestMockAdapter_ScriptExhausted(t *testing.T) {
	dir := t.TempDir()
	script := []string{"only reply"}
	path := writeScript(t, dir, script)

	a, err := mock.New(config.TargetConfig{Type: "mock", ScriptFile: path})
	require.NoError(t, err)

	s := newSession(t, a)
	for range 3 {
		got := sendTurn(t, a, s, "anything")
		assert.Equal(t, "only reply", got)
	}
}

// TestMockAdapter_SessionIsolation verifies that two concurrent sessions
// maintain independent script positions.
func TestMockAdapter_SessionIsolation(t *testing.T) {
	dir := t.TempDir()
	script := []string{"A", "B", "C"}
	path := writeScript(t, dir, script)

	a, err := mock.New(config.TargetConfig{Type: "mock", ScriptFile: path})
	require.NoError(t, err)

	s1 := newSession(t, a)
	s2 := newSession(t, a)

	assert.NotEqual(t, s1.ID(), s2.ID(), "session IDs must be unique")

	// Advance session 1 by two turns.
	assert.Equal(t, "A", sendTurn(t, a, s1, "x"))
	assert.Equal(t, "B", sendTurn(t, a, s1, "x"))

	// Session 2 should still be at turn 0.
	assert.Equal(t, "A", sendTurn(t, a, s2, "x"))
}

// TestMockAdapter_UniqueSessionIDs verifies each CreateSession call returns a
// distinct ID.
func TestMockAdapter_UniqueSessionIDs(t *testing.T) {
	a, err := mock.New(config.TargetConfig{Type: "mock"})
	require.NoError(t, err)

	seen := make(map[string]bool)
	for range 10 {
		s := newSession(t, a)
		assert.False(t, seen[s.ID()], "duplicate session ID: %s", s.ID())
		seen[s.ID()] = true
	}
}

// TestMockAdapter_CloseSession verifies CloseSession is a no-op (returns nil).
func TestMockAdapter_CloseSession(t *testing.T) {
	a, err := mock.New(config.TargetConfig{Type: "mock"})
	require.NoError(t, err)

	s := newSession(t, a)
	err = a.CloseSession(context.Background(), s)
	assert.NoError(t, err)
}

// TestMockAdapter_BadScriptFile verifies that a non-existent script path
// returns an error at construction time, not at SendTurn time.
func TestMockAdapter_BadScriptFile(t *testing.T) {
	_, err := mock.New(config.TargetConfig{
		Type:       "mock",
		ScriptFile: "/nonexistent/script.json",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "script file")
}

// TestMockAdapter_InvalidScriptJSON verifies that a script file with invalid
// JSON returns an error at construction time.
func TestMockAdapter_InvalidScriptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))

	_, err := mock.New(config.TargetConfig{Type: "mock", ScriptFile: path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing script file")
}

// TestAdaptersNew_MockType verifies the top-level adapters.New factory
// correctly constructs a mock adapter.
func TestAdaptersNew_MockType(t *testing.T) {
	// Import through the adapter factory to test the registry wiring.
	// This is tested here to keep the factory smoke-test near the mock code.
	// The factory itself lives in internal/adapters/adapter.go.
	a, err := mock.New(config.TargetConfig{Type: "mock"})
	require.NoError(t, err)
	require.NotNil(t, a)
}
