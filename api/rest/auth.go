package rest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gabinante/flywheel/internal/auth"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
)

// AuthHandler handles GitHub OAuth and token response.
type AuthHandler struct {
	AuthConfig  auth.Config
	Provisioner *auth.Provisioner
	OAuthStore  *auth.OAuthStore // optional; when set, callback can redirect to client redirect_uri with code
	OrgSvc      *org.Service     // optional; when set, new users get a default org named after their email/login
	JWTSecret   string
	JWTExpiry   time.Duration
	// stateCache stores pending state -> redirect_uri for OAuth callback (redirect_uri only for localhost callbacks, e.g. TUI)
	stateCache map[string]string
	stateMu    sync.Mutex
}

// NewAuthHandler creates an auth handler. JWTExpiry 0 = 7 days. OAuthStore optional for MCP OAuth code flow. OrgSvc optional for default org on sign-up.
func NewAuthHandler(authConfig auth.Config, provisioner *auth.Provisioner, oauthStore *auth.OAuthStore, orgSvc *org.Service, jwtSecret string, jwtExpiry time.Duration) *AuthHandler {
	if jwtExpiry == 0 {
		jwtExpiry = 7 * 24 * time.Hour
	}
	return &AuthHandler{
		AuthConfig:  authConfig,
		Provisioner: provisioner,
		OAuthStore:  oauthStore,
		OrgSvc:     orgSvc,
		JWTSecret:   jwtSecret,
		JWTExpiry:   jwtExpiry,
		stateCache:  make(map[string]string),
	}
}

func (h *AuthHandler) githubRedirect(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if v := recover(); v != nil {
			slog.Error("auth panic in githubRedirect", "error", v, "stack", string(debug.Stack()))
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("auth panic: " + fmt.Sprint(v)))
			return
		}
	}()
	if h.AuthConfig.BaseURL == "" {
		slog.Error("auth: BASE_URL not set; cannot build OAuth redirect")
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "auth misconfigured: BASE_URL required", false))
		return
	}
	if h.AuthConfig.DevBypass {
		h.devLogin(w, r)
		return
	}
	if h.AuthConfig.ClientID == "" {
		slog.Error("auth: GITHUB_CLIENT_ID not set")
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "auth misconfigured: GITHUB_CLIENT_ID required", false))
		return
	}
	state, err := randomState()
	if err != nil {
		slog.Error("auth random state failed", "error", err)
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "random state failed", false))
		return
	}
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI != "" && isSafeRedirectURI(redirectURI) {
		h.stateMu.Lock()
		h.stateCache[state] = redirectURI
		h.stateMu.Unlock()
	}
	// Build GitHub auth URL here so we never depend on oauth2.Config (avoids nil deref in some versions).
	authURL := buildGitHubAuthURL(h.AuthConfig.BaseURL, h.AuthConfig.RedirectPath, h.AuthConfig.ClientID, state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// buildGitHubAuthURL returns the GitHub OAuth authorize URL. Kept in rest to avoid oauth2 library panics.
const githubAuthURL = "https://github.com/login/oauth/authorize"

func buildGitHubAuthURL(baseURL, redirectPath, clientID, state string) string {
	v := url.Values{
		"response_type": {"code"},
		"client_id":     {clientID},
		"redirect_uri":  {baseURL + redirectPath},
		"scope":         {"read:user user:email"},
		"state":         {state},
	}
	return githubAuthURL + "?" + v.Encode()
}

// isSafeRedirectURI allows only localhost redirects (e.g. TUI callback).
func isSafeRedirectURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

// devLogin signs in a fixed local user without contacting GitHub. Only
// reachable when AUTH_DEV_BYPASS is set; never enable outside local dev.
func (h *AuthHandler) devLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, agent, err := h.Provisioner.Provision(ctx, &auth.GitHubUser{
		ID:    -1,
		Login: "dev",
		Name:  "Local Dev",
		Email: "dev@localhost",
	})
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "provision failed: "+err.Error(), false))
		return
	}
	if h.OrgSvc != nil {
		if err := h.OrgSvc.EnsureDefaultOrgForUser(ctx, u.ID, u.Email); err != nil {
			slog.Error("auth ensure default org failed", "user", u.ID, "error", err)
		}
	}
	jwtStr, err := auth.IssueJWT(h.JWTSecret, agent.ID, h.JWTExpiry)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "jwt failed", false))
		return
	}
	// TUI/local app parity with githubCallback: honor ?redirect_uri=... for localhost callbacks.
	if redirectURI := r.URL.Query().Get("redirect_uri"); redirectURI != "" && isSafeRedirectURI(redirectURI) {
		target, _ := url.Parse(redirectURI)
		q := target.Query()
		q.Set("token", jwtStr)
		target.RawQuery = q.Encode()
		http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
		return
	}
	if h.AuthConfig.SuccessRedirectURL != "" && redirectWithJWTFragment(w, r, h.AuthConfig.SuccessRedirectURL, jwtStr) {
		return
	}
	if redirectWithJWTFragment(w, r, h.AuthConfig.BaseURL+"/", jwtStr) {
		return
	}
	WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "auth misconfigured: BASE_URL required", false))
}

