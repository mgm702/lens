package simulator_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgm702/lens/internal/simulator"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mock LLM ─────────────────────────────────────────────────────────────────

type mockLLM struct {
	responses   []string
	idx         int
	lastSystem  string
	lastMessage string // formatted user message received
}

func (m *mockLLM) Call(_ context.Context, system, userMessage string) (string, error) {
	m.lastSystem = system
	m.lastMessage = userMessage

	if m.idx >= len(m.responses) {
		return "default response", nil
	}
	resp := m.responses[m.idx]
	m.idx++
	return resp, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writePromptFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "sim.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func mkPersona() types.Persona {
	return types.Persona{
		Name:           "Alice",
		Summary:        "A first-generation student",
		EmotionalState: "nervous",
		Profile:        map[string]any{"gpa": "3.5", "major": "Biology"},
		Constraints:    map[string]any{"budget": "limited"},
		KnowledgeGaps:  []string{"scholarship options", "FAFSA process"},
	}
}

func mkScenario() types.Scenario {
	return types.Scenario{
		Name:            "Initial Advising",
		Description:     "Student needs help planning coursework.",
		OpeningMessage:  "Hi, I need help choosing my classes.",
		SuccessCriteria: []string{"student gets a schedule", "next steps clear"},
	}
}

// ── NextTurn tests ────────────────────────────────────────────────────────────

func TestSimulator_NextTurn_PassesThroughReply(t *testing.T) {
	mock := &mockLLM{responses: []string{"What classes should I take?"}}
	sim := simulator.NewWithCaller(mock, "system prompt")

	reply, err := sim.NextTurn(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "What classes should I take?", reply)
}

func TestSimulator_NextTurn_PassesSystemPrompt(t *testing.T) {
	mock := &mockLLM{responses: []string{"ok"}}
	sim := simulator.NewWithCaller(mock, "you are Alice")

	_, err := sim.NextTurn(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "you are Alice", mock.lastSystem)
}

func TestSimulator_NextTurn_FormatsHistory(t *testing.T) {
	history := []types.Turn{
		{Role: "user", Text: "Hello"},
		{Role: "assistant", Text: "Hi there"},
	}
	mock := &mockLLM{responses: []string{"follow-up"}}
	sim := simulator.NewWithCaller(mock, "")

	_, err := sim.NextTurn(context.Background(), history)
	require.NoError(t, err)
	// History should be formatted as labelled text in the user message.
	assert.Contains(t, mock.lastMessage, "User: Hello")
	assert.Contains(t, mock.lastMessage, "Advisor: Hi there")
}

func TestSimulator_NextTurn_EmptyHistoryPassesEmptyString(t *testing.T) {
	mock := &mockLLM{responses: []string{"opening"}}
	sim := simulator.NewWithCaller(mock, "")

	_, err := sim.NextTurn(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "", mock.lastMessage)
}

func TestSimulator_NextTurn_DoneSignal(t *testing.T) {
	mock := &mockLLM{responses: []string{"[DONE] Thanks for your help!"}}
	sim := simulator.NewWithCaller(mock, "")

	reply, err := sim.NextTurn(context.Background(), nil)
	require.NoError(t, err)
	assert.Contains(t, reply, "[DONE]")
}

func TestSimulator_NextTurn_DropoutSignal(t *testing.T) {
	mock := &mockLLM{responses: []string{"[DROPOUT]"}}
	sim := simulator.NewWithCaller(mock, "")

	reply, err := sim.NextTurn(context.Background(), nil)
	require.NoError(t, err)
	assert.Contains(t, reply, "[DROPOUT]")
}

func TestSimulator_NextTurn_MultiTurn(t *testing.T) {
	responses := []string{"First message", "Second message", "Third message"}
	mock := &mockLLM{responses: responses}
	sim := simulator.NewWithCaller(mock, "")

	for i, want := range responses {
		got, err := sim.NextTurn(context.Background(), nil)
		require.NoError(t, err, "turn %d", i)
		assert.Equal(t, want, got, "turn %d", i)
	}
}

