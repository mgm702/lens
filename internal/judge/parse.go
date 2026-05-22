package judge

import (
	"encoding/json"
	"fmt"
	"strings"
)

// rawVerdict is the shape the LLM is asked to return.
type rawVerdict struct {
	HardGates  map[string]bool             `json:"hard_gates"`
	Outcomes   map[string]rawOutcomeResult `json:"outcomes"`
	Indicators map[string]int              `json:"indicators"`
	Notes      string                      `json:"notes"`
}

type rawOutcomeResult struct {
	Pass  bool   `json:"pass"`
	Notes string `json:"notes"`
}

// parseVerdict extracts a rawVerdict from the LLM's response text.
// It handles three common formats:
//  1. Plain JSON object
//  2. JSON wrapped in markdown code fences (```json ... ```)
//  3. JSON embedded anywhere in free-form text (first { … last })
func parseVerdict(raw string) (*rawVerdict, error) {
	raw = strings.TrimSpace(raw)

	// Attempt 1: direct parse.
	var v rawVerdict
	if err := json.Unmarshal([]byte(raw), &v); err == nil {
		return &v, nil
	}

	// Attempt 2: strip markdown code fences.
	if extracted := extractCodeFence(raw); extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &v); err == nil {
			return &v, nil
		}
	}

	// Attempt 3: find outermost braces.
	if extracted := extractBraces(raw); extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &v); err == nil {
			return &v, nil
		}
	}

	return nil, fmt.Errorf("judge: could not parse verdict JSON from response: %q", truncate(raw, 200))
}

// extractCodeFence pulls the content of the first ```…``` block.
func extractCodeFence(s string) string {
	start := strings.Index(s, "```")
	if start < 0 {
		return ""
	}
	inner := s[start+3:]
	// Skip optional language tag (e.g. "json\n").
	if nl := strings.Index(inner, "\n"); nl >= 0 {
		inner = inner[nl+1:]
	}
	end := strings.Index(inner, "```")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(inner[:end])
}

// extractBraces returns the substring from the first '{' to the last '}'.
func extractBraces(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
