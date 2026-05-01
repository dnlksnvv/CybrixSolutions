package usecase

import (
	"testing"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
)

func TestUserMatchesInvitee(t *testing.T) {
	inv := "alice@example.com"
	t.Run("primary", func(t *testing.T) {
		if !userMatchesInvitee(ports.UserLookup{Email: "Alice@example.com"}, inv) {
			t.Fatal("expected primary email match")
		}
	})
	t.Run("username_when_looks_like_email", func(t *testing.T) {
		if !userMatchesInvitee(ports.UserLookup{Email: "", Username: "alice@example.com"}, inv) {
			t.Fatal("expected username match when primary empty")
		}
	})
	t.Run("identity_details", func(t *testing.T) {
		if !userMatchesInvitee(ports.UserLookup{
			Email:          "",
			IdentityEmails: []string{"other@x.com", "alice@example.com"},
		}, inv) {
			t.Fatal("expected identity email match")
		}
	})
	t.Run("no_match", func(t *testing.T) {
		if userMatchesInvitee(ports.UserLookup{Email: "bob@example.com"}, inv) {
			t.Fatal("expected no match")
		}
	})
}
