package webhook

import (
	"testing"
	"time"
)

func TestSignPayload(t *testing.T) {
	secret := "test-secret-key-12345"
	timestamp := int64(1714262400) // 2026-04-28T00:00:00Z
	payload := []byte(`{"id":"evt_123","type":"payment.completed"}`)

	sig := SignPayload(secret, timestamp, payload)

	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	if len(sig) < len(SignaturePrefix)+64 { // sha256= + 64 hex chars
		t.Fatalf("signature too short: %s", sig)
	}
	if sig[:7] != SignaturePrefix {
		t.Fatalf("expected prefix %q, got %q", SignaturePrefix, sig[:7])
	}

	// Same inputs should produce same signature.
	sig2 := SignPayload(secret, timestamp, payload)
	if sig != sig2 {
		t.Fatal("same inputs should produce same signature")
	}
}

func TestSignPayload_DifferentSecrets(t *testing.T) {
	timestamp := int64(1714262400)
	payload := []byte(`{"id":"evt_123"}`)

	sig1 := SignPayload("secret-a", timestamp, payload)
	sig2 := SignPayload("secret-b", timestamp, payload)

	if sig1 == sig2 {
		t.Fatal("different secrets should produce different signatures")
	}
}

func TestSignPayload_DifferentTimestamps(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"id":"evt_123"}`)

	sig1 := SignPayload(secret, 1000, payload)
	sig2 := SignPayload(secret, 2000, payload)

	if sig1 == sig2 {
		t.Fatal("different timestamps should produce different signatures")
	}
}

func TestSignPayload_DifferentPayloads(t *testing.T) {
	secret := "test-secret"
	timestamp := int64(1714262400)

	sig1 := SignPayload(secret, timestamp, []byte(`{"a":1}`))
	sig2 := SignPayload(secret, timestamp, []byte(`{"a":2}`))

	if sig1 == sig2 {
		t.Fatal("different payloads should produce different signatures")
	}
}

func TestVerifySignature_Valid(t *testing.T) {
	secret := "test-secret"
	timestamp := time.Now().Unix()
	payload := []byte(`{"id":"evt_123"}`)

	sig := SignPayload(secret, timestamp, payload)

	if err := VerifySignature(secret, sig, timestamp, payload); err != nil {
		t.Fatalf("expected valid signature, got error: %v", err)
	}
}

func TestVerifySignature_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	timestamp := time.Now().Unix()
	payload := []byte(`{"id":"evt_123"}`)

	err := VerifySignature(secret, "sha256=invalid", timestamp, payload)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestVerifySignature_WrongSecret(t *testing.T) {
	timestamp := time.Now().Unix()
	payload := []byte(`{"id":"evt_123"}`)

	sig := SignPayload("correct-secret", timestamp, payload)
	err := VerifySignature("wrong-secret", sig, timestamp, payload)
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestVerifySignature_ExpiredTimestamp(t *testing.T) {
	secret := "test-secret"
	// Timestamp older than replay window (5 minutes).
	timestamp := time.Now().Add(-10 * time.Minute).Unix()
	payload := []byte(`{"id":"evt_123"}`)

	sig := SignPayload(secret, timestamp, payload)
	err := VerifySignature(secret, sig, timestamp, payload)
	if err == nil {
		t.Fatal("expected error for expired timestamp")
	}
}

func TestVerifySignature_FutureTimestamp(t *testing.T) {
	secret := "test-secret"
	// Timestamp too far in the future.
	timestamp := time.Now().Add(10 * time.Minute).Unix()
	payload := []byte(`{"id":"evt_123"}`)

	sig := SignPayload(secret, timestamp, payload)
	err := VerifySignature(secret, sig, timestamp, payload)
	if err == nil {
		t.Fatal("expected error for future timestamp")
	}
}

func TestGenerateSecret(t *testing.T) {
	s1, err := GenerateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s1) != SecretLength*2 { // hex encoding doubles length
		t.Fatalf("expected length %d, got %d", SecretLength*2, len(s1))
	}

	s2, err := GenerateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s1 == s2 {
		t.Fatal("two generated secrets should be different")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abcdefghijklmnop", "abcdefgh********"},
		{"short", "*****"},
		{"12345678", "********"},
		{"123456789", "12345678*"},
		{"", ""},
	}
	for _, tc := range tests {
		got := MaskSecret(tc.input)
		if got != tc.expected {
			t.Errorf("MaskSecret(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseSignatureHeader(t *testing.T) {
	digest, err := ParseSignatureHeader("sha256=abcdef1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != "abcdef1234567890" {
		t.Fatalf("expected digest %q, got %q", "abcdef1234567890", digest)
	}
}

func TestParseSignatureHeader_Invalid(t *testing.T) {
	_, err := ParseSignatureHeader("invalid")
	if err == nil {
		t.Fatal("expected error for missing prefix")
	}
}

func TestParseTimestampHeader(t *testing.T) {
	ts, err := ParseTimestampHeader("1714262400")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts != 1714262400 {
		t.Fatalf("expected 1714262400, got %d", ts)
	}
}

func TestParseTimestampHeader_Invalid(t *testing.T) {
	_, err := ParseTimestampHeader("not-a-number")
	if err == nil {
		t.Fatal("expected error for invalid timestamp")
	}
}
