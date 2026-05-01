package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// CreateWorkspace orchestrates workspace creation in Logto:
//  1. create organization,
//  2. add the calling user as a member,
//  3. assign the configured owner role.
//
// No data is persisted locally — Logto is the single source of truth.
type CreateWorkspace struct {
	idp       ports.IdentityProvider
	ownerRole domain.OrganizationRole
}

type CreateWorkspaceConfig struct {
	IdentityProvider ports.IdentityProvider
	OwnerRole        domain.OrganizationRole
}

func NewCreateWorkspace(cfg CreateWorkspaceConfig) (*CreateWorkspace, error) {
	if cfg.IdentityProvider == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	if err := cfg.OwnerRole.Validate(); err != nil {
		return nil, fmt.Errorf("invalid owner role: %w", err)
	}
	return &CreateWorkspace{
		idp:       cfg.IdentityProvider,
		ownerRole: cfg.OwnerRole,
	}, nil
}

func (uc *CreateWorkspace) Execute(ctx context.Context, in domain.CreateWorkspaceRequest) (domain.Workspace, error) {
	req, err := domain.ValidateCreateWorkspaceRequest(in)
	if err != nil {
		return domain.Workspace{}, err
	}

	org, err := uc.idp.CreateOrganization(ctx, ports.CreateOrganizationInput{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create organization: %w", err)
	}

	if err := uc.idp.AddOrganizationMember(ctx, org.ID, req.OwnerSub); err != nil {
		return domain.Workspace{}, fmt.Errorf("add owner as member: %w", err)
	}

	if err := uc.idp.AssignOrganizationRole(ctx, org.ID, req.OwnerSub, uc.ownerRole); err != nil {
		return domain.Workspace{}, fmt.Errorf("assign owner role: %w", err)
	}

	return domain.Workspace{
		ID:          org.ID,
		Name:        org.Name,
		Description: org.Description,
	}, nil
}
