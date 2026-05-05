package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi"
	"github.com/rs/zerolog/log"
	driver "go.mongodb.org/mongo-driver/mongo"
)

// Config описывает зависимости и настройки HTTP сервера.
// Здесь мы сознательно держим только то, что нужно для сборки transport-слоя.
type Config struct {
	Addr            string
	ShutdownTimeout time.Duration
	BodyLimitBytes  int64

	MongoClient *driver.Client
	MongoDB     string
}

// Server — тонкая обёртка над http.Server: отвечает только за запуск и graceful shutdown.
type Server struct {
	cfg    Config
	server *http.Server
}

func New(cfg Config) *Server {
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	router := httpapi.NewRouter(httpapi.RouterDeps{
		BodyLimitBytes: cfg.BodyLimitBytes,
		MongoClient:    cfg.MongoClient,
		MongoDB:        cfg.MongoDB,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	return &Server{cfg: cfg, server: httpServer}
}

func (s *Server) Start() error {
	log.Info().Str("addr", s.cfg.Addr).Msg("agents-service: starting")
	err := s.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.server.Shutdown(shutdownCtx)
}

