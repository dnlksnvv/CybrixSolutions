package ports

import (
	"context"

	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// CreateOrganizationInput carries the data needed to provision an organization
// in the identity provider.
type CreateOrganizationInput struct {
	Name        string
	Description string
}

// UpdateOrganizationInput patches mutable fields. Nil means "leave unchanged".
type UpdateOrganizationInput struct {
	Name        *string
	Description *string
}

// Organization is the IdP-side representation we care about.
type Organization struct {
	ID          string
	Name        string
	Description string
}

// UserLookup is what we need from a Logto user record.
type UserLookup struct {
	UserID string
	Email  string // primaryEmail; may be empty while invitee matches username/identities
	Name   string
	// Username is the Logto username (often the same string as email for email sign-up).
	Username string
	// IdentityEmails collects emails found under identities.*.details (e.g. email connector).
	IdentityEmails []string
}

// CreateInvitationInput is what the IdP needs to provision an invitation.
// MessageLink, when non-empty, is what the email template's `{{link}}` becomes.
type CreateInvitationInput struct {
	OrganizationID string
	Invitee        string
	RoleIDs        []string
	InviterID      string
	MessageLink    string
}

// IdentityProvider abstracts the IAM (Logto) so the use cases stay independent
// of the concrete vendor and protocol.
type IdentityProvider interface {
	CreateOrganization(ctx context.Context, in CreateOrganizationInput) (Organization, error)
	UpdateOrganization(ctx context.Context, organizationID string, in UpdateOrganizationInput) (Organization, error)
	AddOrganizationMember(ctx context.Context, organizationID string, userSub domain.UserSub) error
	RemoveOrganizationMember(ctx context.Context, organizationID string, userSub domain.UserSub) error
	AssignOrganizationRole(ctx context.Context, organizationID string, userSub domain.UserSub, role domain.OrganizationRole) error
	AssignOrganizationRoleByName(ctx context.Context, organizationID string, userSub domain.UserSub, roleName string) error

	GetOrganization(ctx context.Context, organizationID string) (Organization, error)
	ListUserOrganizations(ctx context.Context, userSub domain.UserSub) ([]Organization, error)

	// ListOrganizationMembers returns members of an organization with their organization roles.
	ListOrganizationMembers(ctx context.Context, organizationID string) ([]domain.WorkspaceMember, error)
	// ListOrganizationRoleCatalog returns the roles defined in the tenant's organization template.
	ListOrganizationRoleCatalog(ctx context.Context) ([]domain.OrganizationRoleInfo, error)
	// CreateOrganizationRole adds a new role to the tenant organization template.
	CreateOrganizationRole(ctx context.Context, name, description string) (domain.OrganizationRoleInfo, error)

	// GetUser fetches a user record by sub (id). Returns domain.ErrUserNotFound if missing.
	GetUser(ctx context.Context, userSub domain.UserSub) (UserLookup, error)

	// CreateOrganizationInvitation creates a Pending invitation and (when MessageLink
	// is set) requests Logto to send the invitation email via its email connector.
	CreateOrganizationInvitation(ctx context.Context, in CreateInvitationInput) (domain.Invitation, error)
	// SendOrganizationInvitationMessage (re-)sends the invitation email; the link
	// substitutes Logto's email template `{{link}}` variable.
	SendOrganizationInvitationMessage(ctx context.Context, invitationID, link string) error
	// GetOrganizationInvitation returns a single invitation by id.
	GetOrganizationInvitation(ctx context.Context, invitationID string) (domain.Invitation, error)
	// ListOrganizationInvitations lists invitations scoped to an organization.
	ListOrganizationInvitations(ctx context.Context, organizationID string) ([]domain.Invitation, error)
	// UpdateOrganizationInvitationStatus transitions an invitation to Accepted/Revoked/Expired.
	// acceptedUserID must be set only when status == Accepted.
	UpdateOrganizationInvitationStatus(ctx context.Context, invitationID string, status domain.InvitationStatus, acceptedUserID string) (domain.Invitation, error)
}
