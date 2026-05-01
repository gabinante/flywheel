package rest

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
)

var webUIReverseProxyFactory = func(target *url.URL) http.Handler {
	return newWebUIReverseProxy(target)
}

// MountWebUI registers routes to serve the Vite/React production build from
// distDir. When devProxyURL is set, frontend GET requests that are not claimed
// by more specific API routes are reverse-proxied to the Vite dev server so
// HMR works through the main Flywheel origin. The SPA uses browser-history
// routing, so a catch-all serves index.html for any unmatched GET request.
func MountWebUI(mux *http.ServeMux, distDir, devProxyURL string) {
	if devProxyURL != "" {
		target, err := url.Parse(devProxyURL)
		if err != nil {
			slog.Error("web UI: invalid dev proxy URL", "url", devProxyURL, "error", err)
		} else {
			slog.Info("web UI: proxying frontend requests", "target", target.String())
			mux.Handle("GET /", webUIReverseProxyFactory(target))
			return
		}
	}

	if distDir == "" {
		return
	}
	abs, err := filepath.Abs(distDir)
	if err != nil {
		slog.Error("web UI: resolve dist path failed", "path", distDir, "error", err)
		return
	}
	index := filepath.Join(abs, "index.html")
	if _, err := os.Stat(index); err != nil {
		slog.Info("web UI: skip mount, index.html not found", "path", index, "error", err)
		return
	}

	assetsDir := filepath.Join(abs, "assets")
	assetsServer := http.FileServer(http.Dir(assetsDir))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", assetsServer))

	// SPA fallback: serve index.html for any unmatched GET so the React
	// router can handle client-side routes like /orgs/:orgId/projects/...
	// Skip paths with file extensions (missing assets, source maps, etc.).
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if ext := filepath.Ext(r.URL.Path); ext != "" && ext != ".html" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
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
