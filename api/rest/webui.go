package rest

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var webUIReverseProxyFactory = func(target *url.URL) http.Handler {
	return newWebUIReverseProxy(target)
}

// MountWebUI registers routes to serve the Vite/React production build from
// distDir. When devProxyURL is set, frontend GET requests that are not claimed
// by more specific API routes are reverse-proxied to the Vite dev server so
// HMR works through the main Flywheel origin. The SPA uses hash routing, with
// a fallback for frontend document URLs.
//
// It returns the handler that serves the SPA shell (or nil when the UI is not
// mounted) so the router can also route browser document navigations to it —
// see WebUINavigation.
func MountWebUI(mux *http.ServeMux, distDir, devProxyURL string) http.Handler {
	if devProxyURL != "" {
		target, err := url.Parse(devProxyURL)
		if err != nil {
			slog.Error("web UI: invalid dev proxy URL", "url", devProxyURL, "error", err)
		} else {
			slog.Info("web UI: proxying frontend requests", "target", target.String())
			proxy := webUIReverseProxyFactory(target)
			mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if isReservedWebUIPath(r.URL.Path) && !strings.HasPrefix(r.URL.Path, "/assets/") {
					http.NotFound(w, r)
					return
				}
				proxy.ServeHTTP(w, r)
			}))
			return proxy
		}
	}

	if distDir == "" {
		return nil
	}
	abs, err := filepath.Abs(distDir)
	if err != nil {
		slog.Error("web UI: resolve dist path failed", "path", distDir, "error", err)
		return nil
	}
	index := filepath.Join(abs, "index.html")
	if _, err := os.Stat(index); err != nil {
		slog.Info("web UI: skip mount, index.html not found", "path", index, "error", err)
		return nil
	}

	assetsDir := filepath.Join(abs, "assets")
	assetsServer := http.FileServer(http.Dir(assetsDir))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", assetsServer))

	// SPA fallback: serve index.html for any unmatched GET so the React
	// router can handle client-side routes like /orgs/:orgId/projects/...
	// Skip paths with file extensions (missing assets, source maps, etc.).
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isReservedWebUIPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if ext := filepath.Ext(r.URL.Path); ext != "" && ext != ".html" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
	mux.Handle("GET /", spa)
	return spa
}

// webUIReservedPrefixes are never treated as SPA routes, even for document requests.
var webUIReservedPrefixes = []string{"/api/", "/assets/", "/auth/", "/mcp", "/sse", "/metrics", "/healthz", "/readyz", "/worker-config"}

// WebUINavigation routes browser document navigations (GET with an Accept header
// that asks for text/html) to the SPA shell before the API router sees them.
// Several client-side routes share a path with a REST resource — /settings,
// /orgs, /sessions, /code-reviews — and without this a hard reload of those
// pages would return the API's JSON instead of the app.
func WebUINavigation(spa http.Handler, next http.Handler) http.Handler {
	if spa == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && isDocumentRequest(r) && !isReservedWebUIPath(r.URL.Path) {
			if ext := filepath.Ext(r.URL.Path); ext == "" || ext == ".html" {
				spa.ServeHTTP(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isDocumentRequest(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		return false // Explicit API credentials imply an API request.
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" {
		return dest == "document"
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func isReservedWebUIPath(p string) bool {
	for _, prefix := range webUIReservedPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func newWebUIReverseProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalHost := req.Host
		director(req)
		req.Host = originalHost
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("web UI dev proxy error", "path", r.URL.Path, "error", err)
		http.Error(w, "web UI dev proxy unavailable", http.StatusBadGateway)
	}
	return proxy
}
