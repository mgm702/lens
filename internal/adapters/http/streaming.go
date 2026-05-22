package httpadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

// ── SSE ──────────────────────────────────────────────────────────────────────

// collectSSE reads a text/event-stream response body, extracts the content
// field from each data event using responsePath (gjson), and concatenates
// deltas into the full reply. Stops when a "data: [DONE]" sentinel appears
// or the stream closes.
func collectSSE(ctx context.Context, body io.Reader, responsePath string) (string, error) {
	var sb strings.Builder
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		if responsePath == "" {
			sb.WriteString(data)
			continue
		}
		delta := gjson.Get(data, responsePath).String()
		sb.WriteString(delta)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("http adapter: reading SSE stream: %w", err)
	}
	return sb.String(), nil
}

// ── Generic WebSocket ─────────────────────────────────────────────────────────

// isDone reports whether a message payload signals stream completion.
// If donePath is set, it checks whether gjson.Get(msgJSON, donePath) == doneValue.
// Otherwise it falls back to checking {"done":true} for backwards compatibility.
func isDone(msgJSON, donePath, doneValue string) bool {
	if donePath != "" {
		return gjson.Get(msgJSON, donePath).String() == doneValue
	}
	return gjson.Get(msgJSON, "done").Bool()
}

// collectWebSocket reads JSON text frames from an established WebSocket
// connection until isDone returns true or the connection closes.
// responsePath (gjson) is applied to each frame to extract the content delta.
func collectWebSocket(ctx context.Context, sess *session, responsePath, donePath, doneValue string) (string, error) {
	var sb strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		sess.wsMu.Lock()
		_, msg, err := sess.wsConn.ReadMessage()
		sess.wsMu.Unlock()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				break
			}
			return "", fmt.Errorf("http adapter: websocket read: %w", err)
		}

		raw := string(msg)
		if responsePath != "" {
			delta := gjson.Get(raw, responsePath).String()
			sb.WriteString(delta)
		} else {
			sb.WriteString(raw)
		}

		if isDone(raw, donePath, doneValue) {
			break
		}
	}
	return sb.String(), nil
}

// ── ActionCable ──────────────────────────────────────────────────────────────

// acSubscribeMsg is the ActionCable subscribe command.
type acSubscribeMsg struct {
	Command    string `json:"command"`
	Identifier string `json:"identifier"`
}

// acMessageMsg is the ActionCable message command.
type acMessageMsg struct {
	Command    string `json:"command"`
	Identifier string `json:"identifier"`
	Data       string `json:"data"`
}

// acIncoming is the shape of messages received from ActionCable.
type acIncoming struct {
	Type       string          `json:"type"`
	Identifier string          `json:"identifier"`
	Message    json.RawMessage `json:"message"`
}

// subscribeActionCable sends the subscribe command and waits for confirmation.
func subscribeActionCable(conn *websocket.Conn, channel string) error {
	ident, err := json.Marshal(map[string]string{"channel": channel})
	if err != nil {
		return err
	}
	sub := acSubscribeMsg{
		Command:    "subscribe",
		Identifier: string(ident),
	}
	data, _ := json.Marshal(sub)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("subscribe write: %w", err)
	}
	// Wait for confirm_subscription
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("subscribe read: %w", err)
		}
		var inc acIncoming
		if err := json.Unmarshal(msg, &inc); err != nil {
			continue
		}
		if inc.Type == "confirm_subscription" {
			return nil
		}
		if inc.Type == "reject_subscription" {
			return fmt.Errorf("actioncable: subscription rejected for channel %q", channel)
		}
	}
}

// sendActionCableMessage sends a message over an established ActionCable channel.
// body is the inner data payload (the rendered request template).
func sendActionCableMessage(conn *websocket.Conn, channel, body string) error {
	ident, err := json.Marshal(map[string]string{"channel": channel})
	if err != nil {
		return err
	}
	msg := acMessageMsg{
		Command:    "message",
		Identifier: string(ident),
		Data:       body,
	}
	data, _ := json.Marshal(msg)
	return conn.WriteMessage(websocket.TextMessage, data)
}

// collectActionCable reads ActionCable broadcast messages until isDone returns
// true for the message payload. responsePath (gjson against the inner `message`
// field) extracts content deltas from each broadcast.
func collectActionCable(ctx context.Context, sess *session, responsePath, donePath, doneValue string) (string, error) {
	var sb strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		sess.wsMu.Lock()
		_, raw, err := sess.wsConn.ReadMessage()
		sess.wsMu.Unlock()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				break
			}
			return "", fmt.Errorf("http adapter: actioncable read: %w", err)
		}

		var inc acIncoming
		if err := json.Unmarshal(raw, &inc); err != nil || inc.Message == nil {
			continue // ping, welcome, etc.
		}

		msgJSON := string(inc.Message)
		if responsePath != "" {
			delta := gjson.Get(msgJSON, responsePath).String()
			sb.WriteString(delta)
		} else {
			sb.WriteString(msgJSON)
		}

		if isDone(msgJSON, donePath, doneValue) {
			break
		}
	}
	return sb.String(), nil
}
