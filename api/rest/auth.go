package rest

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gabinante/flywheel/internal/auth"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
)

// AuthHandler signs in the single local operator. Flywheel is local-first and
// single-user: GET /auth/login provisions (once) the operator identity and
// issues a JWT the web UI stores in localStorage. Agents authenticate with API
// keys instead (see AuthMiddleware).
type AuthHandler struct {
	Provisioner        *auth.Provisioner
	OrgSvc             *org.Service // optional; when set, the operator gets a default org
	JWTSecret          string
	JWTExpiry          time.Duration
	BaseURL            string
	SuccessRedirectURL string // optional landing page; defaults to BaseURL + "/"
}

// NewAuthHandler constructs the handler. A zero expiry defaults to 7 days.
func NewAuthHandler(provisioner *auth.Provisioner, orgSvc *org.Service, jwtSecret, baseURL, successRedirectURL string, expiry time.Duration) *AuthHandler {
	if expiry == 0 {
		expiry = 7 * 24 * time.Hour
	}
	return &AuthHandler{
		Provisioner:        provisioner,
		OrgSvc:             orgSvc,
		JWTSecret:          jwtSecret,
		JWTExpiry:          expiry,
		BaseURL:            baseURL,
		SuccessRedirectURL: successRedirectURL,
	}
}

// isSafeRedirectURI allows only localhost redirects (e.g. a local CLI callback).
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

// login provisions the local operator and redirects to the UI with #token=<jwt>.
// ?redirect_uri=http://localhost:... is honored for local CLI callbacks.
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, agent, err := h.Provisioner.Provision(ctx, auth.LocalOperator())
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
	if redirectURI := r.URL.Query().Get("redirect_uri"); redirectURI != "" && isSafeRedirectURI(redirectURI) {
		target, _ := url.Parse(redirectURI)
		q := target.Query()
		q.Set("token", jwtStr)
		target.RawQuery = q.Encode()
		http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
		return
	}
	if h.SuccessRedirectURL != "" && redirectWithJWTFragment(w, r, h.SuccessRedirectURL, jwtStr) {
		return
	}
	if redirectWithJWTFragment(w, r, h.BaseURL+"/", jwtStr) {
		return
	}
	WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "auth misconfigured: BASE_URL required", false))
}

// redirectWithJWTFragment sends a redirect with the JWT in the fragment (#token=...).
// Returns false if landingPage is not a valid absolute URL.
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
