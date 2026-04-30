// Package llm defines the LLM modality contract used by transport and
// implemented by providers.
package llm

import (
	"context"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
)

// ChatRequest is the provider-agnostic input.
type ChatRequest struct {
	RequestID   string
	Model       string
	Messages    []v1.Message
	Temperature *float64
	MaxTokens   *int
}

// ChatStream is a one-shot stream of v1 events. Callers MUST drain Events()
// until it closes, or call Close to abort. Events emitted will include zero
// or more `delta` events, exactly one terminator (`end` or `error`).
type ChatStream interface {
	// Events returns a receive-only channel of v1 events.
	Events() <-chan v1.Event
	// Close aborts the stream; safe to call concurrently and multiple times.
	Close() error
}

// Provider is implemented by every LLM upstream.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (ChatStream, error)
}
