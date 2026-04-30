// Package stt implements Speech-to-Text providers and the modality contract.
package stt

import (
	"context"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
)

// SessionParams are the values resolved from the client's session.start.
type SessionParams struct {
	RequestID   string
	Model       string
	Language    string
	AudioFormat string
	SampleRate  int
}

// Session is a bidirectional STT session driven by the client.
type Session interface {
	PushAudio(ctx context.Context, pcm []byte) error
	Finish(ctx context.Context) error
	Events() <-chan v1.Event
	Close() error
}

// Provider is implemented by every STT upstream.
type Provider interface {
	Recognize(ctx context.Context, p SessionParams) (Session, error)
}
