// Package dashscopews is a tiny wrapper around the DashScope realtime
// WebSocket protocol (`/api-ws/v1/realtime?model=...`).
//
// The protocol is the same for Qwen TTS realtime and Qwen Omni (ASR/LLM
// realtime). All client→server messages are JSON envelopes:
//
//	{"event_id": "event_<32 hex>", "type": "<type>", ...}
//
// The server emits JSON text frames; binary frames are not used.
package dashscopews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Client is a single upstream WebSocket session. It is goroutine-safe to
// call Send concurrently; one read loop is spawned by Run and forwards each
// JSON event to the user-supplied handler.
type Client struct {
	url    string
	apiKey string
	model  string
	dialer *websocket.Dialer

	mu            sync.Mutex
	ws            *websocket.Conn
	lastCloseCode int
	lastCloseText string
}

// New creates a Client that will dial baseURL?model=<model> with bearer auth.
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		url:    buildURL(baseURL, model),
		apiKey: apiKey,
		model:  model,
		dialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

func buildURL(base, model string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("model", model)
	u.RawQuery = q.Encode()
	return u.String()
}

// Connect dials the upstream and returns when the WebSocket handshake is
// complete. Subsequent Send calls are valid until Close.
func (c *Client) Connect(ctx context.Context) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.apiKey)
	header.Set("User-Agent", "cybrix-inference-gateway/1.0")
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ws, resp, err := c.dialer.DialContext(dialCtx, c.url, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dashscopews dial %s: %w (status=%d)", c.url, err, resp.StatusCode)
		}
		return fmt.Errorf("dashscopews dial %s: %w", c.url, err)
	}
	c.ws = ws
	return nil
}

// EventID returns a fresh "event_<hex>" id (DashScope expects this format).
func EventID() string {
	return "event_" + uuid.NewString()
}

// Send marshals the envelope and writes it as a text frame.
func (c *Client) Send(envelope map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ws == nil {
		return fmt.Errorf("dashscopews: not connected")
	}
	if _, ok := envelope["event_id"]; !ok {
		envelope["event_id"] = EventID()
	}
	buf, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("dashscopews marshal: %w", err)
	}
	_ = c.ws.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return c.ws.WriteMessage(websocket.TextMessage, buf)
}

// Close terminates the WebSocket politely.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ws == nil {
		return nil
	}
	_ = c.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	err := c.ws.Close()
	c.ws = nil
	return err
}

// Run reads upstream messages until the connection closes or ctx is done,
// passing each parsed JSON object to handle. handle must not block on the
// websocket; it should fan out into a buffered channel.
//
// gorilla rejects RFC violations on read (e.g. control frame > 125 bytes) and
// returns a generic error like "websocket: len > 125 for control" without
// surfacing the original close reason. To still give operators *some* hint
// about what the upstream said, we also install a CloseHandler that captures
// the (truncated) close payload before gorilla complains.
func (c *Client) Run(ctx context.Context, handle func(map[string]any)) error {
	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return fmt.Errorf("dashscopews: not connected")
	}
	ws.SetCloseHandler(func(code int, text string) error {
		// gorilla's default close handler echoes a close back; replicate it
		// so the upstream sees graceful teardown.
		_ = ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, ""), time.Now().Add(time.Second))
		c.mu.Lock()
		c.lastCloseCode = code
		c.lastCloseText = text
		c.mu.Unlock()
		return nil
	})
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		handle(obj)
	}
}

// LastClose returns the close code/text observed on the most recent close
// frame received from the upstream. Both are zero-valued if the upstream
// dropped without a parseable close (e.g., RFC-violating frame, TCP RST).
func (c *Client) LastClose() (int, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastCloseCode, c.lastCloseText
}

// IsBenignClose reports whether err is the WebSocket-equivalent of a graceful
// connection teardown that callers should not surface as an upstream failure:
//
//   - normal close codes (1000 NormalClosure, 1001 GoingAway, …),
//   - oversized control frames ("websocket: len > 125 for control") — DashScope
//     occasionally sends a long JSON in the close reason, which strictly
//     violates RFC 6455 §5.5 and gorilla/websocket rejects on read,
//   - io.EOF / io.ErrUnexpectedEOF after a clean upstream close,
//   - "use of closed network connection" when our own Close() races the read.
//
// Run still returns the underlying error so the caller can log it; this
// helper just lets the provider downgrade it from "UPSTREAM_5XX" to "session
// finished".
func IsBenignClose(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) {
		return true
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "len > 125 for control"):
		return true
	case strings.Contains(msg, "use of closed network connection"):
		return true
	case strings.Contains(msg, "broken pipe"):
		return true
	}
	return false
}
