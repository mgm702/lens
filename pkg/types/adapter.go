package types

import "context"

// Adapter is the integration interface for any AI system under test.
// Implement this interface to add support for a new target system.
type Adapter interface {
	// CreateSession initializes a new conversation context. The returned Session
	// must be passed to subsequent SendTurn and CloseSession calls.
	CreateSession(ctx context.Context, cfg SessionConfig) (Session, error)

	// SendTurn sends a single user message and returns the system's reply.
	SendTurn(ctx context.Context, s Session, message string) (string, error)

	// CloseSession releases any resources held by the session (HTTP connections,
	// WebSocket connections, server-side state, etc.).
	CloseSession(ctx context.Context, s Session) error
}

// Session represents an active conversation with the target system.
// The concrete type is owned by the adapter implementation.
type Session interface {
	// ID returns a unique identifier for this session, used in logging and results.
	ID() string
}

// SessionConfig carries per-case context passed to CreateSession.
// Adapters may use these fields to set system prompts, auth tokens, or
// initial context appropriate to the persona and scenario.
type SessionConfig struct {
	Persona   Persona
	Scenario  Scenario
	InfoLevel string
	Metadata  map[string]any
}
