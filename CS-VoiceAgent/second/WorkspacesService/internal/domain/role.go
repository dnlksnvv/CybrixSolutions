package domain

import "fmt"

type OrganizationRole string

const (
	RoleOwner  OrganizationRole = "Owner"
	RoleAdmin  OrganizationRole = "Admin"
	RoleMember OrganizationRole = "Member"
)

func (r OrganizationRole) Validate() error {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidRole, string(r))
	}
}
