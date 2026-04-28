package hooks_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events/hooks"
)

func TestGenerateWebhookSecret(t *testing.T) {
	secret, err := hooks.GenerateWebhookSecret()
	if err != nil {
		t.Fatalf("GenerateWebhookSecret() error = %v", err)
	}
	if len(secret) != 64 { // 32 bytes → 64 hex chars
		t.Errorf("expected 64 hex chars, got %d", len(secret))
	}
	// Ensure it's valid hex.
	if _, err := hex.DecodeString(secret); err != nil {
		t.Errorf("generated secret is not valid hex: %v", err)
	}

	// Generate another and ensure they differ (randomness sanity check).
	secret2, err := hooks.GenerateWebhookSecret()
	if err != nil {
		t.Fatalf("second GenerateWebhookSecret() error = %v", err)
	}
	if secret == secret2 {
		t.Error("two generated secrets should not be identical")
	}
}

func TestSignWebhook(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	timestamp := int64(1700000000)
	body := `{"event":"payment.completed","amount":100}`

	sig, err := hooks.SignWebhook(secret, timestamp, body)
	if err != nil {
		t.Fatalf("SignWebhook() error = %v", err)
	}
	if sig == "" {
		t.Fatal("signature should not be empty")
	}
	// Hex-encoded SHA256 HMAC is 64 chars.
	if len(sig) != 64 {
		t.Errorf("expected 64 hex chars signature, got %d", len(sig))
	}

	// Same inputs produce same signature (deterministic).
	sig2, err := hooks.SignWebhook(secret, timestamp, body)
	if err != nil {
		t.Fatalf("second SignWebhook() error = %v", err)
	}
	if sig != sig2 {
		t.Error("same inputs should produce same signature")
	}

	// Different body produces different signature.
	sig3, err := hooks.SignWebhook(secret, timestamp, `{"different":true}`)
	if err != nil {
		t.Fatalf("third SignWebhook() error = %v", err)
	}
	if sig == sig3 {
		t.Error("different body should produce different signature")
	}

	// Different timestamp produces different signature.
	sig4, err := hooks.SignWebhook(secret, timestamp+1, body)
	if err != nil {
		t.Fatalf("fourth SignWebhook() error = %v", err)
	}
	if sig == sig4 {
		t.Error("different timestamp should produce different signature")
	}

	// Different secret produces different signature.
	otherSecret := "1111111111111111111111111111111111111111111111111111111111111111"
	sig5, err := hooks.SignWebhook(otherSecret, timestamp, body)
	if err != nil {
		t.Fatalf("fifth SignWebhook() error = %v", err)
	}
	if sig == sig5 {
		t.Error("different secret should produce different signature")
	}
}

func TestSignWebhook_InvalidHex(t *testing.T) {
	_, err := hooks.SignWebhook("not-valid-hex!", 1700000000, "body")
	if err == nil {
		t.Fatal("expected error for invalid hex secret")
	}
}

func TestVerifyWebhook_Valid(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	timestamp := time.Now().Unix()
	body := `{"event":"payment.completed","amount":100}`

	sig, err := hooks.SignWebhook(secret, timestamp, body)
	if err != nil {
		t.Fatalf("SignWebhook() error = %v", err)
	}

	ok, err := hooks.VerifyWebhook(secret, sig, timestamp, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if !ok {
		t.Error("VerifyWebhook should return true for valid signature")
	}
}

func TestVerifyWebhook_InvalidSignature(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	timestamp := time.Now().Unix()
	body := `{"event":"payment.completed","amount":100}`

	wrongSig := "0000000000000000000000000000000000000000000000000000000000000000"

	ok, err := hooks.VerifyWebhook(secret, wrongSig, timestamp, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if ok {
		t.Error("VerifyWebhook should return false for invalid signature")
	}
}

func TestVerifyWebhook_TamperedBody(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	timestamp := time.Now().Unix()
	body := `{"event":"payment.completed","amount":100}`

	sig, err := hooks.SignWebhook(secret, timestamp, body)
	if err != nil {
		t.Fatalf("SignWebhook() error = %v", err)
	}

	// Verify with tampered body.
	ok, err := hooks.VerifyWebhook(secret, sig, timestamp, `{"event":"payment.completed","amount":999}`)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if ok {
		t.Error("VerifyWebhook should return false for tampered body")
	}
}

func TestVerifyWebhook_ReplayWindowExpired(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	// Timestamp 10 minutes ago — beyond the 5-minute replay window.
	timestamp := time.Now().Add(-10 * time.Minute).Unix()
	body := `{"event":"payment.completed"}`

	sig, err := hooks.SignWebhook(secret, timestamp, body)
	if err != nil {
		t.Fatalf("SignWebhook() error = %v", err)
	}

	_, err = hooks.VerifyWebhook(secret, sig, timestamp, body)
	if err == nil {
		t.Fatal("VerifyWebhook should return error for expired timestamp")
	}
}

func TestVerifyWebhook_FutureTimestamp(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	// Timestamp 10 minutes in the future — beyond the 5-minute window.
	timestamp := time.Now().Add(10 * time.Minute).Unix()
	body := `{"event":"payment.completed"}`

	sig, err := hooks.SignWebhook(secret, timestamp, body)
	if err != nil {
		t.Fatalf("SignWebhook() error = %v", err)
	}

	_, err = hooks.VerifyWebhook(secret, sig, timestamp, body)
	if err == nil {
		t.Fatal("VerifyWebhook should return error for future timestamp outside window")
	}
}

func TestVerifyWebhook_WithinWindow(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	// Timestamp 2 minutes ago — within the 5-minute window.
	timestamp := time.Now().Add(-2 * time.Minute).Unix()
	body := `{"event":"payment.completed"}`

	sig, err := hooks.SignWebhook(secret, timestamp, body)
	if err != nil {
		t.Fatalf("SignWebhook() error = %v", err)
	}

	ok, err := hooks.VerifyWebhook(secret, sig, timestamp, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if !ok {
		t.Error("VerifyWebhook should return true for timestamp within window")
	}
}

func TestVerifyWebhook_WrongSecret(t *testing.T) {
	secret1 := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	secret2 := "1111111111111111111111111111111111111111111111111111111111111111"
	timestamp := time.Now().Unix()
	body := `{"event":"payment.completed"}`

	sig, err := hooks.SignWebhook(secret1, timestamp, body)
	if err != nil {
		t.Fatalf("SignWebhook() error = %v", err)
	}

	// Verify with wrong secret.
	ok, err := hooks.VerifyWebhook(secret2, sig, timestamp, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if ok {
		t.Error("VerifyWebhook should return false for wrong secret")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name     string
		secret   string
		expected string
	}{
		{"full_secret", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", "****a1b2"},
		{"short_secret", "ab", "****"},
		{"four_chars", "abcd", "****"},
		{"five_chars", "abcde", "****bcde"},
		{"empty", "", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hooks.MaskSecret(tt.secret)
			if got != tt.expected {
				t.Errorf("MaskSecret(%q) = %q, want %q", tt.secret, got, tt.expected)
			}
		})
	}
}
