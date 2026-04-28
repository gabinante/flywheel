// Package webhook implements the outbound webhook delivery system.
// It provides HMAC-SHA256 signing, HTTP delivery with retry logic,
// and delivery tracking for webhook events.
package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// SignaturePrefix is prepended to the hex-encoded HMAC digest.
	SignaturePrefix = "sha256="

	// ReplayWindowDuration is the maximum age of a webhook signature before
	// it is considered expired (5 minutes).
	ReplayWindowDuration = 5 * time.Minute

	// SecretLength is the byte length of generated webhook secrets.
	SecretLength = 32
)

// SignPayload computes an HMAC-SHA256 signature for the given payload and timestamp.
// Returns the signature string in the format "sha256={hex_digest}".
func SignPayload(secret string, timestamp int64, payload []byte) string {
	// Construct the signed content: timestamp.payload
	signedContent := fmt.Sprintf("%d.%s", timestamp, string(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedContent))
	digest := mac.Sum(nil)

	return SignaturePrefix + hex.EncodeToString(digest)
}

// VerifySignature verifies an HMAC-SHA256 webhook signature using constant-time
// comparison. It also checks the timestamp is within the replay window.
// Returns nil if valid, an error otherwise.
func VerifySignature(secret string, signature string, timestamp int64, payload []byte) error {
	// Check replay window.
	now := time.Now().Unix()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(ReplayWindowDuration.Seconds()) {
		return fmt.Errorf("webhook signature expired: timestamp %d is %d seconds old (max %d)",
			timestamp, diff, int64(ReplayWindowDuration.Seconds()))
	}

	// Compute expected signature.
	expected := SignPayload(secret, timestamp, payload)

	// Timing-safe comparison.
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return fmt.Errorf("webhook signature mismatch")
	}

	return nil
}

// GenerateSecret generates a cryptographically random webhook secret.
// Returns the secret as a hex-encoded string.
func GenerateSecret() (string, error) {
	b := make([]byte, SecretLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// MaskSecret masks a webhook secret for display, showing only the first 8 characters.
func MaskSecret(secret string) string {
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:8] + strings.Repeat("*", len(secret)-8)
}

// ParseSignatureHeader extracts the hex digest from a signature header value.
// Expects format "sha256={hex}".
func ParseSignatureHeader(header string) (string, error) {
	if !strings.HasPrefix(header, SignaturePrefix) {
		return "", fmt.Errorf("invalid signature format: missing %q prefix", SignaturePrefix)
	}
	return strings.TrimPrefix(header, SignaturePrefix), nil
}

// ParseTimestampHeader parses the X-GHP-Timestamp header value.
func ParseTimestampHeader(header string) (int64, error) {
	return strconv.ParseInt(header, 10, 64)
}
