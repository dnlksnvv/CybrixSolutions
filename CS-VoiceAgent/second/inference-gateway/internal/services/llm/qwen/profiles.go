package qwen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cybrix/inference-gateway/internal/config"
	"gopkg.in/yaml.v3"
)

// ProfileBundle is one DashScope OpenAI-compat LLM preset and all public model ids for it.
type ProfileBundle struct {
	PublicIDs []string
	Config    config.LLMQwenConfig
}

type qwenLLMTemplateDoc struct {
	Provider  string   `yaml:"provider"`
	PublicIDs []string `yaml:"public_ids"`
	APIKeyEnv string   `yaml:"api_key_env"`
	Defaults  struct {
		BaseURL     string   `yaml:"base_url"`
		Model       string   `yaml:"model"`
		Temperature *float64 `yaml:"temperature"`
		MaxTokens   *int     `yaml:"max_tokens"`
	} `yaml:"defaults"`
}

// LoadProfiles loads YAML from LLM_QWEN_TEMPLATE_DIR, or
// filepath.Join(LLM_TEMPLATE_DIR, "qwen") if the former is empty, or default
// model-templates/llm/qwen. If seen is non-nil, public_ids are checked across vendors.
// Missing dir → empty slice. Template must set api_key_env and defaults (no code fallbacks).
func LoadProfiles(seen map[string]struct{}) ([]ProfileBundle, error) {
	dir := resolveQwenTemplateDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return loadQwenLLMTemplatesFromDir(abs, seen)
}

func resolveQwenTemplateDir() string {
	if d := strings.TrimSpace(os.Getenv("LLM_QWEN_TEMPLATE_DIR")); d != "" {
		return d
	}
	if parent := strings.TrimSpace(os.Getenv("LLM_TEMPLATE_DIR")); parent != "" {
		return filepath.Join(parent, "qwen")
	}
	return filepath.Join("model-templates", "llm", "qwen")
}

func loadQwenLLMTemplatesFromDir(dir string, seen map[string]struct{}) ([]ProfileBundle, error) {
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
			return nil, fmt.Errorf("llm qwen template %s: %w", p, err)
		}
		var doc qwenLLMTemplateDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("llm qwen template %s: %w", p, err)
		}
		if prov := strings.ToLower(strings.TrimSpace(doc.Provider)); prov != "" && prov != "qwen" {
			return nil, fmt.Errorf("llm qwen template %s: provider must be qwen or omitted (%q)", p, doc.Provider)
		}
		bundle, err := bundleFromQwenLLMDoc(&doc, localSeen, p)
		if err != nil {
			return nil, fmt.Errorf("llm qwen template %s: %w", p, err)
		}
		out = append(out, bundle)
	}
	return out, nil
}

func bundleFromQwenLLMDoc(doc *qwenLLMTemplateDoc, seen map[string]struct{}, source string) (ProfileBundle, error) {
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
	keyEnv := strings.TrimSpace(doc.APIKeyEnv)
	if keyEnv == "" {
		return zero, fmt.Errorf("api_key_env is required (%s)", source)
	}
	apiKey := strings.TrimSpace(os.Getenv(keyEnv))
	baseURL := strings.TrimSpace(doc.Defaults.BaseURL)
	model := strings.TrimSpace(doc.Defaults.Model)
	if baseURL == "" {
		return zero, fmt.Errorf("defaults.base_url is required (%s)", source)
	}
	if model == "" {
		return zero, fmt.Errorf("defaults.model is required (%s)", source)
	}
	cfg := config.LLMQwenConfig{
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Model:       model,
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
