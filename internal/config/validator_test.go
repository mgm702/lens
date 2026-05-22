package config_test

import (
	"testing"

	"github.com/mgm702/lens/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfig returns a fully valid config with real temp files wired up.
func validConfig(t *testing.T) *config.ExperimentConfig {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "sim.md", "sim prompt")
	writeFile(t, dir, "judge.md", "judge prompt")
	writeFile(t, dir, "rubric.yaml", "")
	writeFile(t, dir, "personas.json", "[]")
	writeFile(t, dir, "scenarios.json", "[]")

	return &config.ExperimentConfig{
		Name: "test",
		Target: config.TargetConfig{
			Type: "mock",
		},
		Simulator: config.LLMConfig{
			Provider:   "anthropic",
			Model:      "claude-haiku-4-5",
			PromptFile: dir + "/sim.md",
			MaxTokens:  1024,
		},
		Judge: config.JudgeConfig{
			LLMConfig: config.LLMConfig{
				Provider:   "anthropic",
				Model:      "claude-opus-4-6",
				PromptFile: dir + "/judge.md",
				MaxTokens:  2048,
			},
			RubricFile: dir + "/rubric.yaml",
		},
		PersonasFile:  dir + "/personas.json",
		ScenariosFile: dir + "/scenarios.json",
		InfoLevels:    []string{"full"},
		Reps:          1,
		MaxTurns:      6,
		Workers:       4,
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validConfig(t)
	errs := config.Validate(cfg)
	assert.Empty(t, errs, "expected no validation errors for a valid config")
}

func TestValidate_MissingName(t *testing.T) {
	cfg := validConfig(t)
	cfg.Name = ""
	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)
	assert.Contains(t, config.FormatErrors(errs), "name")
}

func TestValidate_UnknownAdapterType(t *testing.T) {
	cfg := validConfig(t)
	cfg.Target.Type = "grpc"
	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)
	assert.Contains(t, config.FormatErrors(errs), "target.type")
}

func TestValidate_HTTPAdapterMissingURL(t *testing.T) {
	cfg := validConfig(t)
	cfg.Target.Type = "http"
	cfg.Target.URL = ""
	cfg.Target.ResponsePath = "$.reply"
	errs := config.Validate(cfg)
	fields := fieldNames(errs)
	assert.Contains(t, fields, "target.url")
}

func TestValidate_HTTPAdapterMissingResponsePath(t *testing.T) {
	cfg := validConfig(t)
	cfg.Target.Type = "http"
	cfg.Target.URL = "https://example.com/api"
	cfg.Target.ResponsePath = ""
	errs := config.Validate(cfg)
	fields := fieldNames(errs)
	assert.Contains(t, fields, "target.response_path")
}

func TestValidate_StreamingWithoutType(t *testing.T) {
	cfg := validConfig(t)
	cfg.Target.Type = "http"
	cfg.Target.URL = "https://example.com"
	cfg.Target.ResponsePath = "$.reply"
	cfg.Target.Streaming = true
	cfg.Target.StreamingType = ""
	errs := config.Validate(cfg)
	fields := fieldNames(errs)
	assert.Contains(t, fields, "target.streaming_type")
}

func TestValidate_UnknownInfoLevel(t *testing.T) {
	cfg := validConfig(t)
	cfg.InfoLevels = []string{"full", "unknown_level"}
	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)
	assert.Contains(t, config.FormatErrors(errs), "info_levels")
}

func TestValidate_InvalidReps(t *testing.T) {
	cfg := validConfig(t)
	cfg.Reps = 0
	errs := config.Validate(cfg)
	fields := fieldNames(errs)
	assert.Contains(t, fields, "reps")
}

func TestValidate_MissingPersonasFile(t *testing.T) {
	cfg := validConfig(t)
	cfg.PersonasFile = "/nonexistent/personas.json"
	errs := config.Validate(cfg)
	fields := fieldNames(errs)
	assert.Contains(t, fields, "personas_file")
}

func TestValidate_VerifyClaimsMissingProvider(t *testing.T) {
	cfg := validConfig(t)
	cfg.Judge.VerifyClaims = true
	cfg.Judge.VerifyProvider = ""
	cfg.Judge.VerifyModel = ""
	errs := config.Validate(cfg)
	fields := fieldNames(errs)
	assert.Contains(t, fields, "judge.verify_provider")
	assert.Contains(t, fields, "judge.verify_model")
}

func TestValidate_CollectsAllErrors(t *testing.T) {
	// A config with multiple problems should report all of them at once.
	cfg := &config.ExperimentConfig{} // everything zero/empty
	errs := config.Validate(cfg)
	assert.Greater(t, len(errs), 3, "expected multiple errors for an empty config")
}

func TestFormatErrors_Empty(t *testing.T) {
	assert.Equal(t, "", config.FormatErrors(nil))
}

// fieldNames extracts the Field strings from a slice of ValidationErrors.
func fieldNames(errs []config.ValidationError) []string {
	names := make([]string, len(errs))
	for i, e := range errs {
		names[i] = e.Field
	}
	return names
}
