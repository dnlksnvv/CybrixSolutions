package logto

import (
	"errors"
	"strings"
	"time"
)

type Config struct {
	TenantID             string
	TokenEndpoint        string
	ManagementAPIBaseURL string
	ManagementResource   string
	ClientID             string
	ClientSecret         string
	Scope                string
	HTTPTimeout          time.Duration
	TokenSafetyWindow    time.Duration
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.TenantID) == "" {
		return errors.New("logto: tenant id is required")
	}
	if strings.TrimSpace(c.TokenEndpoint) == "" {
		return errors.New("logto: token endpoint is required")
	}
	if strings.TrimSpace(c.ManagementAPIBaseURL) == "" {
		return errors.New("logto: management API base URL is required")
	}
	if strings.TrimSpace(c.ManagementResource) == "" {
		return errors.New("logto: management resource is required")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return errors.New("logto: M2M client id is required")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		return errors.New("logto: M2M client secret is required")
	}
	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = 5 * time.Second
	}
	if c.TokenSafetyWindow <= 0 {
		c.TokenSafetyWindow = 30 * time.Second
	}
	return nil
}
