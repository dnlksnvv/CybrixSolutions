package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// RemoveWorkspaceMember removes another user from a workspace. Caller must be
// Owner or Admin. Admins cannot remove a user who has the Owner role. Callers
// cannot remove themselves (use LeaveWorkspace).
type RemoveWorkspaceMember struct {
	idp           ports.IdentityProvider
	ownerRoleName string
}

// RemoveWorkspaceMemberConfig wires dependencies. ownerRoleName must match
// WORKSPACE_OWNER_ROLE / the role assigned to workspace creators (default "Owner").
func NewRemoveWorkspaceMember(idp ports.IdentityProvider, ownerRoleName string) (*RemoveWorkspaceMember, error) {
	if idp == nil {
		return nil, fmt.Errorf("identity provider is required")
	}
	ownerRoleName = strings.TrimSpace(ownerRoleName)
	if ownerRoleName == "" {
		ownerRoleName = string(domain.RoleOwner)
	}
	return &RemoveWorkspaceMember{idp: idp, ownerRoleName: ownerRoleName}, nil
}

func (uc *RemoveWorkspaceMember) Execute(ctx context.Context, caller domain.UserSub, workspaceID, targetUserID string) error {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return domain.ErrInvalidOwner
	}
	if caller.String() == targetUserID {
		return domain.ErrCannotRemoveYourself
	}
	if _, err := requireMembership(ctx, uc.idp, caller, workspaceID); err != nil {
		return err
	}
	members, err := uc.idp.ListOrganizationMembers(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list organization members: %w", err)
	}
	callerRow := findWorkspaceMember(members, caller.String())
	if callerRow == nil {
		return domain.ErrWorkspaceNotFound
	}
	if !uc.canManageMembers(callerRow.RoleNames) {
		return domain.ErrForbidden
	}
	targetRow := findWorkspaceMember(members, targetUserID)
	if targetRow == nil {
		return domain.ErrMemberNotFound
	}
	if uc.hasOwnerRole(targetRow.RoleNames) && !uc.hasOwnerRole(callerRow.RoleNames) {
		return domain.ErrForbidden
	}
	if err := uc.idp.RemoveOrganizationMember(ctx, workspaceID, domain.UserSub(targetUserID)); err != nil {
		return fmt.Errorf("remove organization member: %w", err)
	}
	return nil
}

func findWorkspaceMember(members []domain.WorkspaceMember, userID string) *domain.WorkspaceMember {
	for i := range members {
		if members[i].UserID == userID {
			return &members[i]
		}
	}
	return nil
}

func (uc *RemoveWorkspaceMember) canManageMembers(roleNames []string) bool {
	for _, r := range roleNames {
		r = strings.TrimSpace(r)
		if strings.EqualFold(r, uc.ownerRoleName) || strings.EqualFold(r, string(domain.RoleAdmin)) {
			return true
		}
	}
	return false
}

func (uc *RemoveWorkspaceMember) hasOwnerRole(roleNames []string) bool {
	for _, r := range roleNames {
		if strings.EqualFold(strings.TrimSpace(r), uc.ownerRoleName) {
			return true
		}
	}
	return false
}
