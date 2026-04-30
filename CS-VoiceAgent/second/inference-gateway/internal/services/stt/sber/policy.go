package sber

import (
	"context"

	"github.com/cybrix/inference-gateway/internal/services/stt"
)

// PolicyRealtime wraps Realtime and enforces per-profile session.start policies.
type PolicyRealtime struct {
	inner  *Realtime
	policy ClientPolicy
}

// NewPolicyRealtime builds a registry provider with template + field policies.
func NewPolicyRealtime(cfg RealtimeConfig, policy ClientPolicy) *PolicyRealtime {
	return &PolicyRealtime{
		inner:  NewRealtime(cfg),
		policy: policy,
	}
}

// Recognize implements stt.Provider.
func (p *PolicyRealtime) Recognize(ctx context.Context, params stt.SessionParams) (stt.Session, error) {
	resolved, err := ResolveSession(params, p.policy, p.inner.cfg)
	if err != nil {
		return nil, err
	}
	return p.inner.Recognize(ctx, resolved)
}
