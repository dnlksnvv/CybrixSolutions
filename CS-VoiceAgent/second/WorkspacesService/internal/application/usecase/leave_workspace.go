package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// LeaveWorkspace removes the calling user from a workspace they belong to.
type LeaveWorkspace struct {
	idp ports.IdentityProvider
}

func NewLeaveWorkspace(idp ports.IdentityProvider) (*LeaveWorkspace, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &LeaveWorkspace{idp: idp}, nil
}

func (uc *LeaveWorkspace) Execute(ctx context.Context, userSub domain.UserSub, workspaceID string) error {
	if _, err := requireMembership(ctx, uc.idp, userSub, workspaceID); err != nil {
		return err
	}
	if err := uc.idp.RemoveOrganizationMember(ctx, workspaceID, userSub); err != nil {
		return fmt.Errorf("remove organization member: %w", err)
	}
	return nil
}
