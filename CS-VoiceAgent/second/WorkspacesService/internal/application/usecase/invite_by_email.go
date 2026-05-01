package usecase

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// InviteByEmail creates a Logto organization invitation for an email address and
// asks Logto to send the invitation email. It does NOT add anyone to the
// organization — the invitee accepts the invitation via the SPA landing page,
// which calls AcceptInvitation.
type InviteByEmail struct {
	idp                ports.IdentityProvider
	invitationLinkBase string
}

// InviteByEmailConfig wires the use case dependencies.
type InviteByEmailConfig struct {
	IdentityProvider ports.IdentityProvider
	// InvitationLinkBase is the SPA URL prefix the invitee will land on; the
	// invitation id is appended as the last path segment. Example:
	//   https://app.example.com/invitations
	// becomes https://app.example.com/invitations/<id>.
	InvitationLinkBase string
}

func NewInviteByEmail(cfg InviteByEmailConfig) (*InviteByEmail, error) {
	if cfg.IdentityProvider == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	base := strings.TrimSpace(cfg.InvitationLinkBase)
	if base == "" {
		return nil, fmt.Errorf("invitation link base url is required")
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("invitation link base url is invalid: %w", err)
	}
	return &InviteByEmail{
		idp:                cfg.IdentityProvider,
		invitationLinkBase: strings.TrimRight(base, "/"),
	}, nil
}

func (uc *InviteByEmail) Execute(ctx context.Context, caller domain.UserSub, workspaceID string, in domain.CreateInvitationRequest) (domain.Invitation, error) {
	if uc.invitationLinkBase == "" {
		return domain.Invitation{}, domain.ErrInvitationNotConfigured
	}
	req, err := domain.ValidateCreateInvitationRequest(in)
	if err != nil {
		return domain.Invitation{}, err
	}
	if _, err := requireMembership(ctx, uc.idp, caller, workspaceID); err != nil {
		return domain.Invitation{}, err
	}

	roleID, err := uc.resolveRoleID(ctx, req.RoleName)
	if err != nil {
		return domain.Invitation{}, err
	}

	inv, err := uc.idp.CreateOrganizationInvitation(ctx, ports.CreateInvitationInput{
		OrganizationID: workspaceID,
		Invitee:        req.Email,
		RoleIDs:        []string{roleID},
		InviterID:      caller.String(),
	})
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("create organization invitation: %w", err)
	}

	link := uc.invitationLinkBase + "/" + url.PathEscape(inv.ID)
	if err := uc.idp.SendOrganizationInvitationMessage(ctx, inv.ID, link); err != nil {
		return domain.Invitation{}, fmt.Errorf("send invitation email: %w", err)
	}

	if len(inv.RoleNames) == 0 {
		inv.RoleNames = []string{req.RoleName}
	}
	return inv, nil
}

func (uc *InviteByEmail) resolveRoleID(ctx context.Context, roleName string) (string, error) {
	roles, err := uc.idp.ListOrganizationRoleCatalog(ctx)
	if err != nil {
		return "", fmt.Errorf("list organization roles: %w", err)
	}
	for _, role := range roles {
		if strings.EqualFold(role.Name, roleName) {
			return role.ID, nil
		}
	}
	return "", fmt.Errorf("%w: role %q is not in the tenant template", domain.ErrOrganizationRolesMissing, roleName)
}
