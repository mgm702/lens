package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgm702/lens/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoad_MinimalConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "personas.json", `[{"id":"P001","name":"Alice"}]`)
	writeFile(t, dir, "scenarios.json", `[{"id":"T001","name":"Basic"}]`)
	writeFile(t, dir, "rubric.yaml", "hard_gates: []\noutcomes: []\nindicators: []\n")
	writeFile(t, dir, "sim.md", "You are a user.")
	writeFile(t, dir, "judge.md", "You are a judge.")

	path := writeFile(t, dir, "experiment.yaml", `
name: test-experiment
target:
  type: mock
simulator:
  provider: anthropic
  model: claude-haiku-4-5
  prompt_file: sim.md
judge:
  provider: anthropic
  model: claude-opus-4-6
  prompt_file: judge.md
  rubric_file: rubric.yaml
personas_file: personas.json
scenarios_file: scenarios.json
`)

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "test-experiment", cfg.Name)
	assert.Equal(t, "mock", cfg.Target.Type)
	// Defaults applied
	assert.Equal(t, 1, cfg.Reps)
	assert.Equal(t, 6, cfg.MaxTurns)
	assert.Equal(t, 4, cfg.Workers)
	assert.Equal(t, []string{"full"}, cfg.InfoLevels)
}

func TestLoad_RelativePathsResolved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "personas.json", `[]`)
	writeFile(t, dir, "scenarios.json", `[]`)
	writeFile(t, dir, "rubric.yaml", "")
	writeFile(t, dir, "sim.md", "")
	writeFile(t, dir, "judge.md", "")

	path := writeFile(t, dir, "exp.yaml", `
name: rel-test
target:
  type: mock
simulator:
  provider: anthropic
  model: haiku
  prompt_file: sim.md
judge:
  provider: anthropic
  model: opus
  prompt_file: judge.md
  rubric_file: rubric.yaml
personas_file: personas.json
scenarios_file: scenarios.json
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(cfg.PersonasFile), "personas_file should be absolute")
	assert.True(t, filepath.IsAbs(cfg.ScenariosFile), "scenarios_file should be absolute")
	assert.True(t, filepath.IsAbs(cfg.Judge.RubricFile), "rubric_file should be absolute")
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TEST_API_URL", "https://example.com/api")
	writeFile(t, dir, "personas.json", `[]`)
	writeFile(t, dir, "scenarios.json", `[]`)
	writeFile(t, dir, "rubric.yaml", "")
	writeFile(t, dir, "sim.md", "")
	writeFile(t, dir, "judge.md", "")

	path := writeFile(t, dir, "exp.yaml", `
name: env-test
target:
  type: http
  url: ${TEST_API_URL}/chat
  response_path: "$.reply"
simulator:
  provider: anthropic
  model: haiku
  prompt_file: sim.md
judge:
  provider: anthropic
  model: opus
  prompt_file: judge.md
  rubric_file: rubric.yaml
personas_file: personas.json
scenarios_file: scenarios.json
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/api/chat", cfg.Target.URL)
}

func TestLoadPersonas_JSON(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "personas.json", `[
		{"id":"P001","name":"Alice","summary":"First persona"},
		{"id":"P002","name":"Bob","summary":"Second persona"}
	]`)
	personas, err := config.LoadPersonas(path)
	require.NoError(t, err)
	require.Len(t, personas, 2)
	assert.Equal(t, "P001", personas[0].ID)
	assert.Equal(t, "Bob", personas[1].Name)
}

func TestLoadPersonas_YAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "personas.yaml", `
- id: P001
  name: Alice
  emotional_state: nervous
`)
	personas, err := config.LoadPersonas(path)
	require.NoError(t, err)
	require.Len(t, personas, 1)
	assert.Equal(t, "nervous", personas[0].EmotionalState)
}

func TestLoadScenarios(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "scenarios.json", `[
		{"id":"T001","name":"Basic Task","opening_message":"Hello"}
	]`)
	scenarios, err := config.LoadScenarios(path)
	require.NoError(t, err)
	require.Len(t, scenarios, 1)
	assert.Equal(t, "Hello", scenarios[0].OpeningMessage)
}

func TestLoadRubric(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "rubric.yaml", `
hard_gates:
  - id: no_harmful_advice
    type: conversation
    description: "Did not give harmful advice"
outcomes:
  - id: concrete_next_step
    type: item_level
    description: "Gave actionable next step"
indicators:
  - id: clarity
    scale: 5
    description: "Explanation clarity"
    anchor_low: "Very unclear"
    anchor_high: "Very clear"
`)
	rubric, err := config.LoadRubric(path)
	require.NoError(t, err)
	require.Len(t, rubric.HardGates, 1)
	assert.Equal(t, "no_harmful_advice", rubric.HardGates[0].ID)
	require.Len(t, rubric.Outcomes, 1)
	assert.Equal(t, "item_level", rubric.Outcomes[0].Type)
	require.Len(t, rubric.Indicators, 1)
	assert.Equal(t, 5, rubric.Indicators[0].Scale)
}
