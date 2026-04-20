package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabinante/flywheel/internal/auth"
)

func TestGithubRedirect_WithRedirectURI(t *testing.T) {
	h := NewAuthHandler(
		auth.Config{
			ClientID:     "test",
			ClientSecret: "secret",
			BaseURL:      "http://localhost:8080",
			RedirectPath: "/auth/github/callback",
		},
		nil, nil, nil, "jwt-secret", 0,
	)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/auth/github?redirect_uri=http%3A%2F%2F127.0.0.1%3A57276%2Fcallback", nil)
	rec := httptest.NewRecorder()
	h.githubRedirect(rec, req)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("got 500: %s", rec.Body.String())
	}
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("got status %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc == "" || loc[:8] != "https://" {
		t.Errorf("expected redirect to GitHub, got Location: %s", loc)
	}
}
