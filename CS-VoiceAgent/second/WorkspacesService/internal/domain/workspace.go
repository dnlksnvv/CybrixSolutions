package domain

import (
	"strings"
	"unicode/utf8"
)

const (
	workspaceNameMin = 2
	workspaceNameMax = 64
)

// UserSub is the Logto user identifier propagated by Traefik via X-User-Id.
type UserSub string

func (s UserSub) String() string { return string(s) }

// Workspace is a read-only projection of a Logto organization that this
// service treats as a workspace. Logto remains the single source of truth;
// no fields are persisted locally.
type Workspace struct {
	ID          string
	Name        string
	Description string
}

// CreateWorkspaceRequest is the validated input for creating a workspace.
type CreateWorkspaceRequest struct {
	Name        string
	Description string
	OwnerSub    UserSub
}

// ValidateCreateWorkspaceRequest enforces basic invariants before we touch Logto.
func ValidateCreateWorkspaceRequest(in CreateWorkspaceRequest) (CreateWorkspaceRequest, error) {
	name := strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(name) < workspaceNameMin || utf8.RuneCountInString(name) > workspaceNameMax {
		return CreateWorkspaceRequest{}, ErrInvalidName
	}
	if in.OwnerSub == "" {
		return CreateWorkspaceRequest{}, ErrInvalidOwner
	}
	return CreateWorkspaceRequest{
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		OwnerSub:    in.OwnerSub,
	}, nil
}

// UpdateWorkspaceRequest carries optional fields to patch on a workspace.
// Nil pointers mean "leave unchanged".
type UpdateWorkspaceRequest struct {
	Name        *string
	Description *string
}

// ValidateUpdateWorkspaceRequest normalizes the patch and rejects empty payloads.
func ValidateUpdateWorkspaceRequest(in UpdateWorkspaceRequest) (UpdateWorkspaceRequest, error) {
	out := UpdateWorkspaceRequest{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if utf8.RuneCountInString(name) < workspaceNameMin || utf8.RuneCountInString(name) > workspaceNameMax {
			return UpdateWorkspaceRequest{}, ErrInvalidName
		}
		out.Name = &name
	}
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		out.Description = &desc
	}
	if out.Name == nil && out.Description == nil {
		return UpdateWorkspaceRequest{}, ErrInvalidName
	}
	return out, nil
}

// WorkspaceMember is a workspace member with their organization roles.
type WorkspaceMember struct {
	UserID    string
	Name      string
	Username  string
	Email     string
	Avatar    string
	RoleNames []string
}

// PrimaryRole returns the first role name or "" when the member has none.
func (m WorkspaceMember) PrimaryRole() string {
	if len(m.RoleNames) == 0 {
		return ""
	}
	return m.RoleNames[0]
}

// OrganizationRoleInfo describes an organization role available in the tenant template.
type OrganizationRoleInfo struct {
	ID          string
	Name        string
	Description string
	Type        string // Logto returns "User" or "MachineToMachine".
}

const (
	memberInviteEmailMax  = 254
	memberInviteRoleMax   = 64
	roleNameMin           = 2
	roleNameMax           = 64
	roleDescriptionMaxLen = 256
)

// CreateOrganizationRoleRequest is the validated input for creating a tenant org role.
type CreateOrganizationRoleRequest struct {
	Name        string
	Description string
}

// ValidateCreateOrganizationRoleRequest checks name length and trims fields.
func ValidateCreateOrganizationRoleRequest(in CreateOrganizationRoleRequest) (CreateOrganizationRoleRequest, error) {
	name := strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(name) < roleNameMin || utf8.RuneCountInString(name) > roleNameMax {
		return CreateOrganizationRoleRequest{}, ErrInvalidRole
	}
	desc := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(desc) > roleDescriptionMaxLen {
		return CreateOrganizationRoleRequest{}, ErrInvalidRole
	}
	return CreateOrganizationRoleRequest{Name: name, Description: desc}, nil
}
