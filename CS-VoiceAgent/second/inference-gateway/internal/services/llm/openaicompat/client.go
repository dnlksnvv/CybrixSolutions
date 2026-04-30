// Package openaicompat implements a shared streaming SSE client for OpenAI-compatible
// chat/completions endpoints (DashScope Qwen, Sber GigaChat, etc.).
package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/services/llm"
)

// Client is the shared SSE client used by OpenAI-compatible LLM providers.
type Client struct {
	baseURL       string
	authHeader    func(ctx context.Context) (string, error)
	http          *http.Client
	upstream      string
	upstreamModel string
}

// NewClient constructs a client. upstream is a human label for logs/errors.
func NewClient(baseURL string, http *http.Client, upstream, upstreamModel string, authHeader func(ctx context.Context) (string, error)) *Client {
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		authHeader:    authHeader,
		http:          http,
		upstream:      upstream,
		upstreamModel: upstreamModel,
	}
}

// MakeHTTPClient returns an http.Client tuned for streaming responses.
func MakeHTTPClient(timeout time.Duration, verifyTLS bool) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: !verifyTLS},
			DisableCompression:  true,
			MaxIdleConnsPerHost: 32,
		},
	}
}

// Chat streams chat/completions for the given request.
func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatStream, error) {
	body := map[string]any{
		"model":    c.upstreamModel,
		"messages": req.Messages,
		"stream":   true,
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		body["max_tokens"] = *req.MaxTokens
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal: %w", c.upstream, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", c.upstream, err)
	}
	auth, err := c.authHeader(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: auth: %w", c.upstream, err)
	}
	httpReq.Header.Set("Authorization", auth)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: do: %w", c.upstream, err)
	}
	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		code := v1.CodeUpstream5XX
		if resp.StatusCode/100 == 4 {
			code = v1.CodeUpstream4XX
		}
		return nil, &providerError{
			CodeStr: code,
			Message: fmt.Sprintf("%s status %d: %s", c.upstream, resp.StatusCode, strings.TrimSpace(string(snippet))),
		}
	}

	stream := newSSEStream(req.RequestID, resp.Body)
	go stream.run(ctx)
	return stream, nil
}

type providerError struct {
	CodeStr string
	Message string
}

func (e *providerError) Error() string { return e.CodeStr + ": " + e.Message }

// Code implements v1.CodedError.
func (e *providerError) Code() string { return e.CodeStr }

type sseStream struct {
	requestID string
	body      io.ReadCloser
	out       chan v1.Event
	once      sync.Once
}

func newSSEStream(requestID string, body io.ReadCloser) *sseStream {
	return &sseStream{
		requestID: requestID,
		body:      body,
		out:       make(chan v1.Event, 32),
	}
}

func (s *sseStream) Events() <-chan v1.Event { return s.out }

func (s *sseStream) Close() error {
	s.once.Do(func() { _ = s.body.Close() })
	return nil
}

type chunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (s *sseStream) run(ctx context.Context) {
	defer close(s.out)
	defer s.Close()

	r := bufio.NewReader(s.body)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.emit(v1.Event{Type: v1.EventEnd, RequestID: s.requestID})
				return
			}
			s.emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, err.Error(), false))
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				s.emit(v1.Event{Type: v1.EventEnd, RequestID: s.requestID})
				return
			}
			continue
		}
		var ck chunk
		if err := json.Unmarshal([]byte(payload), &ck); err != nil {
			continue
		}
		for _, ch := range ck.Choices {
			if ch.Delta.Content != "" {
				s.emit(v1.Event{
					Type:      v1.EventDelta,
					RequestID: s.requestID,
					Text:      ch.Delta.Content,
				})
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				s.emit(v1.Event{Type: v1.EventEnd, RequestID: s.requestID})
				return
			}
		}
	}
}

func (s *sseStream) emit(e v1.Event) {
	select {
	case s.out <- e:
	default:
	}
}
