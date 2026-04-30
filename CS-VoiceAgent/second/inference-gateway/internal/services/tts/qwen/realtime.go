// realtime.go: DashScope Qwen TTS realtime WebSocket provider.
package qwen

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/cybrix/inference-gateway/internal/dashscopews"
	"github.com/cybrix/inference-gateway/internal/eventbus"
	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/services/tts"
	"github.com/gorilla/websocket"
)

// QwenRealtime is a TTS provider for DashScope qwen3-tts-*-realtime models.
type QwenRealtime struct {
	cfg QwenRealtimeConfig
}

// NewQwenRealtime creates a provider with the given configuration.
func NewQwenRealtime(cfg QwenRealtimeConfig) *QwenRealtime {
	return &QwenRealtime{cfg: cfg}
}

// Synthesize opens an upstream WS, sends session.update with voice/mode and
// returns a streaming session.
func (p *QwenRealtime) Synthesize(ctx context.Context, params tts.SessionParams) (tts.Session, error) {
	voice := pickStr(params.Voice, p.cfg.Voice)
	rate := params.SampleRate
	if rate == 0 {
		rate = p.cfg.SampleRate
	}

	rf := strings.ToLower(strings.TrimSpace(p.cfg.ResponseFormat))
	if rf == "" {
		return nil, fmt.Errorf("qwen-tts-realtime: response_format is empty (set defaults.response_format in template)")
	}
	if rf != "pcm" {
		return nil, fmt.Errorf("qwen-tts-realtime: unsupported response_format %q (only pcm is supported)", rf)
	}

	cli := dashscopews.New(p.cfg.WSURL, p.cfg.APIKey, p.cfg.Model)
	if err := cli.Connect(ctx); err != nil {
		return nil, fmt.Errorf("qwen-tts-realtime connect: %w", err)
	}

	sess := map[string]any{
		"voice":           voice,
		"mode":            qwenTTSMode(params.Mode, p.cfg.Mode),
		"response_format": rf,
		"sample_rate":     rate,
	}
	langType := strings.TrimSpace(params.LanguageType)
	if langType == "" {
		langType = strings.TrimSpace(p.cfg.LanguageType)
	}
	if langType != "" {
		sess["language_type"] = langType
	}
	if err := cli.Send(map[string]any{
		"type":    "session.update",
		"session": sess,
	}); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("qwen-tts-realtime session.update: %w", err)
	}

	q := &qwenRealtimeSession{
		client:     cli,
		bus:        eventbus.New(128),
		requestID:  params.RequestID,
		sampleRate: rate,
		model:      p.cfg.Model,
	}
	go q.readUpstream(ctx)
	return q, nil
}

type qwenRealtimeSession struct {
	client     *dashscopews.Client
	bus        *eventbus.Bus
	requestID  string
	sampleRate int
	model      string

	mu       sync.Mutex
	closed   bool
	seq      int
	gotAudio bool
}

func (s *qwenRealtimeSession) PushText(_ context.Context, text string) error {
	if text == "" {
		return nil
	}
	return s.client.Send(map[string]any{
		"type": "input_text_buffer.append",
		"text": text,
	})
}

func (s *qwenRealtimeSession) Commit(_ context.Context) error {
	return s.client.Send(map[string]any{
		"type": "input_text_buffer.commit",
	})
}

func (s *qwenRealtimeSession) Cancel(_ context.Context) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil
	}
	return s.client.Send(map[string]any{"type": "response.cancel"})
}

func (s *qwenRealtimeSession) Events() <-chan v1.Event { return s.bus.Out() }

func (s *qwenRealtimeSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	_ = s.client.Send(map[string]any{"type": "session.finish"})
	err := s.client.Close()
	s.bus.Close()
	return err
}

func (s *qwenRealtimeSession) readUpstream(ctx context.Context) {
	defer s.bus.Close()
	logger := slog.Default().With("provider", "qwen-tts-realtime", "model", s.model, "request_id", s.requestID)

	err := s.client.Run(ctx, func(obj map[string]any) {
		t, _ := obj["type"].(string)
		if strings.Contains(t, "error") || obj["error"] != nil {
			summary := summarizeUpstream(obj)
			logger.Warn("upstream emitted error", "origin", "upstream", "summary", summary)
			s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, "qwen-tts-realtime: "+summary, false))
			return
		}
		switch t {
		case "response.audio.delta", "response.output_audio.delta":
			delta, _ := obj["delta"].(string)
			if delta == "" {
				return
			}
			pcmB64 := delta
			s.mu.Lock()
			s.seq++
			seq := s.seq
			s.gotAudio = true
			s.mu.Unlock()
			s.bus.Emit(v1.Event{
				Type:       v1.EventAudioChunk,
				RequestID:  s.requestID,
				Seq:        seq,
				PCMB64:     pcmB64,
				MediaType:  v1.MediaPCM16,
				SampleRate: s.sampleRate,
			})
		case "response.done":
			s.bus.Emit(v1.Event{Type: v1.EventAudioEnd, RequestID: s.requestID})
		case "session.finished":
			return
		}
	})

	closedByUs := s.isClosed()
	gotAudio := s.didEmitAudio()
	closeCode, closeText := s.client.LastClose()

	if ce := new(websocket.CloseError); err != nil && errors.As(err, &ce) {
		closeCode, closeText = ce.Code, ce.Text
	}

	switch {
	case err == nil || closedByUs:
		return
	case gotAudio && dashscopews.IsBenignClose(err):
		logger.Debug("upstream WS closed after audio", "origin", "upstream", "err", err, "close_code", closeCode, "close_text", closeText)
		return
	case dashscopews.IsBenignClose(err):
		hint := "upstream closed before producing audio"
		if closeText != "" {
			hint += " (" + closeText + ")"
		} else if err != nil {
			hint += " (" + err.Error() + ")"
		}
		logger.Warn("upstream closed before any audio",
			"origin", "upstream",
			"err", err, "close_code", closeCode, "close_text", closeText)
		s.bus.Emit(v1.ErrorEvent(
			s.requestID,
			v1.CodeUpstream5XX,
			"qwen-tts-realtime: "+hint+". Check voice/language_type/model compatibility and API key.",
			false,
		))
	default:
		logger.Error("upstream read error", "origin", "upstream", "err", err, "close_code", closeCode, "close_text", closeText)
		s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, "qwen-tts-realtime: "+err.Error(), false))
	}
}

func (s *qwenRealtimeSession) didEmitAudio() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gotAudio
}

func summarizeUpstream(obj map[string]any) string {
	if e, ok := obj["error"].(map[string]any); ok {
		code, _ := e["code"].(string)
		msg, _ := e["message"].(string)
		if code != "" || msg != "" {
			return strings.TrimSpace(code + " " + msg)
		}
	}
	if t, ok := obj["type"].(string); ok && t != "" {
		if msg, _ := obj["message"].(string); msg != "" {
			return t + ": " + msg
		}
		return t
	}
	return fmt.Sprintf("%v", obj)
}

func (s *qwenRealtimeSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func pickStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func qwenTTSMode(fromClient, fromEnv string) string {
	for _, cand := range []string{fromClient, fromEnv} {
		switch strings.ToLower(strings.TrimSpace(cand)) {
		case "commit":
			return "commit"
		case "server_commit":
			return "server_commit"
		}
	}
	return "server_commit"
}

var _ tts.Session = (*qwenRealtimeSession)(nil)
