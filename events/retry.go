package events

import "context"

type deliveryResult struct{ err error }
type deliveryKey struct{}

// Retry records a handler failure. A durable bus retries without acknowledging
// this event; handlers must call it before returning, never from a detached task.
func Retry(ctx context.Context, err error) {
	if r, ok := ctx.Value(deliveryKey{}).(*deliveryResult); ok && err != nil {
		r.err = err
	}
}
