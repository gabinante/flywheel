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
