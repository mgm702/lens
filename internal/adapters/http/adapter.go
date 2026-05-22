// Package httpadapter provides an HTTP/WebSocket Adapter that can wrap any
// REST or streaming AI API without requiring a custom Go integration.
package httpadapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"

	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
)

const (
	defaultTemplate = `{"message":"{{.Message}}"}`
	defaultTimeout  = 60 * time.Second
)

var sessionCounter atomic.Int64

// Adapter sends turns to any HTTP or WebSocket-based AI API.
type Adapter struct {
	cfg    config.TargetConfig
	pool   chan string // nil when no per-session auth tokens are needed
	client *nethttp.Client
	tmpl   *template.Template
}

// session holds per-conversation state for a single eval case.
type session struct {
	id         string
	token      string
	sessionCfg types.SessionConfig
	wsConn     *websocket.Conn // non-nil for websocket / actioncable streaming
	wsMu       sync.Mutex      // serialises reads and writes on wsConn
}

func (s *session) ID() string { return s.id }

// templateData is the context available inside request_template.
type templateData struct {
	Message   string
	SessionID string
	Persona   types.Persona
	Scenario  types.Scenario
	Metadata  map[string]any
}

// New constructs an HTTPAdapter. It parses the request template and, when
// bearer or api_key auth is configured, builds the token pool from the
// comma-separated value of the auth.token_env environment variable.
func New(cfg config.TargetConfig) (*Adapter, error) {
	rawTmpl := cfg.RequestTemplate
	if rawTmpl == "" {
		rawTmpl = defaultTemplate
	}
	tmpl, err := template.New("request").Parse(rawTmpl)
	if err != nil {
		return nil, fmt.Errorf("http adapter: parsing request_template: %w", err)
	}

	var pool chan string
	if cfg.Auth.Type == "bearer" || cfg.Auth.Type == "api_key" || cfg.Auth.Type == "custom" {
		if cfg.Auth.TokenEnv == "" {
			return nil, fmt.Errorf("http adapter: auth.token_env is required for %s auth", cfg.Auth.Type)
		}
		val := os.Getenv(cfg.Auth.TokenEnv)
		if val == "" {
			return nil, fmt.Errorf("http adapter: environment variable %q (auth.token_env) is not set", cfg.Auth.TokenEnv)
		}
		tokens := strings.Split(val, ",")
		pool = make(chan string, len(tokens))
		for _, t := range tokens {
			pool <- strings.TrimSpace(t)
		}
	}

	return &Adapter{
		cfg:    cfg,
		pool:   pool,
		client: &nethttp.Client{Timeout: defaultTimeout},
		tmpl:   tmpl,
	}, nil
}

// CreateSession acquires an auth token (if pooled), creates the session, and
// for WebSocket streaming types establishes and subscribes the WS connection.
func (a *Adapter) CreateSession(ctx context.Context, cfg types.SessionConfig) (types.Session, error) {
	var token string
	if a.pool != nil {
		select {
		case token = <-a.pool:
		case <-ctx.Done():
			return nil, fmt.Errorf("http adapter: waiting for token: %w", ctx.Err())
		}
	} else if a.cfg.Auth.Type == "basic" {
		token = a.buildBasicToken()
	}

	sess := &session{
		id:         fmt.Sprintf("http-%d", sessionCounter.Add(1)),
		token:      token,
		sessionCfg: cfg,
	}

	if a.cfg.StreamingType == "websocket" || a.cfg.StreamingType == "actioncable" || a.cfg.StreamingType == "actioncable_rest" {
		wsURL := a.wsURL()
		header := a.wsHeaders(token)
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
		if err != nil {
			a.returnToken(token)
			return nil, fmt.Errorf("http adapter: websocket dial %q: %w", wsURL, err)
		}
		sess.wsConn = conn

		if a.cfg.StreamingType == "actioncable" || a.cfg.StreamingType == "actioncable_rest" {
			channel := a.channel()
			if err := subscribeActionCable(conn, channel); err != nil {
				conn.Close()
				a.returnToken(token)
				return nil, fmt.Errorf("http adapter: actioncable subscribe: %w", err)
			}
		}
	}

	return sess, nil
}

