// Package main is the entrypoint for the inference-gateway service.
//
// Responsibilities:
//   - Load .env / config.
//   - Build the model registry from configured providers.
//   - Wire transport handlers and start the HTTP server.
//   - Handle graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cybrix/inference-gateway/internal/config"
	"github.com/cybrix/inference-gateway/internal/logger"
	"github.com/cybrix/inference-gateway/internal/registry"
	llmqwen "github.com/cybrix/inference-gateway/internal/services/llm/qwen"
	llmsber "github.com/cybrix/inference-gateway/internal/services/llm/sber"
	sttqwen "github.com/cybrix/inference-gateway/internal/services/stt/qwen"
	sttsber "github.com/cybrix/inference-gateway/internal/services/stt/sber"
	"github.com/cybrix/inference-gateway/internal/services/tts/qwen"
	"github.com/cybrix/inference-gateway/internal/services/tts/sber"
	httpserver "github.com/cybrix/inference-gateway/internal/transport/http"
	"github.com/cybrix/inference-gateway/internal/transport/ws"
)

func main() {
	log := logger.New()

	cfg, err := config.Load()
	if err != nil {
		log.Error("config load failed", "err", err)
		os.Exit(1)
	}

	llmSeen := make(map[string]struct{})
	gigaLLM, err := llmsber.LoadProfiles(llmSeen)
	if err != nil {
		log.Error("llm sber (gigachat) profiles load failed", "err", err)
		os.Exit(1)
	}
	qwenLLM, err := llmqwen.LoadProfiles(llmSeen)
	if err != nil {
		log.Error("llm qwen profiles load failed", "err", err)
		os.Exit(1)
	}
	if len(gigaLLM) == 0 {
		log.Info("no llm sber yaml profiles (LLM_SBER_TEMPLATE_DIR / model-templates/llm/sber)")
	}
	if len(qwenLLM) == 0 {
		log.Info("no llm qwen yaml profiles (LLM_QWEN_TEMPLATE_DIR / model-templates/llm/qwen)")
	}

	qwenSTT, err := sttqwen.LoadQwenProfiles()
	if err != nil {
		log.Error("qwen stt profiles load failed", "err", err)
		os.Exit(1)
	}
	if len(qwenSTT) == 0 {
		log.Info("no qwen stt yaml profiles found (STT_QWEN_TEMPLATE_DIR / model-templates/stt/qwen-realtime)")
	}
	sberSTT, err := sttsber.LoadSberProfiles()
	if err != nil {
		log.Error("sber stt profiles load failed", "err", err)
		os.Exit(1)
	}
	if len(sberSTT) == 0 {
		log.Info("no sber stt yaml profiles found (STT_SBER_TEMPLATE_DIR / model-templates/stt/sber-realtime)")
	}

	qwenTTS, err := qwen.LoadQwenProfiles()
	if err != nil {
		log.Error("qwen tts profiles load failed", "err", err)
		os.Exit(1)
	}

	sberTTS, err := sber.LoadSberProfiles()
	if err != nil {
		log.Error("sber tts profiles load failed", "err", err)
		os.Exit(1)
	}

	reg := registry.New()
	registerProviders(reg, cfg, gigaLLM, qwenLLM, qwenSTT, sberSTT, qwenTTS, sberTTS, log)

	llmModels, ttsModels, sttModels := reg.Models()
	log.Info("registry built",
		"llm", llmModels, "tts", ttsModels, "stt", sttModels,
	)

	deps := httpserver.Deps{Logger: log, Registry: reg}
	wsTTS := ws.NewTTSHandler(reg, log)
	wsSTT := ws.NewSTTHandler(reg, log)
	router := httpserver.NewRouter(deps, wsTTS, wsSTT)

	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	go func() {
		log.Info("listening", "addr", cfg.HTTP.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http listen failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	log.Info("bye")
}

func registerProviders(
	reg *registry.Registry,
	cfg *config.Config,
	gigaLLM []llmsber.ProfileBundle,
	qwenLLM []llmqwen.ProfileBundle,
	qwenSTT []sttqwen.ProfileBundle,
	sberSTT []sttsber.ProfileBundle,
	qwenTTS []qwen.ProfileBundle,
	sberTTS []sber.ProfileBundle,
	log *slog.Logger,
) {
	// --- LLM ---

	for _, bundle := range gigaLLM {
		if !bundle.Config.Enabled() {
			log.Info("llm gigachat profile skipped (missing credentials_env)",
				"public_ids", bundle.PublicIDs,
			)
			continue
		}
		prov := llmsber.NewGigaChat(bundle.Config, cfg.UpstreamHTTPTimeout)
		for _, id := range bundle.PublicIDs {
			mustReg(log, reg.RegisterLLM(id, prov))
		}
		log.Info("llm gigachat profile registered", "public_ids", bundle.PublicIDs)
	}

	for _, bundle := range qwenLLM {
		if !bundle.Config.Enabled() {
			log.Info("llm qwen profile skipped (missing api_key_env)",
				"public_ids", bundle.PublicIDs,
			)
			continue
		}
		prov := llmqwen.NewQwen(bundle.Config, cfg.UpstreamHTTPTimeout)
		for _, id := range bundle.PublicIDs {
			mustReg(log, reg.RegisterLLM(id, prov))
		}
		log.Info("llm qwen profile registered",
			"public_ids", bundle.PublicIDs,
			"upstream_model", bundle.Config.Model,
		)
	}

	// --- TTS ---

	for _, bundle := range sberTTS {
		if !bundle.Config.Enabled() {
			log.Info("tts sber profile skipped (missing credentials_env / grpc_addr)",
				"public_ids", bundle.PublicIDs,
			)
			continue
		}
		prov := sber.NewPolicySynth(bundle.Config, bundle.Policy)
		for _, id := range bundle.PublicIDs {
			mustReg(log, reg.RegisterTTS(id, prov))
		}
		log.Info("tts sber profile registered",
			"public_ids", bundle.PublicIDs,
		)
	}

	for _, bundle := range qwenTTS {
		if !bundle.Config.Enabled() {
			log.Info("tts qwen profile skipped (missing api_key_env / upstream_model / ws_url)",
				"public_ids", bundle.PublicIDs,
				"upstream_model", bundle.Config.Model,
			)
			continue
		}
		prov := qwen.NewPolicyQwenRealtime(bundle.Config, bundle.Policy)
		for _, id := range bundle.PublicIDs {
			mustReg(log, reg.RegisterTTS(id, prov))
			reg.MarkTTSDualUpstream(id)
		}
		log.Info("tts qwen profile registered",
			"public_ids", bundle.PublicIDs,
			"upstream_model", bundle.Config.Model,
		)
	}

	// --- STT ---

	for _, bundle := range qwenSTT {
		if !bundle.Config.Enabled() {
			log.Info("stt qwen profile skipped (missing api_key_env / ws_url)",
				"public_ids", bundle.PublicIDs,
				"upstream_model", bundle.Config.Model,
			)
			continue
		}
		prov := sttqwen.NewPolicyQwenRealtime(bundle.Config, bundle.Policy)
		for _, id := range bundle.PublicIDs {
			mustReg(log, reg.RegisterSTT(id, prov))
		}
		log.Info("stt qwen profile registered",
			"public_ids", bundle.PublicIDs,
			"upstream_model", bundle.Config.Model,
		)
	}
	for _, bundle := range sberSTT {
		if !bundle.Config.Enabled() {
			log.Info("stt sber profile skipped (missing credentials_env / grpc_addr)",
				"public_ids", bundle.PublicIDs,
			)
			continue
		}
		prov := sttsber.NewPolicyRealtime(bundle.Config, bundle.Policy)
		for _, id := range bundle.PublicIDs {
			mustReg(log, reg.RegisterSTT(id, prov))
		}
		log.Info("stt sber profile registered",
			"public_ids", bundle.PublicIDs,
			"upstream_addr", bundle.Config.GRPCAddr,
		)
	}
}

func mustReg(log *slog.Logger, err error) {
	if err != nil {
		log.Error("registry registration failed", "err", err)
		os.Exit(1)
	}
}
