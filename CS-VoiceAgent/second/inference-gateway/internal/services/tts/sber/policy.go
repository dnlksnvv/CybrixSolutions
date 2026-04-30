package sber

import (
	"context"

	"github.com/cybrix/inference-gateway/internal/services/tts"
)

// PolicySynth wraps Synth and enforces per-profile session.start policies.
type PolicySynth struct {
	inner  *Synth
	policy ClientPolicy
}

// NewPolicySynth builds a registry provider with template + field policies.
func NewPolicySynth(cfg SynthConfig, policy ClientPolicy) *PolicySynth {
	return &PolicySynth{
		inner:  NewSynth(cfg),
		policy: policy,
	}
}

// Synthesize implements tts.Provider.
func (p *PolicySynth) Synthesize(ctx context.Context, params tts.SessionParams) (tts.Session, error) {
	resolved, err := ResolveSaluteSession(params, p.policy, p.inner.cfg)
	if err != nil {
		return nil, err
	}
	return p.inner.Synthesize(ctx, resolved)
}