// SendTurn renders the request template, sends the message to the target, and
// returns the full reply text (collecting streaming deltas when applicable).
func (a *Adapter) SendTurn(ctx context.Context, s types.Session, message string) (string, error) {
	sess := s.(*session)

	switch a.cfg.StreamingType {
	case "websocket":
		return a.sendTurnWebSocket(ctx, sess, message)
	case "actioncable":
		return a.sendTurnActionCable(ctx, sess, message)
	case "actioncable_rest":
		return a.sendTurnActionCableRest(ctx, sess, message)
	default:
		return a.sendTurnHTTP(ctx, sess, message)
	}
}

// CloseSession returns the token to the pool, closes any WS connection, and
// optionally calls the session reset endpoint.
func (a *Adapter) CloseSession(ctx context.Context, s types.Session) error {
	sess := s.(*session)

	sess.wsMu.Lock()
	if sess.wsConn != nil {
		_ = sess.wsConn.Close()
		sess.wsConn = nil
	}
	sess.wsMu.Unlock()

	if a.cfg.SessionReset != nil {
		if err := a.callSessionReset(ctx, sess); err != nil {
			// Best-effort: log but don't fail — the turn data is already collected.
			_ = err
		}
	}

	a.returnToken(sess.token)
	return nil
}

// ── HTTP turn ────────────────────────────────────────────────────────────────

func (a *Adapter) sendTurnHTTP(ctx context.Context, sess *session, message string) (string, error) {
	body, err := a.renderTemplate(sess, message)
	if err != nil {
		return "", err
	}

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, a.cfg.URL, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("http adapter: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	a.applyAuth(req, sess.token)
	for k, v := range a.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http adapter: POST %s: %w", a.cfg.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("http adapter: server returned %d: %s", resp.StatusCode, raw)
	}

	if a.cfg.Streaming && a.cfg.StreamingType == "sse" {
		return collectSSE(ctx, resp.Body, a.cfg.ResponsePath)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("http adapter: reading response: %w", err)
	}
	if a.cfg.ResponsePath == "" {
		return string(raw), nil
	}
	result := gjson.GetBytes(raw, a.cfg.ResponsePath)
	if !result.Exists() {
		return "", fmt.Errorf("http adapter: response_path %q not found in: %s", a.cfg.ResponsePath, raw)
	}
	return result.String(), nil
}

// ── WebSocket turn ───────────────────────────────────────────────────────────

func (a *Adapter) sendTurnWebSocket(ctx context.Context, sess *session, message string) (string, error) {
	body, err := a.renderTemplate(sess, message)
	if err != nil {
		return "", err
	}
	sess.wsMu.Lock()
	err = sess.wsConn.WriteMessage(websocket.TextMessage, []byte(body))
	sess.wsMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("http adapter: websocket write: %w", err)
	}
	return collectWebSocket(ctx, sess, a.cfg.ResponsePath, a.cfg.DonePath, a.cfg.DoneValue)
}

func (a *Adapter) sendTurnActionCable(ctx context.Context, sess *session, message string) (string, error) {
	body, err := a.renderTemplate(sess, message)
	if err != nil {
		return "", err
	}
	channel := a.channel()
	if err := sendActionCableMessage(sess.wsConn, channel, body); err != nil {
		return "", fmt.Errorf("http adapter: actioncable send: %w", err)
	}
	return collectActionCable(ctx, sess, a.cfg.ResponsePath, a.cfg.DonePath, a.cfg.DoneValue)
}

