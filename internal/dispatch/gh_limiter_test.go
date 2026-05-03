package dispatch

import (
	"context"
	"testing"
	"time"
)

func TestGHRateLimiter_BurstAllowed(t *testing.T) {
	l := newGHRateLimiter(1, 10)
	ctx := context.Background()

	// Should allow burst of 10 without blocking.
	start := time.Now()
	for i := 0; i < 10; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("Wait returned error on burst %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("burst of 10 took %v, expected < 100ms", elapsed)
	}
}

func TestGHRateLimiter_RateLimited(t *testing.T) {
	l := newGHRateLimiter(100, 1) // 100/s rate, burst of 1

	ctx := context.Background()

	// First call consumes the burst token.
	if err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}

	// Second call should be rate-limited (~10ms at 100/s).
	start := time.Now()
	if err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 5*time.Millisecond {
		t.Errorf("expected rate limiting delay, got %v", elapsed)
	}
}

func TestGHRateLimiter_ContextCancelled(t *testing.T) {
	l := newGHRateLimiter(0.01, 0) // very slow, no burst

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := l.Wait(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestGHRateLimiter_TokenRefill(t *testing.T) {
	l := newGHRateLimiter(1000, 2) // 1000/s, burst 2
	ctx := context.Background()

	// Drain burst.
	_ = l.Wait(ctx)
	_ = l.Wait(ctx)

	// Wait for refill.
	time.Sleep(5 * time.Millisecond)

	// Should have refilled ~5 tokens at 1000/s.
	start := time.Now()
	if err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Millisecond {
		t.Errorf("expected immediate token after refill, got %v delay", elapsed)
	}
}
