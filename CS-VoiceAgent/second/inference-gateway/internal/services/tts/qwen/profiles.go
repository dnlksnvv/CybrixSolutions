// Package qwen implements DashScope Qwen TTS (profiles, realtime WS, policies).
package qwen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cybrix/inference-gateway/internal/services/tts"
	"gopkg.in/yaml.v3"
)

// ErrQwenTTSBadRequest marks session.start validation errors for Qwen templates.
var ErrQwenTTSBadRequest = errors.New("qwen tts bad request")

// QwenTTSFieldPolicy controls how session.start fields combine with template defaults.
type QwenTTSFieldPolicy int8

const (
	QwenClientDefault QwenTTSFieldPolicy = iota
	QwenClientOptional
	QwenClientRequired
)

// QwenTTSClientPolicy per-field rules for Qwen realtime TTS (DashScope).
type QwenTTSClientPolicy struct {
	Voice        QwenTTSFieldPolicy
	LanguageType QwenTTSFieldPolicy
	Mode         QwenTTSFieldPolicy
	SampleRate   QwenTTSFieldPolicy
}

// QwenRealtimeConfig is upstream connection + template defaults (from YAML).
type QwenRealtimeConfig struct {
	APIKey         string
	WSURL          string
	Model          string
	Voice          string
	LanguageType   string
	SampleRate     int
	Mode           string
	ResponseFormat string // pcm — upstream session.update; deltas forwarded as pcm_b64 in v1.
}

func (c QwenRealtimeConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != "" &&
		strings.TrimSpace(c.WSURL) != "" &&
		strings.TrimSpace(c.Model) != ""
}

// ProfileBundle is one logical Qwen TTS upstream + all public model ids.
type ProfileBundle struct {
	PublicIDs []string
	Config    QwenRealtimeConfig
	Policy    QwenTTSClientPolicy
}

type ttsQwenTemplateDoc struct {
	PublicIDs     []string `yaml:"public_ids"`
	APIKeyEnv     string   `yaml:"api_key_env"`
	WSURL         string   `yaml:"ws_url"`
	UpstreamModel string   `yaml:"upstream_model"`
	Defaults      struct {
		Voice          string `yaml:"voice"`
		LanguageType   string `yaml:"language_type"`
		Mode           string `yaml:"mode"`
		SampleRate     int    `yaml:"sample_rate"`
		ResponseFormat string `yaml:"response_format"`
	} `yaml:"defaults"`
	FromClient map[string]string `yaml:"from_client"`
}

func parseFieldPolicy(s string, def QwenTTSFieldPolicy) (QwenTTSFieldPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "optional":
		return QwenClientOptional, nil
	case "default":
		return QwenClientDefault, nil
	case "required":
		return QwenClientRequired, nil
	default:
		return def, fmt.Errorf("unknown policy %q (use default, optional, required)", s)
	}
}

func policyFromMap(m map[string]string) (QwenTTSClientPolicy, error) {
	p := QwenTTSClientPolicy{
		Voice:        QwenClientOptional,
		LanguageType: QwenClientOptional,
		Mode:         QwenClientOptional,
		SampleRate:   QwenClientOptional,
	}
	if m == nil {
		return p, nil
	}
	var err error
	if v, ok := m["voice"]; ok {
		if p.Voice, err = parseFieldPolicy(v, p.Voice); err != nil {
			return p, fmt.Errorf("from_client.voice: %w", err)
		}
	}
	if v, ok := m["language_type"]; ok {
		if p.LanguageType, err = parseFieldPolicy(v, p.LanguageType); err != nil {
			return p, fmt.Errorf("from_client.language_type: %w", err)
		}
	}
	if v, ok := m["mode"]; ok {
		if p.Mode, err = parseFieldPolicy(v, p.Mode); err != nil {
			return p, fmt.Errorf("from_client.mode: %w", err)
		}
	}
	if v, ok := m["sample_rate"]; ok {
		if p.SampleRate, err = parseFieldPolicy(v, p.SampleRate); err != nil {
			return p, fmt.Errorf("from_client.sample_rate: %w", err)
		}
	}
	return p, nil
}

