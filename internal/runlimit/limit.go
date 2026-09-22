// Package runlimit bounds all local harness invocations across workflows.
package runlimit

import (
	"context"
	"sync"
)

var mu sync.Mutex
var maximum, active int
var changed = make(chan struct{})

func Configure(n int) {
	mu.Lock()
	maximum = n
	close(changed)
	changed = make(chan struct{})
	mu.Unlock()
}

type priorityKey struct{}

// WithPriority permits one overflow invocation above the shared harness limit.
// Only explicitly prioritized reviews opt in; normal work cannot use that slot.
func WithPriority(ctx context.Context) context.Context {
	return context.WithValue(ctx, priorityKey{}, true)
}

func Acquire(ctx context.Context) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		mu.Lock()
		priority, _ := ctx.Value(priorityKey{}).(bool)
		if maximum <= 0 || active < maximum || (priority && active == maximum) {
			active++
			mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() { mu.Lock(); active--; close(changed); changed = make(chan struct{}); mu.Unlock() })
			}, nil
		}
		wait := changed
		mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}
