package sber

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

var ErrBadRequest = errors.New("sber tts bad request")

type FieldPolicy int8

const (
	ClientDefault FieldPolicy = iota
	ClientOptional
	ClientRequired
)

type ClientPolicy struct {
	Voice      FieldPolicy
	SampleRate FieldPolicy
	Format     FieldPolicy
}

type SynthConfig struct {
	Credentials string
	Scope       string
	OAuthURL    string
	GRPCAddr    string
	CABundle    string
	CABundlePEM string
	VerifyTLS   bool

	Voice      string
	Language   string
	Format     string
	SampleRate int
}

func (c SynthConfig) Enabled() bool {
	return strings.TrimSpace(c.Credentials) != "" && strings.TrimSpace(c.GRPCAddr) != ""
}

type ProfileBundle struct {
	PublicIDs []string
	Config    SynthConfig
	Policy    ClientPolicy
}

type saluteTemplateDoc struct {
	PublicIDs      []string `yaml:"public_ids"`
	CredentialsEnv string   `yaml:"credentials_env"`
	Scope          string   `yaml:"scope"`
	OAuthURL       string   `yaml:"oauth_url"`
	GRPCAddr       string   `yaml:"grpc_addr"`
	CABundleFile   string   `yaml:"ca_bundle_file"`
	CABundlePEMEnv string   `yaml:"ca_bundle_pem_env"`
	Defaults       struct {
		Voice      string `yaml:"voice"`
		Language   string `yaml:"language"`
		Format     string `yaml:"format"`
		SampleRate int    `yaml:"sample_rate"`
		VerifyTLS  *bool  `yaml:"verify_tls"`
	} `yaml:"defaults"`
	FromClient map[string]string `yaml:"from_client"`
}

func parseFieldPolicy(s string, def FieldPolicy) (FieldPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "optional":
		return ClientOptional, nil
	case "default":
		return ClientDefault, nil
	case "required":
		return ClientRequired, nil
	default:
		return def, fmt.Errorf("unknown policy %q (use default, optional, required)", s)
	}
}

func policyFromMap(m map[string]string) (ClientPolicy, error) {
	p := ClientPolicy{Voice: ClientOptional, SampleRate: ClientOptional, Format: ClientOptional}
	if m == nil {
		return p, nil
	}
	var err error
	if v, ok := m["voice"]; ok {
		if p.Voice, err = parseFieldPolicy(v, p.Voice); err != nil {
			return p, fmt.Errorf("from_client.voice: %w", err)
		}
	}
	if v, ok := m["sample_rate"]; ok {
		if p.SampleRate, err = parseFieldPolicy(v, p.SampleRate); err != nil {
			return p, fmt.Errorf("from_client.sample_rate: %w", err)
		}
	}
	if v, ok := m["format"]; ok {
		if p.Format, err = parseFieldPolicy(v, p.Format); err != nil {
			return p, fmt.Errorf("from_client.format: %w", err)
		}
	}
	return p, nil
}

func LoadSberProfiles() ([]ProfileBundle, error) {
	dir := strings.TrimSpace(os.Getenv("TTS_SBER_TEMPLATE_DIR"))
	if dir == "" {
		dir = "model-templates/tts/sber"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return loadSberTemplatesFromDir(abs)
}

func loadSberTemplatesFromDir(dir string) ([]ProfileBundle, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("sber tts templates dir %s: %w", dir, err)
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
			return nil, fmt.Errorf("sber tts template %s: %w", p, err)
		}
		var doc saluteTemplateDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("sber tts template %s: %w", p, err)
		}
		bundle, err := bundleFromTemplateDoc(&doc, seen, p)
		if err != nil {
			return nil, fmt.Errorf("sber tts template %s: %w", p, err)
		}
		out = append(out, bundle)
	}
	return out, nil
}

func bundleFromTemplateDoc(doc *saluteTemplateDoc, seen map[string]struct{}, source string) (ProfileBundle, error) {
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
	scope := strings.TrimSpace(doc.Scope)
	if scope == "" {
		return zero, fmt.Errorf("scope is required (%s)", source)
	}
	oauthURL := strings.TrimSpace(doc.OAuthURL)
	if oauthURL == "" {
		return zero, fmt.Errorf("oauth_url is required (%s)", source)
	}
	grpcAddr := strings.TrimSpace(doc.GRPCAddr)
	if grpcAddr == "" {
		return zero, fmt.Errorf("grpc_addr is required (%s)", source)
	}
	caBundle := strings.TrimSpace(doc.CABundleFile)
	if caBundle != "" && !filepath.IsAbs(caBundle) {
		caBundle = filepath.Join(filepath.Dir(source), caBundle)
	}
	caBundlePEM := ""
	if envName := strings.TrimSpace(doc.CABundlePEMEnv); envName != "" {
		caBundlePEM = strings.TrimSpace(os.Getenv(envName))
		if caBundlePEM == "" {
			return zero, fmt.Errorf("ca_bundle_pem_env %q is empty (%s)", envName, source)
		}
	}

	if doc.Defaults.VerifyTLS == nil {
		return zero, fmt.Errorf("defaults.verify_tls is required (%s)", source)
	}
	voice := strings.TrimSpace(doc.Defaults.Voice)
	if voice == "" {
		return zero, fmt.Errorf("defaults.voice is required (%s)", source)
	}
	lang := strings.TrimSpace(doc.Defaults.Language)
	if lang == "" {
		return zero, fmt.Errorf("defaults.language is required (%s)", source)
	}
	format := strings.TrimSpace(doc.Defaults.Format)
	if format == "" {
		return zero, fmt.Errorf("defaults.format is required (%s)", source)
	}
	if doc.Defaults.SampleRate <= 0 {
		return zero, fmt.Errorf("defaults.sample_rate must be > 0 (%s)", source)
	}
	pol, err := policyFromMap(doc.FromClient)
	if err != nil {
		return zero, err
	}

	return ProfileBundle{
		PublicIDs: ids,
		Config: SynthConfig{
			Credentials: creds,
			Scope:       scope,
			OAuthURL:    oauthURL,
			GRPCAddr:    grpcAddr,
			CABundle:    caBundle,
			CABundlePEM: caBundlePEM,
			VerifyTLS:   *doc.Defaults.VerifyTLS,
			Voice:       voice,
			Language:    lang,
			Format:      format,
			SampleRate:  doc.Defaults.SampleRate,
		},
		Policy: pol,
	}, nil
}

func ResolveSaluteSession(in tts.SessionParams, pol ClientPolicy, def SynthConfig) (tts.SessionParams, error) {
	out := in
	switch pol.Voice {
	case ClientRequired:
		if strings.TrimSpace(in.Voice) == "" {
			return out, fmt.Errorf("%w: voice is required in session.start for this model", ErrBadRequest)
		}
		out.Voice = strings.TrimSpace(in.Voice)
	case ClientDefault:
		out.Voice = ""
	}
	switch pol.SampleRate {
	case ClientRequired:
		if in.SampleRate <= 0 {
			return out, fmt.Errorf("%w: sample_rate is required in session.start for this model", ErrBadRequest)
		}
	case ClientDefault:
		out.SampleRate = 0
	}
	switch pol.Format {
	case ClientRequired:
		if strings.TrimSpace(in.AudioFormat) == "" {
			return out, fmt.Errorf("%w: audio_format is required in session.start for this model", ErrBadRequest)
		}
		out.AudioFormat = strings.TrimSpace(in.AudioFormat)
	case ClientDefault:
		out.AudioFormat = ""
	}
	_ = def
	return out, nil
}
