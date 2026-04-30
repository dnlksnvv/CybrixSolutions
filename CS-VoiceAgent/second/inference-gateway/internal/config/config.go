// Package config is the only place that reads process environment.
//
// Other packages must accept already-parsed sub-structs through constructors;
// they MUST NOT call os.Getenv directly.
//
// Naming convention for env keys: <MODALITY>_<MODEL>_<FIELD>, e.g.
//
//	LLM_GIGA_CHAT_CREDENTIALS
//	TTS_QWEN_REALTIME_API_KEY
//	STT_QWEN_REALTIME_LANGUAGE
//
// Qwen / Sber TTS: YAML under model-templates/tts/{qwen-realtime,sber}/ or
// TTS_QWEN_TEMPLATE_DIR / TTS_SBER_TEMPLATE_DIR.
// Qwen STT: YAML under model-templates/stt/qwen-realtime/ or STT_QWEN_TEMPLATE_DIR.
// LLM: YAML under model-templates/llm/{sber,qwen}/ or LLM_*_TEMPLATE_DIR / LLM_TEMPLATE_DIR.
// Secrets for templates: credentials_env / api_key_env in YAML → os.Getenv in loaders.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the root configuration assembled at process start.
type Config struct {
	HTTP HTTPConfig

	// Global timeouts.
	UpstreamHTTPTimeout time.Duration
	UpstreamWSTimeout   time.Duration
}

// HTTPConfig configures the gateway listener.
type HTTPConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// --- LLM configs ---

// LLMGigaChatConfig — Sber GigaChat (OAuth NGW + OpenAI-compat chat).
type LLMGigaChatConfig struct {
	Credentials string // base64 of client_id:client_secret (no "Basic " prefix)
	Scope       string
	OAuthURL    string
	BaseURL     string
	Model       string
	VerifyTLS   bool

	// Generation defaults; used when the client request omits the field.
	// nil means "don't send to upstream — let upstream pick its own default".
	Temperature *float64
	MaxTokens   *int
}

func (c LLMGigaChatConfig) Enabled() bool { return strings.TrimSpace(c.Credentials) != "" }

// LLMQwenConfig — Alibaba DashScope OpenAI-compatible LLM.
type LLMQwenConfig struct {
	APIKey  string
	BaseURL string
	Model   string

	// Generation defaults; used when the client request omits the field.
	Temperature *float64
	MaxTokens   *int
}

func (c LLMQwenConfig) Enabled() bool { return strings.TrimSpace(c.APIKey) != "" }

// Load reads .env (best-effort) then assembles Config from environment.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		HTTP: HTTPConfig{
			Addr:            envStr("GATEWAY_ADDR", ":8080"),
			ReadTimeout:     envMs("GATEWAY_READ_TIMEOUT_MS", 60_000),
			WriteTimeout:    envMs("GATEWAY_WRITE_TIMEOUT_MS", 0),
			ShutdownTimeout: envMs("GATEWAY_SHUTDOWN_TIMEOUT_MS", 10_000),
		},
		UpstreamHTTPTimeout: envMs("UPSTREAM_HTTP_TIMEOUT_MS", 60_000),
		UpstreamWSTimeout:   envMs("UPSTREAM_WS_TIMEOUT_MS", 30_000),
	}

	if cfg.HTTP.Addr == "" {
		return nil, fmt.Errorf("GATEWAY_ADDR is empty")
	}
	return cfg, nil
}

// --- helpers ---

func envStr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func envMs(k string, defMs int) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return time.Duration(defMs) * time.Millisecond
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return time.Duration(defMs) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}
