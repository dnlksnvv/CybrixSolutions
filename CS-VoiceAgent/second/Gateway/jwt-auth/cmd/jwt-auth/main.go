package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	Issuer            string
	JWKSURI           string
	Audiences         []string
	OrgAudiencePrefix string
	AllowedAlgs       []string
	RequiredScopes    map[string]struct{}
	Port              string
}

func main() {
	cfg := loadConfig()

	ctx := context.Background()
	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{cfg.JWKSURI})
	if err != nil {
		log.Fatalf("failed to initialize JWKS from %s: %v", cfg.JWKSURI, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/auth/verify", verifyHandler(cfg, jwks))

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("jwt-auth listening on :%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func loadConfig() Config {
	issuer := strings.TrimRight(strings.TrimSpace(os.Getenv("LOGTO_ISSUER")), "/")
	if issuer == "" {
		log.Fatal("LOGTO_ISSUER is required")
	}
	jwksURI := strings.TrimSpace(os.Getenv("LOGTO_JWKS_URI"))
	if jwksURI == "" {
		jwksURI = issuer + "/jwks"
	}

	rawAudience := strings.TrimSpace(os.Getenv("LOGTO_AUDIENCE"))
	if rawAudience == "" {
		log.Fatal("LOGTO_AUDIENCE is required")
	}
	audiences := splitByComma(rawAudience)
	orgAudPrefix := strings.TrimSpace(os.Getenv("LOGTO_ORGANIZATION_AUDIENCE_PREFIX"))
	allowedAlgs := splitByComma(strings.TrimSpace(os.Getenv("JWT_ALLOWED_ALGS")))
	if len(allowedAlgs) == 0 {
		// Logto Cloud commonly signs with ES384; keep RS256 for compatibility.
		allowedAlgs = []string{"ES384", "RS256"}
	}

	requiredScopes := make(map[string]struct{})
	for _, scope := range strings.Fields(os.Getenv("REQUIRED_SCOPES")) {
		requiredScopes[scope] = struct{}{}
	}

	port := strings.TrimSpace(os.Getenv("AUTH_PORT"))
	if port == "" {
		port = "8000"
	}

	return Config{
		Issuer:            issuer,
		JWKSURI:           jwksURI,
		Audiences:         audiences,
		OrgAudiencePrefix: orgAudPrefix,
		AllowedAlgs:       allowedAlgs,
		RequiredScopes:    requiredScopes,
		Port:              port,
	}
}

func splitByComma(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func verifyHandler(cfg Config, jwks keyfunc.Keyfunc) http.HandlerFunc {
	parserOptions := []jwt.ParserOption{
		jwt.WithValidMethods(cfg.AllowedAlgs),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	}
	parser := jwt.NewParser(parserOptions...)

	return func(w http.ResponseWriter, r *http.Request) {
		// CORS preflight: no Authorization header; Traefik forwardAuth still calls this endpoint.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		tokenString, err := bearerToken(r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if !looksLikeJWT(tokenString) {
			http.Error(w, "opaque access token provided; request JWT access token with API resource/audience", http.StatusUnauthorized)
			return
		}

		claims := jwt.MapClaims{}
		_, err = parser.ParseWithClaims(tokenString, claims, jwks.Keyfunc)
		if err != nil {
			http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}
		if !audienceAllowed(claims, cfg.Audiences, cfg.OrgAudiencePrefix) {
			http.Error(w, "invalid token: audience mismatch", http.StatusUnauthorized)
			return
		}

		sub, _ := claims["sub"].(string)
		if sub == "" {
			http.Error(w, "invalid token: missing sub", http.StatusUnauthorized)
			return
		}

		scopeString, _ := claims["scope"].(string)
		// Logto organization-scoped tokens (aud urn:logto:organization:<id>) carry org
		// permissions in scope, not API resource scopes like api:read. Do not apply
		// REQUIRED_SCOPES to those tokens or every request returns 403.
		if !tokenHasOrgAudience(claims, cfg.OrgAudiencePrefix) {
			if enforceAPIScopes(r, cfg.RequiredScopes) {
				if missing := missingScopes(scopeString, cfg.RequiredScopes); len(missing) > 0 {
					http.Error(w, "missing scopes: "+strings.Join(missing, " "), http.StatusForbidden)
					return
				}
			}
		}

		w.Header().Set("X-User-Id", sub)
		w.Header().Set("X-User-Scopes", scopeString)
		if orgID := organizationIDFromClaims(claims, cfg.OrgAudiencePrefix); orgID != "" {
			w.Header().Set("X-Organization-Id", orgID)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// enforceAPIScopes is false when REQUIRED_SCOPES is empty or when Traefik forwardAuth
// indicates an invitee-only path (GET invitation / POST accept). Those routes still need
// a valid JWT + audience; WorkspacesService enforces invitee email / membership.
func enforceAPIScopes(r *http.Request, required map[string]struct{}) bool {
	if len(required) == 0 {
		return false
	}
	path := forwardedPathForAuth(r)
	if path != "" && isInviteeInvitationPath(path) {
		return false
	}
	return true
}

// forwardedPathForAuth returns the backend path Traefik was checking (forwardAuth sets
// X-Forwarded-Uri). If missing, we require scopes — safer default than skipping.
func forwardedPathForAuth(r *http.Request) string {
	for _, key := range []string{"X-Forwarded-Uri", "X-Forwarded-URL", "X-Original-URL"} {
		v := strings.TrimSpace(r.Header.Get(key))
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			if u, err := url.Parse(v); err == nil && u.Path != "" {
				return strings.TrimSuffix(u.Path, "/")
			}
		}
		if i := strings.IndexByte(v, '?'); i >= 0 {
			v = v[:i]
		}
		return strings.TrimSuffix(v, "/")
	}
	return ""
}

// isInviteeInvitationPath matches GET /invitations/{id} and POST /invitations/{id}/accept
// (admin flows use /workspaces/.../invitations and keep REQUIRED_SCOPES).
func isInviteeInvitationPath(path string) bool {
	const prefix = "/invitations/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return false
	}
	parts := strings.Split(rest, "/")
	switch len(parts) {
	case 1:
		return parts[0] != ""
	case 2:
		return parts[0] != "" && parts[1] == "accept"
	default:
		return false
	}
}

func bearerToken(v string) (string, error) {
	parts := strings.Fields(v)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("missing or invalid Authorization header")
	}
	return parts[1], nil
}

func looksLikeJWT(token string) bool {
	return strings.Count(token, ".") == 2
}

// audienceAllowed accepts either a normal API resource JWT (aud in LOGTO_AUDIENCE) or a Logto
// organization-scoped token: aud must equal prefix+organization_id and match organization claim.
func audienceAllowed(claims jwt.MapClaims, expected []string, orgAudiencePrefix string) bool {
	if len(expected) == 0 && orgAudiencePrefix == "" {
		return true
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, v := range expected {
		expectedSet[v] = struct{}{}
	}
	// Prefer explicit organization_id claim (API resource + org context); else derive
	// from aud — Logto org-only tokens omit organization_id (see validate-access-tokens docs).
	orgID := organizationIDFromClaims(claims, orgAudiencePrefix)

	for _, aud := range audiencesFromClaim(claims["aud"]) {
		if _, ok := expectedSet[aud]; ok {
			return true
		}
		if orgAudiencePrefix != "" && strings.HasPrefix(aud, orgAudiencePrefix) {
			fromAud := strings.TrimPrefix(aud, orgAudiencePrefix)
			if fromAud == "" {
				continue
			}
			// Accept when aud is the only org hint, or when it matches a declared claim.
			if orgID == "" || fromAud == orgID {
				return true
			}
		}
	}
	return false
}

func audiencesFromClaim(audClaim any) []string {
	switch aud := audClaim.(type) {
	case string:
		if aud != "" {
			return []string{aud}
		}
	case []any:
		out := make([]string, 0, len(aud))
		for _, item := range aud {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// organizationIDFromClaims returns Logto organization id from standard claims or,
// for org-scoped tokens that only set aud to urn:logto:organization:<id>, from aud.
func organizationIDFromClaims(claims jwt.MapClaims, orgAudiencePrefix string) string {
	for _, key := range []string{"organization_id", "org_id", "organizationId"} {
		if v, ok := claims[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	if orgAudiencePrefix == "" {
		return ""
	}
	for _, aud := range audiencesFromClaim(claims["aud"]) {
		if strings.HasPrefix(aud, orgAudiencePrefix) {
			id := strings.TrimPrefix(aud, orgAudiencePrefix)
			if id != "" {
				return id
			}
		}
	}
	return ""
}

func tokenHasOrgAudience(claims jwt.MapClaims, orgAudiencePrefix string) bool {
	if orgAudiencePrefix == "" {
		return false
	}
	for _, aud := range audiencesFromClaim(claims["aud"]) {
		if strings.HasPrefix(aud, orgAudiencePrefix) {
			return true
		}
	}
	return false
}

func missingScopes(scopeString string, required map[string]struct{}) []string {
	if len(required) == 0 {
		return nil
	}
	got := make(map[string]struct{})
	for _, scope := range strings.Fields(scopeString) {
		got[scope] = struct{}{}
	}
	var missing []string
	for scope := range required {
		if _, ok := got[scope]; !ok {
			missing = append(missing, scope)
		}
	}
	return missing
}
