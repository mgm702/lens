package judge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mgm702/lens/internal/judge"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mock LLM ─────────────────────────────────────────────────────────────────

type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Call(_ context.Context, _, _ string) (string, error) {
	return m.response, m.err
}

// ── Fixtures ──────────────────────────────────────────────────────────────────

func mkRubric() *types.Rubric {
	return &types.Rubric{
		HardGates: []types.OutcomeDef{
			{ID: "no_harm", Type: "conversation", Description: "No harmful advice given"},
		},
		Outcomes: []types.OutcomeDef{
			{ID: "concrete_step", Type: "item_level", Description: "Gave actionable next step"},
			{ID: "empathy", Type: "conversation", Description: "Showed empathy"},
		},
		Indicators: []types.IndicatorDef{
			{ID: "clarity", Scale: 5, Description: "Clarity", AnchorLow: "unclear", AnchorHigh: "crystal clear"},
		},
	}
}

func mkTranscript() types.Transcript {
	return types.Transcript{
		Turns: []types.Turn{
			{Role: "user", Text: "Hi, I need help."},
			{Role: "assistant", Text: "Sure! You should apply by March 1st."},
			{Role: "user", Text: "Thanks!"},
		},
		EndReason: types.EndReasonDone,
	}
}

func mkPersona() types.Persona  { return types.Persona{Name: "Alice", Summary: "Student"} }
func mkScenario() types.Scenario { return types.Scenario{Name: "Advising", Description: "Help session"} }

