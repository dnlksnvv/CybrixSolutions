package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cybrix-solutions/workspaces-service/internal/application/usecase"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

type WorkspaceHandler struct {
	createWorkspace       *usecase.CreateWorkspace
	listMyWorkspaces      *usecase.ListMyWorkspaces
	getWorkspace          *usecase.GetWorkspace
	updateWorkspace       *usecase.UpdateWorkspace
	listWorkspaceMembers  *usecase.ListWorkspaceMembers
	listWorkspaceRoles    *usecase.ListWorkspaceRoles
	leaveWorkspace        *usecase.LeaveWorkspace
	removeWorkspaceMember *usecase.RemoveWorkspaceMember
	createWorkspaceRole   *usecase.CreateWorkspaceRole
	inviteByEmail         *usecase.InviteByEmail
	listInvitations       *usecase.ListInvitations
	revokeInvitation      *usecase.RevokeInvitation
	resendInvitation      *usecase.ResendInvitation
}

type WorkspaceHandlerConfig struct {
	Create           *usecase.CreateWorkspace
	List             *usecase.ListMyWorkspaces
	Get              *usecase.GetWorkspace
	Update           *usecase.UpdateWorkspace
	Members          *usecase.ListWorkspaceMembers
	Roles            *usecase.ListWorkspaceRoles
	Leave            *usecase.LeaveWorkspace
	RemoveMember     *usecase.RemoveWorkspaceMember
	CreateRole       *usecase.CreateWorkspaceRole
	InviteByEmail    *usecase.InviteByEmail
	ListInvitations  *usecase.ListInvitations
	RevokeInvitation *usecase.RevokeInvitation
	ResendInvitation *usecase.ResendInvitation
}

func NewWorkspaceHandler(cfg WorkspaceHandlerConfig) *WorkspaceHandler {
	return &WorkspaceHandler{
		createWorkspace:       cfg.Create,
		listMyWorkspaces:      cfg.List,
		getWorkspace:          cfg.Get,
		updateWorkspace:       cfg.Update,
		listWorkspaceMembers:  cfg.Members,
		listWorkspaceRoles:    cfg.Roles,
		leaveWorkspace:        cfg.Leave,
		removeWorkspaceMember: cfg.RemoveMember,
		createWorkspaceRole:   cfg.CreateRole,
		inviteByEmail:         cfg.InviteByEmail,
		listInvitations:       cfg.ListInvitations,
		revokeInvitation:      cfg.RevokeInvitation,
		resendInvitation:      cfg.ResendInvitation,
	}
}

type createWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type workspaceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type listWorkspacesResponse struct {
	Items []workspaceResponse `json:"items"`
}

func toResponse(w domain.Workspace) workspaceResponse {
	return workspaceResponse{
		ID:          w.ID,
		Name:        w.Name,
		Description: w.Description,
	}
}

func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	owner, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}

	var req createWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	out, err := h.createWorkspace.Execute(r.Context(), domain.CreateWorkspaceRequest{
		Name:        req.Name,
		Description: req.Description,
		OwnerSub:    owner,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toResponse(out))
}

func (h *WorkspaceHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	owner, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}

	items, err := h.listMyWorkspaces.Execute(r.Context(), owner)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	resp := listWorkspacesResponse{Items: make([]workspaceResponse, 0, len(items))}
	for _, it := range items {
		resp.Items = append(resp.Items, toResponse(it))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *WorkspaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	owner, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}

	id := chi.URLParam(r, "id")
	out, err := h.getWorkspace.Execute(r.Context(), owner, id)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(out))
}

type updateWorkspaceRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	owner, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}

	id := chi.URLParam(r, "id")
	var req updateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	out, err := h.updateWorkspace.Execute(r.Context(), owner, id, domain.UpdateWorkspaceRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(out))
}

type memberResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Username      string `json:"username,omitempty"`
	Email         string `json:"email,omitempty"`
	Avatar        string `json:"avatar,omitempty"`
	Role          string `json:"role,omitempty"`
	IsCurrentUser bool   `json:"isCurrentUser"`
}

type listMembersResponse struct {
	Items []memberResponse `json:"items"`
}

func toMemberResponse(m domain.WorkspaceMember, callerID string) memberResponse {
	return memberResponse{
		ID:            m.UserID,
		Name:          m.Name,
		Username:      m.Username,
		Email:         m.Email,
		Avatar:        m.Avatar,
		Role:          m.PrimaryRole(),
		IsCurrentUser: m.UserID == callerID,
	}
}

func (h *WorkspaceHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	owner, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}

	id := chi.URLParam(r, "id")
	members, err := h.listWorkspaceMembers.Execute(r.Context(), owner, id)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	callerID := owner.String()
	resp := listMembersResponse{Items: make([]memberResponse, 0, len(members))}
	for _, m := range members {
		resp.Items = append(resp.Items, toMemberResponse(m, callerID))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *WorkspaceHandler) LeaveWorkspace(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.leaveWorkspace.Execute(r.Context(), caller, id); err != nil {
		writeUseCaseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember handles DELETE /workspaces/{id}/members/{userId} — remove another member (Owner/Admin only).
func (h *WorkspaceHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	workspaceID := chi.URLParam(r, "id")
	targetID := chi.URLParam(r, "userId")
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "missing member user id")
		return
	}
	if err := h.removeWorkspaceMember.Execute(r.Context(), caller, workspaceID, targetID); err != nil {
		writeUseCaseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type roleResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

type listRolesResponse struct {
	Items []roleResponse `json:"items"`
}

func toRoleResponse(role domain.OrganizationRoleInfo) roleResponse {
	return roleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Type:        role.Type,
	}
}

func (h *WorkspaceHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	owner, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}

	id := chi.URLParam(r, "id")
	roles, err := h.listWorkspaceRoles.Execute(r.Context(), owner, id)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	resp := listRolesResponse{Items: make([]roleResponse, 0, len(roles))}
	for _, role := range roles {
		resp.Items = append(resp.Items, toRoleResponse(role))
	}
	writeJSON(w, http.StatusOK, resp)
}

type createWorkspaceRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *WorkspaceHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	caller, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user identity")
		return
	}

	id := chi.URLParam(r, "id")
	var req createWorkspaceRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	role, err := h.createWorkspaceRole.Execute(r.Context(), caller, id, domain.CreateOrganizationRoleRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRoleResponse(role))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeUseCaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrWorkspaceNotFound):
		writeError(w, http.StatusNotFound, "workspace not found")
	case errors.Is(err, domain.ErrInvitationNotFound):
		writeError(w, http.StatusNotFound, "invitation not found")
	case errors.Is(err, domain.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, domain.ErrInvalidName),
		errors.Is(err, domain.ErrInvalidOwner),
		errors.Is(err, domain.ErrInvalidRole),
		errors.Is(err, domain.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrOrganizationRolesMissing),
		errors.Is(err, domain.ErrInvitationNotActive):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrInvitationNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "set INVITATION_LINK_BASE_URL to enable email invitations")
	case errors.Is(err, domain.ErrEmailMismatch):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrMemberNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrCannotRemoveYourself):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrIdentityProvider):
		writeError(w, http.StatusBadGateway, "identity provider error")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
