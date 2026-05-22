package runner_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/internal/judge"
	"github.com/mgm702/lens/internal/progress"
	"github.com/mgm702/lens/internal/results"
	"github.com/mgm702/lens/internal/runner"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mocks ─────────────────────────────────────────────────────────────────────

// mockAdapter implements types.Adapter with scripted replies.
type mockAdapter struct {
	reply    string
	replyErr error
	sessions int
	closes   int
	sessionErr error
}

type mockSession struct{ id string }

func (s *mockSession) ID() string { return s.id }

func (a *mockAdapter) CreateSession(_ context.Context, _ types.SessionConfig) (types.Session, error) {
	if a.sessionErr != nil {
		return nil, a.sessionErr
	}
	a.sessions++
	return &mockSession{id: fmt.Sprintf("sess-%d", a.sessions)}, nil
}

func (a *mockAdapter) SendTurn(_ context.Context, _ types.Session, _ string) (string, error) {
	return a.reply, a.replyErr
}

func (a *mockAdapter) CloseSession(_ context.Context, _ types.Session) error {
	a.closes++
	return nil
}

// mockScorer implements runner.Scorer.
type mockScorer struct {
	verdict *judge.Verdict
	err     error
}

func (s *mockScorer) Score(_ context.Context, _ types.Persona, _ types.Scenario, _ types.Transcript) (*judge.Verdict, error) {
	return s.verdict, s.err
}

// mockTurnGen implements runner.TurnGenerator with a scripted response sequence.
type mockTurnGen struct {
	responses []string
	idx       int
}

