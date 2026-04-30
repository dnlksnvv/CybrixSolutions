package sber

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/cybrix/inference-gateway/internal/eventbus"
	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/sberauth"
	"github.com/cybrix/inference-gateway/internal/services/tts"
	synthesis "github.com/cybrix/inference-gateway/internal/services/tts/sber/gen/synthesis"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type Synth struct {
	cfg SynthConfig
	src *sberauth.Source
}

func NewSynth(cfg SynthConfig) *Synth {
	return &Synth{
		cfg: cfg,
		src: sberauth.NewSourceWithCA(cfg.Credentials, cfg.Scope, cfg.OAuthURL, cfg.VerifyTLS, cfg.CABundle, cfg.CABundlePEM),
	}
}

func (p *Synth) Synthesize(ctx context.Context, params tts.SessionParams) (tts.Session, error) {
	voice := strings.TrimSpace(params.Voice)
	if voice == "" {
		voice = p.cfg.Voice
	}
	lang := resolveLanguage(params.LanguageType, p.cfg.Language)
	rate := params.SampleRate
	if rate == 0 {
		rate = p.cfg.SampleRate
	}
	enc, media, err := resolveEncoding(params.AudioFormat, p.cfg.Format)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	conn, err := dialGRPC(p.cfg)
	if err != nil {
		return nil, fmt.Errorf("sber tts dial: %w", err)
	}
	cctx, ccancel := context.WithCancel(ctx)
	s := &synthSession{
		parent:     p,
		conn:       conn,
		client:     synthesis.NewSmartSpeechClient(conn),
		requestID:  params.RequestID,
		voice:      voice,
		language:   lang,
		sampleRate: rate,
		encoding:   enc,
		mediaType:  media,
		bus:        eventbus.New(128),
		queue:      make(chan synthJob, 128),
		ctx:        cctx,
		cancelAll:  ccancel,
	}
	go s.worker()
	return s, nil
}

type synthJob struct{ text string }

type synthSession struct {
	parent *Synth
	conn   *grpc.ClientConn
	client synthesis.SmartSpeechClient

	requestID  string
	voice      string
	language   string
	sampleRate int
	encoding   synthesis.SynthesisRequest_AudioEncoding
	mediaType  string

	mu           sync.Mutex
	buf          strings.Builder
	closed       bool
	currentTurn  string
	activeCancel context.CancelFunc

	bus       *eventbus.Bus
	queue     chan synthJob
	ctx       context.Context
	cancelAll context.CancelFunc
}

func (s *synthSession) PushText(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("sber tts: session closed")
	}
	s.buf.WriteString(text)
	return nil
}

func (s *synthSession) Commit(_ context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("sber tts: session closed")
	}
	text := s.buf.String()
	s.buf.Reset()
	s.mu.Unlock()
	if strings.TrimSpace(text) == "" {
		s.bus.Emit(v1.Event{Type: v1.EventAudioEnd, RequestID: s.requestID})
		return nil
	}
	select {
	case <-s.ctx.Done():
		return fmt.Errorf("sber tts: session closed")
	case s.queue <- synthJob{text: text}:
		return nil
	}
}

func (s *synthSession) Cancel(_ context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.buf.Reset()
	cancel := s.activeCancel
	s.activeCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	for {
		select {
		case <-s.queue:
		default:
			return nil
		}
	}
}

func (s *synthSession) OnTurn(turnID string) {
	t := strings.TrimSpace(turnID)
	if t == "" {
		return
	}
	s.mu.Lock()
	same := t == s.currentTurn
	if !same {
		s.currentTurn = t
	}
	s.mu.Unlock()
	if !same {
		_ = s.Cancel(context.Background())
	}
}

func (s *synthSession) Events() <-chan v1.Event { return s.bus.Out() }

func (s *synthSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.activeCancel
	s.activeCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.cancelAll()
	close(s.queue)
	_ = s.conn.Close()
	s.bus.Close()
	return nil
}

func (s *synthSession) worker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case job, ok := <-s.queue:
			if !ok {
				return
			}
			s.runJob(job)
		}
	}
}

