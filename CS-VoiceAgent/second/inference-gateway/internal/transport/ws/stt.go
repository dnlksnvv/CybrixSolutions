// stt.go: WebSocket handler for /v1/stt/ws.
//
// Symmetric to tts.go but the data direction is mostly client -> upstream.
// The client streams audio.chunk frames; the gateway forwards them to the
// provider session and emits transcript events back.
package ws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/registry"
	"github.com/cybrix/inference-gateway/internal/services/stt"
)

// NewSTTHandler returns the http.Handler for /v1/stt/ws.
func NewSTTHandler(reg *registry.Registry, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("stt ws upgrade failed", "err", err)
			return
		}
		defer conn.Close()
		runSTTSession(r.Context(), conn, reg, logger)
	})
}

func runSTTSession(parentCtx context.Context, conn *websocket.Conn, reg *registry.Registry, logger *slog.Logger) {
	conn.SetReadLimit(1 << 18)
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	mt, raw, err := conn.ReadMessage()
	if err != nil || mt != websocket.TextMessage {
		writeWSError(conn, "", v1.CodeBadRequest, "expected text session.start frame")
		return
	}
	var start v1.STTSessionStart
	if err := json.Unmarshal(raw, &start); err != nil || start.Type != v1.EventSessionStart {
		writeWSError(conn, "", v1.CodeBadRequest, "first message must be session.start json")
		return
	}
	if start.RequestID == "" || start.Model == "" {
		writeWSError(conn, start.RequestID, v1.CodeBadRequest, "request_id and model are required")
		return
	}

	provider := reg.STT(start.Model)
	if provider == nil {
		writeWSError(conn, start.RequestID, v1.CodeModelUnknown, "unknown stt model: "+start.Model)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1008, "model unknown"), time.Now().Add(time.Second))
		return
	}

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	session, err := provider.Recognize(ctx, stt.SessionParams{
		RequestID:   start.RequestID,
		Model:       start.Model,
		Language:    start.Language,
		AudioFormat: start.AudioFormat,
		SampleRate:  start.SampleRate,
	})
	if err != nil {
		writeWSError(conn, start.RequestID, v1.CodeInternal, err.Error())
		return
	}
	defer session.Close()

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		for ev := range session.Events() {
			if err := writeJSONFrame(conn, ev); err != nil {
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		readCh := make(chan readResult, 1)
		go pumpReads(conn, readCh)
		for {
			select {
			case <-gctx.Done():
				return gctx.Err()
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(2*time.Second))
			case msg := <-readCh:
				if msg.err != nil {
					return msg.err
				}
				if err := dispatchSTTClient(gctx, session, msg.payload); err != nil {
					return err
				}
			}
		}
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		origin, reason := classifySTTWSEndOrigin(err)
		logger.Info("stt ws session ended",
			"request_id", start.RequestID,
			"component", "stt",
			"origin", origin,
			"reason", reason,
			"err", err,
		)
	}
}

func classifySTTWSEndOrigin(err error) (origin, reason string) {
	if err == nil {
		return "proxy", "none"
	}
	if ce := new(websocket.CloseError); errors.As(err, &ce) {
		return "client", "ws_close"
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return "client", "io_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "proxy", "context_canceled"
	}
	return "proxy", "transport_error"
}

func dispatchSTTClient(ctx context.Context, session stt.Session, raw []byte) error {
	var ev v1.Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil
	}
	switch ev.Type {
	case v1.EventAudioChunk:
		if ev.PCMB64 == "" {
			return nil
		}
		pcm, err := base64.StdEncoding.DecodeString(ev.PCMB64)
		if err != nil {
			return nil
		}
		return session.PushAudio(ctx, pcm)
	case v1.EventInputFinish:
		return session.Finish(ctx)
	case v1.EventCancel:
		return session.Close()
	default:
		return nil
	}
}
