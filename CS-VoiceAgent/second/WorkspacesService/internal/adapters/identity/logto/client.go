package logto

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cybrix-solutions/workspaces-service/internal/application/ports"
	"github.com/cybrix-solutions/workspaces-service/internal/domain"
)

type Client struct {
	cfg        Config
	httpClient *http.Client

	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 5 * time.Second
	}
	if cfg.TokenSafetyWindow <= 0 {
		cfg.TokenSafetyWindow = 30 * time.Second
	}
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("resource", c.cfg.ManagementResource)
	if c.cfg.Scope != "" {
		form.Set("scope", c.cfg.Scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	credentials := base64.StdEncoding.EncodeToString([]byte(c.cfg.ClientID + ":" + c.cfg.ClientSecret))
	req.Header.Set("Authorization", "Basic "+credentials)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint: status %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errors.New("token endpoint returned empty access token")
	}

	ttl := time.Duration(tr.ExpiresIn)*time.Second - c.cfg.TokenSafetyWindow
	if ttl < 0 {
		ttl = 0
	}
	c.cachedToken = tr.AccessToken
	c.tokenExpiry = time.Now().Add(ttl)
	return c.cachedToken, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = strings.NewReader(string(buf))
	}

	endpoint := strings.TrimRight(c.cfg.ManagementAPIBaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("management api request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return translateManagementAPIFailure(resp.StatusCode, respBytes, method, endpoint)
	}
	if out != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func translateManagementAPIFailure(status int, respBytes []byte, method, endpoint string) error {
	switch status {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s %s", domain.ErrWorkspaceNotFound, method, endpoint)
	case http.StatusConflict:
		return fmt.Errorf("%w: %s %s: %s", domain.ErrAlreadyExists, method, endpoint, string(respBytes))
	case http.StatusUnprocessableEntity:
		var le struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBytes, &le) == nil {
			switch le.Code {
			case "organization.role_names_not_found":
				return fmt.Errorf("%w: %s", domain.ErrOrganizationRolesMissing, strings.TrimSpace(le.Message))
			case "entity.duplicate_value", "organization_role.name_exists":
				return fmt.Errorf("%w: %s", domain.ErrAlreadyExists, strings.TrimSpace(le.Message))
			}
		}
	}
	return fmt.Errorf("%w: %s %s -> %d: %s", domain.ErrIdentityProvider, method, endpoint, status, string(respBytes))
}

