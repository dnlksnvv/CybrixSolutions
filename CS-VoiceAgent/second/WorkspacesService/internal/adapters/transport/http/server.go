package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type ServerConfig struct {
	Addr            string
	Logger          *slog.Logger
	WorkspaceH      *WorkspaceHandler
	InvitationH     *InvitationHandler
	ReadHeaderTime  time.Duration
	WriteTime       time.Duration
	ShutdownTimeout time.Duration
}

type Server struct {
	cfg    ServerConfig
	server *http.Server
}

func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ReadHeaderTime <= 0 {
		cfg.ReadHeaderTime = 5 * time.Second
	}
	if cfg.WriteTime <= 0 {
		cfg.WriteTime = 15 * time.Second
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	// After forwardAuth accepts OPTIONS, Chi must answer preflight (no RequireUserID).
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodOptions && (strings.HasPrefix(req.URL.Path, "/workspaces") || strings.HasPrefix(req.URL.Path, "/invitations")) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Group(func(api chi.Router) {
		api.Use(RequireUserID)
		// Paths match Traefik PathPrefix(`/workspaces`) — no extra `/workspaces` segment.
		api.Post("/workspaces", cfg.WorkspaceH.Create)
		api.Get("/workspaces", cfg.WorkspaceH.ListMine)
		api.Get("/workspaces/{id}", cfg.WorkspaceH.Get)
		api.Patch("/workspaces/{id}", cfg.WorkspaceH.Update)
		api.Get("/workspaces/{id}/members", cfg.WorkspaceH.ListMembers)
		api.Delete("/workspaces/{id}/members/me", cfg.WorkspaceH.LeaveWorkspace)
		api.Delete("/workspaces/{id}/members/{userId}", cfg.WorkspaceH.RemoveMember)
		api.Get("/workspaces/{id}/roles", cfg.WorkspaceH.ListRoles)
		api.Post("/workspaces/{id}/roles", cfg.WorkspaceH.CreateRole)

		// Invitations — admin side (caller must be a workspace member).
		api.Post("/workspaces/{id}/invitations", cfg.InvitationH.CreateForWorkspace)
		api.Get("/workspaces/{id}/invitations", cfg.InvitationH.ListForWorkspace)
		api.Delete("/workspaces/{id}/invitations/{invitationId}", cfg.InvitationH.RevokeForWorkspace)
		api.Post("/workspaces/{id}/invitations/{invitationId}/resend", cfg.InvitationH.ResendForWorkspace)

		// Invitations — invitee side (caller must be the invitee or a member).
		api.Get("/invitations/{id}", cfg.InvitationH.Get)
		api.Post("/invitations/{id}/accept", cfg.InvitationH.Accept)
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: cfg.ReadHeaderTime,
		WriteTimeout:      cfg.WriteTime,
	}
	return &Server{cfg: cfg, server: httpServer}
}

func (s *Server) Start() error {
	s.cfg.Logger.Info("workspaces-service: starting", "addr", s.cfg.Addr)
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.server.Shutdown(shutdownCtx)
}