// LoadQwenProfiles loads YAML from TTS_QWEN_TEMPLATE_DIR (default
// model-templates/tts/qwen-realtime). Missing dir or no *.yaml → empty slice.
func LoadQwenProfiles() ([]ProfileBundle, error) {
	dir := strings.TrimSpace(os.Getenv("TTS_QWEN_TEMPLATE_DIR"))
	if dir == "" {
		dir = "model-templates/tts/qwen-realtime"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return loadQwenTemplatesFromDir(abs)
}

func loadQwenTemplatesFromDir(dir string) ([]ProfileBundle, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("qwen tts templates dir %s: %w", dir, err)
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
			return nil, fmt.Errorf("qwen tts template %s: %w", p, err)
		}
		var doc ttsQwenTemplateDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("qwen tts template %s: %w", p, err)
		}
		bundle, err := bundleFromTemplateDoc(&doc, seen, p)
		if err != nil {
			return nil, fmt.Errorf("qwen tts template %s: %w", p, err)
		}
		out = append(out, bundle)
	}
	return out, nil
}

func bundleFromTemplateDoc(doc *ttsQwenTemplateDoc, seen map[string]struct{}, source string) (ProfileBundle, error) {
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
	pol, err := policyFromMap(doc.FromClient)
	if err != nil {
		return zero, err
	}
	rf := strings.ToLower(strings.TrimSpace(doc.Defaults.ResponseFormat))
	if rf == "" {
		return zero, fmt.Errorf("defaults.response_format is required (use pcm) (%s)", source)
	}
	if rf != "pcm" {
		return zero, fmt.Errorf("defaults.response_format must be pcm (%s)", source)
	}
	langType := strings.TrimSpace(doc.Defaults.LanguageType)
	if langType == "" {
		return zero, fmt.Errorf("defaults.language_type is required (%s)", source)
	}
	mode := strings.TrimSpace(doc.Defaults.Mode)
	if mode == "" {
		return zero, fmt.Errorf("defaults.mode is required (%s)", source)
	}
	if doc.Defaults.SampleRate <= 0 {
		return zero, fmt.Errorf("defaults.sample_rate must be > 0 (%s)", source)
	}
	cfg := QwenRealtimeConfig{
		APIKey:         apiKey,
		WSURL:          wsURL,
		Model:          model,
		Voice:          doc.Defaults.Voice,
		LanguageType:   langType,
		SampleRate:     doc.Defaults.SampleRate,
		Mode:           mode,
		ResponseFormat: rf,
	}
	return ProfileBundle{
		PublicIDs: ids,
		Config:    cfg,
		Policy:    pol,
	}, nil
}

// ResolveQwenTTSSession applies template + policy to session.start-derived params.
func ResolveQwenTTSSession(in tts.SessionParams, pol QwenTTSClientPolicy, def QwenRealtimeConfig) (tts.SessionParams, error) {
	out := in

	switch pol.Voice {
	case QwenClientRequired:
		if strings.TrimSpace(in.Voice) == "" {
			return out, fmt.Errorf("%w: voice is required in session.start for this model", ErrQwenTTSBadRequest)
		}
		out.Voice = strings.TrimSpace(in.Voice)
	case QwenClientDefault:
		out.Voice = ""
	case QwenClientOptional:
	}

	switch pol.LanguageType {
	case QwenClientRequired:
		if strings.TrimSpace(in.LanguageType) == "" {
			return out, fmt.Errorf("%w: language_type is required in session.start for this model", ErrQwenTTSBadRequest)
		}
		out.LanguageType = strings.TrimSpace(in.LanguageType)
	case QwenClientDefault:
		out.LanguageType = ""
	case QwenClientOptional:
	}

	switch pol.Mode {
	case QwenClientRequired:
		if strings.TrimSpace(in.Mode) == "" {
			return out, fmt.Errorf("%w: mode is required in session.start for this model", ErrQwenTTSBadRequest)
		}
		out.Mode = strings.TrimSpace(in.Mode)
	case QwenClientDefault:
		out.Mode = ""
	case QwenClientOptional:
	}

	switch pol.SampleRate {
	case QwenClientRequired:
		if in.SampleRate <= 0 {
			return out, fmt.Errorf("%w: sample_rate is required in session.start for this model", ErrQwenTTSBadRequest)
		}
		out.SampleRate = in.SampleRate
	case QwenClientDefault:
		out.SampleRate = 0
	case QwenClientOptional:
	}

	_ = def
	return out, nil
}
