package config

// ExperimentConfig is the top-level shape of an experiment YAML file.
type ExperimentConfig struct {
	Name          string      `yaml:"name"`
	Description   string      `yaml:"description"`
	Target        TargetConfig `yaml:"target"`
	Simulator     LLMConfig   `yaml:"simulator"`
	Judge         JudgeConfig `yaml:"judge"`
	PersonasFile  string      `yaml:"personas_file"`
	ScenariosFile string      `yaml:"scenarios_file"`
	InfoLevels    []string    `yaml:"info_levels"`
	Reps          int         `yaml:"reps"`
	MaxTurns      int         `yaml:"max_turns"`
	Workers       int         `yaml:"workers"`
	OutputDir     string      `yaml:"output_dir"`
}

// TargetConfig describes the AI system under test.
// Fields used depend on Type: LLM adapters use Model/SystemPromptFile/MaxTokens;
// the HTTP adapter uses URL/Auth/RequestTemplate/ResponsePath/Streaming/Headers.
type TargetConfig struct {
	// Type selects the adapter: "anthropic" | "openai" | "http" | "bedrock" | "mock"
	Type string `yaml:"type"`

	// LLM adapter fields (anthropic, openai, bedrock)
	Model            string `yaml:"model"`
	SystemPromptFile string `yaml:"system_prompt_file"`
	MaxTokens        int    `yaml:"max_tokens"`

	// HTTP adapter fields
	URL             string            `yaml:"url"`
	Auth            AuthConfig        `yaml:"auth"`
	RequestTemplate string            `yaml:"request_template"`
	ResponsePath    string            `yaml:"response_path"`
	Streaming       bool              `yaml:"streaming"`
	StreamingType   string            `yaml:"streaming_type"` // "sse" | "actioncable" | "websocket"
	Headers         map[string]string `yaml:"headers"`

	// WebSocketURL is the WebSocket endpoint used for "websocket" and "actioncable"
	// streaming types. If empty, the adapter derives it from URL by replacing
	// the http(s):// scheme with ws(s)://.
	WebSocketURL string `yaml:"websocket_url"`

	// StreamingChannel is the ActionCable channel class name to subscribe to,
	// e.g. "SafeToEarnChannel" or "ChatChannel".
	// Ignored for non-actioncable streaming types.
	StreamingChannel string `yaml:"streaming_channel"`

	// DonePath is a gjson path evaluated against each WebSocket/ActionCable
	// broadcast message payload to detect stream completion.
	// Defaults to "done" (checks for {"done":true} — legacy behaviour).
	DonePath string `yaml:"done_path"`

	// DoneValue is the string value at DonePath that signals completion.
	// Defaults to "true". For zetta-style {"type":"done"} use done_path: type, done_value: done.
	DoneValue string `yaml:"done_value"`

	// Session management: optional endpoint to reset server-side conversation state
	SessionReset *SessionResetConfig `yaml:"session_reset"`

	// Mock adapter field
	ScriptFile string `yaml:"script_file"`
}

// AuthConfig describes how the HTTP adapter authenticates requests.
type AuthConfig struct {
	// Type is "bearer" | "api_key" | "basic" | "custom"
	Type string `yaml:"type"`
	// TokenEnv is the name of the environment variable holding the token or password.
	TokenEnv string `yaml:"token_env"`
	// Header is the custom header name used when Type is "custom" or "api_key".
	Header string `yaml:"header"`
	// UsernameEnv is the env var for the username when Type is "basic".
	UsernameEnv string `yaml:"username_env"`
}

// SessionResetConfig describes an HTTP endpoint the adapter calls to wipe
// server-side conversation state between eval cases.
type SessionResetConfig struct {
	// Method is "DELETE" or "POST"
	Method string `yaml:"method"`
	// Path is relative to the target URL base, e.g. "/api/chat/session"
	Path string `yaml:"path"`
}

// LLMConfig configures any LLM used by Lens itself (simulator or judge base).
type LLMConfig struct {
	// Provider is "anthropic" | "openai" | "bedrock"
	Provider   string `yaml:"provider"`
	Model      string `yaml:"model"`
	PromptFile string `yaml:"prompt_file"`
	MaxTokens  int    `yaml:"max_tokens"`
}

// JudgeConfig extends LLMConfig with rubric and optional claim-verification settings.
type JudgeConfig struct {
	LLMConfig      `yaml:",inline"`
	RubricFile     string `yaml:"rubric_file"`
	VerifyClaims   bool   `yaml:"verify_claims"`
	VerifyProvider string `yaml:"verify_provider"`
	VerifyModel    string `yaml:"verify_model"`
}

// knownAdapterTypes is the set of valid values for TargetConfig.Type.
var knownAdapterTypes = map[string]bool{
	"anthropic": true,
	"openai":    true,
	"http":      true,
	"bedrock":   true,
	"mock":      true,
}

// knownProviders is the set of valid values for LLMConfig.Provider.
var knownProviders = map[string]bool{
	"anthropic": true,
	"openai":    true,
	"bedrock":   true,
}

// knownInfoLevels is the set of valid values for ExperimentConfig.InfoLevels.
var knownInfoLevels = map[string]bool{
	"full":    true,
	"partial": true,
	"none":    true,
}
