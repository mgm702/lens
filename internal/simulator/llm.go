package simulator

import (
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/internal/llm"
)

// newLLMCaller constructs the provider-specific caller for cfg.
// Delegated entirely to internal/llm so provider SDK code lives in one place.
func newLLMCaller(cfg config.LLMConfig) (llm.Caller, error) {
	return llm.New(cfg)
}
