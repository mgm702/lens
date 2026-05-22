package httpadapter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	httpadapter "github.com/mgm702/lens/internal/adapters/http"
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func echoServer(t *testing.T, handler nethttp.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func cfg(url string) config.TargetConfig {
	return config.TargetConfig{Type: "http", URL: url, ResponsePath: "reply"}
}

func newSession(t *testing.T, a *httpadapter.Adapter) types.Session {
	t.Helper()
	s, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)
	return s
}

func sendTurn(t *testing.T, a *httpadapter.Adapter, s types.Session, msg string) string {
	t.Helper()
	reply, err := a.SendTurn(context.Background(), s, msg)
	require.NoError(t, err)
	return reply
}

// ── Non-streaming HTTP ────────────────────────────────────────────────────────

func TestHTTPAdapter_BasicPost(t *testing.T) {
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"reply":"hello from server"}`)
	})

	a, err := httpadapter.New(cfg(srv.URL))
	require.NoError(t, err)

	s := newSession(t, a)
	reply := sendTurn(t, a, s, "hi")
	assert.Equal(t, "hello from server", reply)
}

func TestHTTPAdapter_NoResponsePath(t *testing.T) {
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		fmt.Fprint(w, "plain text reply")
	})

	c := cfg(srv.URL)
	c.ResponsePath = "" // no JSONPath extraction
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	reply := sendTurn(t, a, s, "anything")
	assert.Equal(t, "plain text reply", reply)
}

func TestHTTPAdapter_ServerError(t *testing.T) {
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		nethttp.Error(w, "internal error", nethttp.StatusInternalServerError)
	})

	a, err := httpadapter.New(cfg(srv.URL))
	require.NoError(t, err)

	s := newSession(t, a)
	_, err = a.SendTurn(context.Background(), s, "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// ── Request template rendering ────────────────────────────────────────────────

func TestHTTPAdapter_TemplateRendering(t *testing.T) {
	var gotBody string
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.RequestTemplate = `{"msg":"{{.Message}}","sid":"{{.SessionID}}"}`
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	sendTurn(t, a, s, "test message")

	assert.Contains(t, gotBody, `"msg":"test message"`)
	assert.Contains(t, gotBody, `"sid":"`)
}

func TestHTTPAdapter_TemplatePersonaFields(t *testing.T) {
	var gotBody string
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.RequestTemplate = `{"msg":"{{.Message}}","persona":"{{.Persona.Name}}"}`
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s, err := a.CreateSession(context.Background(), types.SessionConfig{
		Persona: types.Persona{Name: "Alice"},
	})
	require.NoError(t, err)
	a.SendTurn(context.Background(), s, "hello") //nolint:errcheck

	assert.Contains(t, gotBody, `"persona":"Alice"`)
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func TestHTTPAdapter_AuthBearer(t *testing.T) {
	t.Setenv("TEST_TOKEN", "tok1,tok2")
	var gotAuth string
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.Auth = config.AuthConfig{Type: "bearer", TokenEnv: "TEST_TOKEN"}
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	sendTurn(t, a, s, "hi")
	assert.True(t, strings.HasPrefix(gotAuth, "Bearer tok"), "want Bearer header, got %q", gotAuth)
}

func TestHTTPAdapter_AuthAPIKey(t *testing.T) {
	t.Setenv("API_KEY", "secret-key")
	var gotHeader string
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotHeader = r.Header.Get("X-My-Key")
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.Auth = config.AuthConfig{Type: "api_key", TokenEnv: "API_KEY", Header: "X-My-Key"}
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	sendTurn(t, a, s, "hi")
	assert.Equal(t, "secret-key", gotHeader)
}

func TestHTTPAdapter_AuthBasic(t *testing.T) {
	t.Setenv("BASIC_USER", "alice")
	t.Setenv("BASIC_PASS", "pw123")
	var gotAuth string
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.Auth = config.AuthConfig{Type: "basic", UsernameEnv: "BASIC_USER", TokenEnv: "BASIC_PASS"}
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	sendTurn(t, a, s, "hi")
	assert.True(t, strings.HasPrefix(gotAuth, "Basic "), "want Basic header, got %q", gotAuth)
}

// ── Token pool ────────────────────────────────────────────────────────────────

func TestHTTPAdapter_TokenPool_MultipleTokens(t *testing.T) {
	t.Setenv("TOKENS", "alpha,beta")
	seen := make(map[string]bool)
	var mu sync.Mutex
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seen[auth] = true
		mu.Unlock()
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.Auth = config.AuthConfig{Type: "bearer", TokenEnv: "TOKENS"}
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	// Run two sessions concurrently; each should get a different token.
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := newSession(t, a)
			sendTurn(t, a, s, "turn")
			require.NoError(t, a.CloseSession(context.Background(), s))
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, seen, 2, "expected both tokens to be used")
}

func TestHTTPAdapter_TokenPool_BlocksWhenExhausted(t *testing.T) {
	t.Setenv("ONE_TOKEN", "only-token")
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.Auth = config.AuthConfig{Type: "bearer", TokenEnv: "ONE_TOKEN"}
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	// Acquire the only token.
	s1, err := a.CreateSession(context.Background(), types.SessionConfig{})
	require.NoError(t, err)

	// Second session should block until s1 is closed.
	unblocked := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s2, err := a.CreateSession(ctx, types.SessionConfig{})
		if err == nil {
			_ = a.CloseSession(context.Background(), s2)
		}
		close(unblocked)
	}()

	// Give the goroutine a moment to block, then free the token.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, a.CloseSession(context.Background(), s1))

	select {
	case <-unblocked:
		// success: second session unblocked after first closed
	case <-time.After(2 * time.Second):
		t.Fatal("second CreateSession did not unblock after token was returned")
	}
}

