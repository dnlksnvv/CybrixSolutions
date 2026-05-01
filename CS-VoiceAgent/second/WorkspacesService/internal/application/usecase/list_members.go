package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// ListWorkspaceMembers returns members of a workspace with their organization
// roles. The caller must be a member of the workspace.
type ListWorkspaceMembers struct {
	idp ports.IdentityProvider
}

func NewListWorkspaceMembers(idp ports.IdentityProvider) (*ListWorkspaceMembers, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &ListWorkspaceMembers{idp: idp}, nil
}

func (uc *ListWorkspaceMembers) Execute(ctx context.Context, userSub domain.UserSub, workspaceID string) ([]domain.WorkspaceMember, error) {
	if _, err := requireMembership(ctx, uc.idp, userSub, workspaceID); err != nil {
		return nil, err
	}
	return uc.idp.ListOrganizationMembers(ctx, workspaceID)
}
