// Package tts implements Text-to-Speech providers and the modality contract.
package tts

import (
	"context"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
)

// SessionParams are the values resolved from the client's session.start.
type SessionParams struct {
	RequestID    string
	Model        string
	Voice        string
	AudioFormat  string
	SampleRate   int
	LanguageType string // DashScope: Russian | Auto | English | …
	Mode         string // DashScope: commit | server_commit
}

// Session is a bidirectional TTS session driven by the client.
type Session interface {
	PushText(ctx context.Context, text string) error
	Commit(ctx context.Context) error
	Cancel(ctx context.Context) error
	Events() <-chan v1.Event
	Close() error
}

// Provider is implemented by every TTS upstream.
type Provider interface {
	Synthesize(ctx context.Context, p SessionParams) (Session, error)
}
