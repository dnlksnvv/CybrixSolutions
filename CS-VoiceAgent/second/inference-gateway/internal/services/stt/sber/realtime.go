package sber

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cybrix/inference-gateway/internal/eventbus"
	v1 "github.com/cybrix/inference-gateway/internal/protocol/v1"
	"github.com/cybrix/inference-gateway/internal/sberauth"
	"github.com/cybrix/inference-gateway/internal/services/stt"
	recognition "github.com/cybrix/inference-gateway/internal/services/stt/sber/gen/recognition"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Realtime is STT provider for SaluteSpeech gRPC Recognize.
type Realtime struct {
	cfg RealtimeConfig
	src *sberauth.Source
}

// NewRealtime creates a provider from a resolved template config.
func NewRealtime(cfg RealtimeConfig) *Realtime {
	return &Realtime{
		cfg: cfg,
		src: sberauth.NewSourceWithCA(
			cfg.Credentials,
			cfg.Scope,
			cfg.OAuthURL,
			cfg.VerifyTLS,
			cfg.CABundle,
			cfg.CABundlePEM,
		),
	}
}

// Recognize opens an upstream gRPC stream and returns a session bridge.
func (p *Realtime) Recognize(ctx context.Context, params stt.SessionParams) (stt.Session, error) {
	lang := strings.TrimSpace(params.Language)
	if lang == "" {
		lang = p.cfg.Language
	}
	sr := params.SampleRate
	if sr == 0 {
		sr = p.cfg.SampleRate
	}
	tok, err := p.src.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("sber-stt token: %w", err)
	}
	conn, err := dialGRPC(p.cfg)
	if err != nil {
		return nil, fmt.Errorf("sber-stt dial: %w", err)
	}
	mdCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+tok))
	client := recognition.NewSmartSpeechClient(conn)
	stream, err := client.Recognize(mdCtx)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sber-stt recognize: %w", err)
	}
	opts := &recognition.RecognitionOptions{
		AudioEncoding:         recognition.RecognitionOptions_PCM_S16LE,
		SampleRate:            int32(sr),
		Language:              lang,
		ChannelsCount:         1,
		EnablePartialResults:  p.cfg.PartialResults,
		EnableMultiUtterance:  p.cfg.MultiUtterance,
		HypothesesCount:       int32(p.cfg.HypothesesCount),
		NoSpeechTimeout:       durationpb.New(7 * time.Second),
		MaxSpeechTimeout:      durationpb.New(20 * time.Second),
		Hints:                 &recognition.Hints{EouTimeout: durationpb.New(time.Duration(p.cfg.EOUTimeoutMs) * time.Millisecond)},
		EnableProfanityFilter: false,
	}
	if err := stream.Send(&recognition.RecognitionRequest{
		Request: &recognition.RecognitionRequest_Options{Options: opts},
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sber-stt send options: %w", err)
	}

	s := &realtimeSession{
		stream:    stream,
		conn:      conn,
		bus:       eventbus.New(128),
		requestID: params.RequestID,
	}
	go s.readLoop(ctx)
	return s, nil
}

type realtimeSession struct {
	stream recognition.SmartSpeech_RecognizeClient
	conn   *grpc.ClientConn
	bus    *eventbus.Bus

	requestID string

	mu     sync.Mutex
	closed bool
}

func (s *realtimeSession) PushAudio(_ context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil
	}
	return s.stream.Send(&recognition.RecognitionRequest{
		Request: &recognition.RecognitionRequest_AudioChunk{AudioChunk: pcm},
	})
}

func (s *realtimeSession) Finish(_ context.Context) error {
	return s.stream.CloseSend()
}

func (s *realtimeSession) Events() <-chan v1.Event { return s.bus.Out() }

func (s *realtimeSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	_ = s.stream.CloseSend()
	_ = s.conn.Close()
	s.bus.Close()
	return nil
}

func (s *realtimeSession) readLoop(ctx context.Context) {
	defer s.bus.Close()
	for {
		resp, err := s.stream.Recv()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if !closed && ctx.Err() == nil {
				s.bus.Emit(v1.ErrorEvent(s.requestID, v1.CodeUpstream5XX, "sber-stt recv: "+err.Error(), false))
			}
			return
		}
		text := bestText(resp.GetResults())
		if strings.TrimSpace(text) == "" {
			continue
		}
		evType := v1.EventTranscriptPartial
		if resp.GetEou() {
			evType = v1.EventTranscriptFinal
		}
		s.bus.Emit(v1.Event{
			Type:      evType,
			RequestID: s.requestID,
			Text:      text,
		})
	}
}

func bestText(h []*recognition.Hypothesis) string {
	if len(h) == 0 || h[0] == nil {
		return ""
	}
	if t := strings.TrimSpace(h[0].GetNormalizedText()); t != "" {
		return t
	}
	return strings.TrimSpace(h[0].GetText())
}

func dialGRPC(cfg RealtimeConfig) (*grpc.ClientConn, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: !cfg.VerifyTLS,
		MinVersion:         tls.VersionTLS12,
	}
	if cfg.VerifyTLS {
		pem := strings.TrimSpace(cfg.CABundlePEM)
		if pem == "" && strings.TrimSpace(cfg.CABundle) != "" {
			raw, err := os.ReadFile(strings.TrimSpace(cfg.CABundle))
			if err != nil {
				return nil, fmt.Errorf("read ca_bundle_file: %w", err)
			}
			pem = string(raw)
		}
		if pem == "" {
			creds := credentials.NewTLS(tlsCfg)
			return grpc.DialContext(context.Background(), cfg.GRPCAddr, grpc.WithTransportCredentials(creds))
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(pem)) {
			return nil, fmt.Errorf("invalid PEM in ca_bundle_file")
		}
		tlsCfg.RootCAs = pool
	}
	creds := credentials.NewTLS(tlsCfg)
	return grpc.DialContext(context.Background(), cfg.GRPCAddr, grpc.WithTransportCredentials(creds))
}

var _ stt.Session = (*realtimeSession)(nil)
