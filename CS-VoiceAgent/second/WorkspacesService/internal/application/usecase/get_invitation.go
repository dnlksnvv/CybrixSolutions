package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// GetInvitation returns a single invitation by id. The caller must be either:
//   - the invitee (matched by email), or
//   - already a member of the workspace.
//
// Used by the SPA invitation landing page after the user signs in / signs up.
type GetInvitation struct {
	idp ports.IdentityProvider
}

func NewGetInvitation(idp ports.IdentityProvider) (*GetInvitation, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &GetInvitation{idp: idp}, nil
}

func (uc *GetInvitation) Execute(ctx context.Context, caller domain.UserSub, invitationID string) (domain.Invitation, error) {
	if invitationID == "" {
		return domain.Invitation{}, domain.ErrInvitationNotFound
	}
	inv, err := uc.idp.GetOrganizationInvitation(ctx, invitationID)
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("get invitation: %w", err)
	}

	// Authorise: invitee (email match) OR existing member.
	allowed, err := callerAllowedForInvitation(ctx, uc.idp, caller, inv)
	if err != nil {
		return domain.Invitation{}, err
	}
	if !allowed {
		return domain.Invitation{}, domain.ErrInvitationNotFound
	}

	if inv.OrganizationName == "" && inv.OrganizationID != "" {
		if org, err := uc.idp.GetOrganization(ctx, inv.OrganizationID); err == nil {
			inv.OrganizationName = org.Name
		}
	}
	return inv, nil
}

// callerAllowedForInvitation verifies the caller is the invitee or already a workspace member.
// Errors related to identity lookups are propagated; non-matching callers return (false, nil).
func callerAllowedForInvitation(ctx context.Context, idp ports.IdentityProvider, caller domain.UserSub, inv domain.Invitation) (bool, error) {
	if caller == "" {
		return false, domain.ErrInvalidOwner
	}
	user, err := idp.GetUser(ctx, caller)
	if err != nil {
		// Treat missing user as unauthorised rather than 5xx.
		if errors.Is(err, domain.ErrUserNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load caller user: %w", err)
	}
	if userMatchesInvitee(user, inv.Invitee) {
		return true, nil
	}
	if _, err := requireMembership(ctx, idp, caller, inv.OrganizationID); err == nil {
		return true, nil
	}
	return false, nil
}
