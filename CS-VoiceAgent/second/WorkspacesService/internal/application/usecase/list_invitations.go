package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// ListInvitations returns Logto invitations for a workspace; the caller must be
// a member of the workspace.
type ListInvitations struct {
	idp ports.IdentityProvider
}

func NewListInvitations(idp ports.IdentityProvider) (*ListInvitations, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &ListInvitations{idp: idp}, nil
}

func (uc *ListInvitations) Execute(ctx context.Context, caller domain.UserSub, workspaceID string) ([]domain.Invitation, error) {
	if _, err := requireMembership(ctx, uc.idp, caller, workspaceID); err != nil {
		return nil, err
	}
	return uc.idp.ListOrganizationInvitations(ctx, workspaceID)
}
