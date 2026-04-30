// llm.go: handler for POST /v1/llm/chat. Streams provider events as NDJSON.
package httpserver

import (
	"encoding/json"
	"net/http"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/services/llm"
)

type llmHandler struct{ deps Deps }

func (h *llmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONErr(w, http.StatusMethodNotAllowed, v1.CodeBadRequest, "method not allowed")
		return
	}

	var req v1.LLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONErr(w, http.StatusBadRequest, v1.CodeBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.RequestID == "" || req.Model == "" || len(req.Input.Messages) == 0 {
		writeJSONErr(w, http.StatusBadRequest, v1.CodeBadRequest, "request_id, model and input.messages are required")
		return
	}

	provider := h.deps.Registry.LLM(req.Model)
	if provider == nil {
		writeJSONErr(w, http.StatusBadRequest, v1.CodeModelUnknown, "unknown llm model: "+req.Model)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONErr(w, http.StatusInternalServerError, v1.CodeInternal, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	stream, err := provider.Chat(r.Context(), llm.ChatRequest{
		RequestID:   req.RequestID,
		Model:       req.Model,
		Messages:    req.Input.Messages,
		Temperature: req.Input.Temperature,
		MaxTokens:   req.Input.MaxTokens,
	})
	if err != nil {
		writeNDJSON(w, flusher, v1.ErrorEvent(req.RequestID, providerErrCode(err), err.Error(), false))
		return
	}
	defer stream.Close()

	enc := json.NewEncoder(w)
	for ev := range stream.Events() {
		if err := enc.Encode(ev); err != nil {
			h.deps.Logger.Warn("write llm event failed", "err", err, "request_id", req.RequestID)
			return
		}
		flusher.Flush()
	}
}

func writeNDJSON(w http.ResponseWriter, f http.Flusher, ev v1.Event) {
	enc := json.NewEncoder(w)
	_ = enc.Encode(ev)
	f.Flush()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"type": "error", "code": code, "message": msg})
}

// providerErrCode unwraps a typed v1.CodedError to its stable code, falling
// back to INTERNAL.
func providerErrCode(err error) string {
	if c, ok := err.(v1.CodedError); ok {
		return c.Code()
	}
	return v1.CodeInternal
}
