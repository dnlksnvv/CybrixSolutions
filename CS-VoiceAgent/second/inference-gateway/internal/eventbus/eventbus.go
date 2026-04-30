// Package eventbus is a tiny helper used by provider sessions to safely
// publish v1 events to a bounded channel without leaking goroutines or
// risking panics on Close.
//
// Usage:
//
//	bus := eventbus.New(64)
//	go func() { defer bus.Close(); bus.Emit(...) ... }()
//	for ev := range bus.Out() { ... }
//
// Close is idempotent and safe to call from any goroutine. After Close, Emit
// becomes a no-op.
package eventbus

import (
	"sync"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
)

// Bus is a single-producer/single-consumer-friendly event channel.
//
// Internally a `done` channel guards the data channel so we never close `out`
// while emitters might still write to it; producers select on `done` first.
type Bus struct {
	out  chan v1.Event
	done chan struct{}
	once sync.Once
}

// New allocates a Bus with the given send-buffer size.
func New(buf int) *Bus {
	if buf <= 0 {
		buf = 32
	}
	return &Bus{
		out:  make(chan v1.Event, buf),
		done: make(chan struct{}),
	}
}

// Out returns the receive channel for consumers. After Close it eventually
// drains and is closed by the bus itself.
func (b *Bus) Out() <-chan v1.Event { return b.out }

// Emit non-blockingly writes ev unless the bus has been closed; in the
// closed case it is a no-op. If the buffer is full, Emit blocks until either
// a receiver pulls or the bus is closed.
func (b *Bus) Emit(ev v1.Event) {
	select {
	case <-b.done:
		return
	default:
	}
	select {
	case b.out <- ev:
	case <-b.done:
	}
}

// Close marks the bus closed and closes the output channel. Calling Close
// multiple times is safe.
func (b *Bus) Close() {
	b.once.Do(func() {
		close(b.done)
		close(b.out)
	})
}

// Done returns a channel that is closed once Close has been called. Useful
// for upstream goroutines that should exit when the consumer disappears.
func (b *Bus) Done() <-chan struct{} { return b.done }
