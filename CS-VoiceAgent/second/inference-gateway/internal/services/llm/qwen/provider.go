// Package qwen implements the Alibaba DashScope Qwen LLM provider
// (OpenAI-compatible compatible-mode/v1/chat/completions).
package qwen

import (
	"context"
	"time"

	"github.com/cybrix/inference-gateway/internal/config"
	"github.com/cybrix/inference-gateway/internal/services/llm"
	"github.com/cybrix/inference-gateway/internal/services/llm/openaicompat"
)

// Qwen is the DashScope OpenAI-compatible LLM provider.
type Qwen struct{ inner *openaicompat.Client }

// NewQwen constructs a provider from an already-loaded provider config.
func NewQwen(cfg config.LLMQwenConfig, upstreamTimeout time.Duration) *Qwen {
	httpc := openaicompat.MakeHTTPClient(upstreamTimeout, true)
	auth := func(ctx context.Context) (string, error) {
		return "Bearer " + cfg.APIKey, nil
	}
	return &Qwen{
		inner: openaicompat.NewClient(cfg.BaseURL, httpc, "qwen", cfg.Model, auth),
	}
}

// Chat implements llm.Provider.
func (p *Qwen) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatStream, error) {
	return p.inner.Chat(ctx, req)
}
