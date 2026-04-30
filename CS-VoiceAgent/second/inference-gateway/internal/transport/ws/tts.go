// tts.go: WebSocket handler for /v1/tts/ws.
//
// Lifecycle (per architecture rules):
//  1. Upgrade WS.
//  2. Read session.start, resolve provider via registry.
//  3. Open provider session.
//  4. Run two goroutines (clientReader, upstreamReader) under errgroup.
//  5. Close upstream only after client connection terminates or client
//     sends `cancel`.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/registry"
	"github.com/cybrix/inference-gateway/internal/services/tts"
	"github.com/cybrix/inference-gateway/internal/services/tts/qwen"
	"github.com/cybrix/inference-gateway/internal/services/tts/sber"
)

// NewTTSHandler returns the http.Handler for /v1/tts/ws.
func NewTTSHandler(reg *registry.Registry, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("tts ws upgrade failed", "err", err)
			return
		}
		defer conn.Close()
		runTTSSession(r.Context(), conn, reg, logger)
	})
}

func runTTSSession(parentCtx context.Context, conn *websocket.Conn, reg *registry.Registry, logger *slog.Logger) {
	conn.SetReadLimit(1 << 18) // 256KB cap on incoming messages

	// 1) Wait for session.start.
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	mt, raw, err := conn.ReadMessage()
	if err != nil || mt != websocket.TextMessage {
		writeWSError(conn, "", v1.CodeBadRequest, "expected text session.start frame")
		return
	}
	var start v1.TTSSessionStart
	if err := json.Unmarshal(raw, &start); err != nil || start.Type != v1.EventSessionStart {
		writeWSError(conn, "", v1.CodeBadRequest, "first message must be session.start json")
		return
	}
	if start.RequestID == "" || start.Model == "" {
		writeWSError(conn, start.RequestID, v1.CodeBadRequest, "request_id and model are required")
		return
	}

	provider := reg.TTS(start.Model)
	if provider == nil {
		writeWSError(conn, start.RequestID, v1.CodeModelUnknown, "unknown tts model: "+start.Model)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1008, "model unknown"), time.Now().Add(time.Second))
		return
	}

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	params := tts.SessionParams{
		RequestID:    start.RequestID,
		Model:        start.Model,
		Voice:        start.Voice,
		AudioFormat:  start.AudioFormat,
		SampleRate:   start.SampleRate,
		LanguageType: start.LanguageType,
		Mode:         start.Mode,
	}
	turns := newTTSTurnRouter(start.Model)

	var session tts.Session
	if reg.TTSDualUpstream(start.Model) {
		session, err = newDualTTSSession(ctx, provider, params, logger)
	} else {
		session, err = provider.Synthesize(ctx, params)
	}
	if err != nil {
		code := v1.CodeInternal
		if errors.Is(err, qwen.ErrQwenTTSBadRequest) || errors.Is(err, sber.ErrBadRequest) {
			code = v1.CodeBadRequest
		}
		writeWSError(conn, start.RequestID, code, err.Error())
		return
	}
	defer session.Close()

	// Heartbeat: server pings, expects pong.
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})

	g, gctx := errgroup.WithContext(ctx)
	logger.Info("tts turn router",
		"component", "tts",
		"request_id", start.RequestID,
		"model", start.Model,
		"enabled", turns.enabled,
	)

	// upstream -> client
	g.Go(func() error {
		for ev := range session.Events() {
			if out, ok := turns.Apply(logger, start.RequestID, ev); ok {
				ev = out
			} else {
				continue
			}
			if err := writeJSONFrame(conn, ev); err != nil {
				return err
			}
		}
		return nil
	})

	// client -> upstream
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
				if err := dispatchTTSClient(gctx, session, turns, logger, msg.payload, start.RequestID); err != nil {
					return err
				}
			}
		}
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		origin, reason := classifyWSEndOrigin(err)
		logger.Info("tts ws session ended",
			"request_id", start.RequestID,
			"component", "tts",
			"origin", origin,
			"reason", reason,
			"err", err,
		)
	}
}

