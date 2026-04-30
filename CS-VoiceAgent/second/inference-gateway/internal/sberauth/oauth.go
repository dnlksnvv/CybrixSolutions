// Package sberauth implements the Sber NGW OAuth2 token exchange shared by
// GigaChat and SaluteSpeech.
//
// Tokens are cached in-process and refreshed automatically a few seconds
// before their declared expiry.
package sberauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Source mints fresh access tokens against an NGW endpoint and caches the
// result. Goroutine-safe.
type Source struct {
	credentials string
	scope       string
	oauthURL    string
	verifyTLS   bool

	http    *http.Client
	initErr error

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewSource creates a token source. credentials must be the pre-built
// "Authorization: Basic" payload (without the "Basic " prefix).
func NewSource(credentials, scope, oauthURL string, verifyTLS bool) *Source {
	return NewSourceWithCA(credentials, scope, oauthURL, verifyTLS, "", "")
}

// NewSourceWithCABundle is like NewSource, but allows pinning a custom PEM bundle.
func NewSourceWithCABundle(
	credentials,
	scope,
	oauthURL string,
	verifyTLS bool,
	caBundleFile string,
) *Source {
	return NewSourceWithCA(credentials, scope, oauthURL, verifyTLS, caBundleFile, "")
}

// NewSourceWithCA allows pinning CA from either a bundle file or PEM text.
// caBundlePEM takes precedence over caBundleFile when both are provided.
func NewSourceWithCA(
	credentials,
	scope,
	oauthURL string,
	verifyTLS bool,
	caBundleFile string,
	caBundlePEM string,
) *Source {
	tlsCfg := &tls.Config{InsecureSkipVerify: !verifyTLS}
	var initErr error
	if verifyTLS {
		pem := strings.TrimSpace(caBundlePEM)
		if pem == "" && strings.TrimSpace(caBundleFile) != "" {
			raw, err := os.ReadFile(strings.TrimSpace(caBundleFile))
			if err != nil {
				initErr = fmt.Errorf("sberauth: read ca bundle: %w", err)
			} else {
				pem = string(raw)
			}
		}
		if pem != "" {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(pem)) {
				initErr = fmt.Errorf("sberauth: invalid PEM in ca bundle")
			} else {
				tlsCfg.RootCAs = pool
			}
		}
	}
	return &Source{
		credentials: strings.TrimSpace(credentials),
		scope:       strings.TrimSpace(scope),
		oauthURL:    strings.TrimSpace(oauthURL),
		verifyTLS:   verifyTLS,
		initErr:     initErr,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsCfg,
			},
		},
	}
}

// Token returns a valid access token, refreshing it if necessary.
func (s *Source) Token(ctx context.Context) (string, error) {
	if s.initErr != nil {
		return "", s.initErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Until(s.expiry) > 30*time.Second {
		return s.token, nil
	}
	tok, exp, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}
	s.token = tok
	s.expiry = exp
	return tok, nil
}

func (s *Source) fetch(ctx context.Context) (string, time.Time, error) {
	form := url.Values{}
	form.Set("scope", s.scope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.oauthURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sberauth: build request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+s.credentials)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", uuid.NewString())

	resp, err := s.http.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sberauth: do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", time.Time{}, fmt.Errorf("sberauth: oauth status %d", resp.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"` // ms epoch
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", time.Time{}, fmt.Errorf("sberauth: decode: %w", err)
	}
	if payload.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("sberauth: empty access_token in response")
	}
	exp := time.UnixMilli(payload.ExpiresAt)
	if exp.Before(time.Now()) {
		exp = time.Now().Add(25 * time.Minute) // sane default
	}
	return payload.AccessToken, exp, nil
}
