package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// CreateWorkspaceRole adds a role to the tenant organization template. Caller
// must be a member of the workspace from which they invoke the action; the role
// itself is tenant-wide because Logto stores roles in the org template.
type CreateWorkspaceRole struct {
	idp ports.IdentityProvider
}

func NewCreateWorkspaceRole(idp ports.IdentityProvider) (*CreateWorkspaceRole, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &CreateWorkspaceRole{idp: idp}, nil
}

func (uc *CreateWorkspaceRole) Execute(ctx context.Context, caller domain.UserSub, workspaceID string, in domain.CreateOrganizationRoleRequest) (domain.OrganizationRoleInfo, error) {
	req, err := domain.ValidateCreateOrganizationRoleRequest(in)
	if err != nil {
		return domain.OrganizationRoleInfo{}, err
	}
	if _, err := requireMembership(ctx, uc.idp, caller, workspaceID); err != nil {
		return domain.OrganizationRoleInfo{}, err
	}
	role, err := uc.idp.CreateOrganizationRole(ctx, req.Name, req.Description)
	if err != nil {
		return domain.OrganizationRoleInfo{}, fmt.Errorf("create organization role: %w", err)
	}
	return role, nil
}
