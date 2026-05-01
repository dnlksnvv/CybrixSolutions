package usecase

import (
	"strings"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
)

// userMatchesInvitee returns true if the Logto user should be treated as the
// invitation recipient. Logto may leave primaryEmail empty while the address
// still appears on username (email-as-username) or under identities.*.details.
func userMatchesInvitee(user ports.UserLookup, invitee string) bool {
	if equalEmail(user.Email, invitee) {
		return true
	}
	u := strings.TrimSpace(user.Username)
	if strings.Contains(u, "@") && equalEmail(u, invitee) {
		return true
	}
	for _, e := range user.IdentityEmails {
		if equalEmail(e, invitee) {
			return true
		}
	}
	return false
}

// equalEmail compares two email addresses case-insensitively after trimming.
func equalEmail(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
