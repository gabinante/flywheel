package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/auth"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// OAuthHandler serves OAuth 2.1 discovery, authorize, and token endpoints for MCP clients (e.g. Cursor).
type OAuthHandler struct {
	BaseURL      string
	AuthConfig   auth.Config
	OAuthStore   *auth.OAuthStore
	Provisioner  *auth.Provisioner
	JWTSecret    string
	JWTExpirySec int
}

// RFC 9728 Protected Resource Metadata (PRM).
func (h *OAuthHandler) serveProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	resource := h.BaseURL + "/mcp"
	issuer := h.BaseURL
	meta := map[string]any{
		"resource":              resource,
		"authorization_servers":  []string{issuer},
		"scopes_supported":       []string{"flywheel:mcp"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

// RFC 8414 Authorization Server Metadata.
func (h *OAuthHandler) serveAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	issuer := h.BaseURL
	meta := map[string]any{
		"issuer":                            issuer,
		"authorization_endpoint":             issuer + "/oauth/authorize",
		"token_endpoint":                    issuer + "/oauth/token",
		"registration_endpoint":             issuer + "/oauth/register",
		"scopes_supported":                  []string{"flywheel:mcp"},
		"response_types_supported":          []string{"code"},
		"grant_types_supported":            []string{"authorization_code"},
		"code_challenge_methods_supported":  []string{"S256"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

func (h *OAuthHandler) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if v := recover(); v != nil {
			slog.Error("oauth/authorize panic", "error", v, "stack", string(debug.Stack()))
			if w != nil {
				WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "internal error", false))
			}
		}
	}()
	if r == nil || r.URL == nil {
		slog.Error("oauth/authorize: request or URL is nil")
		if w != nil {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "internal error", false))
		}
		return
	}
	if h == nil {
		slog.Error("oauth/authorize: receiver (OAuthHandler) is nil")
		if w != nil {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "internal error", false))
		}
		return
	}
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	clientState := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")

	if clientID == "" || redirectURI == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "missing client_id or redirect_uri", false))
		return
	}
	if h.OAuthStore == nil {
		slog.Error("oauth/authorize: OAuthStore is nil")
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "internal error", false))
		return
	}

	stateData := auth.OAuthState{
		RedirectURI:   redirectURI,
		ClientID:      clientID,
		CodeChallenge: codeChallenge,
		ClientState:   clientState,
	}
	// Use background context so Redis write isn't cancelled if the client closes the request (e.g. redirect race).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state, err := h.OAuthStore.CreateState(ctx, stateData)
	if err != nil {
		slog.Error("oauth/authorize CreateState failed", "error", err)
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "internal error", false))
		return
	}

	cfg := h.AuthConfig.OAuth2()
	if cfg == nil || cfg.Endpoint.AuthURL == "" {
		slog.Error("oauth/authorize: OAuth2 config or AuthURL missing")
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "internal error", false))
		return
	}
	// Build GitHub auth URL ourselves to avoid nil receiver in oauth2.Config.AuthCodeURL.
	v := url.Values{
		"response_type": {"code"},
		"client_id":     {cfg.ClientID},
		"state":         {state},
	}
	if cfg.RedirectURL != "" {
		v.Set("redirect_uri", cfg.RedirectURL)
	}
	if len(cfg.Scopes) > 0 {
		v.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	authURL := cfg.Endpoint.AuthURL
	if strings.Contains(authURL, "?") {
		authURL += "&" + v.Encode()
	} else {
		authURL += "?" + v.Encode()
	}
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) oauthToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid form", false))
		return
	}
	grantType := r.FormValue("grant_type")
	if grantType != "authorization_code" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "unsupported grant_type", false))
		return
	}
	code := r.FormValue("code")
	if code == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "missing code", false))
		return
	}

	agentID, err := h.OAuthStore.GetAndDeleteCode(r.Context(), code)
	if err != nil || agentID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid or expired code", false))
		return
	}

	// Optional: validate redirect_uri and client_id match the code's session; for now we accept any.
	expiry := auth.ExpiryFromSeconds(h.JWTExpirySec)
	token, err := auth.IssueJWT(h.JWTSecret, agentID, expiry)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "token error", false))
		return
	}

	expiresIn := h.JWTExpirySec
	if expiresIn <= 0 {
		expiresIn = 604800 // 7 days
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
	})
}

// dcrRequest is the minimal RFC 7591 Dynamic Client Registration request body.
type dcrRequest struct {
	RedirectURIs []string `json:"redirect_uris"`
	ClientName   string   `json:"client_name,omitempty"`
}

// oauthRegister implements RFC 7591 Dynamic Client Registration so Cursor can obtain a client_id.
func (h *OAuthHandler) oauthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "Content-Type: application/json required", false))
		return
	}
	var req dcrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid JSON", false))
		return
	}
	if len(req.RedirectURIs) == 0 {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "redirect_uris required", false))
		return
	}
	clientID, err := h.OAuthStore.RegisterClient(r.Context(), req.RedirectURIs)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"client_id":           clientID,
		"redirect_uris":       req.RedirectURIs,
		"client_id_issued_at": time.Now().Unix(),
	})
}
