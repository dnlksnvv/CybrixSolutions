package qwen

import (
	"errors"
	"testing"

	"github.com/cybrix/inference-gateway/internal/services/tts"
)

func TestResolveQwenTTSSessionRequiredVoice(t *testing.T) {
	pol := QwenTTSClientPolicy{Voice: QwenClientRequired, LanguageType: QwenClientOptional}
	def := QwenRealtimeConfig{Voice: "template-voice", LanguageType: "template-lang"}
	_, err := ResolveQwenTTSSession(tts.SessionParams{}, pol, def)
	if err == nil || !errors.Is(err, ErrQwenTTSBadRequest) {
		t.Fatalf("expected ErrQwenTTSBadRequest, got %v", err)
	}
	out, err := ResolveQwenTTSSession(tts.SessionParams{Voice: "from-client-voice"}, pol, def)
	if err != nil {
		t.Fatal(err)
	}
	if out.Voice != "from-client-voice" {
		t.Fatalf("voice %q", out.Voice)
	}
}

func TestResolveQwenTTSSessionDefaultVoiceIgnoresClient(t *testing.T) {
	pol := QwenTTSClientPolicy{Voice: QwenClientDefault}
	def := QwenRealtimeConfig{Voice: "template-voice"}
	out, err := ResolveQwenTTSSession(tts.SessionParams{Voice: "should-ignore"}, pol, def)
	if err != nil {
		t.Fatal(err)
	}
	if out.Voice != "" {
		t.Fatalf("expected empty param so provider uses cfg default, got %q", out.Voice)
	}
}
