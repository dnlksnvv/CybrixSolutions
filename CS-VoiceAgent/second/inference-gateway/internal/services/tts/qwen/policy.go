package qwen

import (
	"context"

	"github.com/cybrix/inference-gateway/internal/services/tts"
)

// PolicyQwenRealtime wraps QwenRealtime and enforces per-profile session.start policies.
type PolicyQwenRealtime struct {
	inner  *QwenRealtime
	policy QwenTTSClientPolicy
}

// NewPolicyQwenRealtime builds a registry provider with template + field policies.
func NewPolicyQwenRealtime(cfg QwenRealtimeConfig, policy QwenTTSClientPolicy) *PolicyQwenRealtime {
	return &PolicyQwenRealtime{
		inner:  NewQwenRealtime(cfg),
		policy: policy,
	}
}

// Synthesize implements Provider.
func (p *PolicyQwenRealtime) Synthesize(ctx context.Context, params tts.SessionParams) (tts.Session, error) {
	resolved, err := ResolveQwenTTSSession(params, p.policy, p.inner.cfg)
	if err != nil {
		return nil, err
	}
	return p.inner.Synthesize(ctx, resolved)
}
