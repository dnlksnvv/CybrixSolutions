// Package qwen implements DashScope Qwen ASR realtime (WebSocket) for STT.
//
// Wire protocol (matches dashscope.audio.qwen_omni.OmniRealtimeConversation):
//
//	URL: <base>?model=<MODEL>             (qwen3-asr-flash-realtime)
//	auth: Authorization: Bearer <DASHSCOPE_API_KEY>
//	c→s 1) {"type":"session.update", "session": {
//	          "modalities": ["text"], "voice": null,
//	          "input_audio_format": "pcm", "output_audio_format": "pcm16",
//	          "input_audio_transcription": {"language": "ru"},
//	          "turn_detection": {"type":"server_vad","threshold":...,
//	             "prefix_padding_ms":...,"silence_duration_ms":...},
//	          "sample_rate": 16000
//	      }}
//	c→s 2) {"type":"input_audio_buffer.append", "audio": "<base64 PCM>"}
//	c→s 3) {"type":"input_audio_buffer.commit"}
//	c→s 4) {"type":"session.finish"}
//	s→c    conversation.item.input_audio_transcription.text  (interim, "stash")
//	s→c    conversation.item.input_audio_transcription.completed (final, "transcript")
//	s→c    session.finished / error / response.error / session.error
package qwen

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/cybrix/inference-gateway/internal/dashscopews"
	"github.com/cybrix/inference-gateway/internal/eventbus"
	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/services/stt"
	"github.com/gorilla/websocket"
)

// QwenRealtimeConfig is upstream connection + template defaults (from YAML).
type QwenRealtimeConfig struct {
	APIKey             string
	WSURL              string
	Model              string
	Language           string
	SampleRate         int
	VADThreshold       float64
	VADSilenceMs       int
	VADPrefixPaddingMs int
}

// Enabled reports whether this profile can open an upstream session.
func (c QwenRealtimeConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.WSURL) != ""
}

// QwenRealtime is an STT provider for DashScope qwen3-asr-*-realtime models.
type QwenRealtime struct {
	cfg QwenRealtimeConfig
}

// NewQwenRealtime creates a provider from a resolved template config.
func NewQwenRealtime(cfg QwenRealtimeConfig) *QwenRealtime {
	return &QwenRealtime{cfg: cfg}
}

// Recognize opens an upstream WS, sends session.update with VAD/transcription
// params and returns a streaming session.
func (p *QwenRealtime) Recognize(ctx context.Context, params stt.SessionParams) (stt.Session, error) {
	lang := pickStr(params.Language, p.cfg.Language)
	rate := params.SampleRate
	if rate == 0 {
		rate = p.cfg.SampleRate
	}

	cli := dashscopews.New(p.cfg.WSURL, p.cfg.APIKey, p.cfg.Model)
	if err := cli.Connect(ctx); err != nil {
		return nil, fmt.Errorf("qwen-stt-realtime connect: %w", err)
	}

	transcription := map[string]any{}
	if lang != "" {
		transcription["language"] = lang
	}
	sess := map[string]any{
		"modalities":                []string{"text"},
		"voice":                     nil,
		"input_audio_format":        "pcm",
		"output_audio_format":       "pcm16",
		"input_audio_transcription": transcription,
		"turn_detection": map[string]any{
			"type":                "server_vad",
			"threshold":           p.cfg.VADThreshold,
			"prefix_padding_ms":   p.cfg.VADPrefixPaddingMs,
			"silence_duration_ms": p.cfg.VADSilenceMs,
		},
		"sample_rate": rate,
	}
	if err := cli.Send(map[string]any{"type": "session.update", "session": sess}); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("qwen-stt-realtime session.update: %w", err)
	}

	s := &qwenRealtimeSession{
		client:    cli,
		bus:       eventbus.New(128),
		requestID: params.RequestID,
		language:  lang,
		model:     p.cfg.Model,
	}
	go s.readUpstream(ctx)
	return s, nil
}

