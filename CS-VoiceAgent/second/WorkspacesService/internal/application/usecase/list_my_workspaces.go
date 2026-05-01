package usecase

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// ListMyWorkspaces returns workspaces (Logto organizations) the user belongs
// to. Logto is consulted on every call so the response always reflects the
// current membership state.
type ListMyWorkspaces struct {
	idp ports.IdentityProvider
}

func NewListMyWorkspaces(idp ports.IdentityProvider) (*ListMyWorkspaces, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	return &ListMyWorkspaces{idp: idp}, nil
}

func (uc *ListMyWorkspaces) Execute(ctx context.Context, userSub domain.UserSub) ([]domain.Workspace, error) {
	if userSub == "" {
		return nil, domain.ErrInvalidOwner
	}
	orgs, err := uc.idp.ListUserOrganizations(ctx, userSub)
	if err != nil {
		return nil, fmt.Errorf("list user organizations: %w", err)
	}
	out := make([]domain.Workspace, 0, len(orgs))
	for _, org := range orgs {
		out = append(out, domain.Workspace{
			ID:          org.ID,
			Name:        org.Name,
			Description: org.Description,
		})
	}
	return out, nil
}