func verdictJSON(hardGates map[string]bool, outcomes map[string]bool) string {
	ov := map[string]map[string]any{}
	for id, pass := range outcomes {
		ov[id] = map[string]any{"pass": pass, "notes": "test"}
	}
	v := map[string]any{
		"hard_gates": hardGates,
		"outcomes":   ov,
		"indicators": map[string]int{"clarity": 4},
		"notes":      "looks good",
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// ── Score tests ───────────────────────────────────────────────────────────────

func TestJudge_Score_AllPass(t *testing.T) {
	mock := &mockLLM{response: verdictJSON(
		map[string]bool{"no_harm": true},
		map[string]bool{"concrete_step": true, "empathy": true},
	)}

	j := judge.NewWithCaller(mock, "you are a judge", mkRubric())
	v, err := j.Score(context.Background(), mkPersona(), mkScenario(), mkTranscript())
	require.NoError(t, err)

	assert.True(t, v.HardGates["no_harm"])
	assert.True(t, v.Outcomes["concrete_step"].Pass)
	assert.True(t, v.Outcomes["empathy"].Pass)
	assert.Equal(t, 4, v.Indicators["clarity"])
	assert.Equal(t, "looks good", v.Notes)
	assert.InDelta(t, 1.0, v.Composite, 0.001)
}

func TestJudge_Score_HardGateFail(t *testing.T) {
	mock := &mockLLM{response: verdictJSON(
		map[string]bool{"no_harm": false},
		map[string]bool{"concrete_step": true, "empathy": true},
	)}

	j := judge.NewWithCaller(mock, "", mkRubric())
	v, err := j.Score(context.Background(), mkPersona(), mkScenario(), mkTranscript())
	require.NoError(t, err)
	assert.Equal(t, 0.0, v.Composite, "hard gate failure must zero composite")
}

func TestJudge_Score_PartialOutcomes(t *testing.T) {
	mock := &mockLLM{response: verdictJSON(
		map[string]bool{"no_harm": true},
		map[string]bool{"concrete_step": true, "empathy": false},
	)}

	j := judge.NewWithCaller(mock, "", mkRubric())
	v, err := j.Score(context.Background(), mkPersona(), mkScenario(), mkTranscript())
	require.NoError(t, err)
	// item_level: 1/1 = 1.0; conversation: 0/1 = 0.0 → mean = 0.5
	assert.InDelta(t, 0.5, v.Composite, 0.001)
}

func TestJudge_Score_LLMError(t *testing.T) {
	mock := &mockLLM{err: fmt.Errorf("api failure")}
	j := judge.NewWithCaller(mock, "", mkRubric())
	_, err := j.Score(context.Background(), mkPersona(), mkScenario(), mkTranscript())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api failure")
}

func TestJudge_Score_UnparseableVerdict(t *testing.T) {
	mock := &mockLLM{response: "I think the conversation was good but I can't rate it numerically."}
	j := judge.NewWithCaller(mock, "", mkRubric())
	_, err := j.Score(context.Background(), mkPersona(), mkScenario(), mkTranscript())
	require.Error(t, err)
}

// ── Composite tests ───────────────────────────────────────────────────────────

func TestComposite_NoOutcomes(t *testing.T) {
	v := &judge.Verdict{
		HardGates: map[string]bool{},
		Outcomes:  map[string]judge.OutcomeResult{},
	}
	rubric := &types.Rubric{}
	assert.Equal(t, 1.0, judge.Composite(v, rubric), "no outcomes → perfect score")
}

func TestComposite_OnlyItemLevel(t *testing.T) {
	rubric := &types.Rubric{
		Outcomes: []types.OutcomeDef{
			{ID: "a", Type: "item_level"},
			{ID: "b", Type: "item_level"},
			{ID: "c", Type: "item_level"},
		},
	}
	v := &judge.Verdict{
		HardGates: map[string]bool{},
		Outcomes: map[string]judge.OutcomeResult{
			"a": {Pass: true},
			"b": {Pass: true},
			"c": {Pass: false},
		},
	}
	// 2/3 = 0.667
	assert.InDelta(t, 2.0/3.0, judge.Composite(v, rubric), 0.001)
}

func TestComposite_MixedTypes(t *testing.T) {
	rubric := &types.Rubric{
		Outcomes: []types.OutcomeDef{
			{ID: "item1", Type: "item_level"},
			{ID: "conv1", Type: "conversation"},
		},
	}
	v := &judge.Verdict{
		HardGates: map[string]bool{},
		Outcomes: map[string]judge.OutcomeResult{
			"item1": {Pass: true},  // item_level: 1/1 = 1.0
			"conv1": {Pass: false}, // conversation: 0/1 = 0.0
		},
	}
	// mean(1.0, 0.0) = 0.5
	assert.InDelta(t, 0.5, judge.Composite(v, rubric), 0.001)
}

// ── Parse tests ───────────────────────────────────────────────────────────────

func TestParseVerdict_PlainJSON(t *testing.T) {
	raw := `{"hard_gates":{"g1":true},"outcomes":{"o1":{"pass":true,"notes":"ok"}},"indicators":{"i1":3},"notes":"all good"}`
	v, err := judge.ExportedParseVerdict(raw)
	require.NoError(t, err)
	assert.True(t, v.HardGates["g1"])
	assert.True(t, v.Outcomes["o1"].Pass)
	assert.Equal(t, 3, v.Indicators["i1"])
}

func TestParseVerdict_MarkdownFence(t *testing.T) {
	raw := "Here is my evaluation:\n```json\n{\"hard_gates\":{},\"outcomes\":{\"o1\":{\"pass\":false,\"notes\":\"\"}},\"indicators\":{},\"notes\":\"\"}\n```\nThat's it."
	v, err := judge.ExportedParseVerdict(raw)
	require.NoError(t, err)
	assert.False(t, v.Outcomes["o1"].Pass)
}

func TestParseVerdict_EmbeddedJSON(t *testing.T) {
	raw := `After careful analysis I give this score: {"hard_gates":{"g":true},"outcomes":{},"indicators":{},"notes":"good"} Thanks.`
	v, err := judge.ExportedParseVerdict(raw)
	require.NoError(t, err)
	assert.True(t, v.HardGates["g"])
}

func TestParseVerdict_Invalid(t *testing.T) {
	_, err := judge.ExportedParseVerdict("This conversation was very good, 10/10!")
	require.Error(t, err)
}

// ── Claims tests ──────────────────────────────────────────────────────────────

func TestExtractClaims_ParsesJSONArray(t *testing.T) {
	mock := &mockLLM{response: `["Deadline is March 1st", "Must have 2.5 GPA"]`}
	transcript := types.Transcript{
		Turns: []types.Turn{{Role: "assistant", Text: "The deadline is March 1st."}},
	}
	claims, err := judge.ExtractClaims(context.Background(), mock, transcript)
	require.NoError(t, err)
	require.Len(t, claims, 2)
	assert.Equal(t, "Deadline is March 1st", claims[0])
}

func TestExtractClaims_MarkdownWrapped(t *testing.T) {
	mock := &mockLLM{response: "```json\n[\"claim one\", \"claim two\"]\n```"}
	transcript := types.Transcript{
		Turns: []types.Turn{{Role: "assistant", Text: "claim one. claim two."}},
	}
	claims, err := judge.ExtractClaims(context.Background(), mock, transcript)
	require.NoError(t, err)
	assert.Len(t, claims, 2)
}

func TestVerifyClaims_ParsesJSONMap(t *testing.T) {
	mock := &mockLLM{response: `{"The deadline is March 1st": "correct", "Must have 4.0 GPA": "incorrect"}`}
	results, err := judge.VerifyClaims(context.Background(), mock, []string{"The deadline is March 1st", "Must have 4.0 GPA"})
	require.NoError(t, err)
	assert.Equal(t, "correct", results["The deadline is March 1st"])
	assert.Equal(t, "incorrect", results["Must have 4.0 GPA"])
}

func TestVerifyClaims_EmptyClaims(t *testing.T) {
	mock := &mockLLM{}
	results, err := judge.VerifyClaims(context.Background(), mock, nil)
	require.NoError(t, err)
	assert.Nil(t, results)
}
