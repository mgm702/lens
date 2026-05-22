package config

import (
	"fmt"
	"os"
	"strings"
)

// ValidationError describes a single config problem with enough context
// to let the user fix it without reading source code.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate checks cfg for required fields, known enum values, and
// reachable file paths. Returns all errors found — not just the first —
// so the user can fix everything in one pass.
func Validate(cfg *ExperimentConfig) []ValidationError {
	var errs []ValidationError
	add := func(field, msg string, args ...any) {
		errs = append(errs, ValidationError{
			Field:   field,
			Message: fmt.Sprintf(msg, args...),
		})
	}

	// Top-level required fields
	if cfg.Name == "" {
		add("name", "experiment name is required")
	}

	// Target
	if cfg.Target.Type == "" {
		add("target.type", "required; must be one of: anthropic, openai, http, bedrock, mock")
	} else if !knownAdapterTypes[cfg.Target.Type] {
		add("target.type", "unknown adapter type %q; must be one of: anthropic, openai, http, bedrock, mock", cfg.Target.Type)
	}

	switch cfg.Target.Type {
	case "anthropic", "openai", "bedrock":
		if cfg.Target.Model == "" {
			add("target.model", "required for %s adapter", cfg.Target.Type)
		}
		if cfg.Target.SystemPromptFile != "" {
			if err := fileExists(cfg.Target.SystemPromptFile); err != nil {
				add("target.system_prompt_file", "%s", err)
			}
		}
	case "http":
		if cfg.Target.URL == "" {
			add("target.url", "required for http adapter")
		}
		if cfg.Target.ResponsePath == "" {
			add("target.response_path", "required for http adapter (JSONPath to extract reply text)")
		}
		if cfg.Target.Auth.Type != "" {
			validAuth := map[string]bool{"bearer": true, "api_key": true, "basic": true, "custom": true}
			if !validAuth[cfg.Target.Auth.Type] {
				add("target.auth.type", "unknown value %q; must be one of: bearer, api_key, basic, custom", cfg.Target.Auth.Type)
			}
			if cfg.Target.Auth.TokenEnv == "" && cfg.Target.Auth.Type != "basic" {
				add("target.auth.token_env", "required when auth.type is %q", cfg.Target.Auth.Type)
			}
		}
		if cfg.Target.Streaming && cfg.Target.StreamingType == "" {
			add("target.streaming_type", "required when streaming is true; must be one of: sse, actioncable, websocket")
		}
		validStreaming := map[string]bool{"sse": true, "actioncable": true, "actioncable_rest": true, "websocket": true}
		if cfg.Target.StreamingType != "" && !validStreaming[cfg.Target.StreamingType] {
			add("target.streaming_type", "unknown value %q; must be one of: sse, actioncable, websocket", cfg.Target.StreamingType)
		}
	case "mock":
		if cfg.Target.ScriptFile != "" {
			if err := fileExists(cfg.Target.ScriptFile); err != nil {
				add("target.script_file", "%s", err)
			}
		}
	}

	// Simulator
	if cfg.Simulator.Provider == "" {
		add("simulator.provider", "required; must be one of: anthropic, openai, bedrock")
	} else if !knownProviders[cfg.Simulator.Provider] {
		add("simulator.provider", "unknown value %q; must be one of: anthropic, openai, bedrock", cfg.Simulator.Provider)
	}
	if cfg.Simulator.Model == "" {
		add("simulator.model", "required")
	}
	if cfg.Simulator.PromptFile == "" {
		add("simulator.prompt_file", "required")
	} else if err := fileExists(cfg.Simulator.PromptFile); err != nil {
		add("simulator.prompt_file", "%s", err)
	}

	// Judge
	if cfg.Judge.Provider == "" {
		add("judge.provider", "required; must be one of: anthropic, openai, bedrock")
	} else if !knownProviders[cfg.Judge.Provider] {
		add("judge.provider", "unknown value %q; must be one of: anthropic, openai, bedrock", cfg.Judge.Provider)
	}
	if cfg.Judge.Model == "" {
		add("judge.model", "required")
	}
	if cfg.Judge.PromptFile == "" {
		add("judge.prompt_file", "required")
	} else if err := fileExists(cfg.Judge.PromptFile); err != nil {
		add("judge.prompt_file", "%s", err)
	}
	if cfg.Judge.RubricFile == "" {
		add("judge.rubric_file", "required")
	} else if err := fileExists(cfg.Judge.RubricFile); err != nil {
		add("judge.rubric_file", "%s", err)
	}
	if cfg.Judge.VerifyClaims {
		if cfg.Judge.VerifyProvider == "" {
			add("judge.verify_provider", "required when verify_claims is true")
		} else if !knownProviders[cfg.Judge.VerifyProvider] {
			add("judge.verify_provider", "unknown value %q", cfg.Judge.VerifyProvider)
		}
		if cfg.Judge.VerifyModel == "" {
			add("judge.verify_model", "required when verify_claims is true")
		}
	}

	// Persona and scenario files
	if cfg.PersonasFile == "" {
		add("personas_file", "required")
	} else if err := fileExists(cfg.PersonasFile); err != nil {
		add("personas_file", "%s", err)
	}
	if cfg.ScenariosFile == "" {
		add("scenarios_file", "required")
	} else if err := fileExists(cfg.ScenariosFile); err != nil {
		add("scenarios_file", "%s", err)
	}

	// Info levels
	if len(cfg.InfoLevels) == 0 {
		add("info_levels", "must specify at least one info level")
	}
	for _, lvl := range cfg.InfoLevels {
		if !knownInfoLevels[lvl] {
			add("info_levels", "unknown value %q; must be one of: full, partial, none", lvl)
		}
	}

	// Numeric sanity checks
	if cfg.Reps < 1 {
		add("reps", "must be >= 1, got %d", cfg.Reps)
	}
	if cfg.MaxTurns < 1 {
		add("max_turns", "must be >= 1, got %d", cfg.MaxTurns)
	}
	if cfg.Workers < 1 {
		add("workers", "must be >= 1, got %d", cfg.Workers)
	}

	return errs
}

// FormatErrors returns a human-readable summary of validation errors
// suitable for printing directly to stderr before exiting.
func FormatErrors(errs []ValidationError) string {
	if len(errs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d validation error(s) found:\n", len(errs))
	for _, e := range errs {
		fmt.Fprintf(&b, "  • %s\n", e.Error())
	}
	return b.String()
}

// fileExists returns an error if path does not point to a readable file.
func fileExists(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", path)
	}
	if err != nil {
		return fmt.Errorf("cannot access: %s (%w)", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, expected a file: %s", path)
	}
	return nil
}
