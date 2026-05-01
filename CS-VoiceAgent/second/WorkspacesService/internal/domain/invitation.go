package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// InvitationStatus mirrors Logto's organization invitation status values.
type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "Pending"
	InvitationStatusAccepted InvitationStatus = "Accepted"
	InvitationStatusRevoked  InvitationStatus = "Revoked"
	InvitationStatusExpired  InvitationStatus = "Expired"
)

// Invitation is the workspace-level projection of a Logto organization invitation.
// Only fields the frontend actually renders are kept here; the rest is opaque.
type Invitation struct {
	ID               string
	OrganizationID   string
	OrganizationName string
	Invitee          string
	Status           InvitationStatus
	RoleNames        []string
	InviterID        string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PrimaryRole returns the first role assigned to the invitation or "" when none.
func (i Invitation) PrimaryRole() string {
	if len(i.RoleNames) == 0 {
		return ""
	}
	return i.RoleNames[0]
}

// CreateInvitationRequest carries validated input for creating a workspace invitation.
type CreateInvitationRequest struct {
	Email    string
	RoleName string
}

// ValidateCreateInvitationRequest reuses the same constraints as direct invites:
// non-empty email containing "@" and a non-empty role name within size limits.
func ValidateCreateInvitationRequest(in CreateInvitationRequest) (CreateInvitationRequest, error) {
	email := strings.TrimSpace(in.Email)
	if email == "" || !strings.Contains(email, "@") || utf8.RuneCountInString(email) > memberInviteEmailMax {
		return CreateInvitationRequest{}, ErrInvalidEmail
	}
	role := strings.TrimSpace(in.RoleName)
	if role == "" || utf8.RuneCountInString(role) > memberInviteRoleMax {
		return CreateInvitationRequest{}, ErrInvalidRole
	}
	return CreateInvitationRequest{Email: email, RoleName: role}, nil
}
