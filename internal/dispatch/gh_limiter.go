package dispatch

import (
	"context"
	"math"
	"sync"
	"time"
)

// ghRateLimiter implements a token-bucket rate limiter for GitHub CLI calls.
// Sustained rate of 1 token/second with burst of 10 keeps us well under
// GitHub's 5000 requests/hour API limit (~83/min).
type ghRateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	maxBurst float64
	rate     float64 // tokens per second
	lastTime time.Time
}

func newGHRateLimiter(rate float64, maxBurst float64) *ghRateLimiter {
	return &ghRateLimiter{
		tokens:   maxBurst,
		maxBurst: maxBurst,
		rate:     rate,
		lastTime: time.Now(),
	}
}

// Wait blocks until a token is available or ctx is cancelled.
func (l *ghRateLimiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(l.lastTime).Seconds()
		l.tokens = math.Min(l.maxBurst, l.tokens+elapsed*l.rate)
		l.lastTime = now

		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}

		// Calculate wait time for next token.
		deficit := 1 - l.tokens
		wait := time.Duration(deficit / l.rate * float64(time.Second))
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}