func (h *AuthHandler) githubCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "missing code", false))
		return
	}
	stateParam := r.URL.Query().Get("state")
	ctx := r.Context()
	token, err := h.AuthConfig.ExchangeCode(ctx, code)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "exchange failed: "+err.Error(), false))
		return
	}
	ghUser, err := auth.FetchUser(ctx, token)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "fetch user failed: "+err.Error(), false))
		return
	}
	u, agent, err := h.Provisioner.Provision(ctx, ghUser)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "provision failed: "+err.Error(), false))
		return
	}
	// New users get a default org named after their email (or login) so list_projects returns something without creating a collaboration org.
	if h.OrgSvc != nil {
		displayName := u.Email
		if displayName == "" {
			displayName = u.Login
		}
		if err := h.OrgSvc.EnsureDefaultOrgForUser(ctx, u.ID, displayName); err != nil {
			slog.Error("auth ensure default org failed", "user", u.ID, "error", err)
		}
	}
	// If this callback was started from /oauth/authorize (MCP client), redirect back with one-time code.
	if h.OAuthStore != nil && stateParam != "" {
		oauthState, err := h.OAuthStore.GetAndDeleteState(ctx, stateParam)
		if err == nil && oauthState != nil && oauthState.RedirectURI != "" {
			oneTimeCode, err := h.OAuthStore.CreateCode(ctx, agent.ID)
			if err != nil {
				WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "internal error", false))
				return
			}
			u, _ := url.Parse(oauthState.RedirectURI)
			q := u.Query()
			q.Set("code", oneTimeCode)
			if oauthState.ClientState != "" {
				q.Set("state", oauthState.ClientState)
			}
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
			return
		}
	}
	jwtStr, err := auth.IssueJWT(h.JWTSecret, agent.ID, h.JWTExpiry)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "jwt failed", false))
		return
	}
	// TUI/local app: redirect_uri was stored in stateCache when user hit /auth/github?redirect_uri=...
	if stateParam != "" {
		h.stateMu.Lock()
		redirectURI, ok := h.stateCache[stateParam]
		if ok {
			delete(h.stateCache, stateParam)
		}
		h.stateMu.Unlock()
		if ok && redirectURI != "" && isSafeRedirectURI(redirectURI) {
			u, _ := url.Parse(redirectURI)
			q := u.Query()
			q.Set("token", jwtStr)
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
			return
		}
	}
	if h.AuthConfig.SuccessRedirectURL != "" {
		if redirectWithJWTFragment(w, r, h.AuthConfig.SuccessRedirectURL, jwtStr) {
			return
		}
	}
	// Default: redirect into the SPA with token in the URL fragment (not sent to the server on navigation / avoids Referer leaks).
	if redirectWithJWTFragment(w, r, h.AuthConfig.BaseURL+"/", jwtStr) {
		return
	}
	// Fallback if BaseURL is unusable: HTML with token for MCP copy-paste
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Flywheel – Signed in</title></head><body>
<h1>You're signed in</h1>
<p><strong>Agent ID</strong> (use in MCP tools like <code>claim_ticket</code>):</p>
<pre style="background:#f4f4f4;padding:1em;overflow:auto;">` + agent.ID + `</pre>
<p><strong>Token</strong> (set as <code>FLYWHEEL_TOKEN</code> in your MCP env):</p>
<pre style="background:#f4f4f4;padding:1em;overflow:auto;">` + jwtStr + `</pre>
<p>See <code>docs/cursor-mcp.md</code> for Cursor setup.</p>
</body></html>`))
}

// redirectWithJWTFragment sends a redirect with JWT in the fragment (#token=...). Returns false if landingPage is not a valid absolute URL.
func redirectWithJWTFragment(w http.ResponseWriter, r *http.Request, landingPage, jwtStr string) bool {
	u, err := url.Parse(landingPage)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawQuery = ""
	u.Fragment = "token=" + url.QueryEscape(jwtStr)
	http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
	return true
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