// ── Prompt rendering tests ────────────────────────────────────────────────────

func TestRenderPrompt_BasicFields(t *testing.T) {
	dir := t.TempDir()
	tmpl := `Name: {{.Name}}
State: {{.EmotionalState}}
Summary: {{.Summary}}
Level: {{.InfoLevel}}`
	path := writePromptFile(t, dir, tmpl)

	result, err := simulator.ExportedRenderPrompt(path, mkPersona(), mkScenario(), "full")
	require.NoError(t, err)
	assert.Contains(t, result, "Name: Alice")
	assert.Contains(t, result, "State: nervous")
	assert.Contains(t, result, "Summary: A first-generation student")
	assert.Contains(t, result, "Level: full")
}

func TestRenderPrompt_PersonaProfile(t *testing.T) {
	dir := t.TempDir()
	path := writePromptFile(t, dir, "Profile:\n{{.ProfileText}}")

	result, err := simulator.ExportedRenderPrompt(path, mkPersona(), mkScenario(), "partial")
	require.NoError(t, err)
	assert.Contains(t, result, "gpa: 3.5")
	assert.Contains(t, result, "major: Biology")
}

func TestRenderPrompt_KnowledgeGaps(t *testing.T) {
	dir := t.TempDir()
	path := writePromptFile(t, dir, "Gaps:\n{{.KnowledgeGapsText}}")

	result, err := simulator.ExportedRenderPrompt(path, mkPersona(), mkScenario(), "none")
	require.NoError(t, err)
	assert.Contains(t, result, "- scholarship options")
	assert.Contains(t, result, "- FAFSA process")
}

func TestRenderPrompt_ScenarioFields(t *testing.T) {
	dir := t.TempDir()
	path := writePromptFile(t, dir,
		"Scenario: {{.ScenarioName}}\n{{.ScenarioDescription}}\nOpening: {{.OpeningMessage}}\nCriteria:\n{{.SuccessCriteriaText}}")

	result, err := simulator.ExportedRenderPrompt(path, mkPersona(), mkScenario(), "full")
	require.NoError(t, err)
	assert.Contains(t, result, "Scenario: Initial Advising")
	assert.Contains(t, result, "Student needs help")
	assert.Contains(t, result, "Opening: Hi, I need help choosing my classes.")
	assert.Contains(t, result, "- student gets a schedule")
}

func TestRenderPrompt_EmptyPersonaFields(t *testing.T) {
	dir := t.TempDir()
	path := writePromptFile(t, dir, "Profile:\n{{.ProfileText}}\nConstraints:\n{{.ConstraintsText}}")

	bare := types.Persona{Name: "Bob"}
	result, err := simulator.ExportedRenderPrompt(path, bare, mkScenario(), "full")
	require.NoError(t, err)
	_ = result
}

func TestRenderPrompt_MissingFile(t *testing.T) {
	_, err := simulator.ExportedRenderPrompt("/nonexistent/sim.md", mkPersona(), mkScenario(), "full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sim.md")
}

// ── formatHistory tests ───────────────────────────────────────────────────────

func TestFormatHistory_Empty(t *testing.T) {
	result := simulator.ExportedFormatHistory(nil)
	assert.Equal(t, "", result)
}

func TestFormatHistory_SingleUserTurn(t *testing.T) {
	history := []types.Turn{{Role: "user", Text: "Hello"}}
	result := simulator.ExportedFormatHistory(history)
	assert.Contains(t, result, "User: Hello")
}

func TestFormatHistory_AlternatingTurns(t *testing.T) {
	history := []types.Turn{
		{Role: "user", Text: "Hi"},
		{Role: "assistant", Text: "Hello there"},
		{Role: "user", Text: "How are you?"},
	}
	result := simulator.ExportedFormatHistory(history)
	assert.Contains(t, result, "User: Hi")
	assert.Contains(t, result, "Advisor: Hello there")
	assert.Contains(t, result, "User: How are you?")
	assert.Less(t, strings.Index(result, "User: Hi"), strings.Index(result, "Advisor:"))
}