type createOrgRequest struct {
	TenantID    string `json:"tenantId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type organizationDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (c *Client) CreateOrganization(ctx context.Context, in ports.CreateOrganizationInput) (ports.Organization, error) {
	var dto organizationDTO
	err := c.doJSON(ctx, http.MethodPost, "/api/organizations", createOrgRequest{
		TenantID:    c.cfg.TenantID,
		Name:        in.Name,
		Description: in.Description,
	}, &dto)
	if err != nil {
		return ports.Organization{}, err
	}
	return ports.Organization{ID: dto.ID, Name: dto.Name, Description: dto.Description}, nil
}

type addUsersRequest struct {
	UserIDs []string `json:"userIds"`
}

func (c *Client) AddOrganizationMember(ctx context.Context, organizationID string, userSub domain.UserSub) error {
	if organizationID == "" {
		return fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	path := "/api/organizations/" + url.PathEscape(organizationID) + "/users"
	return c.doJSON(ctx, http.MethodPost, path, addUsersRequest{UserIDs: []string{userSub.String()}}, nil)
}

type assignRoleRequest struct {
	OrganizationRoleNames []string `json:"organizationRoleNames"`
}

func (c *Client) AssignOrganizationRole(ctx context.Context, organizationID string, userSub domain.UserSub, role domain.OrganizationRole) error {
	if organizationID == "" {
		return fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	if err := role.Validate(); err != nil {
		return err
	}
	path := "/api/organizations/" + url.PathEscape(organizationID) +
		"/users/" + url.PathEscape(userSub.String()) + "/roles"
	return c.doJSON(ctx, http.MethodPost, path, assignRoleRequest{OrganizationRoleNames: []string{string(role)}}, nil)
}

func (c *Client) GetOrganization(ctx context.Context, organizationID string) (ports.Organization, error) {
	if organizationID == "" {
		return ports.Organization{}, fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	var dto organizationDTO
	path := "/api/organizations/" + url.PathEscape(organizationID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &dto); err != nil {
		return ports.Organization{}, err
	}
	return ports.Organization{ID: dto.ID, Name: dto.Name, Description: dto.Description}, nil
}

func (c *Client) ListUserOrganizations(ctx context.Context, userSub domain.UserSub) ([]ports.Organization, error) {
	if userSub == "" {
		return nil, fmt.Errorf("%w: empty user sub", domain.ErrIdentityProvider)
	}
	var dtos []organizationDTO
	path := "/api/users/" + url.PathEscape(userSub.String()) + "/organizations"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &dtos); err != nil {
		return nil, err
	}
	out := make([]ports.Organization, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, ports.Organization{ID: dto.ID, Name: dto.Name, Description: dto.Description})
	}
	return out, nil
}

type updateOrgRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (c *Client) UpdateOrganization(ctx context.Context, organizationID string, in ports.UpdateOrganizationInput) (ports.Organization, error) {
	if organizationID == "" {
		return ports.Organization{}, fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	if in.Name == nil && in.Description == nil {
		return ports.Organization{}, fmt.Errorf("%w: empty patch", domain.ErrIdentityProvider)
	}
	var dto organizationDTO
	path := "/api/organizations/" + url.PathEscape(organizationID)
	err := c.doJSON(ctx, http.MethodPatch, path, updateOrgRequest{
		Name:        in.Name,
		Description: in.Description,
	}, &dto)
	if err != nil {
		return ports.Organization{}, err
	}
	return ports.Organization{ID: dto.ID, Name: dto.Name, Description: dto.Description}, nil
}

type orgUserDTO struct {
	ID                string           `json:"id"`
	Username          string           `json:"username"`
	PrimaryEmail      string           `json:"primaryEmail"`
	Name              string           `json:"name"`
	Avatar            string           `json:"avatar"`
	OrganizationRoles []orgUserRoleDTO `json:"organizationRoles"`
	// Tolerate Logto API variants that return only role names.
	OrganizationRoleNames []string `json:"organizationRoleNames"`
}

type orgUserRoleDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) ListOrganizationMembers(ctx context.Context, organizationID string) ([]domain.WorkspaceMember, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	var dtos []orgUserDTO
	path := "/api/organizations/" + url.PathEscape(organizationID) + "/users"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &dtos); err != nil {
		return nil, err
	}
	out := make([]domain.WorkspaceMember, 0, len(dtos))
	for _, dto := range dtos {
		roles := make([]string, 0, len(dto.OrganizationRoles)+len(dto.OrganizationRoleNames))
		for _, r := range dto.OrganizationRoles {
			if r.Name != "" {
				roles = append(roles, r.Name)
			}
		}
		for _, name := range dto.OrganizationRoleNames {
			if name != "" {
				roles = append(roles, name)
			}
		}
		out = append(out, domain.WorkspaceMember{
			UserID:    dto.ID,
			Name:      dto.Name,
			Username:  dto.Username,
			Email:     dto.PrimaryEmail,
			Avatar:    dto.Avatar,
			RoleNames: roles,
		})
	}
	return out, nil
}

type orgRoleCatalogDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

func (c *Client) ListOrganizationRoleCatalog(ctx context.Context) ([]domain.OrganizationRoleInfo, error) {
	var dtos []orgRoleCatalogDTO
	if err := c.doJSON(ctx, http.MethodGet, "/api/organization-roles", nil, &dtos); err != nil {
		return nil, err
	}
	out := make([]domain.OrganizationRoleInfo, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, domain.OrganizationRoleInfo{
			ID:          dto.ID,
			Name:        dto.Name,
			Description: dto.Description,
			Type:        dto.Type,
		})
	}
	return out, nil
}

type createOrgRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (c *Client) CreateOrganizationRole(ctx context.Context, name, description string) (domain.OrganizationRoleInfo, error) {
	var dto orgRoleCatalogDTO
	err := c.doJSON(ctx, http.MethodPost, "/api/organization-roles", createOrgRoleRequest{
		Name:        name,
		Description: description,
	}, &dto)
	if err != nil {
		return domain.OrganizationRoleInfo{}, err
	}
	return domain.OrganizationRoleInfo{
		ID:          dto.ID,
		Name:        dto.Name,
		Description: dto.Description,
		Type:        dto.Type,
	}, nil
}

func (c *Client) RemoveOrganizationMember(ctx context.Context, organizationID string, userSub domain.UserSub) error {
	if organizationID == "" {
		return fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	if userSub == "" {
		return fmt.Errorf("%w: empty user sub", domain.ErrIdentityProvider)
	}
	// Logto: DELETE /api/organizations/{id}/users/{userId}
	path := "/api/organizations/" + url.PathEscape(organizationID) +
		"/users/" + url.PathEscape(userSub.String())
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) AssignOrganizationRoleByName(ctx context.Context, organizationID string, userSub domain.UserSub, roleName string) error {
	if organizationID == "" {
		return fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	if userSub == "" {
		return fmt.Errorf("%w: empty user sub", domain.ErrIdentityProvider)
	}
	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return fmt.Errorf("%w: empty role name", domain.ErrInvalidRole)
	}
	path := "/api/organizations/" + url.PathEscape(organizationID) +
		"/users/" + url.PathEscape(userSub.String()) + "/roles"
	return c.doJSON(ctx, http.MethodPost, path, assignRoleRequest{OrganizationRoleNames: []string{roleName}}, nil)
}

type userDTO struct {
	ID           string                     `json:"id"`
	Username     string                     `json:"username"`
	PrimaryEmail string                     `json:"primaryEmail"`
	Name         string                     `json:"name"`
	Identities   map[string]userIdentityDTO `json:"identities"`
}

type userIdentityDTO struct {
	Details map[string]interface{} `json:"details"`
}

func emailsFromIdentityDetails(details map[string]interface{}) []string {
	if details == nil {
		return nil
	}
	var out []string
	for _, key := range []string{"email", "primaryEmail", "primary_email"} {
		v, ok := details[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func collectIdentityEmails(identities map[string]userIdentityDTO) []string {
	if len(identities) == 0 {
		return nil
	}
	var out []string
	for _, id := range identities {
		out = append(out, emailsFromIdentityDetails(id.Details)...)
	}
	return out
}

func (c *Client) GetUser(ctx context.Context, userSub domain.UserSub) (ports.UserLookup, error) {
	if userSub == "" {
		return ports.UserLookup{}, fmt.Errorf("%w: empty user sub", domain.ErrIdentityProvider)
	}
	var dto userDTO
	path := "/api/users/" + url.PathEscape(userSub.String())
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &dto); err != nil {
		// Logto returns 404 when missing; translateManagementAPIFailure maps that to
		// ErrWorkspaceNotFound by default. Promote it to ErrUserNotFound for the user route.
		if errors.Is(err, domain.ErrWorkspaceNotFound) {
			return ports.UserLookup{}, fmt.Errorf("%w: %s", domain.ErrUserNotFound, userSub.String())
		}
		return ports.UserLookup{}, err
	}
	return ports.UserLookup{
		UserID:         dto.ID,
		Email:          dto.PrimaryEmail,
		Name:           dto.Name,
		Username:       dto.Username,
		IdentityEmails: collectIdentityEmails(dto.Identities),
	}, nil
}

// --- Organization invitations ---

type createInvitationRequest struct {
	OrganizationID      string   `json:"organizationId"`
	Invitee             string   `json:"invitee"`
	OrganizationRoleIDs []string `json:"organizationRoleIds,omitempty"`
	InviterID           string   `json:"inviterId,omitempty"`
	ExpiresAt           int64    `json:"expiresAt,omitempty"`
	// Logto: object with link/locale for immediate email, or false to skip email on create.
	// See https://openapi.logto.io/operation/operation-createorganizationinvitation
	MessagePayload any `json:"messagePayload"`
}

type invitationDTO struct {
	ID                string           `json:"id"`
	Invitee           string           `json:"invitee"`
	InviterID         string           `json:"inviterId"`
	OrganizationID    string           `json:"organizationId"`
	OrganizationName  string           `json:"organizationName"`
	Status            string           `json:"status"`
	ExpiresAt         int64            `json:"expiresAt"`
	CreatedAt         int64            `json:"createdAt"`
	UpdatedAt         int64            `json:"updatedAt"`
	OrganizationRoles []orgUserRoleDTO `json:"organizationRoles"`
}

func (d invitationDTO) toDomain() domain.Invitation {
	roles := make([]string, 0, len(d.OrganizationRoles))
	for _, r := range d.OrganizationRoles {
		if r.Name != "" {
			roles = append(roles, r.Name)
		}
	}
	return domain.Invitation{
		ID:               d.ID,
		OrganizationID:   d.OrganizationID,
		OrganizationName: d.OrganizationName,
		Invitee:          d.Invitee,
		Status:           domain.InvitationStatus(d.Status),
		RoleNames:        roles,
		InviterID:        d.InviterID,
		ExpiresAt:        epochMillis(d.ExpiresAt),
		CreatedAt:        epochMillis(d.CreatedAt),
		UpdatedAt:        epochMillis(d.UpdatedAt),
	}
}

func epochMillis(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

const defaultInvitationTTL = 7 * 24 * time.Hour

func (c *Client) CreateOrganizationInvitation(ctx context.Context, in ports.CreateInvitationInput) (domain.Invitation, error) {
	if in.OrganizationID == "" {
		return domain.Invitation{}, fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	invitee := strings.TrimSpace(in.Invitee)
	if invitee == "" {
		return domain.Invitation{}, fmt.Errorf("%w: empty invitee", domain.ErrInvalidEmail)
	}
	var msgPayload any = false
	if link := strings.TrimSpace(in.MessageLink); link != "" {
		msgPayload = map[string]interface{}{"link": link}
	}
	body := createInvitationRequest{
		OrganizationID:      in.OrganizationID,
		Invitee:             invitee,
		OrganizationRoleIDs: in.RoleIDs,
		InviterID:           strings.TrimSpace(in.InviterID),
		ExpiresAt:           time.Now().Add(defaultInvitationTTL).UnixMilli(),
		MessagePayload:      msgPayload,
	}
	var dto invitationDTO
	if err := c.doJSON(ctx, http.MethodPost, "/api/organization-invitations", body, &dto); err != nil {
		return domain.Invitation{}, err
	}
	return dto.toDomain(), nil
}

// invitationMessageBody matches Logto POST /api/organization-invitations/{id}/message:
// top-level "link", not nested under messagePayload.
// https://openapi.logto.io/operation/operation-createorganizationinvitationmessage
type invitationMessageBody struct {
	Link      string `json:"link"`
	Locale    string `json:"locale,omitempty"`
	UILocales string `json:"uiLocales,omitempty"`
}

func (c *Client) SendOrganizationInvitationMessage(ctx context.Context, invitationID, link string) error {
	if invitationID == "" {
		return fmt.Errorf("%w: empty invitation id", domain.ErrIdentityProvider)
	}
	link = strings.TrimSpace(link)
	if link == "" {
		return fmt.Errorf("%w: empty invitation link", domain.ErrIdentityProvider)
	}
	path := "/api/organization-invitations/" + url.PathEscape(invitationID) + "/message"
	body := invitationMessageBody{Link: link}
	if err := c.doJSON(ctx, http.MethodPost, path, body, nil); err != nil {
		if errors.Is(err, domain.ErrWorkspaceNotFound) {
			return fmt.Errorf("%w: %s", domain.ErrInvitationNotFound, invitationID)
		}
		return err
	}
	return nil
}

func (c *Client) GetOrganizationInvitation(ctx context.Context, invitationID string) (domain.Invitation, error) {
	if invitationID == "" {
		return domain.Invitation{}, fmt.Errorf("%w: empty invitation id", domain.ErrIdentityProvider)
	}
	var dto invitationDTO
	path := "/api/organization-invitations/" + url.PathEscape(invitationID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &dto); err != nil {
		if errors.Is(err, domain.ErrWorkspaceNotFound) {
			return domain.Invitation{}, fmt.Errorf("%w: %s", domain.ErrInvitationNotFound, invitationID)
		}
		return domain.Invitation{}, err
	}
	return dto.toDomain(), nil
}

func (c *Client) ListOrganizationInvitations(ctx context.Context, organizationID string) ([]domain.Invitation, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("%w: empty organization id", domain.ErrIdentityProvider)
	}
	endpoint := "/api/organization-invitations?" + url.Values{"organizationId": {organizationID}}.Encode()
	var dtos []invitationDTO
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &dtos); err != nil {
		return nil, err
	}
	out := make([]domain.Invitation, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, dto.toDomain())
	}
	return out, nil
}

type updateInvitationStatusRequest struct {
	Status         string `json:"status"`
	AcceptedUserID string `json:"acceptedUserId,omitempty"`
}

func (c *Client) UpdateOrganizationInvitationStatus(ctx context.Context, invitationID string, status domain.InvitationStatus, acceptedUserID string) (domain.Invitation, error) {
	if invitationID == "" {
		return domain.Invitation{}, fmt.Errorf("%w: empty invitation id", domain.ErrIdentityProvider)
	}
	body := updateInvitationStatusRequest{Status: string(status)}
	if status == domain.InvitationStatusAccepted {
		body.AcceptedUserID = strings.TrimSpace(acceptedUserID)
	}
	var dto invitationDTO
	path := "/api/organization-invitations/" + url.PathEscape(invitationID) + "/status"
	if err := c.doJSON(ctx, http.MethodPut, path, body, &dto); err != nil {
		if errors.Is(err, domain.ErrWorkspaceNotFound) {
			return domain.Invitation{}, fmt.Errorf("%w: %s", domain.ErrInvitationNotFound, invitationID)
		}
		return domain.Invitation{}, err
	}
	return dto.toDomain(), nil
}
