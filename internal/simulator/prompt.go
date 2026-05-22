package simulator

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/mgm702/lens/pkg/types"
)

// promptData is the template context injected into the simulator prompt file.
// All maps and slices are pre-formatted as human-readable text so that prompt
// authors don't need to use template range loops for the common case.
type promptData struct {
	// Persona fields
	Name              string
	Summary           string
	EmotionalState    string
	ProfileText       string // formatted key: value lines
	ConstraintsText   string // formatted key: value lines
	KnowledgeGapsText string // formatted "- item" lines

	// Scenario fields
	ScenarioName        string
	ScenarioDescription string
	OpeningMessage      string
	SuccessCriteriaText string // formatted "- item" lines

	// Eval settings
	InfoLevel string
}

// renderPrompt reads the template at path and renders it with the given
// persona, scenario, and info level. Returns the rendered system prompt string.
func renderPrompt(path string, p types.Persona, sc types.Scenario, infoLevel string) (string, error) {
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return "", fmt.Errorf("simulator: parsing prompt template %q: %w", path, err)
	}

	data := buildPromptData(p, sc, infoLevel)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("simulator: rendering prompt template: %w", err)
	}
	return buf.String(), nil
}

func buildPromptData(p types.Persona, sc types.Scenario, infoLevel string) promptData {
	return promptData{
		Name:                p.Name,
		Summary:             p.Summary,
		EmotionalState:      p.EmotionalState,
		ProfileText:         formatMap(p.Profile),
		ConstraintsText:     formatMap(p.Constraints),
		KnowledgeGapsText:   formatStringSlice(p.KnowledgeGaps),
		ScenarioName:        sc.Name,
		ScenarioDescription: sc.Description,
		OpeningMessage:      sc.OpeningMessage,
		SuccessCriteriaText: formatStringSlice(sc.SuccessCriteria),
		InfoLevel:           infoLevel,
	}
}

// formatMap renders a map[string]any as sorted "key: value" lines.
// Returns an empty string for nil or empty maps.
func formatMap(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%s: %v\n", k, m[k]))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatStringSlice renders a slice as "- item" lines.
// Returns an empty string for nil or empty slices.
func formatStringSlice(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString("- ")
		sb.WriteString(item)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatHistory converts a turn slice into a plain-text conversation block
// suitable for inclusion in the simulator's user message each call.
// The target system's replies are labelled "Advisor:" and the simulated
// user's prior messages are labelled "User:".
func formatHistory(history []types.Turn) string {
	if len(history) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, t := range history {
		label := "User"
		if t.Role == "assistant" {
			label = "Advisor"
		}
		sb.WriteString(label)
		sb.WriteString(": ")
		sb.WriteString(t.Text)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
