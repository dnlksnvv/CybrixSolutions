package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// UpdateWorkspace patches mutable fields on a workspace (Logto organization),
// after verifying the caller is a member.
type UpdateWorkspace struct {
	idp ports.IdentityProvider
}

func NewUpdateWorkspace(idp ports.IdentityProvider) (*UpdateWorkspace, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &UpdateWorkspace{idp: idp}, nil
}

func (uc *UpdateWorkspace) Execute(ctx context.Context, userSub domain.UserSub, workspaceID string, in domain.UpdateWorkspaceRequest) (domain.Workspace, error) {
	patch, err := domain.ValidateUpdateWorkspaceRequest(in)
	if err != nil {
		return domain.Workspace{}, err
	}
	if _, err := requireMembership(ctx, uc.idp, userSub, workspaceID); err != nil {
		return domain.Workspace{}, err
	}
	org, err := uc.idp.UpdateOrganization(ctx, workspaceID, ports.UpdateOrganizationInput{
		Name:        patch.Name,
		Description: patch.Description,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("update organization: %w", err)
	}
	return domain.Workspace{
		ID:          org.ID,
		Name:        org.Name,
		Description: org.Description,
	}, nil
}