func classifyWSEndOrigin(err error) (origin, reason string) {
	if err == nil {
		return "proxy", "none"
	}
	if ce := new(websocket.CloseError); errors.As(err, &ce) {
		// close frame from downstream client side of gateway WS
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

type readResult struct {
	mt      int
	payload []byte
	err     error
}

func pumpReads(conn *websocket.Conn, out chan<- readResult) {
	defer close(out)
	for {
		mt, raw, err := conn.ReadMessage()
		out <- readResult{mt: mt, payload: raw, err: err}
		if err != nil {
			return
		}
	}
}

func dispatchTTSClient(
	ctx context.Context,
	session tts.Session,
	turns *ttsTurnRouter,
	logger *slog.Logger,
	raw []byte,
	requestID string,
) error {
	var ev v1.Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil // ignore malformed frames silently; upstream still streams
	}
	switch ev.Type {
	case v1.EventInputText:
		turns.OnInputText(ev.TurnID)
		if ta, ok := session.(turnAwareSession); ok {
			ta.OnTurn(ev.TurnID)
		}
		if turns.enabled && ev.TurnID != "" {
			logger.Info("tts turn input",
				"component", "tts",
				"request_id", requestID,
				"turn_id", ev.TurnID,
				"text_len", len(ev.Text),
			)
		}
		return session.PushText(ctx, ev.Text)
	case v1.EventInputCommit:
		return session.Commit(ctx)
	case v1.EventCancel:
		turns.OnCancel()
		return session.Cancel(ctx)
	default:
		_ = requestID
		return nil
	}
}

type ttsTurnRouter struct {
	enabled bool
	mu      sync.Mutex
	pending string
	active  string
}

type turnAwareSession interface {
	OnTurn(turnID string)
}

type dualTTSSession struct {
	ctx      context.Context
	provider tts.Provider
	params   tts.SessionParams
	logger   *slog.Logger

	mu          sync.RWMutex
	sessions    [2]tts.Session
	active      int
	currentTurn string
	closed      bool

	out chan v1.Event
	wg  sync.WaitGroup
}

func newTTSTurnRouter(model string) *ttsTurnRouter {
	_ = model
	// Universal turn-id tagging layer for every TTS model.
	return &ttsTurnRouter{enabled: true}
}

func newDualTTSSession(
	ctx context.Context,
	provider tts.Provider,
	params tts.SessionParams,
	logger *slog.Logger,
) (*dualTTSSession, error) {
	d := &dualTTSSession{
		ctx:      ctx,
		provider: provider,
		params:   params,
		logger:   logger,
		active:   0,
		out:      make(chan v1.Event, 256),
	}
	for i := 0; i < 2; i++ {
		s, err := provider.Synthesize(ctx, params)
		if err != nil {
			d.closeSessions()
			return nil, fmt.Errorf("dual tts init slot %d: %w", i, err)
		}
		d.sessions[i] = s
		d.startForwarder(i, s)
	}
	logger.Info("tts dual upstream enabled",
		"component", "tts",
		"request_id", params.RequestID,
		"model", params.Model,
	)
	return d, nil
}

func (d *dualTTSSession) OnTurn(turnID string) {
	if turnID == "" {
		return
	}
	var old tts.Session
	var oldIdx int
	d.mu.Lock()
	if d.closed || turnID == d.currentTurn {
		d.mu.Unlock()
		return
	}
	d.currentTurn = turnID
	oldIdx = d.active
	nextIdx := 1 - d.active
	if d.sessions[nextIdx] == nil {
		s, err := d.provider.Synthesize(d.ctx, d.params)
		if err == nil {
			d.sessions[nextIdx] = s
			d.startForwarder(nextIdx, s)
			d.logger.Info("tts slot reopen",
				"component", "tts",
				"request_id", d.params.RequestID,
				"slot", nextIdx,
				"reason", "missing_on_switch",
			)
		}
	}
	if d.sessions[nextIdx] != nil {
		d.active = nextIdx
		old = d.sessions[oldIdx]
		d.sessions[oldIdx] = nil
	}
	d.mu.Unlock()

	if old != nil {
		d.logger.Info("tts slot switch",
			"component", "tts",
			"request_id", d.params.RequestID,
			"turn_id", turnID,
			"from_slot", oldIdx,
			"to_slot", 1-oldIdx,
		)
	}
	if old != nil {
		d.logger.Info("tts slot close",
			"component", "tts",
			"request_id", d.params.RequestID,
			"slot", oldIdx,
			"reason", "turn_switch",
		)
		_ = old.Close()
		go d.reopenSlot(oldIdx)
	}
}

func (d *dualTTSSession) reopenSlot(idx int) {
	s, err := d.provider.Synthesize(d.ctx, d.params)
	if err != nil {
		d.logger.Warn("tts dual reopen failed", "component", "tts", "slot", idx, "err", err)
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		_ = s.Close()
		return
	}
	d.sessions[idx] = s
	d.mu.Unlock()
	d.logger.Info("tts slot reopen",
		"component", "tts",
		"request_id", d.params.RequestID,
		"slot", idx,
		"reason", "background_recover",
	)
	d.startForwarder(idx, s)
}

func (d *dualTTSSession) startForwarder(idx int, s tts.Session) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for ev := range s.Events() {
			d.mu.RLock()
			closed := d.closed
			active := d.active
			d.mu.RUnlock()
			if closed {
				return
			}
			if idx != active {
				continue
			}
			d.out <- ev
		}
	}()
}

