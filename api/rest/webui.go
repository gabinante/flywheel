package rest

import (
	"log"
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
// HMR works through the main Flywheel origin. The SPA uses hash-based routing
// (/#/...) so REST paths like /orgs are not claimed by the frontend router.
func MountWebUI(mux *http.ServeMux, distDir, devProxyURL string) {
	if devProxyURL != "" {
		target, err := url.Parse(devProxyURL)
		if err != nil {
			log.Printf("web UI: invalid dev proxy URL %q: %v", devProxyURL, err)
		} else {
			log.Printf("web UI: proxying frontend requests to %s", target)
			mux.Handle("GET /", webUIReverseProxyFactory(target))
			return
		}
	}

	if distDir == "" {
		return
	}
	abs, err := filepath.Abs(distDir)
	if err != nil {
		log.Printf("web UI: resolve dist path %q: %v", distDir, err)
		return
	}
	index := filepath.Join(abs, "index.html")
	if _, err := os.Stat(index); err != nil {
		log.Printf("web UI: skip mount (no %s): %v", index, err)
		return
	}

	assetsDir := filepath.Join(abs, "assets")
	assetsServer := http.FileServer(http.Dir(assetsDir))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", assetsServer))

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("web UI: dev proxy error for %s: %v", r.URL.Path, err)
		http.Error(w, "web UI dev proxy unavailable", http.StatusBadGateway)
	}
	return proxy
}
