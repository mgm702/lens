// Package judge scores completed eval transcripts against a rubric using an LLM.
package judge

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/internal/llm"
	"github.com/mgm702/lens/pkg/types"
)

// OutcomeResult holds the LLM's pass/fail verdict and rationale for one outcome.
type OutcomeResult struct {
	Pass  bool   `json:"pass"`
	Notes string `json:"notes,omitempty"`
}

// Verdict is the complete scoring result for one eval case.
type Verdict struct {
	HardGates           map[string]bool           `json:"hard_gates"`
	Outcomes            map[string]OutcomeResult  `json:"outcomes"`
	Indicators          map[string]int            `json:"indicators"`
	Notes               string                    `json:"notes"`
	Composite           float64                   `json:"composite"`
	ClaimsVerification  map[string]string         `json:"claims_verification,omitempty"`
}

// Judge scores transcripts against a rubric via an LLM.
type Judge struct {
	llm       llm.Caller
	verifyLLM llm.Caller // nil when verify_claims is disabled
	prompt    string     // system prompt loaded from judge.prompt_file
	rubric    *types.Rubric
}

// New constructs a Judge from the experiment's judge config.
// The judge prompt file is loaded as-is (plain text, not a template) and used
// as the LLM system prompt. The rubric is loaded from the rubric_file path.
func New(cfg config.JudgeConfig, rubric *types.Rubric) (*Judge, error) {
	data, err := os.ReadFile(cfg.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("judge: reading prompt file %q: %w", cfg.PromptFile, err)
	}

	caller, err := llm.New(cfg.LLMConfig)
	if err != nil {
		return nil, fmt.Errorf("judge: creating LLM caller: %w", err)
	}

	j := &Judge{
		llm:    caller,
		prompt: string(data),
		rubric: rubric,
	}

	if cfg.VerifyClaims && cfg.VerifyProvider != "" {
		verifyCfg := config.LLMConfig{
			Provider:  cfg.VerifyProvider,
			Model:     cfg.VerifyModel,
			MaxTokens: 1024,
		}
		vcaller, err := llm.New(verifyCfg)
		if err != nil {
			return nil, fmt.Errorf("judge: creating verify LLM caller: %w", err)
		}
		j.verifyLLM = vcaller
	}

	return j, nil
}

// NewWithCaller creates a Judge with pre-built callers and a pre-loaded prompt.
// Intended for testing.
func NewWithCaller(caller llm.Caller, system string, rubric *types.Rubric) *Judge {
	return &Judge{llm: caller, prompt: system, rubric: rubric}
}

// Score evaluates a completed transcript and returns a Verdict.
func (j *Judge) Score(ctx context.Context, p types.Persona, sc types.Scenario, t types.Transcript) (*Verdict, error) {
	msg := buildEvalMessage(t, p, sc, j.rubric)

	raw, err := j.llm.Call(ctx, j.prompt, msg)
	if err != nil {
		return nil, fmt.Errorf("judge: LLM call failed: %w", err)
	}

	rv, err := parseVerdict(raw)
	if err != nil {
		return nil, err
	}

	verdict := convertVerdict(rv)
	verdict.Composite = Composite(verdict, j.rubric)

	if j.verifyLLM != nil {
		claims, err := ExtractClaims(ctx, j.verifyLLM, t)
		if err == nil && len(claims) > 0 {
			verdict.ClaimsVerification, _ = VerifyClaims(ctx, j.verifyLLM, claims)
		}
	}

	return verdict, nil
}

