package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// RevokeInvitation marks a Pending invitation as Revoked. Caller must be a
// member of the workspace the invitation belongs to.
type RevokeInvitation struct {
	idp ports.IdentityProvider
}

func NewRevokeInvitation(idp ports.IdentityProvider) (*RevokeInvitation, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &RevokeInvitation{idp: idp}, nil
}

func (uc *RevokeInvitation) Execute(ctx context.Context, caller domain.UserSub, workspaceID, invitationID string) error {
	if invitationID == "" {
		return domain.ErrInvitationNotFound
	}
	if _, err := requireMembership(ctx, uc.idp, caller, workspaceID); err != nil {
		return err
	}
	inv, err := uc.idp.GetOrganizationInvitation(ctx, invitationID)
	if err != nil {
		return fmt.Errorf("get invitation: %w", err)
	}
	if inv.OrganizationID != workspaceID {
		return domain.ErrInvitationNotFound
	}
	if _, err := uc.idp.UpdateOrganizationInvitationStatus(ctx, invitationID, domain.InvitationStatusRevoked, ""); err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	return nil
}