// sendTurnActionCableRest handles the REST-trigger + WS-receive pattern used by
// apps like Zetta's SafeToEarnChannel: the message is sent via HTTP POST (which
// enqueues a background job), and the streaming response arrives as ActionCable
// broadcasts on the already-subscribed WebSocket connection.
func (a *Adapter) sendTurnActionCableRest(ctx context.Context, sess *session, message string) (string, error) {
	body, err := a.renderTemplate(sess, message)
	if err != nil {
		return "", err
	}

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, a.cfg.URL, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("http adapter: building REST trigger request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	a.applyAuth(req, sess.token)
	for k, v := range a.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http adapter: REST trigger POST %s: %w", a.cfg.URL, err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http adapter: REST trigger returned %d", resp.StatusCode)
	}

	return collectActionCable(ctx, sess, a.cfg.ResponsePath, a.cfg.DonePath, a.cfg.DoneValue)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func (a *Adapter) renderTemplate(sess *session, message string) (string, error) {
	// JSON-encode the message content (outer quotes stripped) so it is safe to
	// embed inside a JSON string literal in the request template, e.g. "{{.Message}}".
	// This handles newlines, quotes, and other characters that would break JSON.
	msgBytes, _ := json.Marshal(message)
	msgEscaped := string(msgBytes[1 : len(msgBytes)-1]) // strip leading/trailing "

	data := templateData{
		Message:   msgEscaped,
		SessionID: sess.id,
		Persona:   sess.sessionCfg.Persona,
		Scenario:  sess.sessionCfg.Scenario,
		Metadata:  sess.sessionCfg.Metadata,
	}
	var buf bytes.Buffer
	if err := a.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("http adapter: rendering request template: %w", err)
	}
	return buf.String(), nil
}

func (a *Adapter) applyAuth(req *nethttp.Request, token string) {
	switch a.cfg.Auth.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+token)
	case "api_key":
		header := a.cfg.Auth.Header
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, token)
	case "basic":
		req.Header.Set("Authorization", "Basic "+token)
	case "custom":
		req.Header.Set(a.cfg.Auth.Header, token)
	}
}

func (a *Adapter) buildBasicToken() string {
	username := os.Getenv(a.cfg.Auth.UsernameEnv)
	password := os.Getenv(a.cfg.Auth.TokenEnv)
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func (a *Adapter) wsURL() string {
	if a.cfg.WebSocketURL != "" {
		return a.cfg.WebSocketURL
	}
	// Convert http:// → ws:// and https:// → wss://
	u := a.cfg.URL
	u = strings.Replace(u, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	return u
}

func (a *Adapter) wsHeaders(token string) nethttp.Header {
	h := nethttp.Header{}
	switch a.cfg.Auth.Type {
	case "bearer":
		h.Set("Authorization", "Bearer "+token)
	case "api_key":
		header := a.cfg.Auth.Header
		if header == "" {
			header = "X-API-Key"
		}
		h.Set(header, token)
	case "custom":
		if a.cfg.Auth.Header != "" {
			h.Set(a.cfg.Auth.Header, token)
		}
	}

	// ActionCable (and most WebSocket servers) require an Origin header.
	// Derive it from the WebSocket URL: wss:// → https://, ws:// → http://
	if u, err := url.Parse(a.wsURL()); err == nil {
		scheme := "https"
		if u.Scheme == "ws" {
			scheme = "http"
		}
		h.Set("Origin", scheme+"://"+u.Host)
	}

	return h
}

func (a *Adapter) channel() string {
	if a.cfg.StreamingChannel != "" {
		return a.cfg.StreamingChannel
	}
	if ch := a.cfg.Headers["actioncable_channel"]; ch != "" {
		return ch
	}
	return "ChatChannel"
}

func (a *Adapter) callSessionReset(ctx context.Context, sess *session) error {
	method := strings.ToUpper(a.cfg.SessionReset.Method)
	if method == "" {
		method = nethttp.MethodDelete
	}
	parsed, err := url.Parse(a.cfg.URL)
	if err != nil {
		return fmt.Errorf("http adapter: parsing target URL for session reset: %w", err)
	}
	resetURL := parsed.Scheme + "://" + parsed.Host + a.cfg.SessionReset.Path
	req, err := nethttp.NewRequestWithContext(ctx, method, resetURL, nil)
	if err != nil {
		return err
	}
	a.applyAuth(req, sess.token)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (a *Adapter) returnToken(token string) {
	if a.pool != nil && token != "" {
		a.pool <- token
	}
}
