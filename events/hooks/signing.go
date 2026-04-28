// Package hooks — webhook signing utilities for outbound webhook deliveries.
// Signs payloads with HMAC-SHA256 and validates signatures with timing-safe
// comparison and a 5-minute replay window.
package hooks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"time"
)

// ReplayWindowSeconds is the maximum allowed age (in seconds) of a webhook
// timestamp before it is rejected to prevent replay attacks.
const ReplayWindowSeconds = 300 // 5 minutes

// GenerateWebhookSecret generates a cryptographically random 32-byte hex
// string suitable for use as an HMAC-SHA256 signing key.
func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SignWebhook computes an HMAC-SHA256 signature over the canonical
// string "{timestamp}.{body}" using the provided secret.
// Returns the hex-encoded signature.
func SignWebhook(secret string, timestamp int64, body string) (string, error) {
	secretBytes, err := hex.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("sign webhook: invalid secret hex: %w", err)
	}
	mac := hmac.New(sha256.New, secretBytes)
	message := fmt.Sprintf("%d.%s", timestamp, body)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// VerifyWebhook recomputes the HMAC-SHA256 signature and compares it to the
// provided signature using constant-time comparison. It also rejects
// timestamps outside the replay window (±5 minutes from now).
func VerifyWebhook(secret string, signature string, timestamp int64, body string) (bool, error) {
	// Check replay window.
	now := time.Now().Unix()
	if math.Abs(float64(now-timestamp)) > ReplayWindowSeconds {
		return false, fmt.Errorf("verify webhook: timestamp %d is outside the %d-second replay window (now=%d)", timestamp, ReplayWindowSeconds, now)
	}

	expected, err := SignWebhook(secret, timestamp, body)
	if err != nil {
		return false, fmt.Errorf("verify webhook: %w", err)
	}

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false, fmt.Errorf("verify webhook: invalid signature hex: %w", err)
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false, fmt.Errorf("verify webhook: internal error decoding expected: %w", err)
	}

	return hmac.Equal(sigBytes, expectedBytes), nil
}

// MaskSecret returns a masked version of the webhook secret, showing only
// the last 4 characters: "****...abcd".
func MaskSecret(secret string) string {
	if len(secret) <= 4 {
		return "****"
	}
	return "****" + secret[len(secret)-4:]
}
