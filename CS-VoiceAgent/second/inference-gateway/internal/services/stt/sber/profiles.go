package sber

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

// ErrBadRequest marks session.start validation errors for Sber STT templates.
var ErrBadRequest = errors.New("sber stt bad request")

// FieldPolicy controls how session.start fields combine with template defaults.
type FieldPolicy int8

const (
	ClientDefault FieldPolicy = iota
	ClientOptional
	ClientRequired
)

// ClientPolicy per-field rules for Salute gRPC STT.
type ClientPolicy struct {
	Language   FieldPolicy
	SampleRate FieldPolicy
}

// RealtimeConfig is upstream connection + template defaults (from YAML + env).
type RealtimeConfig struct {
	Credentials string
	Scope       string
	OAuthURL    string
	GRPCAddr    string
	CABundle    string
	CABundlePEM string
	VerifyTLS   bool

	Language        string
	SampleRate      int
	HypothesesCount int
	EOUTimeoutMs    int
	PartialResults  bool
	MultiUtterance  bool
}

// Enabled reports whether this profile has credentials and grpc address.
func (c RealtimeConfig) Enabled() bool {
	return strings.TrimSpace(c.Credentials) != "" && strings.TrimSpace(c.GRPCAddr) != ""
}

// ProfileBundle is one Salute upstream preset and all public model ids for it.
type ProfileBundle struct {
	PublicIDs []string
	Config    RealtimeConfig
	Policy    ClientPolicy
}

type templateDoc struct {
	PublicIDs      []string `yaml:"public_ids"`
	CredentialsEnv string   `yaml:"credentials_env"`
	Scope          string   `yaml:"scope"`
	OAuthURL       string   `yaml:"oauth_url"`
	GRPCAddr       string   `yaml:"grpc_addr"`
	CABundleFile   string   `yaml:"ca_bundle_file"`
	CABundlePEMEnv string   `yaml:"ca_bundle_pem_env"`
	Defaults       struct {
		VerifyTLS       *bool  `yaml:"verify_tls"`
		Language        string `yaml:"language"`
		SampleRate      int    `yaml:"sample_rate"`
		HypothesesCount int    `yaml:"hypotheses_count"`
		EOUTimeoutMs    int    `yaml:"eou_timeout_ms"`
		PartialResults  *bool  `yaml:"partial_results"`
		MultiUtterance  *bool  `yaml:"multi_utterance"`
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
	p := ClientPolicy{
		Language:   ClientOptional,
		SampleRate: ClientOptional,
	}
	if m == nil {
		return p, nil
	}
	var err error
	if v, ok := m["language"]; ok {
		if p.Language, err = parseFieldPolicy(v, p.Language); err != nil {
			return p, fmt.Errorf("from_client.language: %w", err)
		}
	}
	if v, ok := m["sample_rate"]; ok {
		if p.SampleRate, err = parseFieldPolicy(v, p.SampleRate); err != nil {
			return p, fmt.Errorf("from_client.sample_rate: %w", err)
		}
	}
	return p, nil
}

// LoadSberProfiles loads YAML from STT_SBER_TEMPLATE_DIR (default model-templates/stt/sber-realtime).
func LoadSberProfiles() ([]ProfileBundle, error) {
	dir := strings.TrimSpace(os.Getenv("STT_SBER_TEMPLATE_DIR"))
	if dir == "" {
		dir = "model-templates/stt/sber-realtime"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return loadTemplatesFromDir(abs)
}

func loadTemplatesFromDir(dir string) ([]ProfileBundle, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("sber stt templates dir %s: %w", dir, err)
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
			return nil, fmt.Errorf("sber stt template %s: %w", p, err)
		}
		var doc templateDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("sber stt template %s: %w", p, err)
		}
		bundle, err := bundleFromTemplateDoc(&doc, seen, p)
		if err != nil {
			return nil, fmt.Errorf("sber stt template %s: %w", p, err)
		}
		out = append(out, bundle)
	}
	return out, nil
}

func bundleFromTemplateDoc(doc *templateDoc, seen map[string]struct{}, source string) (ProfileBundle, error) {
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
	if creds == "" {
		return zero, fmt.Errorf("credentials_env %q is empty (%s)", keyEnv, source)
	}
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
	if caBundle != "" {
		if !filepath.IsAbs(caBundle) {
			caBundle = filepath.Join(filepath.Dir(source), caBundle)
		}
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
	if doc.Defaults.PartialResults == nil {
		return zero, fmt.Errorf("defaults.partial_results is required (%s)", source)
	}
	if doc.Defaults.MultiUtterance == nil {
		return zero, fmt.Errorf("defaults.multi_utterance is required (%s)", source)
	}
	lang := strings.TrimSpace(doc.Defaults.Language)
	if lang == "" {
		return zero, fmt.Errorf("defaults.language is required (%s)", source)
	}
	if doc.Defaults.SampleRate <= 0 {
		return zero, fmt.Errorf("defaults.sample_rate must be > 0 (%s)", source)
	}
	if doc.Defaults.HypothesesCount < 0 || doc.Defaults.HypothesesCount > 10 {
		return zero, fmt.Errorf("defaults.hypotheses_count must be in [0..10] (%s)", source)
	}
	if doc.Defaults.EOUTimeoutMs <= 0 {
		return zero, fmt.Errorf("defaults.eou_timeout_ms must be > 0 (%s)", source)
	}

	pol, err := policyFromMap(doc.FromClient)
	if err != nil {
		return zero, err
	}
	return ProfileBundle{
		PublicIDs: ids,
		Config: RealtimeConfig{
			Credentials: creds,
			Scope:       scope,
			OAuthURL:    oauthURL,
			GRPCAddr:    grpcAddr,
			CABundle:    caBundle,
			CABundlePEM: caBundlePEM,
			VerifyTLS:   *doc.Defaults.VerifyTLS,

			Language:        lang,
			SampleRate:      doc.Defaults.SampleRate,
			HypothesesCount: doc.Defaults.HypothesesCount,
			EOUTimeoutMs:    doc.Defaults.EOUTimeoutMs,
			PartialResults:  *doc.Defaults.PartialResults,
			MultiUtterance:  *doc.Defaults.MultiUtterance,
		},
		Policy: pol,
	}, nil
}

// ResolveSession applies template + policy to session.start-derived params.
func ResolveSession(in stt.SessionParams, pol ClientPolicy, def RealtimeConfig) (stt.SessionParams, error) {
	out := in
	switch pol.Language {
	case ClientRequired:
		if strings.TrimSpace(in.Language) == "" {
			return out, fmt.Errorf("%w: language is required in session.start for this model", ErrBadRequest)
		}
		out.Language = strings.TrimSpace(in.Language)
	case ClientDefault:
		out.Language = ""
	case ClientOptional:
	}

	switch pol.SampleRate {
	case ClientRequired:
		if in.SampleRate <= 0 {
			return out, fmt.Errorf("%w: sample_rate is required in session.start for this model", ErrBadRequest)
		}
	case ClientDefault:
		out.SampleRate = 0
	case ClientOptional:
	}
	_ = def
	return out, nil
}
