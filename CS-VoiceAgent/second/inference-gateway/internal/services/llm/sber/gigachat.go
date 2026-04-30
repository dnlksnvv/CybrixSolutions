// Package sber implements the Sber GigaChat LLM provider (OpenAI-compatible
// /api/v1/chat/completions). Auth: NGW OAuth2 via sberauth.Source.
package sber

import (
	"context"
	"time"

	"github.com/cybrix/inference-gateway/internal/config"
	"github.com/cybrix/inference-gateway/internal/sberauth"
	"github.com/cybrix/inference-gateway/internal/services/llm"
	"github.com/cybrix/inference-gateway/internal/services/llm/openaicompat"
)

// GigaChat is an LLM provider for Sber GigaChat.
type GigaChat struct{ inner *openaicompat.Client }

// NewGigaChat constructs a provider from an already-loaded provider config.
func NewGigaChat(cfg config.LLMGigaChatConfig, upstreamTimeout time.Duration) *GigaChat {
	src := sberauth.NewSource(cfg.Credentials, cfg.Scope, cfg.OAuthURL, cfg.VerifyTLS)
	httpc := openaicompat.MakeHTTPClient(upstreamTimeout, cfg.VerifyTLS)
	auth := func(ctx context.Context) (string, error) {
		t, err := src.Token(ctx)
		if err != nil {
			return "", err
		}
		return "Bearer " + t, nil
	}
	return &GigaChat{
		inner: openaicompat.NewClient(cfg.BaseURL, httpc, "gigachat", cfg.Model, auth),
	}
}

// Chat implements llm.Provider.
func (p *GigaChat) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatStream, error) {
	return p.inner.Chat(ctx, req)
}
