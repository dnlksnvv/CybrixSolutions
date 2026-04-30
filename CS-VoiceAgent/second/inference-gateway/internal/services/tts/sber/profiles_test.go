package sber

import (
	"errors"
	"testing"

	"github.com/cybrix/inference-gateway/internal/services/tts"
)

func TestResolveSaluteSessionRequiredVoice(t *testing.T) {
	pol := ClientPolicy{Voice: ClientRequired, SampleRate: ClientOptional}
	def := SynthConfig{Voice: "template-voice", SampleRate: 24000}
	_, err := ResolveSaluteSession(tts.SessionParams{}, pol, def)
	if err == nil || !errors.Is(err, ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
	out, err := ResolveSaluteSession(tts.SessionParams{Voice: "from-client"}, pol, def)
	if err != nil {
		t.Fatal(err)
	}
	if out.Voice != "from-client" {
		t.Fatalf("voice %q", out.Voice)
	}
}

func TestResolveSaluteSessionDefaultVoiceUsesTemplateInSynth(t *testing.T) {
	pol := ClientPolicy{Voice: ClientDefault}
	def := SynthConfig{Voice: "template-voice"}
	out, err := ResolveSaluteSession(tts.SessionParams{Voice: "ignored"}, pol, def)
	if err != nil {
		t.Fatal(err)
	}
	if out.Voice != "" {
		t.Fatalf("expected empty so Synth uses cfg default, got %q", out.Voice)
	}
}