func (s *synthSession) runJob(job synthJob) {
	tok, err := s.parent.src.Token(s.ctx)
	if err != nil {
		s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeInternal, "sber tts auth: "+err.Error(), false))
		return
	}

	jobCtx, jobCancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		jobCancel()
		return
	}
	s.activeCancel = jobCancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.activeCancel = nil
		s.mu.Unlock()
		jobCancel()
	}()

	mdCtx := metadata.NewOutgoingContext(jobCtx, metadata.Pairs("authorization", "Bearer "+tok))
	req := &synthesis.SynthesisRequest{
		Text:          job.text,
		AudioEncoding: s.encoding,
		Language:      s.language,
		ContentType:   synthesis.SynthesisRequest_TEXT,
		Voice:         s.voice,
	}
	stream, err := s.client.Synthesize(mdCtx, req)
	if err != nil {
		if jobCtx.Err() != nil {
			return
		}
		s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, "sber tts synth: "+err.Error(), false))
		return
	}
	seq := 0
	var durationMs int64
	for {
		resp, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				s.bus.Emit(v1.Event{Type: v1.EventAudioEnd, RequestID: s.requestID, DurationMs: durationMs})
				return
			}
			if jobCtx.Err() != nil {
				return
			}
			s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, "sber tts recv: "+recvErr.Error(), false))
			return
		}
		data := resp.GetData()
		if len(data) == 0 {
			continue
		}
		seq++
		s.bus.Emit(v1.Event{
			Type:       v1.EventAudioChunk,
			RequestID:  s.requestID,
			Seq:        seq,
			PCMB64:     base64.StdEncoding.EncodeToString(data),
			MediaType:  s.mediaType,
			SampleRate: s.sampleRate,
		})
		if d := resp.GetAudioDuration(); d != nil {
			durationMs = d.GetSeconds()*1000 + int64(d.GetNanos())/1_000_000
		}
	}
}

func resolveEncoding(clientHint, templateFormat string) (synthesis.SynthesisRequest_AudioEncoding, string, error) {
	hint := strings.ToLower(strings.TrimSpace(clientHint))
	if hint == "" {
		hint = strings.ToLower(strings.TrimSpace(templateFormat))
	}
	switch hint {
	case "pcm16", "pcm_s16le", "s16le", "audio/pcm", "audio/pcm16", "":
		return synthesis.SynthesisRequest_PCM_S16LE, v1.MediaPCM16, nil
	case "wav16", "wav", "audio/wav":
		return synthesis.SynthesisRequest_WAV, v1.MediaWAV, nil
	case "opus", "ogg", "audio/opus":
		return synthesis.SynthesisRequest_OPUS, v1.MediaOpus, nil
	default:
		return synthesis.SynthesisRequest_AUDIO_ENCODING_UNSPECIFIED, "", fmt.Errorf("unsupported audio_format %q", clientHint)
	}
}

func resolveLanguage(fromClient, fromTemplate string) string {
	c := strings.TrimSpace(fromClient)
	if c == "" {
		return strings.TrimSpace(fromTemplate)
	}
	low := strings.ToLower(c)
	// Qwen-style language_type values are not valid for Salute Speech gRPC.
	switch low {
	case "auto", "russian", "english":
		return strings.TrimSpace(fromTemplate)
	}
	// Salute expects RFC-like codes, e.g. ru-RU / en-US.
	if strings.Contains(c, "-") {
		return c
	}
	return strings.TrimSpace(fromTemplate)
}

func dialGRPC(cfg SynthConfig) (*grpc.ClientConn, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: !cfg.VerifyTLS, MinVersion: tls.VersionTLS12}
	if cfg.VerifyTLS {
		pem := strings.TrimSpace(cfg.CABundlePEM)
		if pem == "" && strings.TrimSpace(cfg.CABundle) != "" {
			raw, err := os.ReadFile(strings.TrimSpace(cfg.CABundle))
			if err != nil {
				return nil, fmt.Errorf("read ca_bundle_file: %w", err)
			}
			pem = string(raw)
		}
		if pem != "" {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(pem)) {
				return nil, fmt.Errorf("invalid PEM in ca bundle")
			}
			tlsCfg.RootCAs = pool
		}
	}
	return grpc.DialContext(context.Background(), cfg.GRPCAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}

var _ tts.Session = (*synthSession)(nil)
