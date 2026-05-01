package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// GetWorkspace returns a workspace by Logto organization id, but only if the
// requesting user is a member. Membership is verified against Logto on every
// call to avoid stale local state.
type GetWorkspace struct {
	idp ports.IdentityProvider
}

func NewGetWorkspace(idp ports.IdentityProvider) (*GetWorkspace, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &GetWorkspace{idp: idp}, nil
}

func (uc *GetWorkspace) Execute(ctx context.Context, userSub domain.UserSub, workspaceID string) (domain.Workspace, error) {
	org, err := requireMembership(ctx, uc.idp, userSub, workspaceID)
	if err != nil {
		return domain.Workspace{}, err
	}
	return domain.Workspace{
		ID:          org.ID,
		Name:        org.Name,
		Description: org.Description,
	}, nil
}
