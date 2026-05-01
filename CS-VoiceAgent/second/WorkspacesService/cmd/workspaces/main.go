package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cybrix-solutions/workspaces-service/internal/adapters/identity/logto"
	httpadapter "github.com/cybrix-solutions/workspaces-service/internal/adapters/transport/http"
	"github.com/cybrix-solutions/workspaces-service/internal/application/usecase"
	"github.com/cybrix-solutions/workspaces-service/internal/config"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

func main() {
	logger := newLogger("info")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	logger = newLogger(cfg.LogLevel)

	logtoClient, err := logto.NewClient(logto.Config{
		TenantID:             cfg.LogtoTenantID,
		TokenEndpoint:        cfg.LogtoTokenEndpoint,
		ManagementAPIBaseURL: cfg.LogtoManagementBaseURL,
		ManagementResource:   cfg.LogtoManagementResource,
		ClientID:             cfg.LogtoM2MClientID,
		ClientSecret:         cfg.LogtoM2MClientSecret,
		Scope:                cfg.LogtoM2MScope,
		HTTPTimeout:          cfg.LogtoHTTPTimeout,
		TokenSafetyWindow:    30 * time.Second,
	})
	if err != nil {
		logger.Error("init logto client", "error", err)
		os.Exit(1)
	}

	createWorkspace, err := usecase.NewCreateWorkspace(usecase.CreateWorkspaceConfig{
		IdentityProvider: logtoClient,
		OwnerRole:        domain.OrganizationRole(cfg.WorkspaceOwnerRole),
	})
	if err != nil {
		logger.Error("init create workspace use case", "error", err)
		os.Exit(1)
	}

	listMine, err := usecase.NewListMyWorkspaces(logtoClient)
	if err != nil {
		logger.Error("init list workspaces use case", "error", err)
		os.Exit(1)
	}

	getWorkspace, err := usecase.NewGetWorkspace(logtoClient)
	if err != nil {
		logger.Error("init get workspace use case", "error", err)
		os.Exit(1)
	}

	updateWorkspace, err := usecase.NewUpdateWorkspace(logtoClient)
	if err != nil {
		logger.Error("init update workspace use case", "error", err)
		os.Exit(1)
	}

	listMembers, err := usecase.NewListWorkspaceMembers(logtoClient)
	if err != nil {
		logger.Error("init list members use case", "error", err)
		os.Exit(1)
	}

	listRoles, err := usecase.NewListWorkspaceRoles(logtoClient)
	if err != nil {
		logger.Error("init list roles use case", "error", err)
		os.Exit(1)
	}

	leaveWorkspace, err := usecase.NewLeaveWorkspace(logtoClient)
	if err != nil {
		logger.Error("init leave workspace use case", "error", err)
		os.Exit(1)
	}

	removeMember, err := usecase.NewRemoveWorkspaceMember(logtoClient, cfg.WorkspaceOwnerRole)
	if err != nil {
		logger.Error("init remove workspace member use case", "error", err)
		os.Exit(1)
	}

	createRole, err := usecase.NewCreateWorkspaceRole(logtoClient)
	if err != nil {
		logger.Error("init create role use case", "error", err)
		os.Exit(1)
	}

	inviteByEmail, err := usecase.NewInviteByEmail(usecase.InviteByEmailConfig{
		IdentityProvider:   logtoClient,
		InvitationLinkBase: cfg.InvitationLinkBase,
	})
	if err != nil {
		logger.Error("init invite by email use case", "error", err)
		os.Exit(1)
	}

	listInvitations, err := usecase.NewListInvitations(logtoClient)
	if err != nil {
		logger.Error("init list invitations use case", "error", err)
		os.Exit(1)
	}

	revokeInvitation, err := usecase.NewRevokeInvitation(logtoClient)
	if err != nil {
		logger.Error("init revoke invitation use case", "error", err)
		os.Exit(1)
	}

	resendInvitation, err := usecase.NewResendInvitation(usecase.ResendInvitationConfig{
		IdentityProvider:   logtoClient,
		InvitationLinkBase: cfg.InvitationLinkBase,
	})
	if err != nil {
		logger.Error("init resend invitation use case", "error", err)
		os.Exit(1)
	}

	getInvitation, err := usecase.NewGetInvitation(logtoClient)
	if err != nil {
		logger.Error("init get invitation use case", "error", err)
		os.Exit(1)
	}

	acceptInvitation, err := usecase.NewAcceptInvitation(logtoClient)
	if err != nil {
		logger.Error("init accept invitation use case", "error", err)
		os.Exit(1)
	}

	workspaceHandler := httpadapter.NewWorkspaceHandler(httpadapter.WorkspaceHandlerConfig{
		Create:           createWorkspace,
		List:             listMine,
		Get:              getWorkspace,
		Update:           updateWorkspace,
		Members:          listMembers,
		Roles:            listRoles,
		Leave:            leaveWorkspace,
		RemoveMember:     removeMember,
		CreateRole:       createRole,
		InviteByEmail:    inviteByEmail,
		ListInvitations:  listInvitations,
		RevokeInvitation: revokeInvitation,
		ResendInvitation: resendInvitation,
	})
	invitationHandler := httpadapter.NewInvitationHandler(httpadapter.InvitationHandlerConfig{
		Invite: inviteByEmail,
		List:   listInvitations,
		Revoke: revokeInvitation,
		Resend: resendInvitation,
		Get:    getInvitation,
		Accept: acceptInvitation,
	})
	server := httpadapter.NewServer(httpadapter.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Logger:          logger,
		WorkspaceH:      workspaceHandler,
		InvitationH:     invitationHandler,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	go func() {
		if err := server.Start(); err != nil {
			logger.Error("http server stopped", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	if err := server.Shutdown(context.Background()); err != nil {
		logger.Error("shutdown", "error", err)
		os.Exit(1)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}