func (g *mockTurnGen) NextTurn(_ context.Context, _ []types.Turn) (string, error) {
	if g.idx >= len(g.responses) {
		return "[DONE]", nil
	}
	r := g.responses[g.idx]
	g.idx++
	return r, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func mkCfg(workers, maxTurns, reps int, infoLevels []string) *config.ExperimentConfig {
	return &config.ExperimentConfig{
		Workers:    workers,
		MaxTurns:   maxTurns,
		Reps:       reps,
		InfoLevels: infoLevels,
	}
}

func mkPersona(id string) types.Persona { return types.Persona{ID: id, Name: id} }
func mkScenario(id string) types.Scenario {
	return types.Scenario{ID: id, Name: id, OpeningMessage: "Hello!"}
}

func okVerdict() *judge.Verdict {
	return &judge.Verdict{
		HardGates:  map[string]bool{},
		Outcomes:   map[string]judge.OutcomeResult{},
		Indicators: map[string]int{},
		Composite:  1.0,
	}
}

func simFactory(responses []string) runner.SimFactory {
	return func(_ types.Persona, _ types.Scenario, _ string) (runner.TurnGenerator, error) {
		return &mockTurnGen{responses: responses}, nil
	}
}

func newRunner(
	cfg *config.ExperimentConfig,
	adapter types.Adapter,
	scorer runner.Scorer,
	factory runner.SimFactory,
	events chan<- progress.Event,
) *runner.Runner {
	return runner.NewWithFactory(cfg, adapter, factory, scorer, events)
}

// ── ExpandGrid tests ──────────────────────────────────────────────────────────

func TestExpandGrid_AllCombinations(t *testing.T) {
	cfg := mkCfg(1, 10, 2, []string{"full", "partial"})
	personas := []types.Persona{mkPersona("P001"), mkPersona("P002")}
	scenarios := []types.Scenario{mkScenario("T001")}

	cases := runner.ExpandGrid(cfg, personas, scenarios)
	// 2 personas × 1 scenario × 2 info_levels × 2 reps = 8
	assert.Len(t, cases, 8)
}

func TestExpandGrid_LabelFormat(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	personas := []types.Persona{mkPersona("P001")}
	scenarios := []types.Scenario{mkScenario("T001")}

	cases := runner.ExpandGrid(cfg, personas, scenarios)
	require.Len(t, cases, 1)
	assert.Equal(t, "P001_T001_full_r0", cases[0].Label)
}

func TestExpandGrid_EmptyInfoLevels(t *testing.T) {
	cfg := mkCfg(1, 10, 2, nil) // no info levels
	cases := runner.ExpandGrid(cfg, []types.Persona{mkPersona("P1")}, []types.Scenario{mkScenario("T1")})
	assert.Empty(t, cases)
}

func TestExpandGrid_EmptyPersonas(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	cases := runner.ExpandGrid(cfg, nil, []types.Scenario{mkScenario("T1")})
	assert.Empty(t, cases)
}

func TestExpandGrid_Order(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full", "partial"})
	p := []types.Persona{mkPersona("P1"), mkPersona("P2")}
	sc := []types.Scenario{mkScenario("T1"), mkScenario("T2")}

	cases := runner.ExpandGrid(cfg, p, sc)
	require.Len(t, cases, 8) // 2×2×2×1

	// First case should be P1, T1, full, r0
	assert.Equal(t, "P1", cases[0].Persona.ID)
	assert.Equal(t, "T1", cases[0].Scenario.ID)
	assert.Equal(t, "full", cases[0].InfoLevel)
	assert.Equal(t, 0, cases[0].Rep)
}

// ── Runner.Run tests ──────────────────────────────────────────────────────────

func TestRun_Empty(t *testing.T) {
	cfg := mkCfg(1, 5, 1, []string{"full"})
	r := newRunner(cfg, &mockAdapter{reply: "ok"}, &mockScorer{verdict: okVerdict()}, simFactory([]string{"[DONE]"}), nil)
	res, err := r.Run(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, res)
}

func TestRun_SingleCase_DoneAfterFirstTurn(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	adapter := &mockAdapter{reply: "Great question!"}
	scorer := &mockScorer{verdict: okVerdict()}
	// Simulator returns [DONE] after first advisor reply.
	factory := simFactory([]string{"[DONE] Thanks!"})

	r := newRunner(cfg, adapter, scorer, factory, nil)
	cases := []runner.Case{{
		Persona:   mkPersona("P1"),
		Scenario:  mkScenario("T1"),
		InfoLevel: "full",
		Rep:       0,
		Label:     "P1_T1_full_r0",
	}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	require.Len(t, res, 1)

	assert.Equal(t, types.EndReasonDone, res[0].EndReason)
	assert.Nil(t, res[0].Err)
	assert.NotNil(t, res[0].Verdict)
	// 1 user turn (opening) + 1 advisor turn
	assert.Len(t, res[0].Transcript.Turns, 2)
}

func TestRun_SingleCase_Dropout(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	factory := simFactory([]string{"[DROPOUT] I give up."})

	r := newRunner(cfg, &mockAdapter{reply: "here is info"}, &mockScorer{verdict: okVerdict()}, factory, nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	assert.Equal(t, types.EndReasonDropout, res[0].EndReason)
}

func TestRun_SingleCase_MaxTurns(t *testing.T) {
	cfg := mkCfg(1, 2, 1, []string{"full"})
	// Simulator never signals done — returns plain messages forever.
	factory := simFactory([]string{"more info please", "still more info"})

	r := newRunner(cfg, &mockAdapter{reply: "answer"}, &mockScorer{verdict: okVerdict()}, factory, nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	assert.Equal(t, types.EndReasonMaxTurns, res[0].EndReason)
}

func TestRun_AdapterCreateSessionError(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	adapter := &mockAdapter{sessionErr: fmt.Errorf("auth failed")}
	r := newRunner(cfg, adapter, &mockScorer{verdict: okVerdict()}, simFactory(nil), nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, types.EndReasonError, res[0].EndReason)
	assert.ErrorContains(t, res[0].Err, "auth failed")
}

func TestRun_AdapterSendTurnError(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	adapter := &mockAdapter{replyErr: fmt.Errorf("network timeout")}
	r := newRunner(cfg, adapter, &mockScorer{verdict: okVerdict()}, simFactory(nil), nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	assert.Equal(t, types.EndReasonError, res[0].EndReason)
	assert.ErrorContains(t, res[0].Err, "network timeout")
}

func TestRun_JudgeError(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	factory := simFactory([]string{"[DONE]"})
	scorer := &mockScorer{err: fmt.Errorf("judge exploded")}

	r := newRunner(cfg, &mockAdapter{reply: "ok"}, scorer, factory, nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	assert.Equal(t, types.EndReasonError, res[0].EndReason)
	assert.ErrorContains(t, res[0].Err, "judge exploded")
}

func TestRun_SimFactoryError(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	badFactory := func(_ types.Persona, _ types.Scenario, _ string) (runner.TurnGenerator, error) {
		return nil, fmt.Errorf("prompt file missing")
	}
	r := newRunner(cfg, &mockAdapter{reply: "ok"}, &mockScorer{verdict: okVerdict()}, badFactory, nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	assert.Equal(t, types.EndReasonError, res[0].EndReason)
	assert.ErrorContains(t, res[0].Err, "prompt file missing")
}

func TestRun_MultipleCases_AllComplete(t *testing.T) {
	cfg := mkCfg(2, 10, 1, []string{"full"})
	factory := simFactory([]string{"[DONE]"})
	scorer := &mockScorer{verdict: okVerdict()}

	r := newRunner(cfg, &mockAdapter{reply: "ok"}, scorer, factory, nil)
	cases := runner.ExpandGrid(cfg,
		[]types.Persona{mkPersona("P1"), mkPersona("P2")},
		[]types.Scenario{mkScenario("T1")},
	)

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	assert.Len(t, res, 2)
	for _, r := range res {
		assert.Nil(t, r.Err)
	}
}

func TestRun_ContextCancelled(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before running

	factory := simFactory([]string{"[DONE]"})
	r := newRunner(cfg, &mockAdapter{reply: "ok"}, &mockScorer{verdict: okVerdict()}, factory, nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	res, err := r.Run(ctx, cases)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, types.EndReasonError, res[0].EndReason)
	assert.ErrorIs(t, res[0].Err, context.Canceled)
}

func TestRun_EventsEmitted(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	events := make(chan progress.Event, 10)
	factory := simFactory([]string{"[DONE]"})

	r := runner.NewWithFactory(cfg, &mockAdapter{reply: "ok"}, factory, &mockScorer{verdict: okVerdict()}, events)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "lbl"}}

	_, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	close(events)

	var types_ []progress.EventType
	for e := range events {
		types_ = append(types_, e.Type)
		assert.Equal(t, "lbl", e.CaseLabel)
	}
	assert.Contains(t, types_, progress.EventCaseStarted)
	assert.Contains(t, types_, progress.EventCaseCompleted)
}

func TestRun_SessionClosedOnError(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	adapter := &mockAdapter{replyErr: fmt.Errorf("boom")}
	factory := simFactory(nil)

	r := newRunner(cfg, adapter, &mockScorer{verdict: okVerdict()}, factory, nil)
	cases := []runner.Case{{Persona: mkPersona("P1"), Scenario: mkScenario("T1"), InfoLevel: "full", Label: "x"}}

	r.Run(context.Background(), cases) //nolint:errcheck
	// CloseSession is deferred — even on error it must be called.
	assert.Equal(t, 1, adapter.closes)
}

func TestRun_ScenarioOpeningMessageUsed(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	// Scenario has an opening message; simulator should only be called AFTER
	// the advisor replies, i.e. the first NextTurn call gets full history.
	var gotHistory []types.Turn
	captureFactory := func(_ types.Persona, _ types.Scenario, _ string) (runner.TurnGenerator, error) {
		return &captureTurnGen{responses: []string{"[DONE]"}, capture: &gotHistory}, nil
	}

	adapter := &mockAdapter{reply: "Hello, how can I help?"}
	r := runner.NewWithFactory(cfg, adapter, captureFactory, &mockScorer{verdict: okVerdict()}, nil)
	cases := []runner.Case{{
		Persona:   mkPersona("P1"),
		Scenario:  types.Scenario{ID: "T1", Name: "T1", OpeningMessage: "Hi there!"},
		InfoLevel: "full",
		Label:     "x",
	}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	require.Len(t, res, 1)

	// History passed to simulator should include the opening user message and the advisor reply.
	require.Len(t, gotHistory, 2)
	assert.Equal(t, "user", gotHistory[0].Role)
	assert.Equal(t, "Hi there!", gotHistory[0].Text)
	assert.Equal(t, "assistant", gotHistory[1].Role)
}

// captureTurnGen records the history it receives on the first call.
type captureTurnGen struct {
	responses []string
	idx       int
	capture   *[]types.Turn
}

func (g *captureTurnGen) NextTurn(_ context.Context, history []types.Turn) (string, error) {
	if g.capture != nil && g.idx == 0 {
		*g.capture = history
	}
	if g.idx >= len(g.responses) {
		return "[DONE]", nil
	}
	r := g.responses[g.idx]
	g.idx++
	return r, nil
}

// ── results.CaseResult sanity ─────────────────────────────────────────────────

func TestCaseResult_FieldsPopulated(t *testing.T) {
	cfg := mkCfg(1, 10, 1, []string{"full"})
	factory := simFactory([]string{"[DONE]"})
	verdict := &judge.Verdict{
		HardGates:  map[string]bool{"g": true},
		Outcomes:   map[string]judge.OutcomeResult{"o": {Pass: true}},
		Indicators: map[string]int{},
		Composite:  1.0,
	}
	scorer := &mockScorer{verdict: verdict}

	r := newRunner(cfg, &mockAdapter{reply: "great!"}, scorer, factory, nil)
	cases := []runner.Case{{
		Persona:   mkPersona("P42"),
		Scenario:  mkScenario("T99"),
		InfoLevel: "partial",
		Rep:       3,
		Label:     "P42_T99_partial_r3",
	}}

	res, err := r.Run(context.Background(), cases)
	require.NoError(t, err)
	require.Len(t, res, 1)

	cr := res[0]
	assert.Equal(t, "P42_T99_partial_r3", cr.Label)
	assert.Equal(t, "P42", cr.PersonaID)
	assert.Equal(t, "T99", cr.ScenarioID)
	assert.Equal(t, "partial", cr.InfoLevel)
	assert.Equal(t, 3, cr.Rep)
	assert.Equal(t, types.EndReasonDone, cr.EndReason)
	assert.NotNil(t, cr.Verdict)
	assert.InDelta(t, 1.0, cr.Verdict.Composite, 0.001)
	assert.True(t, cr.Perf.WallMs >= 0)
}

// compile-time check: *runner.Runner must be constructable via both ctors.
var _ = results.CaseResult{}
