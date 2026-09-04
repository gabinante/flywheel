package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsSafeRedirectURI(t *testing.T) {
	cases := map[string]bool{
		"http://localhost:57276/callback": true,
		"http://127.0.0.1:1234/cb":        true,
		"https://localhost/cb":            false, // only plain http for local callbacks
		"http://evil.example.com/cb":      false,
		"not a url":                       false,
	}
	for uri, want := range cases {
		if got := isSafeRedirectURI(uri); got != want {
			t.Errorf("isSafeRedirectURI(%q) = %v, want %v", uri, got, want)
		}
	}
}

func TestRedirectWithJWTFragment(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8090/auth/login", nil)
	if ok := redirectWithJWTFragment(rec, req, "http://localhost:8090", "tok.en"); !ok {
		t.Fatal("expected redirect to succeed for absolute landing page")
	}
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("got status %d, want 307", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "http://localhost:8090/#token=") {
		t.Errorf("unexpected Location: %s", loc)
	}
	if redirectWithJWTFragment(httptest.NewRecorder(), req, "relative/path", "x") {
		t.Error("relative landing page must be rejected")
	}
}
