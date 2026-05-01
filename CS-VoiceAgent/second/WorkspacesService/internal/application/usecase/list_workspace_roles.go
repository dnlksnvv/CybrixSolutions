package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// ListWorkspaceRoles returns the roles available to a workspace. Roles live in
// the tenant's organization template, so the result is the same for every
// workspace the user belongs to. We still verify membership before returning
// to avoid leaking the catalog to non-members.
type ListWorkspaceRoles struct {
	idp ports.IdentityProvider
}

func NewListWorkspaceRoles(idp ports.IdentityProvider) (*ListWorkspaceRoles, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &ListWorkspaceRoles{idp: idp}, nil
}

func (uc *ListWorkspaceRoles) Execute(ctx context.Context, userSub domain.UserSub, workspaceID string) ([]domain.OrganizationRoleInfo, error) {
	if _, err := requireMembership(ctx, uc.idp, userSub, workspaceID); err != nil {
		return nil, err
	}
	return uc.idp.ListOrganizationRoleCatalog(ctx)
}
