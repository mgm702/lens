package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgm702/lens/pkg/types"
	"gopkg.in/yaml.v3"
)

// Load reads an experiment YAML file, expands ${ENV_VAR} references in string
// values, and applies sensible defaults for omitted fields.
//
// If a .env file exists in the same directory as the experiment file, it is
// loaded first so its KEY=VALUE pairs are available for ${VAR} expansion.
// Variables already set in the shell environment take precedence over .env.
func Load(path string) (*ExperimentConfig, error) {
	loadDotEnv(filepath.Join(filepath.Dir(path), ".env"))

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading experiment file %q: %w", path, err)
	}

	expanded := expandEnvVars(string(data))

	var cfg ExperimentConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing experiment file %q: %w", path, err)
	}

	applyDefaults(&cfg)

	// Resolve relative paths against the directory containing the experiment file.
	base := filepath.Dir(path)
	cfg.PersonasFile = resolvePath(base, cfg.PersonasFile)
	cfg.ScenariosFile = resolvePath(base, cfg.ScenariosFile)
	cfg.Judge.RubricFile = resolvePath(base, cfg.Judge.RubricFile)
	cfg.Judge.PromptFile = resolvePath(base, cfg.Judge.PromptFile)
	cfg.Simulator.PromptFile = resolvePath(base, cfg.Simulator.PromptFile)
	if cfg.Target.SystemPromptFile != "" {
		cfg.Target.SystemPromptFile = resolvePath(base, cfg.Target.SystemPromptFile)
	}
	if cfg.Target.ScriptFile != "" {
		cfg.Target.ScriptFile = resolvePath(base, cfg.Target.ScriptFile)
	}
	if cfg.OutputDir != "" && !filepath.IsAbs(cfg.OutputDir) {
		cfg.OutputDir = filepath.Join(base, cfg.OutputDir)
	}

	return &cfg, nil
}

// LoadPersonas reads a JSON or YAML file and returns the slice of Persona values.
func LoadPersonas(path string) ([]types.Persona, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading personas file %q: %w", path, err)
	}
	var personas []types.Persona
	if err := unmarshalAuto(path, data, &personas); err != nil {
		return nil, fmt.Errorf("parsing personas file %q: %w", path, err)
	}
	return personas, nil
}

// LoadScenarios reads a JSON or YAML file and returns the slice of Scenario values.
func LoadScenarios(path string) ([]types.Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading scenarios file %q: %w", path, err)
	}
	var scenarios []types.Scenario
	if err := unmarshalAuto(path, data, &scenarios); err != nil {
		return nil, fmt.Errorf("parsing scenarios file %q: %w", path, err)
	}
	return scenarios, nil
}

// LoadRubric reads a YAML rubric file and returns the Rubric.
func LoadRubric(path string) (*types.Rubric, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading rubric file %q: %w", path, err)
	}
	var rubric types.Rubric
	if err := unmarshalAuto(path, data, &rubric); err != nil {
		return nil, fmt.Errorf("parsing rubric file %q: %w", path, err)
	}
	return &rubric, nil
}

// applyDefaults fills in zero-value fields with sensible values.
func applyDefaults(cfg *ExperimentConfig) {
	if cfg.Reps == 0 {
		cfg.Reps = 1
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 6
	}
	if cfg.Workers == 0 {
		cfg.Workers = 4
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "results"
	}
	if len(cfg.InfoLevels) == 0 {
		cfg.InfoLevels = []string{"full"}
	}
	if cfg.Target.MaxTokens == 0 {
		cfg.Target.MaxTokens = 2048
	}
	if cfg.Simulator.MaxTokens == 0 {
		cfg.Simulator.MaxTokens = 1024
	}
	if cfg.Judge.MaxTokens == 0 {
		cfg.Judge.MaxTokens = 2048
	}
}

// loadDotEnv parses a .env file and sets any KEY=VALUE pairs as environment
// variables, skipping keys that are already set in the environment. Blank lines
// and lines beginning with # are ignored. Silently no-ops if the file does not
// exist.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file not present — not an error
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip optional surrounding quotes from the value.
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		// Shell environment takes precedence over .env.
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val) //nolint:errcheck
		}
	}
}

// expandEnvVars replaces ${VAR} and $VAR patterns in the raw YAML string.
func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return "${" + key + "}"
	})
}

// resolvePath resolves p relative to base if p is not already absolute.
// Returns p unchanged if it is empty or already absolute.
func resolvePath(base, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// unmarshalAuto detects JSON vs YAML from the file extension and unmarshals
// accordingly. Defaults to YAML for unknown extensions.
func unmarshalAuto(path string, data []byte, v any) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		return json.Unmarshal(data, v)
	}
	return yaml.Unmarshal(data, v)
}
