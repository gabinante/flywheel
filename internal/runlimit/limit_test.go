package runlimit

import (
	"context"
	"testing"
	"time"
)

func TestSharedCapacityAndCancellation(t *testing.T) {
	Configure(1)
	defer Configure(0)
	release, err := Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx); err == nil {
		t.Fatal("exceeded global limit")
	}
	release()
	release()
	next, err := Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next()
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := Acquire(canceled); err == nil {
		t.Fatal("admitted canceled run")
	}
}