func (d *dualTTSSession) activeSession() tts.Session {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sessions[d.active]
}

func (d *dualTTSSession) PushText(ctx context.Context, text string) error {
	s := d.activeSession()
	if s == nil {
		return fmt.Errorf("dual tts: active session unavailable")
	}
	return s.PushText(ctx, text)
}

func (d *dualTTSSession) Commit(ctx context.Context) error {
	s := d.activeSession()
	if s == nil {
		return fmt.Errorf("dual tts: active session unavailable")
	}
	return s.Commit(ctx)
}

func (d *dualTTSSession) Cancel(ctx context.Context) error {
	s := d.activeSession()
	if s == nil {
		return nil
	}
	return s.Cancel(ctx)
}

func (d *dualTTSSession) Events() <-chan v1.Event { return d.out }

func (d *dualTTSSession) closeSessions() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	ss := d.sessions
	d.sessions = [2]tts.Session{}
	d.mu.Unlock()
	for _, s := range ss {
		if s != nil {
			d.logger.Info("tts slot close",
				"component", "tts",
				"request_id", d.params.RequestID,
				"reason", "session_close",
			)
			_ = s.Close()
		}
	}
	d.wg.Wait()
	close(d.out)
}

func (d *dualTTSSession) Close() error {
	d.closeSessions()
	return nil
}

func (r *ttsTurnRouter) OnInputText(turnID string) {
	if !r.enabled || turnID == "" {
		return
	}
	r.mu.Lock()
	r.pending = turnID
	r.mu.Unlock()
}

func (r *ttsTurnRouter) OnCancel() {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	r.active = ""
	r.mu.Unlock()
}

func (r *ttsTurnRouter) Apply(logger *slog.Logger, requestID string, ev v1.Event) (v1.Event, bool) {
	if !r.enabled {
		return ev, true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch ev.Type {
	case v1.EventAudioChunk:
		// No filtering: never drop chunks. We only attach the latest pending
		// turn id (if any) for observability/correlation in logs and client.
		if r.pending != "" {
			r.active = r.pending
		}
		if ev.TurnID == "" && r.active != "" {
			ev.TurnID = r.active
		}
		if ev.TurnID != "" {
			logger.Info("tts turn output",
				"component", "tts",
				"request_id", requestID,
				"turn_id", ev.TurnID,
				"seq", ev.Seq,
			)
		}
		return ev, true
	case v1.EventAudioEnd:
		if ev.TurnID == "" && r.active != "" {
			ev.TurnID = r.active
		}
		r.active = ""
		return ev, true
	case v1.EventError:
		if r.active != "" {
			ev.TurnID = r.active
		} else if r.pending != "" {
			ev.TurnID = r.pending
		}
		return ev, true
	default:
		return ev, true
	}
}

func writeJSONFrame(conn *websocket.Conn, payload any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, buf)
}

func writeWSError(conn *websocket.Conn, requestID, code, msg string) {
	_ = writeJSONFrame(conn, v1.ErrorEvent(requestID, code, msg, false))
}