type qwenRealtimeSession struct {
	client    *dashscopews.Client
	bus       *eventbus.Bus
	requestID string
	language  string
	model     string

	mu            sync.Mutex
	closed        bool
	gotTranscript bool
}

func (s *qwenRealtimeSession) PushAudio(_ context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	return s.client.Send(map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(pcm),
	})
}

func (s *qwenRealtimeSession) Finish(_ context.Context) error {
	if err := s.client.Send(map[string]any{"type": "input_audio_buffer.commit"}); err != nil {
		return err
	}
	return s.client.Send(map[string]any{"type": "session.finish"})
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
	err := s.client.Close()
	s.bus.Close()
	return err
}

func (s *qwenRealtimeSession) readUpstream(ctx context.Context) {
	defer s.bus.Close()
	logger := slog.Default().With("provider", "qwen-stt-realtime", "model", s.model, "request_id", s.requestID)

	err := s.client.Run(ctx, func(obj map[string]any) {
		t, _ := obj["type"].(string)
		if obj["error"] != nil || strings.HasSuffix(t, ".error") || t == "error" {
			summary := summarizeUpstream(obj)
			logger.Warn("upstream emitted error", "origin", "upstream", "summary", summary)
			s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, "qwen-stt-realtime: "+summary, false))
			return
		}
		switch t {
		case "conversation.item.input_audio_transcription.text":
			text, _ := obj["stash"].(string)
			text = strings.TrimSpace(text)
			if text == "" {
				return
			}
			s.markGotTranscript()
			s.bus.Emit(v1.Event{
				Type:      v1.EventTranscriptPartial,
				RequestID: s.requestID,
				Text:      text,
			})
		case "conversation.item.input_audio_transcription.completed":
			text, _ := obj["transcript"].(string)
			text = strings.TrimSpace(text)
			if text == "" {
				return
			}
			s.markGotTranscript()
			s.bus.Emit(v1.Event{
				Type:      v1.EventTranscriptFinal,
				RequestID: s.requestID,
				Text:      text,
			})
		case "session.finished":
			s.bus.Emit(v1.Event{Type: v1.EventSessionEnd, RequestID: s.requestID})
		}
	})

	closedByUs := s.isClosed()
	gotTranscript := s.didGetTranscript()
	closeCode, closeText := s.client.LastClose()
	if ce := new(websocket.CloseError); err != nil && errors.As(err, &ce) {
		closeCode, closeText = ce.Code, ce.Text
	}

	switch {
	case err == nil || closedByUs:
		return
	case gotTranscript && dashscopews.IsBenignClose(err):
		logger.Debug("upstream WS closed after transcripts", "origin", "upstream", "err", err, "close_code", closeCode, "close_text", closeText)
		return
	case dashscopews.IsBenignClose(err):
		hint := "upstream closed before producing any transcript"
		if closeText != "" {
			hint += " (" + closeText + ")"
		} else if err != nil {
			hint += " (" + err.Error() + ")"
		}
		logger.Warn("upstream closed before any transcript",
			"origin", "upstream",
			"err", err, "close_code", closeCode, "close_text", closeText)
		s.bus.Emit(v1.ErrorEvent(
			s.requestID,
			v1.CodeUpstream5XX,
			"qwen-stt-realtime: "+hint+". Check language/sample_rate/model compatibility and API key.",
			false,
		))
	default:
		logger.Error("upstream read error", "origin", "upstream", "err", err, "close_code", closeCode, "close_text", closeText)
		s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, "qwen-stt-realtime: "+err.Error(), false))
	}
}

func (s *qwenRealtimeSession) markGotTranscript() {
	s.mu.Lock()
	s.gotTranscript = true
	s.mu.Unlock()
}

func (s *qwenRealtimeSession) didGetTranscript() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gotTranscript
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

var _ stt.Session = (*qwenRealtimeSession)(nil)
