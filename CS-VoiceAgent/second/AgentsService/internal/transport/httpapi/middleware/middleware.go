package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const (
	// HeaderWorkspaceID — обязательный заголовок для всех endpoint'ов `/api/v1`.
	HeaderWorkspaceID = "X-Workspace-Id"
)

type ctxKey string

const (
	ctxKeyWorkspaceID ctxKey = "workspace_id"
)

// RequireWorkspaceID проверяет наличие `X-Workspace-Id` и кладёт его в контекст запроса.
// Это ключевое бизнес-правило из ТЗ: сервис обязан фильтровать все данные по workspace_id,
// а отсутствие заголовка считается ошибкой клиента.
func RequireWorkspaceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		ws := strings.TrimSpace(c.GetHeader(HeaderWorkspaceID))
		if ws == "" {
			// Формат ошибок и коды будут централизованы позже (domain/errors + http mapper).
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "validation_error",
					"field":   nil,
					"code":    "missing_workspace_id",
					"message": "X-Workspace-Id header is required",
				},
			})
			return
		}
		c.Set(string(ctxKeyWorkspaceID), ws)
		c.Next()
	}
}

// BodyLimit ограничивает максимальный размер тела запроса.
// В ТЗ явно указан лимит 8MB (выбранный как канонический в рамках этой реализации).
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = 8 * 1024 * 1024
	}
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// RequestLogger пишет компактный structured-log по каждому запросу.
// Важный нюанс: мы не логируем request body целиком (там может быть PII и большие workflow).
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Для ошибок 4xx полезно понимать, что пришло, но body может быть большим,
		// поэтому читаем только небольшой префикс (и только если нужно будет).
		var bodyPrefix string
		if c.Request != nil && c.Request.Body != nil && (c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPatch) {
			var raw []byte
			bodyPrefix, raw = peekBodyPrefixAndReset(c.Request.Body, 1024)
			c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		}

		c.Next()

		dur := time.Since(start)
		ev := log.Info()
		if c.Writer.Status() >= 500 {
			ev = log.Error()
		} else if c.Writer.Status() >= 400 {
			ev = log.Warn()
		}

		ev.
			Str("method", c.Request.Method).
			Str("path", c.FullPath()).
			Str("raw_path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("duration", dur).
			Int("bytes_out", c.Writer.Size()).
			Str("workspace_id", strings.TrimSpace(c.GetHeader(HeaderWorkspaceID))).
			Str("body_prefix", bodyPrefix).
			Msg("http request")
	}
}

func peekBodyPrefixAndReset(rc io.ReadCloser, n int64) (prefix string, raw []byte) {
	if rc == nil || n <= 0 {
		return "", nil
	}
	buf := &bytes.Buffer{}
	_, _ = io.CopyN(buf, rc, n)
	rest, _ := io.ReadAll(rc)
	_ = rc.Close()
	p := buf.Bytes()
	all := append(p, rest...)
	s := strings.ReplaceAll(string(p), "\n", "\\n")
	if len(s) > 1024 {
		s = s[:1024]
	}
	return s, all
}

