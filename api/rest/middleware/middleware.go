package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ContextKeyRequestID is the context key for the request ID.
type ContextKeyRequestID struct{}

const requestIDHeader = "X-Request-Id"

// RequestID sets a request ID in context (from X-Request-Id header or new UUID) and on response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = uuid.Must(uuid.NewV7()).String()
		}
		ctx := context.WithValue(r.Context(), ContextKeyRequestID{}, id)
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request ID from context, or "".
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyRequestID{}).(string); ok {
		return id
	}
	return ""
}

// RealIP sets r.RemoteAddr from X-Real-IP or X-Forwarded-For when behind a trusted proxy.
// Use only when a reverse proxy you control sets these headers.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			r.RemoteAddr = net.JoinHostPort(strings.TrimSpace(ip), "0")
		} else if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			// First element is client (per spec)
			ip = strings.TrimSpace(strings.Split(fwd, ",")[0])
			if ip != "" {
				r.RemoteAddr = net.JoinHostPort(ip, "0")
			}
		}
		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and size.
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *responseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	return n, err
}

// Flush implements http.Flusher so SSE and streaming responses work through the logger.
func (w *responseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for middleware compatibility (e.g., http.NewResponseController).
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Logger logs each request: method, path, remote, status, duration, size.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrap := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrap, r)
		log.Printf("%s %s %s %d %s %d",
			r.Method, r.URL.Path, r.RemoteAddr, wrap.status, time.Since(start), wrap.size)
	})
}

// Recoverer recovers from panics and returns 500.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"internal server error","code":"internal","retriable":false}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
