package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cybrix-solutions/workspaces-service/internal/application/usecase"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

// InvitationHandler exposes invitation-related routes split between two URL
// trees:
//   - /workspaces/{id}/invitations*  — admin-style flows (invite, list, revoke, resend)
//   - /invitations/{id}*             — invitee-facing flows (view, accept)
type InvitationHandler struct {
	inviteByEmail    *usecase.InviteByEmail
	listInvitations  *usecase.ListInvitations
	revokeInvitation *usecase.RevokeInvitation
	resendInvitation *usecase.ResendInvitation
	getInvitation    *usecase.GetInvitation
	acceptInvitation *usecase.AcceptInvitation
}

type InvitationHandlerConfig struct {
	Invite *usecase.InviteByEmail
	List   *usecase.ListInvitations
	Revoke *usecase.RevokeInvitation
	Resend *usecase.ResendInvitation
	Get    *usecase.GetInvitation
	Accept *usecase.AcceptInvitation
}

func NewInvitationHandler(cfg InvitationHandlerConfig) *InvitationHandler {
	return &InvitationHandler{
		inviteByEmail:    cfg.Invite,
		listInvitations:  cfg.List,
		revokeInvitation: cfg.Revoke,
		resendInvitation: cfg.Resend,
		getInvitation:    cfg.Get,
		acceptInvitation: cfg.Accept,
	}
}

type invitationResponse struct {
	ID               string   `json:"id"`
	OrganizationID   string   `json:"organizationId"`
	OrganizationName string   `json:"organizationName,omitempty"`
	Email            string   `json:"email"`
	Status           string   `json:"status"`
	Role             string   `json:"role,omitempty"`
	Roles            []string `json:"roles,omitempty"`
	InviterID        string   `json:"inviterId,omitempty"`
	ExpiresAt        string   `json:"expiresAt,omitempty"`
	CreatedAt        string   `json:"createdAt,omitempty"`
}

type listInvitationsResponse struct {
	Items []invitationResponse `json:"items"`
}

func toInvitationResponse(inv domain.Invitation) invitationResponse {
	resp := invitationResponse{
		ID:               inv.ID,
		OrganizationID:   inv.OrganizationID,
		OrganizationName: inv.OrganizationName,
		Email:            inv.Invitee,
		Status:           string(inv.Status),
		Role:             inv.PrimaryRole(),
		Roles:            inv.RoleNames,
		InviterID:        inv.InviterID,
	}
	if !inv.ExpiresAt.IsZero() {
		resp.ExpiresAt = inv.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if !inv.CreatedAt.IsZero() {
		resp.CreatedAt = inv.CreatedAt.UTC().Format(time.RFC3339)
	}
	return resp
}

type createInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// CreateForWorkspace handles POST /workspaces/{id}/invitations.
func (h *InvitationHandler) CreateForWorkspace(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	workspaceID := chi.URLParam(r, "id")

	var req createInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	inv, err := h.inviteByEmail.Execute(r.Context(), caller, workspaceID, domain.CreateInvitationRequest{
		Email:    req.Email,
		RoleName: req.Role,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toInvitationResponse(inv))
}

// ListForWorkspace handles GET /workspaces/{id}/invitations.
func (h *InvitationHandler) ListForWorkspace(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	workspaceID := chi.URLParam(r, "id")
	items, err := h.listInvitations.Execute(r.Context(), caller, workspaceID)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	resp := listInvitationsResponse{Items: make([]invitationResponse, 0, len(items))}
	for _, inv := range items {
		resp.Items = append(resp.Items, toInvitationResponse(inv))
	}
	writeJSON(w, http.StatusOK, resp)
}

// RevokeForWorkspace handles DELETE /workspaces/{id}/invitations/{invitationId}.
func (h *InvitationHandler) RevokeForWorkspace(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	workspaceID := chi.URLParam(r, "id")
	invitationID := chi.URLParam(r, "invitationId")
	if err := h.revokeInvitation.Execute(r.Context(), caller, workspaceID, invitationID); err != nil {
		writeUseCaseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ResendForWorkspace handles POST /workspaces/{id}/invitations/{invitationId}/resend.
func (h *InvitationHandler) ResendForWorkspace(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	workspaceID := chi.URLParam(r, "id")
	invitationID := chi.URLParam(r, "invitationId")
	if err := h.resendInvitation.Execute(r.Context(), caller, workspaceID, invitationID); err != nil {
		writeUseCaseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Get handles GET /invitations/{id} — invitee-facing landing page lookup.
func (h *InvitationHandler) Get(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	id := chi.URLParam(r, "id")
	inv, err := h.getInvitation.Execute(r.Context(), caller, id)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInvitationResponse(inv))
}

// Accept handles POST /invitations/{id}/accept.
func (h *InvitationHandler) Accept(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	id := chi.URLParam(r, "id")
	inv, err := h.acceptInvitation.Execute(r.Context(), caller, id)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInvitationResponse(inv))
}
