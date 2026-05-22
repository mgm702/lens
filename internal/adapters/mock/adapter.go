// Package mock provides a scripted Adapter implementation for harness
// self-testing and CI runs that do not require a live AI backend.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
)

// defaultResponse is returned when no script is configured.
const defaultResponse = "I understand. How can I help you further?"

// sessionCounter is incremented atomically to generate unique session IDs.
var sessionCounter atomic.Int64

// Adapter is a mock target that returns scripted responses. It implements
// types.Adapter without any network I/O — suitable for unit tests and CI runs.
type Adapter struct {
	// script holds the ordered response strings loaded from ScriptFile.
	// nil means no script; the adapter returns defaultResponse for every turn.
	script []string
}

// session holds per-conversation state for the mock adapter.
type session struct {
	id    string
	idx   int    // next index into adapter.script to serve
	owner *Adapter
}

func (s *session) ID() string { return s.id }

// New constructs a mock Adapter. If cfg.ScriptFile is set, it must be a JSON
// file containing an array of strings — one entry per expected turn reply.
func New(cfg config.TargetConfig) (*Adapter, error) {
	a := &Adapter{}
	if cfg.ScriptFile == "" {
		return a, nil
	}

	data, err := os.ReadFile(cfg.ScriptFile)
	if err != nil {
		return nil, fmt.Errorf("mock adapter: reading script file %q: %w", cfg.ScriptFile, err)
	}
	if err := json.Unmarshal(data, &a.script); err != nil {
		return nil, fmt.Errorf("mock adapter: parsing script file %q (expected JSON array of strings): %w", cfg.ScriptFile, err)
	}
	return a, nil
}

// CreateSession starts a new mock conversation. The SessionConfig is accepted
// but not used — the mock is stateless with respect to persona and scenario.
func (a *Adapter) CreateSession(_ context.Context, _ types.SessionConfig) (types.Session, error) {
	return &session{
		id:    fmt.Sprintf("mock-%d", sessionCounter.Add(1)),
		owner: a,
	}, nil
}

// SendTurn returns the next scripted response, or defaultResponse if no script
// was loaded. Once the script is exhausted the last entry is repeated.
func (a *Adapter) SendTurn(_ context.Context, s types.Session, _ string) (string, error) {
	sess := s.(*session)

	if len(a.script) == 0 {
		return defaultResponse, nil
	}

	idx := sess.idx
	if idx >= len(a.script) {
		idx = len(a.script) - 1 // repeat last entry when script is exhausted
	} else {
		sess.idx++
	}
	return a.script[idx], nil
}

// CloseSession is a no-op for the mock adapter — there are no resources to release.
func (a *Adapter) CloseSession(_ context.Context, _ types.Session) error {
	return nil
}
