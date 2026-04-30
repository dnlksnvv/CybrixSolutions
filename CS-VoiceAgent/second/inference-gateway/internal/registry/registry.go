// Package registry maps stable public model IDs to provider implementations.
//
// The registry is populated once in cmd/server/main.go after config has been
// loaded. Lookups are read-only and concurrent-safe (the underlying maps are
// never mutated after Build).
package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cybrix/inference-gateway/internal/services/llm"
	"github.com/cybrix/inference-gateway/internal/services/stt"
	"github.com/cybrix/inference-gateway/internal/services/tts"
)

// Registry holds modal lookups.
type Registry struct {
	llm map[string]llm.Provider
	tts map[string]tts.Provider
	stt map[string]stt.Provider

	// ttsDualUpstream lists public TTS model ids that use two upstream slots
	// (see transport/ws dual session). Populated at startup — not inferred from
	// string heuristics in the WS handler.
	ttsDualUpstream map[string]struct{}
}

// New creates an empty registry to be filled at startup.
func New() *Registry {
	return &Registry{
		llm:             make(map[string]llm.Provider),
		tts:             make(map[string]tts.Provider),
		stt:             make(map[string]stt.Provider),
		ttsDualUpstream: make(map[string]struct{}),
	}
}

// RegisterLLM binds a model id to an LLM provider. Re-registration is a fatal
// error during startup (caller should treat the returned error as fatal).
func (r *Registry) RegisterLLM(model string, p llm.Provider) error {
	model = normalize(model)
	if _, ok := r.llm[model]; ok {
		return fmt.Errorf("llm model %q already registered", model)
	}
	r.llm[model] = p
	return nil
}

// RegisterTTS binds a model id to a TTS provider.
func (r *Registry) RegisterTTS(model string, p tts.Provider) error {
	model = normalize(model)
	if _, ok := r.tts[model]; ok {
		return fmt.Errorf("tts model %q already registered", model)
	}
	r.tts[model] = p
	return nil
}

// RegisterSTT binds a model id to an STT provider.
func (r *Registry) RegisterSTT(model string, p stt.Provider) error {
	model = normalize(model)
	if _, ok := r.stt[model]; ok {
		return fmt.Errorf("stt model %q already registered", model)
	}
	r.stt[model] = p
	return nil
}

// LLM returns the registered provider or nil if unknown.
func (r *Registry) LLM(model string) llm.Provider { return r.llm[normalize(model)] }

// TTS returns the registered provider or nil if unknown.
func (r *Registry) TTS(model string) tts.Provider { return r.tts[normalize(model)] }

// MarkTTSDualUpstream records that this public model id should use dual upstream
// TTS sessions. Call once per id after RegisterTTS.
func (r *Registry) MarkTTSDualUpstream(model string) {
	model = normalize(model)
	if model == "" {
		return
	}
	r.ttsDualUpstream[model] = struct{}{}
}

// TTSDualUpstream reports whether the public model id was marked at registration.
func (r *Registry) TTSDualUpstream(model string) bool {
	_, ok := r.ttsDualUpstream[normalize(model)]
	return ok
}

// STT returns the registered provider or nil if unknown.
func (r *Registry) STT(model string) stt.Provider { return r.stt[normalize(model)] }

// Models returns the sorted list of registered model ids per modality.
// Useful for /healthz and structured startup logging.
func (r *Registry) Models() (llm, tts, stt []string) {
	llm = keys(r.llm)
	tts = keys(r.tts)
	stt = keys(r.stt)
	return
}

func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
