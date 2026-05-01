package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// requireMembership confirms the user belongs to the workspace via Logto.
// Returns the matching organization or domain.ErrWorkspaceNotFound.
func requireMembership(ctx context.Context, idp ports.IdentityProvider, userSub domain.UserSub, workspaceID string) (ports.Organization, error) {
	if userSub == "" {
		return ports.Organization{}, domain.ErrInvalidOwner
	}
	if workspaceID == "" {
		return ports.Organization{}, domain.ErrWorkspaceNotFound
	}
	orgs, err := idp.ListUserOrganizations(ctx, userSub)
	if err != nil {
		return ports.Organization{}, fmt.Errorf("list user organizations: %w", err)
	}
	for _, org := range orgs {
		if org.ID == workspaceID {
			return org, nil
		}
	}
	return ports.Organization{}, domain.ErrWorkspaceNotFound
}
