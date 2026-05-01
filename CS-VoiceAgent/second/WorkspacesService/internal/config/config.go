package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTPAddr        string        `env:"HTTP_ADDR" envDefault:":8080"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
	LogLevel        string        `env:"LOG_LEVEL" envDefault:"info"`

	LogtoIssuer             string        `env:"LOGTO_ISSUER,required"`
	LogtoTokenEndpoint      string        `env:"LOGTO_TOKEN_ENDPOINT"`
	LogtoTenantID           string        `env:"LOGTO_TENANT_ID"`
	LogtoManagementBaseURL  string        `env:"LOGTO_MANAGEMENT_BASE_URL,required"`
	LogtoManagementResource string        `env:"LOGTO_MANAGEMENT_RESOURCE,required"`
	LogtoM2MClientID        string        `env:"LOGTO_M2M_CLIENT_ID,required"`
	LogtoM2MClientSecret    string        `env:"LOGTO_M2M_CLIENT_SECRET,required"`
	LogtoM2MScope           string        `env:"LOGTO_M2M_SCOPE" envDefault:"all"`
	LogtoHTTPTimeout        time.Duration `env:"LOGTO_HTTP_TIMEOUT" envDefault:"5s"`

	WorkspaceOwnerRole string `env:"WORKSPACE_OWNER_ROLE" envDefault:"Owner"`

	// InvitationLinkBase is the SPA URL prefix for invitation landing pages.
	// Optional: if empty, POST invite / resend return 503 until you set it.
	InvitationLinkBase string `env:"INVITATION_LINK_BASE_URL"`
}

func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(cfg.LogtoTokenEndpoint) == "" {
		cfg.LogtoTokenEndpoint = strings.TrimRight(cfg.LogtoIssuer, "/") + "/token"
	}
	tid, err := resolveLogtoTenantID(cfg.LogtoTenantID, cfg.LogtoManagementBaseURL)
	if err != nil {
		return Config{}, err
	}
	cfg.LogtoTenantID = tid
	return cfg, nil
}

// resolveLogtoTenantID returns explicit tenant id or derives it from the cloud
// hostname pattern https://{tenant}.logto.app (also works without scheme if URL parses).
func resolveLogtoTenantID(explicit, managementBaseURL string) (string, error) {
	if t := strings.TrimSpace(explicit); t != "" {
		return t, nil
	}
	u, err := url.Parse(strings.TrimSpace(managementBaseURL))
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("LOGTO_TENANT_ID is empty and LOGTO_MANAGEMENT_BASE_URL has no parseable host")
	}
	host := strings.ToLower(u.Hostname())
	const suf = ".logto.app"
	if strings.HasSuffix(host, suf) {
		return strings.TrimSuffix(host, suf), nil
	}
	return "", fmt.Errorf("LOGTO_TENANT_ID is empty and host %q does not match *.logto.app; set LOGTO_TENANT_ID explicitly", host)
}