func TestHTTPAdapter_TokenPool_EmptyEnvFails(t *testing.T) {
	t.Setenv("EMPTY_TOKEN", "")
	c := cfg("http://localhost")
	c.Auth = config.AuthConfig{Type: "bearer", TokenEnv: "EMPTY_TOKEN"}
	_, err := httpadapter.New(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EMPTY_TOKEN")
}

// ── Session reset ─────────────────────────────────────────────────────────────

func TestHTTPAdapter_SessionReset(t *testing.T) {
	resetCalled := false
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method == nethttp.MethodDelete && strings.HasSuffix(r.URL.Path, "/session") {
			resetCalled = true
		}
		fmt.Fprintln(w, `{"reply":"ok"}`)
	})

	c := cfg(srv.URL)
	c.SessionReset = &config.SessionResetConfig{Method: "DELETE", Path: "/session"}
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	require.NoError(t, a.CloseSession(context.Background(), s))
	assert.True(t, resetCalled, "expected session reset endpoint to be called on CloseSession")
}

// ── SSE streaming ─────────────────────────────────────────────────────────────

func TestHTTPAdapter_SSEStreaming(t *testing.T) {
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(nethttp.Flusher)
		require.True(t, ok)

		events := []string{
			`{"delta":"Hello"}`,
			`{"delta":" world"}`,
			`{"delta":"!"}`,
		}
		for _, ev := range events {
			fmt.Fprintf(w, "data: %s\n\n", ev)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})

	c := cfg(srv.URL)
	c.Streaming = true
	c.StreamingType = "sse"
	c.ResponsePath = "delta"
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	reply := sendTurn(t, a, s, "hi")
	assert.Equal(t, "Hello world!", reply)
}

// ── WebSocket streaming ───────────────────────────────────────────────────────

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *nethttp.Request) bool { return true },
}

func TestHTTPAdapter_WebSocketStreaming(t *testing.T) {
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Read incoming message (the rendered template).
		_, _, err = conn.ReadMessage()
		require.NoError(t, err)

		// Send three delta frames then a done frame.
		deltas := []map[string]any{
			{"content": "Hi ", "done": false},
			{"content": "there", "done": false},
			{"content": "!", "done": true},
		}
		for _, d := range deltas {
			data, _ := json.Marshal(d)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
		}
	})

	c := cfg(srv.URL)
	c.WebSocketURL = "ws" + strings.TrimPrefix(srv.URL, "http") // convert to ws://
	c.Streaming = true
	c.StreamingType = "websocket"
	c.ResponsePath = "content"
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	reply := sendTurn(t, a, s, "hello")
	require.NoError(t, a.CloseSession(context.Background(), s))
	assert.Equal(t, "Hi there!", reply)
}

// ── ActionCable streaming ─────────────────────────────────────────────────────

func TestHTTPAdapter_ActionCableStreaming(t *testing.T) {
	srv := echoServer(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Handle subscribe command → send confirm_subscription
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var sub map[string]string
		require.NoError(t, json.Unmarshal(msg, &sub))
		assert.Equal(t, "subscribe", sub["command"])

		confirm := map[string]string{"type": "confirm_subscription", "identifier": sub["identifier"]}
		data, _ := json.Marshal(confirm)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))

		// Read the message command.
		_, _, err = conn.ReadMessage()
		require.NoError(t, err)

		// Send broadcast deltas.
		ident := sub["identifier"]
		deltas := []map[string]any{
			{"content": "Sure", "done": false},
			{"content": ", here", "done": false},
			{"content": " you go.", "done": true},
		}
		for _, d := range deltas {
			msgData, _ := json.Marshal(d)
			frame := map[string]any{"identifier": ident, "message": json.RawMessage(msgData)}
			raw, _ := json.Marshal(frame)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, raw))
		}
	})

	c := cfg(srv.URL)
	c.WebSocketURL = "ws" + strings.TrimPrefix(srv.URL, "http")
	c.Streaming = true
	c.StreamingType = "actioncable"
	c.ResponsePath = "content"
	a, err := httpadapter.New(c)
	require.NoError(t, err)

	s := newSession(t, a)
	reply := sendTurn(t, a, s, "please help")
	require.NoError(t, a.CloseSession(context.Background(), s))
	assert.Equal(t, "Sure, here you go.", reply)
}

// ── New() errors ──────────────────────────────────────────────────────────────

func TestHTTPAdapter_BadTemplate(t *testing.T) {
	c := cfg("http://localhost")
	c.RequestTemplate = `{{.Unclosed`
	_, err := httpadapter.New(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request_template")
}

func TestHTTPAdapter_MissingTokenEnv(t *testing.T) {
	c := cfg("http://localhost")
	c.Auth = config.AuthConfig{Type: "bearer", TokenEnv: "NONEXISTENT_ENV_VAR_XYZ"}
	_, err := httpadapter.New(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NONEXISTENT_ENV_VAR_XYZ")
}
