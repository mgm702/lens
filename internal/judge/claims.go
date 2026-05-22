package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mgm702/lens/internal/llm"
	"github.com/mgm702/lens/pkg/types"
)

const extractClaimsSystem = `You extract factual claims from conversation transcripts.
Return ONLY a JSON array of strings, one claim per element.
Claims should be specific, verifiable statements made by the Advisor.
Example: ["The deadline for FAFSA is June 30", "You need a 3.0 GPA to qualify"]`

const verifyClaimsSystem = `You verify factual claims for accuracy.
For each claim return one of: "correct", "incorrect", or "unverifiable".
Return ONLY a JSON object mapping claim text to verdict.
Example: {"The sky is blue": "correct", "The moon is made of cheese": "incorrect"}`

// ExtractClaims asks the LLM to list verifiable factual claims made by the
// Advisor in the transcript. Returns a slice of claim strings.
func ExtractClaims(ctx context.Context, caller llm.Caller, t types.Transcript) ([]string, error) {
	var sb strings.Builder
	sb.WriteString("Extract all verifiable factual claims from the Advisor's messages below.\n\n")
	for _, turn := range t.Turns {
		if turn.Role != "assistant" {
			continue
		}
		sb.WriteString("Advisor: ")
		sb.WriteString(turn.Text)
		sb.WriteString("\n\n")
	}

	raw, err := caller.Call(ctx, extractClaimsSystem, sb.String())
	if err != nil {
		return nil, fmt.Errorf("judge: extracting claims: %w", err)
	}

	return parseStringArray(raw)
}

// VerifyClaims asks the LLM to assess the accuracy of each claim.
// Returns a map of claim text → "correct" | "incorrect" | "unverifiable".
func VerifyClaims(ctx context.Context, caller llm.Caller, claims []string) (map[string]string, error) {
	if len(claims) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("Verify the accuracy of each claim below:\n\n")
	for i, c := range claims {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}

	raw, err := caller.Call(ctx, verifyClaimsSystem, sb.String())
	if err != nil {
		return nil, fmt.Errorf("judge: verifying claims: %w", err)
	}

	return parseStringMap(raw)
}

// ── JSON parsing helpers ──────────────────────────────────────────────────────

// parseStringArray extracts a JSON array of strings from raw LLM output,
// tolerating markdown fences and surrounding text.
func parseStringArray(raw string) ([]string, error) {
	candidates := []string{
		strings.TrimSpace(raw),
		extractCodeFence(raw),
	}
	// Find first '[' … last ']' as a fallback.
	if s := extractArrayBraces(raw); s != "" {
		candidates = append(candidates, s)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		var result []string
		if err := json.Unmarshal([]byte(c), &result); err == nil {
			return result, nil
		}
	}
	// Last resort: split on newlines and strip punctuation.
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.Trim(line, `"- ,`)
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

// parseStringMap extracts a JSON map[string]string from raw LLM output.
func parseStringMap(raw string) (map[string]string, error) {
	candidates := []string{
		strings.TrimSpace(raw),
		extractCodeFence(raw),
		extractBraces(raw),
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		var result map[string]string
		if err := json.Unmarshal([]byte(c), &result); err == nil {
			return result, nil
		}
	}
	return nil, fmt.Errorf("judge: could not parse string map from: %q", truncate(raw, 200))
}

// extractArrayBraces returns the substring from the first '[' to the last ']'.
func extractArrayBraces(s string) string {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
