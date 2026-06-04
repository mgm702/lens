// Package runner expands an experiment grid into individual cases and executes
// them concurrently using a bounded worker pool.
package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/internal/judge"
	"github.com/mgm702/lens/internal/progress"
	"github.com/mgm702/lens/internal/results"
	"github.com/mgm702/lens/internal/simulator"
	"github.com/mgm702/lens/pkg/types"
)

// ── Interfaces ────────────────────────────────────────────────────────────────

// Scorer scores a completed transcript. *judge.Judge satisfies this interface.
type Scorer interface {
	Score(ctx context.Context, p types.Persona, sc types.Scenario, t types.Transcript) (*judge.Verdict, error)
}

// TurnGenerator produces the next simulated-user message.
// *simulator.Simulator satisfies this interface.
type TurnGenerator interface {
	NextTurn(ctx context.Context, history []types.Turn) (string, error)
}

// SimFactory builds a TurnGenerator for a given (persona, scenario, info-level)
// combination. Injected so tests can supply a fake without touching the disk.
type SimFactory func(p types.Persona, sc types.Scenario, infoLevel string) (TurnGenerator, error)

// ── Case ──────────────────────────────────────────────────────────────────────

// Case is a single unit of work in the eval grid.
type Case struct {
	Persona   types.Persona
	Scenario  types.Scenario
	InfoLevel string
	Rep       int
	Label     string
}

// ── Runner ────────────────────────────────────────────────────────────────────

// Runner executes an experiment.
type Runner struct {
	cfg     *config.ExperimentConfig
	adapter types.Adapter
	newSim  SimFactory
	judge   Scorer
	events  chan<- progress.Event // nil disables progress reporting
}

// New creates a Runner wired to the real simulator.
// Pass a nil events channel to suppress progress events.
func New(
	cfg *config.ExperimentConfig,
	adapter types.Adapter,
	j Scorer,
	events chan<- progress.Event,
) *Runner {
	return &Runner{
		cfg:     cfg,
		adapter: adapter,
		newSim:  realSimFactory(cfg.Simulator),
		judge:   j,
		events:  events,
	}
}

// NewWithFactory creates a Runner with an injected SimFactory. Intended for
// testing — allows fake simulators without filesystem or LLM calls.
func NewWithFactory(
	cfg *config.ExperimentConfig,
	adapter types.Adapter,
	factory SimFactory,
	j Scorer,
	events chan<- progress.Event,
) *Runner {
	return &Runner{
		cfg:     cfg,
		adapter: adapter,
		newSim:  factory,
		judge:   j,
		events:  events,
	}
}

func realSimFactory(cfg config.LLMConfig) SimFactory {
	return func(p types.Persona, sc types.Scenario, infoLevel string) (TurnGenerator, error) {
		return simulator.New(cfg, p, sc, infoLevel)
	}
}

// ExpandGrid returns every (persona × scenario × info_level × rep) combination
// derived from the experiment config. Order: persona → scenario → info_level → rep.
func ExpandGrid(cfg *config.ExperimentConfig, personas []types.Persona, scenarios []types.Scenario) []Case {
	var cases []Case
	for _, p := range personas {
		for _, sc := range scenarios {
			for _, lvl := range cfg.InfoLevels {
				for r := 0; r < cfg.Reps; r++ {
					cases = append(cases, Case{
						Persona:   p,
						Scenario:  sc,
						InfoLevel: lvl,
						Rep:       r,
						Label:     fmt.Sprintf("%s_%s_%s_r%d", p.ID, sc.ID, lvl, r),
					})
				}
			}
		}
	}
	return cases
}

// Run executes all cases using a bounded goroutine pool and collects results.
// It blocks until every case has finished or ctx is cancelled.
// Results are returned in completion order (not input order).
func (r *Runner) Run(ctx context.Context, cases []Case) ([]results.CaseResult, error) {
	if len(cases) == 0 {
		return nil, nil
	}

	workers := r.cfg.Workers
	if workers <= 0 {
		workers = 1
	}

	jobs := make(chan Case, len(cases))
	out := make(chan results.CaseResult, len(cases))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				select {
				case <-ctx.Done():
					// Context already cancelled — produce an error result immediately.
					base := results.CaseResult{
						Label:      c.Label,
						PersonaID:  c.Persona.ID,
						ScenarioID: c.Scenario.ID,
						InfoLevel:  c.InfoLevel,
						Rep:        c.Rep,
						EndReason:  types.EndReasonError,
						Err:        ctx.Err(),
					}
					out <- base
				default:
					out <- r.runOne(ctx, c)
				}
			}
		}()
	}

	for _, c := range cases {
		jobs <- c
	}
	close(jobs)

	// Close output channel once all workers finish.
	go func() {
		wg.Wait()
		close(out)
	}()

	all := make([]results.CaseResult, 0, len(cases))
	for res := range out {
		all = append(all, res)
	}
	return all, nil
}

