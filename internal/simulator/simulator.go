// Package simulator implements the user-side LLM that drives multi-turn
// conversations with the target AI system being evaluated.
//
// The Simulator is initialised with a rendered system prompt (from the
// experiment's simulator.prompt_file template) and on each call to NextTurn
// it formats the full conversation history into a text block, then calls the
// backing LLM to produce the next message the simulated user would send.
//
// Returning the sentinel strings "[DONE]" or "[DROPOUT]" signals to the
// runner that the conversation should end.
package simulator

import (
	"context"
	"fmt"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/internal/llm"
	"github.com/mgm702/lens/pkg/types"
)

// Simulator drives the user side of an eval conversation.
type Simulator struct {
	llm    llm.Caller
	system string // rendered simulator prompt (baked in at New time)
}

// New builds a Simulator for one (persona, scenario, info-level) combination.
// The simulator prompt template at cfg.PromptFile is rendered once and stored;
// the llm.Caller is wired to the provider specified in cfg.
func New(cfg config.LLMConfig, p types.Persona, sc types.Scenario, infoLevel string) (*Simulator, error) {
	system, err := renderPrompt(cfg.PromptFile, p, sc, infoLevel)
	if err != nil {
		return nil, fmt.Errorf("simulator: rendering prompt: %w", err)
	}

	caller, err := newLLMCaller(cfg)
	if err != nil {
		return nil, fmt.Errorf("simulator: creating LLM caller: %w", err)
	}

	return &Simulator{llm: caller, system: system}, nil
}

// NewWithCaller creates a Simulator with an already-constructed llm.Caller and
// a pre-rendered system prompt. Intended for testing and advanced use cases.
func NewWithCaller(caller llm.Caller, system string) *Simulator {
	return &Simulator{llm: caller, system: system}
}

// NextTurn formats the current conversation history as labelled text, then
// calls the simulator LLM and returns the next message the simulated user
// would send.
//
// The returned string may contain "[DONE]" or "[DROPOUT]" anywhere — the
// runner uses these to end the conversation.
func (s *Simulator) NextTurn(ctx context.Context, history []types.Turn) (string, error) {
	return s.llm.Call(ctx, s.system, formatHistory(history))
}
