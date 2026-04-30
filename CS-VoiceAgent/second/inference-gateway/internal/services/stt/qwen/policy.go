package qwen

import (
	"context"

	"github.com/cybrix/inference-gateway/internal/services/stt"
)

// PolicyQwenRealtime wraps QwenRealtime and enforces per-profile session.start policies.
type PolicyQwenRealtime struct {
	inner  *QwenRealtime
	policy QwenSTTClientPolicy
}

// NewPolicyQwenRealtime builds a registry provider with template + field policies.
func NewPolicyQwenRealtime(cfg QwenRealtimeConfig, policy QwenSTTClientPolicy) *PolicyQwenRealtime {
	return &PolicyQwenRealtime{
		inner:  NewQwenRealtime(cfg),
		policy: policy,
	}
}

// Recognize implements stt.Provider.
func (p *PolicyQwenRealtime) Recognize(ctx context.Context, params stt.SessionParams) (stt.Session, error) {
	resolved, err := ResolveQwenSTTSession(params, p.policy, p.inner.cfg)
	if err != nil {
		return nil, err
	}
	return p.inner.Recognize(ctx, resolved)
}
