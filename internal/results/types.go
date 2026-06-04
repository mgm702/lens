// Package results defines the output types produced by the runner and consumed
// by the store, aggregator, and report builder.
package results

import (
	"encoding/json"
	"time"

	"github.com/mgm702/lens/internal/judge"
	"github.com/mgm702/lens/pkg/types"
)

// CaseResult holds the complete output of a single eval case.
type CaseResult struct {
	// Identification
	Label      string `json:"label"`
	PersonaID  string `json:"persona_id"`
	ScenarioID string `json:"scenario_id"`
	InfoLevel  string `json:"info_level"`
	Rep        int    `json:"rep"`

	// RunAt is when this case started executing.
	RunAt time.Time `json:"run_at"`

	// Outcome
	EndReason  string           `json:"end_reason"`
	Transcript types.Transcript `json:"transcript"`
	Verdict    *judge.Verdict   `json:"verdict,omitempty"`

	// Performance
	Perf CasePerf `json:"perf"`

	// Err is non-nil when the case failed due to an infrastructure error
	// (adapter failure, judge failure), not a scoring outcome.
	// Stored as a string so it serialises correctly to JSON.
	Err error `json:"-"`

	// ErrMsg is the string form of Err; populated whenever Err is non-nil.
	ErrMsg string `json:"error,omitempty"`
}

// MarshalJSON ensures Err is reflected in ErrMsg before serialisation.
func (r CaseResult) MarshalJSON() ([]byte, error) {
	type alias CaseResult // avoids infinite recursion
	a := alias(r)
	if r.Err != nil && r.ErrMsg == "" {
		a.ErrMsg = r.Err.Error()
	}
	return json.Marshal(a)
}

// UnmarshalJSON restores Err from ErrMsg after deserialisation.
func (r *CaseResult) UnmarshalJSON(data []byte) error {
	type alias CaseResult
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = CaseResult(a)
	if r.ErrMsg != "" && r.Err == nil {
		r.Err = &errString{r.ErrMsg}
	}
	return nil
}

// errString is a simple error implementation used when deserialising ErrMsg.
type errString struct{ s string }

func (e *errString) Error() string { return e.s }

// CasePerf summarises wall-clock time and estimated cost for an entire case.
type CasePerf struct {
	WallMs int64   `json:"wall_ms"`
	USD    float64 `json:"usd"`
}
