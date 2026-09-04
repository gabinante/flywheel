package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// GitHubUser is the response from GitHub /user API.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// Config holds GitHub OAuth2 config.
type Config struct {
	ClientID           string
	ClientSecret       string
	BaseURL            string
	RedirectPath       string // e.g. /auth/github/callback
	SuccessRedirectURL string // optional; if set, callback redirects here with #token=... (fragment)
	DevBypass          bool   // AUTH_DEV_BYPASS: skip GitHub and sign in a fixed local user. Local dev only.
}

// OAuth2 returns the oauth2.Config for GitHub (used for ExchangeCode).
func (c *Config) OAuth2() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     github.Endpoint,
		Scopes:       []string{"read:user", "user:email"},
		RedirectURL:  c.BaseURL + c.RedirectPath,
	}
}

// AuthCodeURL returns the GitHub authorization URL for the given state.
// Built explicitly to avoid relying on oauth2.Config.AuthCodeURL, which can
// panic with some oauth2 library versions when unexported config fields are zero.
func (c *Config) AuthCodeURL(state string) string {
	redirectURL := c.BaseURL + c.RedirectPath
	v := url.Values{
		"response_type": {"code"},
		"client_id":     {c.ClientID},
		"redirect_uri":  {redirectURL},
		"scope":         {"read:user user:email"},
		"state":         {state},
	}
	return github.Endpoint.AuthURL + "?" + v.Encode()
}

// ExchangeCode exchanges the auth code for a token.
func (c *Config) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	return c.OAuth2().Exchange(ctx, code)
}

// FetchUser gets the GitHub user for the given token.
func FetchUser(ctx context.Context, token *oauth2.Token) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github user api: %s", resp.Status)
	}
	var u GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}
