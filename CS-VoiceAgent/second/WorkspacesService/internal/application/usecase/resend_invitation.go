package usecase

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// ResendInvitation re-sends the invitation email for a Pending invitation.
type ResendInvitation struct {
	idp                ports.IdentityProvider
	invitationLinkBase string
}

// ResendInvitationConfig wires dependencies and the SPA landing URL prefix.
type ResendInvitationConfig struct {
	IdentityProvider   ports.IdentityProvider
	InvitationLinkBase string
}

func NewResendInvitation(cfg ResendInvitationConfig) (*ResendInvitation, error) {
	if cfg.IdentityProvider == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	base := strings.TrimSpace(cfg.InvitationLinkBase)
	if base != "" {
		if _, err := url.Parse(base); err != nil {
			return nil, fmt.Errorf("invitation link base url is invalid: %w", err)
		}
		base = strings.TrimRight(base, "/")
	}
	return &ResendInvitation{
		idp:                cfg.IdentityProvider,
		invitationLinkBase: base,
	}, nil
}

func (uc *ResendInvitation) Execute(ctx context.Context, caller domain.UserSub, workspaceID, invitationID string) error {
	if uc.invitationLinkBase == "" {
		return domain.ErrInvitationNotConfigured
	}
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
	if inv.Status != domain.InvitationStatusPending {
		return fmt.Errorf("%w: status %s", domain.ErrInvitationNotActive, inv.Status)
	}
	link := uc.invitationLinkBase + "/" + url.PathEscape(inv.ID)
	if err := uc.idp.SendOrganizationInvitationMessage(ctx, inv.ID, link); err != nil {
		return fmt.Errorf("send invitation email: %w", err)
	}
	return nil
}
