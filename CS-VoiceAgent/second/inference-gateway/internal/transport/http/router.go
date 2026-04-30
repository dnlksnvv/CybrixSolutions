// Package httpserver wires HTTP and WebSocket handlers into a single net/http
// router. WebSocket endpoints are mounted from the ws subpackage.
package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/cybrix/inference-gateway/internal/registry"
)

// Deps groups everything a transport layer needs.
type Deps struct {
	Logger   *slog.Logger
	Registry *registry.Registry
}

// NewRouter builds the top-level mux. wsHandlerTTS / wsHandlerSTT are
// constructed by the ws package and passed in to keep this file
// transport-agnostic.
func NewRouter(d Deps, wsHandlerTTS, wsHandlerSTT http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		llm, tts, stt := d.Registry.Models()
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"models": map[string][]string{"llm": llm, "tts": tts, "stt": stt},
		})
	})

	mux.Handle("POST /v1/llm/chat", &llmHandler{deps: d})
	mux.Handle("GET /v1/tts/ws", wsHandlerTTS)
	mux.Handle("GET /v1/stt/ws", wsHandlerSTT)

	return logging(d.Logger, mux)
}

// logging is a tiny middleware that adds a structured log line per request.
func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("http request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
