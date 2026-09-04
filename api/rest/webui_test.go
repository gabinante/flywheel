package rest

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMountWebUI_servesIndexAndAssets(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<!doctype html><html><body>ok</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(tmp, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "x.txt"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	MountWebUI(mux, tmp, "")

	t.Run("index", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if body := rec.Body.String(); body == "" || body[0] != '<' {
			t.Fatalf("body %q", body)
		}
	})

	t.Run("spa fallback on deep path", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/orgs/abc/projects/def/infrastructure", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if body := rec.Body.String(); body == "" || body[0] != '<' {
			t.Fatalf("expected index.html, got %q", body)
		}
	})

	t.Run("missing asset returns 404", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/missing-file.js", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing .js, got %d", rec.Code)
		}
	})

	t.Run("assets", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/assets/x.txt", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if rec.Body.String() != "asset" {
			t.Fatalf("body %q", rec.Body.String())
		}
	})
}

func TestMountWebUI_proxiesDevServerAndRespectsAPIHandlers(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("api"))
	})

	originalFactory := webUIReverseProxyFactory
	webUIReverseProxyFactory = func(target *url.URL) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "vite:%s", r.URL.Path)
		})
	}
	t.Cleanup(func() {
		webUIReverseProxyFactory = originalFactory
	})

	MountWebUI(mux, "", "http://vite.test")

	t.Run("frontend paths proxy to vite", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/@vite/client", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if rec.Body.String() != "vite:/@vite/client" {
			t.Fatalf("body %q", rec.Body.String())
		}
	})

	t.Run("api handlers take precedence", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if rec.Body.String() != "api" {
			t.Fatalf("body %q", rec.Body.String())
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewWebUIReverseProxy_targetsViteAndPreservesOriginalHost(t *testing.T) {
	t.Parallel()

	target, err := url.Parse("http://127.0.0.1:5173")
	if err != nil {
		t.Fatal(err)
	}
	proxy := newWebUIReverseProxy(target)
	proxy.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.Scheme; got != "http" {
			t.Fatalf("scheme = %q, want http", got)
		}
		if got := req.URL.Host; got != "127.0.0.1:5173" {
			t.Fatalf("host = %q, want 127.0.0.1:5173", got)
		}
		if got := req.URL.Path; got != "/@vite/client" {
			t.Fatalf("path = %q, want /@vite/client", got)
		}
		if got := req.Host; got != "localhost:8080" {
			t.Fatalf("request host = %q, want localhost:8080", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("vite:/@vite/client")),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/@vite/client", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Body.String() != "vite:/@vite/client" {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestWebUINavigation_prefersSPAForDocumentRequests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized"}`))
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	spa := MountWebUI(mux, dir, "")
	if spa == nil {
		t.Fatal("expected SPA handler")
	}
	h := WebUINavigation(spa, mux)

	cases := []struct {
		name     string
		headers  map[string]string
		wantHTML bool
	}{
		{"browser navigation", map[string]string{"Accept": "text/html,application/xhtml+xml", "Sec-Fetch-Dest": "document"}, true},
		{"accept only", map[string]string{"Accept": "text/html"}, true},
		{"fetch with token", map[string]string{"Accept": "text/html", "Authorization": "Bearer x"}, false},
		{"api client", map[string]string{"Accept": "application/json"}, false},
		{"no accept", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/settings", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			gotHTML := strings.Contains(rec.Body.String(), "<html>")
			if gotHTML != tc.wantHTML {
				t.Fatalf("html=%v want %v (status %d body %q)", gotHTML, tc.wantHTML, rec.Code, rec.Body.String())
			}
		})
	}

	// Reserved and asset paths never go to the SPA.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "<html>") {
		t.Fatal("reserved path served the SPA")
	}
}