// runOne executes a single eval case from session creation through judging.
func (r *Runner) runOne(ctx context.Context, c Case) results.CaseResult {
	start := time.Now()

	base := results.CaseResult{
		Label:      c.Label,
		PersonaID:  c.Persona.ID,
		ScenarioID: c.Scenario.ID,
		InfoLevel:  c.InfoLevel,
		Rep:        c.Rep,
		RunAt:      start,
	}

	r.emit(progress.Event{Type: progress.EventCaseStarted, CaseLabel: c.Label})

	// Build simulator for this (persona, scenario, info-level) combination.
	sim, err := r.newSim(c.Persona, c.Scenario, c.InfoLevel)
	if err != nil {
		return r.fail(base, err, start)
	}

	// Open a session with the target system.
	session, err := r.adapter.CreateSession(ctx, types.SessionConfig{
		Persona:   c.Persona,
		Scenario:  c.Scenario,
		InfoLevel: c.InfoLevel,
	})
	if err != nil {
		return r.fail(base, err, start)
	}
	defer func() { _ = r.adapter.CloseSession(ctx, session) }()

	maxTurns := r.cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20
	}

	var transcript types.Transcript
	endReason := types.EndReasonMaxTurns

	// First user message: use the scenario's opening message if set, otherwise
	// ask the simulator for the first turn.
	userMsg := c.Scenario.OpeningMessage
	if userMsg == "" {
		userMsg, err = sim.NextTurn(ctx, nil)
		if err != nil {
			return r.fail(base, err, start)
		}
	}

	// Conversation loop — each iteration is one (user → advisor) exchange.
	for turn := 0; turn < maxTurns; turn++ {
		// Record the user's message.
		transcript.Turns = append(transcript.Turns, types.Turn{Role: "user", Text: userMsg})

		// Get the advisor's reply from the target system.
		reply, err := r.adapter.SendTurn(ctx, session, userMsg)
		if err != nil {
			return r.fail(base, err, start)
		}
		transcript.Turns = append(transcript.Turns, types.Turn{Role: "assistant", Text: reply})

		// Ask the simulator for the next user message.
		nextMsg, err := sim.NextTurn(ctx, transcript.Turns)
		if err != nil {
			return r.fail(base, err, start)
		}

		// Check for end-of-conversation signals embedded in the simulator's output.
		if strings.Contains(nextMsg, "[DONE]") {
			endReason = types.EndReasonDone
			break
		}
		if strings.Contains(nextMsg, "[DROPOUT]") {
			endReason = types.EndReasonDropout
			break
		}

		userMsg = nextMsg
	}

	transcript.EndReason = endReason

	// Score the completed transcript.
	verdict, err := r.judge.Score(ctx, c.Persona, c.Scenario, transcript)
	if err != nil {
		return r.fail(base, err, start)
	}

	wallMs := time.Since(start).Milliseconds()
	res := base
	res.EndReason = endReason
	res.Transcript = transcript
	res.Verdict = verdict
	res.Perf = results.CasePerf{WallMs: wallMs}

	r.emit(progress.Event{
		Type:      progress.EventCaseCompleted,
		CaseLabel: c.Label,
		EndReason: endReason,
		Turns:     len(transcript.Turns),
		WallMs:    wallMs,
	})

	return res
}

// fail constructs an error CaseResult and emits a failed event.
func (r *Runner) fail(base results.CaseResult, err error, start time.Time) results.CaseResult {
	wallMs := time.Since(start).Milliseconds()
	base.Err = err
	base.EndReason = types.EndReasonError
	base.Perf = results.CasePerf{WallMs: wallMs}
	r.emit(progress.Event{
		Type:      progress.EventCaseFailed,
		CaseLabel: base.Label,
		Err:       err,
		WallMs:    wallMs,
	})
	return base
}

// emit sends an event if the events channel is non-nil.
// It is non-blocking: if the channel is full the event is dropped to avoid
// deadlocking the worker.
func (r *Runner) emit(e progress.Event) {
	if r.events == nil {
		return
	}
	select {
	case r.events <- e:
	default:
	}
}
