// Package v1 holds the public envelope for the inference-gateway API.
//
// This is the single source of truth for request/response JSON shapes.
// Adding fields is allowed; renaming/removing is a breaking change and must
// happen in v2.
package v1

import "encoding/json"

const Version = "v1"

// Event types (server -> client and client -> server).
const (
	EventDelta             = "delta"
	EventEnd               = "end"
	EventError             = "error"
	EventSessionStart      = "session.start"
	EventSessionEnd        = "session.end"
	EventInputText         = "input.text"
	EventInputCommit       = "input.commit"
	EventInputFinish       = "input.finish"
	EventCancel            = "cancel"
	EventAudioChunk        = "audio.chunk"
	EventAudioURL          = "audio.url"
	EventAudioEnd          = "audio.end"
	EventTranscriptPartial = "transcript.partial"
	EventTranscriptFinal   = "transcript.final"
)

// Common media types for Event.MediaType. Empty means raw PCM s16le mono.
const (
	MediaPCM16 = "audio/pcm"
	MediaWAV   = "audio/wav"
	MediaMP3   = "audio/mpeg"
	MediaOpus  = "audio/opus"
)

// Stable error codes.
const (
	CodeBadRequest      = "BAD_REQUEST"
	CodeModelUnknown    = "MODEL_UNKNOWN"
	CodeUpstreamTimeout = "UPSTREAM_TIMEOUT"
	CodeUpstream4XX     = "UPSTREAM_4XX"
	CodeUpstream5XX     = "UPSTREAM_5XX"
	CodeInternal        = "INTERNAL"
	CodeNotImplemented  = "NOT_IMPLEMENTED"
)

// Message is a single chat-completion message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMInput is the LLM-specific input block.
//
// Universal across every LLM model in the registry — providers translate it
// into their wire format (OpenAI-compat for Qwen/GigaChat, etc.). Fields that
// a given provider doesn't support are silently ignored.
type LLMInput struct {
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

// LLMRequest is the POST body for /v1/llm/chat. Response is NDJSON of Event
// (delta / end / error). Streaming is implied by the endpoint; LLMInput.Stream
// is kept for forward compat with non-streaming providers.
type LLMRequest struct {
	RequestID string   `json:"request_id"`
	CallID    string   `json:"call_id,omitempty"`
	Model     string   `json:"model"`
	Input     LLMInput `json:"input"`
}

// Event is the canonical streaming event used on every transport.
//
// JSON shape on the wire is flat (one object). Fields that don't apply to a
// given Type are omitted. This struct is intentionally permissive so we can
// reuse it across LLM/TTS/STT without introducing a separate union type for
// each modality.
type Event struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`

	// LLM
	Text string `json:"text,omitempty"`

	// Audio
	Seq        int    `json:"seq,omitempty"`
	PCMB64     string `json:"pcm_b64,omitempty"`
	URL        string `json:"url,omitempty"`        // for audio.url
	MediaType  string `json:"media_type,omitempty"` // pcm/wav/mp3/opus
	SampleRate int    `json:"sample_rate,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`

	// STT timing
	StartMs *int64 `json:"start_ms,omitempty"`
	EndMs   *int64 `json:"end_ms,omitempty"`

	// Error
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

// TTSSessionStart is the first message on /v1/tts/ws.
//
// Universal across every TTS model in the registry. For Qwen TTS realtime,
// each YAML profile under model-templates/tts/qwen-realtime may mark fields
// as default-only, optional override, or required — see config qwen_tts.go.
// Salute ignores language_type / mode / sample_rate hints. Type MUST be
// EventSessionStart.
type TTSSessionStart struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	CallID    string `json:"call_id,omitempty"`
	Model     string `json:"model"`

	// Voice is provider-specific:
	//   - Qwen flash: preset name (Cherry, …)
	//   - Qwen VC:    cloned voice id (qwen-tts-vc-…)
	//   - Salute:     stock voice id (Bys_24000, …)
	// Empty → take from .env for that model.
	Voice string `json:"voice,omitempty"`

	// LanguageType is the DashScope-style synthesis language for Qwen TTS:
	// "Russian" | "Auto" | "English" | … . Salute ignores this field.
	// Empty → take from .env for that model.
	LanguageType string `json:"language_type,omitempty"`

	// Mode is DashScope Qwen TTS realtime text buffering: "commit" (client must
	// send input_text_buffer.commit to flush) or "server_commit" (server
	// commits on silence / end). Salute ignores this field.
	// Empty → template default from gateway Qwen TTS YAML profile.
	Mode string `json:"mode,omitempty"`

	// SampleRate (Hz) for output audio. Empty → take from .env. Qwen
	// realtime defaults to 24000.
	SampleRate int `json:"sample_rate,omitempty"`

	// AudioFormat is a hint, e.g. "pcm_s16le" (LiveKit) or "pcm16". Qwen
	// realtime ignores it. Salute maps pcm_s16le → pcm16 for the REST API.
	AudioFormat string `json:"audio_format,omitempty"`
}

// STTSessionStart is the first message on /v1/stt/ws.
//
// Universal across every STT model in the registry. Type MUST be
// EventSessionStart.
type STTSessionStart struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	CallID    string `json:"call_id,omitempty"`
	Model     string `json:"model"`

	// Language is the ISO code of the spoken audio (e.g. "ru", "en"). For
	// Qwen ASR realtime it is forwarded as transcription_params.language.
	// Empty → take from .env for that model.
	Language string `json:"language,omitempty"`

	// SampleRate (Hz) of the PCM in audio.chunk frames. Qwen ASR realtime
	// expects 16000. Empty → take from .env.
	SampleRate int `json:"sample_rate,omitempty"`

	// AudioFormat hint: "pcm_s16le" mono. Currently informational only;
	// providers consume PCM s16le mono regardless.
	AudioFormat string `json:"audio_format,omitempty"`
}

// ErrorEvent is a small helper to construct a v1 error event.
func ErrorEvent(requestID, code, message string, retryable bool) Event {
	return Event{
		Type:      EventError,
		RequestID: requestID,
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
}

// Marshal serialises an Event to bytes (no newline). Callers add framing.
func (e Event) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// CodedError is implemented by errors that carry a stable v1 error code.
// Providers emit these so transport can pick the right code without
// stringy unwrapping.
type CodedError interface {
	error
	Code() string
}
