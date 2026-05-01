package http

import (
	"context"
	"net/http"

	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

const (
	HeaderUserID         = "X-User-Id"
	HeaderUserScopes     = "X-User-Scopes"
	HeaderOrganizationID = "X-Organization-Id"
)

type contextKey string

const (
	ctxKeyUserID         contextKey = "user_id"
	ctxKeyUserScopes     contextKey = "user_scopes"
	ctxKeyOrganizationID contextKey = "organization_id"
)

func RequireUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get(HeaderUserID)
		if userID == "" {
			writeError(w, http.StatusUnauthorized, "missing X-User-Id header")
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxKeyUserID, domain.UserSub(userID))
		if scopes := r.Header.Get(HeaderUserScopes); scopes != "" {
			ctx = context.WithValue(ctx, ctxKeyUserScopes, scopes)
		}
		if orgID := r.Header.Get(HeaderOrganizationID); orgID != "" {
			ctx = context.WithValue(ctx, ctxKeyOrganizationID, orgID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFrom(ctx context.Context) (domain.UserSub, bool) {
	v, ok := ctx.Value(ctxKeyUserID).(domain.UserSub)
	return v, ok
}
