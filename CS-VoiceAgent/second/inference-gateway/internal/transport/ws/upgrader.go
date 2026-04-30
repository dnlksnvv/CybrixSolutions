// Package ws hosts WebSocket handlers for TTS and STT.
//
// Sessions: each handler upgrades the client request, then waits for a
// `session.start` event before opening the upstream provider. Upstream is
// closed only after the client side of the WS terminates (or sends `cancel`).
package ws

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// pingInterval and pongTimeout govern WS heartbeats. Per architecture rules:
// ping every 20s, drop session if no pong in 60s.
const (
	pingInterval = 20 * time.Second
	pongTimeout  = 60 * time.Second
)

// upgrader is shared by all WS handlers. Origin checks are intentionally
// permissive — the gateway is meant to be deployed behind a proper edge that
// enforces origin/IP/auth. Tighten this when those policies move in-house.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1 << 14,
	WriteBufferSize: 1 << 14,
	CheckOrigin:     func(*http.Request) bool { return true },
}
