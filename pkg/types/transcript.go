package types

// EndReason values emitted when a conversation terminates.
const (
	EndReasonDone     = "DONE"      // user signaled task complete
	EndReasonDropout  = "DROPOUT"   // user gave up
	EndReasonMaxTurns = "MAX_TURNS" // hit configured turn limit
	EndReasonError    = "ERROR"     // adapter or judge failure
)

// Turn is a single message exchange in the conversation.
// Role is "user" or "assistant".
type Turn struct {
	Role string   `json:"role"`
	Text string   `json:"text"`
	Perf TurnPerf `json:"perf"`
}

// TurnPerf captures latency and cost for a single turn.
type TurnPerf struct {
	WallMs       int64   `json:"wall_ms"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	USD          float64 `json:"usd"`
}

// Transcript is the full record of a conversation.
type Transcript struct {
	Turns     []Turn `json:"turns"`
	EndReason string `json:"end_reason"`
}

// TotalPerf sums cost and token counts across all turns matching the given role.
// Pass an empty role string to sum across all turns.
func (t *Transcript) TotalPerf(role string) TurnPerf {
	var out TurnPerf
	for _, turn := range t.Turns {
		if role != "" && turn.Role != role {
			continue
		}
		out.WallMs += turn.Perf.WallMs
		out.InputTokens += turn.Perf.InputTokens
		out.OutputTokens += turn.Perf.OutputTokens
		out.USD += turn.Perf.USD
	}
	return out
}
