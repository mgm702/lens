// Package adapters provides the Adapter factory and the concrete adapter
// implementations for the various target system types supported by Lens.
package adapters

import (
	"fmt"

	anthropicadapter "github.com/mgm702/lens/internal/adapters/anthropic"
	bedrockadapter "github.com/mgm702/lens/internal/adapters/bedrock"
	httpadapter "github.com/mgm702/lens/internal/adapters/http"
	"github.com/mgm702/lens/internal/adapters/mock"
	openaiadapter "github.com/mgm702/lens/internal/adapters/openai"
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
)

// New constructs and returns the Adapter for the given target configuration.
// Returns an error if the adapter type is unknown or the adapter fails to
// initialize (e.g. missing credentials, unreachable script file).
func New(cfg config.TargetConfig) (types.Adapter, error) {
	switch cfg.Type {
	case "mock":
		return mock.New(cfg)
	case "http":
		return httpadapter.New(cfg)
	case "anthropic":
		return anthropicadapter.New(cfg)
	case "openai":
		return openaiadapter.New(cfg)
	case "bedrock":
		return bedrockadapter.New(cfg)
	default:
		return nil, fmt.Errorf("unknown adapter type %q; must be one of: anthropic, openai, http, bedrock, mock", cfg.Type)
	}
}
