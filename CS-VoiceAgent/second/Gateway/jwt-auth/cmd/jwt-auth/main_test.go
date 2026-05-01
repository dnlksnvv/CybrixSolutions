package main

import (
	"net/http"
	"testing"
)

func TestIsInviteeInvitationPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/invitations/142u5e8xnyisup3y1y7yn", true},
		{"/invitations/142u5e8xnyisup3y1y7yn/accept", true},
		{"/invitations/", false},
		{"/invitations", false},
		{"/workspaces/x/invitations", false},
		{"/workspaces/x/invitations/y", false},
		{"/invitations/id/extra/bad", false},
	}
	for _, tt := range tests {
		if got := isInviteeInvitationPath(tt.path); got != tt.want {
			t.Errorf("isInviteeInvitationPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestEnforceAPIScopes_InviteePathSkips(t *testing.T) {
	req := httptestNewRequestWithHeader("/auth/verify", map[string]string{
		"X-Forwarded-Uri": "/invitations/abc123",
	})
	required := map[string]struct{}{"api:read": {}}
	if enforceAPIScopes(req, required) {
		t.Fatal("expected invitee path to skip API scope enforcement")
	}
}

func TestEnforceAPIScopes_WorkspacesStillEnforced(t *testing.T) {
	req := httptestNewRequestWithHeader("/auth/verify", map[string]string{
		"X-Forwarded-Uri": "/workspaces/ws1/invitations",
	})
	required := map[string]struct{}{"api:read": {}}
	if !enforceAPIScopes(req, required) {
		t.Fatal("expected workspace invitation admin path to enforce scopes")
	}
}

func TestEnforceAPIScopes_NoHeaderEnforces(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/auth/verify", nil)
	required := map[string]struct{}{"api:read": {}}
	if !enforceAPIScopes(req, required) {
		t.Fatal("expected missing forwarded path to enforce scopes")
	}
}

func httptestNewRequestWithHeader(path string, hdr map[string]string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}
