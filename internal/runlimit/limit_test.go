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

func TestPriorityGetsExactlyOneOverflowSlot(t *testing.T) {
	Configure(1)
	defer Configure(0)
	normal, err := Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer normal()
	ctx, cancel := context.WithTimeout(WithPriority(context.Background()), time.Second)
	defer cancel()
	priority, err := Acquire(ctx)
	if err != nil {
		t.Fatal("priority did not bypass full capacity:", err)
	}
	defer priority()
	for _, c := range []context.Context{context.Background(), WithPriority(context.Background())} {
		limited, stop := context.WithTimeout(c, 30*time.Millisecond)
		if release, err := Acquire(limited); err == nil {
			release()
			t.Error("allowed more than one overflow slot")
		}
		stop()
	}
	priority()
	next, err := Acquire(ctx)
	if err != nil {
		t.Fatal("overflow slot was not released:", err)
	}
	next()
}
