package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cybrix-solutions/agents-service/internal/config"
	"github.com/cybrix-solutions/agents-service/internal/logger"
	"github.com/cybrix-solutions/agents-service/internal/repository/mongo"
	"github.com/cybrix-solutions/agents-service/internal/server"
	"github.com/rs/zerolog/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logg := logger.New(cfg.LogLevel)
	log.Logger = logg

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mongoClient, err := mongo.Connect(ctx, cfg.Mongo)
	if err != nil {
		log.Fatal().Err(err).Msg("mongo: connect failed")
	}
	if err := mongo.EnsureIndexes(ctx, mongoClient.Database(cfg.Mongo.DB)); err != nil {
		log.Fatal().Err(err).Msg("mongo: ensure indexes failed")
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mongoClient.Disconnect(shutdownCtx)
	}()

	srv := server.New(server.Config{
		Addr:           cfg.HTTPAddr,
		ShutdownTimeout: cfg.ShutdownTimeout,
		BodyLimitBytes: cfg.HTTPBodyLimitBytes,
		MongoClient:    mongoClient,
		MongoDB:        cfg.Mongo.DB,
	})

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http: server failed")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown: signal received")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Error().Err(err).Msg("shutdown: http server shutdown failed")
		os.Exit(1)
	}
	log.Info().Msg("shutdown: complete")
}

