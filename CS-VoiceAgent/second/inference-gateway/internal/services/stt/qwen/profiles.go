// profiles.go: YAML model-templates for Qwen STT realtime (one file = one profile).
package qwen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cybrix/inference-gateway/internal/services/stt"
	"gopkg.in/yaml.v3"
)

// ErrQwenSTTBadRequest marks session.start validation errors for Qwen STT templates.
var ErrQwenSTTBadRequest = errors.New("qwen stt bad request")

// QwenSTTFieldPolicy controls how session.start fields combine with template defaults.
type QwenSTTFieldPolicy int8

const (
	QwenSTTClientDefault QwenSTTFieldPolicy = iota
	QwenSTTClientOptional
	QwenSTTClientRequired
)

// QwenSTTClientPolicy per-field rules for Qwen realtime STT (DashScope).
type QwenSTTClientPolicy struct {
	Language   QwenSTTFieldPolicy
	SampleRate QwenSTTFieldPolicy
}

// ProfileBundle is one logical Qwen STT upstream + all public model ids.
type ProfileBundle struct {
	PublicIDs []string
	Config    QwenRealtimeConfig
	Policy    QwenSTTClientPolicy
}

type sttQwenTemplateDoc struct {
	PublicIDs []string `yaml:"public_ids"`
	APIKeyEnv string   `yaml:"api_key_env"`
	WSURL     string   `yaml:"ws_url"`
	Defaults  struct {
		Language           string  `yaml:"language"`
		SampleRate         int     `yaml:"sample_rate"`
		VADThreshold       float64 `yaml:"vad_threshold"`
		VADSilenceMs       int     `yaml:"vad_silence_ms"`
		VADPrefixPaddingMs int     `yaml:"vad_prefix_padding_ms"`
	} `yaml:"defaults"`
	UpstreamModel string            `yaml:"upstream_model"`
	FromClient    map[string]string `yaml:"from_client"`
}

func parseSTTFieldPolicy(s string, def QwenSTTFieldPolicy) (QwenSTTFieldPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "optional":
		return QwenSTTClientOptional, nil
	case "default":
		return QwenSTTClientDefault, nil
	case "required":
		return QwenSTTClientRequired, nil
	default:
		return def, fmt.Errorf("unknown policy %q (use default, optional, required)", s)
	}
}

func sttPolicyFromMap(m map[string]string) (QwenSTTClientPolicy, error) {
	p := QwenSTTClientPolicy{
		Language:   QwenSTTClientOptional,
		SampleRate: QwenSTTClientOptional,
	}
	if m == nil {
		return p, nil
	}
	var err error
	if v, ok := m["language"]; ok {
		if p.Language, err = parseSTTFieldPolicy(v, p.Language); err != nil {
			return p, fmt.Errorf("from_client.language: %w", err)
		}
	}
	if v, ok := m["sample_rate"]; ok {
		if p.SampleRate, err = parseSTTFieldPolicy(v, p.SampleRate); err != nil {
			return p, fmt.Errorf("from_client.sample_rate: %w", err)
		}
	}
	return p, nil
}

// LoadQwenProfiles loads YAML from STT_QWEN_TEMPLATE_DIR (default
// model-templates/stt/qwen-realtime). Missing dir or no *.yaml → empty slice.
func LoadQwenProfiles() ([]ProfileBundle, error) {
	dir := strings.TrimSpace(os.Getenv("STT_QWEN_TEMPLATE_DIR"))
	if dir == "" {
		dir = "model-templates/stt/qwen-realtime"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return loadSTTQwenTemplatesFromDir(abs)
}

func loadSTTQwenTemplatesFromDir(dir string) ([]ProfileBundle, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("qwen stt templates dir %s: %w", dir, err)
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := strings.ToLower(e.Name())
		if strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	sort.Strings(paths)

	seen := make(map[string]struct{})
	out := make([]ProfileBundle, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("qwen stt template %s: %w", p, err)
		}
		var doc sttQwenTemplateDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("qwen stt template %s: %w", p, err)
		}
		bundle, err := sttBundleFromTemplateDoc(&doc, seen, p)
		if err != nil {
			return nil, fmt.Errorf("qwen stt template %s: %w", p, err)
		}
		out = append(out, bundle)
	}
	return out, nil
}

func sttBundleFromTemplateDoc(doc *sttQwenTemplateDoc, seen map[string]struct{}, source string) (ProfileBundle, error) {
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
	wsURL := strings.TrimSpace(doc.WSURL)
	if wsURL == "" {
		return zero, fmt.Errorf("ws_url is required (%s)", source)
	}
	model := strings.TrimSpace(doc.UpstreamModel)
	if model == "" {
		return zero, fmt.Errorf("upstream_model is required (%s)", source)
	}
	pol, err := sttPolicyFromMap(doc.FromClient)
	if err != nil {
		return zero, err
	}
	lang := strings.TrimSpace(doc.Defaults.Language)
	if lang == "" {
		return zero, fmt.Errorf("defaults.language is required (%s)", source)
	}
	if doc.Defaults.SampleRate <= 0 {
		return zero, fmt.Errorf("defaults.sample_rate must be > 0 (%s)", source)
	}
	if doc.Defaults.VADThreshold <= 0 {
		return zero, fmt.Errorf("defaults.vad_threshold must be > 0 (%s)", source)
	}
	if doc.Defaults.VADSilenceMs <= 0 {
		return zero, fmt.Errorf("defaults.vad_silence_ms must be > 0 (%s)", source)
	}
	if doc.Defaults.VADPrefixPaddingMs < 0 {
		return zero, fmt.Errorf("defaults.vad_prefix_padding_ms must be >= 0 (%s)", source)
	}
	cfg := QwenRealtimeConfig{
		APIKey:             apiKey,
		WSURL:              wsURL,
		Model:              model,
		Language:           lang,
		SampleRate:         doc.Defaults.SampleRate,
		VADThreshold:       doc.Defaults.VADThreshold,
		VADSilenceMs:       doc.Defaults.VADSilenceMs,
		VADPrefixPaddingMs: doc.Defaults.VADPrefixPaddingMs,
	}
	return ProfileBundle{
		PublicIDs: ids,
		Config:    cfg,
		Policy:    pol,
	}, nil
}

// ResolveQwenSTTSession applies template + policy to session.start-derived params.
func ResolveQwenSTTSession(in stt.SessionParams, pol QwenSTTClientPolicy, def QwenRealtimeConfig) (stt.SessionParams, error) {
	out := in

	switch pol.Language {
	case QwenSTTClientRequired:
		if strings.TrimSpace(in.Language) == "" {
			return out, fmt.Errorf("%w: language is required in session.start for this model", ErrQwenSTTBadRequest)
		}
		out.Language = strings.TrimSpace(in.Language)
	case QwenSTTClientDefault:
		out.Language = ""
	case QwenSTTClientOptional:
	}

	switch pol.SampleRate {
	case QwenSTTClientRequired:
		if in.SampleRate <= 0 {
			return out, fmt.Errorf("%w: sample_rate is required in session.start for this model", ErrQwenSTTBadRequest)
		}
		out.SampleRate = in.SampleRate
	case QwenSTTClientDefault:
		out.SampleRate = 0
	case QwenSTTClientOptional:
	}

	_ = def
	return out, nil
}
