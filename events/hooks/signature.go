package hooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
)

// SignatureConfig configures webhook signature verification for a specific source.
type SignatureConfig struct {
	// Secret is the shared secret for HMAC-SHA256 computation.
	// If empty, signature verification is skipped with a warning (dev mode).
	Secret string

	// Header is the HTTP header name containing the signature.
	// Default: "X-Webhook-Signature".
	Header string
}

// header returns the configured header name, or the default.
func (sc SignatureConfig) header() string {
	if sc.Header != "" {
		return sc.Header
	}
	return "X-Webhook-Signature"
}

// AuditEntry records a rejected webhook attempt.
type AuditEntry struct {
	Action   string         `json:"action"`
	Resource string         `json:"resource"`
	Details  map[string]any `json:"details"`
}

// AuditLogger logs webhook audit events.
type AuditLogger interface {
	Log(entry AuditEntry)
}

// slogAuditLogger logs audit entries via slog.
type slogAuditLogger struct{}

func (slogAuditLogger) Log(entry AuditEntry) {
	slog.Warn("webhook audit",
		"action", entry.Action,
		"resource", entry.Resource,
		"details", entry.Details,
	)
}

// DefaultAuditLogger returns a logger that writes to slog.
func DefaultAuditLogger() AuditLogger {
	return slogAuditLogger{}
}

// VerifySignature checks the HMAC-SHA256 signature of the request body.
// It returns nil if the signature is valid.
//
// If the secret is empty (dev mode), it logs a warning and returns nil.
// If the signature header is missing or invalid, it returns an error describing the reason.
func VerifySignature(cfg SignatureConfig, body []byte, r *http.Request) error {
	if cfg.Secret == "" {
		slog.Warn("webhook signature verification skipped: no secret configured (dev mode)",
			"source", r.PathValue("source"),
			"header", cfg.header(),
		)
		return nil
	}

	sig := r.Header.Get(cfg.header())
	if sig == "" {
		return fmt.Errorf("missing signature header %q", cfg.header())
	}

	expected := ComputeHMACSHA256(cfg.Secret, body)

	if !timingSafeEqual(sig, expected) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// ComputeHMACSHA256 computes the HMAC-SHA256 hex digest of body using the given secret.
func ComputeHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// timingSafeEqual compares two strings in constant time to prevent timing attacks.
func timingSafeEqual(a, b string) bool {
	ab := []byte(a)
	bb := []byte(b)
	// subtle.ConstantTimeCompare returns 1 iff the two slices have equal contents.
	// It requires equal lengths; pad if needed.
	if len(ab) != len(bb) {
		return false
	}
	return subtle.ConstantTimeCompare(ab, bb) == 1
}

// sourceIP extracts the client IP from the request. It checks X-Forwarded-For
// and X-Real-Ip headers (set by RealIP middleware), falling back to RemoteAddr.
func sourceIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

// flattenHeaders returns a simplified map of request headers for audit logging.
func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) == 1 {
			out[k] = v[0]
		} else {
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}
