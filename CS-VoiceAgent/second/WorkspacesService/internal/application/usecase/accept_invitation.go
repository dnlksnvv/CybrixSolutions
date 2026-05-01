package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// AcceptInvitation finalises an invitation: Logto adds the caller to the
// organization with the invitation's roles. The caller MUST be the invitee —
// we enforce this by comparing primary email server-side, never trust the
// frontend.
type AcceptInvitation struct {
	idp ports.IdentityProvider
}

func NewAcceptInvitation(idp ports.IdentityProvider) (*AcceptInvitation, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &AcceptInvitation{idp: idp}, nil
}

func (uc *AcceptInvitation) Execute(ctx context.Context, caller domain.UserSub, invitationID string) (domain.Invitation, error) {
	if caller == "" {
		return domain.Invitation{}, domain.ErrInvalidOwner
	}
	if invitationID == "" {
		return domain.Invitation{}, domain.ErrInvitationNotFound
	}

	inv, err := uc.idp.GetOrganizationInvitation(ctx, invitationID)
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("get invitation: %w", err)
	}
	if inv.Status != domain.InvitationStatusPending {
		return domain.Invitation{}, fmt.Errorf("%w: status %s", domain.ErrInvitationNotActive, inv.Status)
	}

	user, err := uc.idp.GetUser(ctx, caller)
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("load caller user: %w", err)
	}
	if !userMatchesInvitee(user, inv.Invitee) {
		return domain.Invitation{}, domain.ErrEmailMismatch
	}

	updated, err := uc.idp.UpdateOrganizationInvitationStatus(ctx, invitationID, domain.InvitationStatusAccepted, caller.String())
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("accept invitation: %w", err)
	}
	if len(updated.RoleNames) == 0 {
		updated.RoleNames = inv.RoleNames
	}
	if updated.OrganizationName == "" {
		updated.OrganizationName = inv.OrganizationName
	}
	return updated, nil
}
