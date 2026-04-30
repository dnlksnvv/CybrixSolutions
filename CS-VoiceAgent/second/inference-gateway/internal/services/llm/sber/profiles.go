package sber

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cybrix/inference-gateway/internal/config"
	"gopkg.in/yaml.v3"
)

// ProfileBundle is one GigaChat upstream preset and all public model ids for it.
type ProfileBundle struct {
	PublicIDs []string
	Config    config.LLMGigaChatConfig
}

type gigaLLMTemplateDoc struct {
	Provider       string   `yaml:"provider"`
	PublicIDs      []string `yaml:"public_ids"`
	CredentialsEnv string   `yaml:"credentials_env"`
	Defaults       struct {
		Scope       string   `yaml:"scope"`
		OAuthURL    string   `yaml:"oauth_url"`
		BaseURL     string   `yaml:"base_url"`
		Model       string   `yaml:"model"`
		VerifyTLS   *bool    `yaml:"verify_tls"`
		Temperature *float64 `yaml:"temperature"`
		MaxTokens   *int     `yaml:"max_tokens"`
	} `yaml:"defaults"`
}

// LoadProfiles loads YAML from LLM_SBER_TEMPLATE_DIR (or legacy LLM_GIGACHAT_TEMPLATE_DIR),
// else filepath.Join(LLM_TEMPLATE_DIR, "sber"), else default model-templates/llm/sber.
// If seen is non-nil, public_ids are checked for duplicates across LLM vendors.
// Missing dir → empty slice. Template must set credentials_env and defaults (no code fallbacks).
func LoadProfiles(seen map[string]struct{}) ([]ProfileBundle, error) {
	dir := resolveSberTemplateDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return loadGigaTemplatesFromDir(abs, seen)
}

func resolveSberTemplateDir() string {
	if d := strings.TrimSpace(os.Getenv("LLM_SBER_TEMPLATE_DIR")); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("LLM_GIGACHAT_TEMPLATE_DIR")); d != "" {
		return d
	}
	if parent := strings.TrimSpace(os.Getenv("LLM_TEMPLATE_DIR")); parent != "" {
		return filepath.Join(parent, "sber")
	}
	return filepath.Join("model-templates", "llm", "sber")
}

func loadGigaTemplatesFromDir(dir string, seen map[string]struct{}) ([]ProfileBundle, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil, nil
	}
	paths, err := collectYAMLFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	localSeen := seen
	if localSeen == nil {
		localSeen = make(map[string]struct{})
	}
	out := make([]ProfileBundle, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("llm gigachat template %s: %w", p, err)
		}
		var doc gigaLLMTemplateDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("llm gigachat template %s: %w", p, err)
		}
		if prov := strings.ToLower(strings.TrimSpace(doc.Provider)); prov != "" && prov != "gigachat" {
			return nil, fmt.Errorf("llm gigachat template %s: provider must be gigachat or omitted (%q)", p, doc.Provider)
		}
		bundle, err := bundleFromGigaDoc(&doc, localSeen, p)
		if err != nil {
			return nil, fmt.Errorf("llm gigachat template %s: %w", p, err)
		}
		out = append(out, bundle)
	}
	return out, nil
}

func bundleFromGigaDoc(doc *gigaLLMTemplateDoc, seen map[string]struct{}, source string) (ProfileBundle, error) {
	var zero ProfileBundle
	ids := make([]string, 0, len(doc.PublicIDs))
	for _, id := range doc.PublicIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			return zero, fmt.Errorf("duplicate public_id %q (also in another template)", id)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return zero, fmt.Errorf("need non-empty public_ids (%s)", source)
	}
	keyEnv := strings.TrimSpace(doc.CredentialsEnv)
	if keyEnv == "" {
		return zero, fmt.Errorf("credentials_env is required (%s)", source)
	}
	creds := strings.TrimSpace(os.Getenv(keyEnv))
	scope := strings.TrimSpace(doc.Defaults.Scope)
	oauthURL := strings.TrimSpace(doc.Defaults.OAuthURL)
	baseURL := strings.TrimSpace(doc.Defaults.BaseURL)
	model := strings.TrimSpace(doc.Defaults.Model)
	if scope == "" {
		return zero, fmt.Errorf("defaults.scope is required (%s)", source)
	}
	if oauthURL == "" {
		return zero, fmt.Errorf("defaults.oauth_url is required (%s)", source)
	}
	if baseURL == "" {
		return zero, fmt.Errorf("defaults.base_url is required (%s)", source)
	}
	if model == "" {
		return zero, fmt.Errorf("defaults.model is required (%s)", source)
	}
	if doc.Defaults.VerifyTLS == nil {
		return zero, fmt.Errorf("defaults.verify_tls is required (%s)", source)
	}
	cfg := config.LLMGigaChatConfig{
		Credentials: creds,
		Scope:       scope,
		OAuthURL:    oauthURL,
		BaseURL:     baseURL,
		Model:       model,
		VerifyTLS:   *doc.Defaults.VerifyTLS,
		Temperature: doc.Defaults.Temperature,
		MaxTokens:   doc.Defaults.MaxTokens,
	}
	return ProfileBundle{PublicIDs: ids, Config: cfg}, nil
}

func collectYAMLFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		n := strings.ToLower(info.Name())
		if strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
