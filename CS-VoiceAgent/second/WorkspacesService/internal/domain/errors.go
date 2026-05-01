package domain

import "errors"

var (
	ErrInvalidName         = errors.New("invalid workspace name")
	ErrInvalidOwner        = errors.New("invalid owner")
	ErrInvalidRole         = errors.New("invalid organization role")
	ErrInvalidEmail        = errors.New("invalid email")
	ErrUserNotFound        = errors.New("user not found")
	ErrWorkspaceNotFound   = errors.New("workspace not found")
	ErrAlreadyExists       = errors.New("already exists")
	ErrInvitationNotFound  = errors.New("invitation not found")
	ErrInvitationNotActive = errors.New("invitation is not pending")
	// ErrInvitationNotConfigured means INVITATION_LINK_BASE_URL is unset; invite/resend are disabled.
	ErrInvitationNotConfigured = errors.New("invitation flow not configured")
	ErrEmailMismatch           = errors.New("email does not match invitation")
	// ErrOrganizationRolesMissing means Logto rejected organization role names
	// (e.g. template role not created yet). Wrap with fmt.Errorf("%w: ...", ...)
	// to attach Logto's message.
	ErrOrganizationRolesMissing = errors.New("organization roles missing in Logto")
	ErrIdentityProvider         = errors.New("identity provider error")
	// ErrForbidden is returned when the caller may not perform an action (e.g. kick without role).
	ErrForbidden = errors.New("forbidden")
	// ErrMemberNotFound means the target user is not a member of the workspace.
	ErrMemberNotFound = errors.New("member not found in workspace")
	// ErrCannotRemoveYourself tells clients to use DELETE .../members/me instead.
	ErrCannotRemoveYourself = errors.New("remove yourself via DELETE /workspaces/{id}/members/me")
)