// Composite calculates the composite score for a verdict against the rubric.
//
//  1. Any hard gate false  → 0.0
//  2. Item-level outcomes  → fraction passing
//  3. Conversation outcomes → fraction passing
//  4. Return mean of whichever groups are non-empty
func Composite(v *Verdict, rubric *types.Rubric) float64 {
	// Hard gate check.
	for _, gate := range rubric.HardGates {
		if pass, ok := v.HardGates[gate.ID]; ok && !pass {
			return 0.0
		}
	}

	var itemPass, itemTotal, convPass, convTotal int
	for _, def := range rubric.Outcomes {
		result, ok := v.Outcomes[def.ID]
		if !ok {
			continue
		}
		switch def.Type {
		case "item_level":
			itemTotal++
			if result.Pass {
				itemPass++
			}
		case "conversation":
			convTotal++
			if result.Pass {
				convPass++
			}
		}
	}

	var scores []float64
	if itemTotal > 0 {
		scores = append(scores, float64(itemPass)/float64(itemTotal))
	}
	if convTotal > 0 {
		scores = append(scores, float64(convPass)/float64(convTotal))
	}
	if len(scores) == 0 {
		return 1.0 // no outcomes defined — nothing to fail
	}

	var sum float64
	for _, s := range scores {
		sum += s
	}
	return sum / float64(len(scores))
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func convertVerdict(rv *rawVerdict) *Verdict {
	v := &Verdict{
		HardGates:  rv.HardGates,
		Indicators: rv.Indicators,
		Notes:      rv.Notes,
		Outcomes:   make(map[string]OutcomeResult, len(rv.Outcomes)),
	}
	if v.HardGates == nil {
		v.HardGates = map[string]bool{}
	}
	if v.Indicators == nil {
		v.Indicators = map[string]int{}
	}
	for id, r := range rv.Outcomes {
		v.Outcomes[id] = OutcomeResult{Pass: r.Pass, Notes: r.Notes}
	}
	return v
}

// buildEvalMessage constructs the user-turn sent to the judge LLM.
// All dynamic content (transcript, rubric definitions, persona, scenario) is
// included here so the system prompt can be a brief, stable instruction.
func buildEvalMessage(t types.Transcript, p types.Persona, sc types.Scenario, rubric *types.Rubric) string {
	var sb strings.Builder

	sb.WriteString("## Persona\n")
	sb.WriteString(fmt.Sprintf("Name: %s\n", p.Name))
	if p.Summary != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", p.Summary))
	}

	sb.WriteString("\n## Scenario: ")
	sb.WriteString(sc.Name)
	sb.WriteString("\n")
	if sc.Description != "" {
		sb.WriteString(sc.Description)
		sb.WriteString("\n")
	}
	if len(sc.SuccessCriteria) > 0 {
		sb.WriteString("Success criteria:\n")
		for _, c := range sc.SuccessCriteria {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n## Rubric\n")
	if len(rubric.HardGates) > 0 {
		sb.WriteString("Hard Gates (any failure → score 0):\n")
		for _, g := range rubric.HardGates {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", g.ID, g.Description))
		}
	}
	if len(rubric.Outcomes) > 0 {
		sb.WriteString("Outcomes:\n")
		for _, o := range rubric.Outcomes {
			sb.WriteString(fmt.Sprintf("- %s [%s]: %s\n", o.ID, o.Type, o.Description))
		}
	}
	if len(rubric.Indicators) > 0 {
		sb.WriteString("Indicators (diagnostic, not scored):\n")
		for _, ind := range rubric.Indicators {
			sb.WriteString(fmt.Sprintf("- %s (1-%d): %s  [1=%s, %d=%s]\n",
				ind.ID, ind.Scale, ind.Description, ind.AnchorLow, ind.Scale, ind.AnchorHigh))
		}
	}

	sb.WriteString("\n## Conversation\n")
	for _, turn := range t.Turns {
		label := "User"
		if turn.Role == "assistant" {
			label = "Advisor"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n\n", label, turn.Text))
	}

	sb.WriteString("\n## Instructions\n")
	sb.WriteString("Score the conversation against the rubric above.\n")
	sb.WriteString("Return ONLY valid JSON in this exact shape:\n")
	sb.WriteString(`{
  "hard_gates": {"<id>": true_or_false},
  "outcomes":   {"<id>": {"pass": true_or_false, "notes": "brief reason"}},
  "indicators": {"<id>": integer_1_to_N},
  "notes": "overall assessment"
}`)
	sb.WriteString("\n")

	return sb.String()
}
