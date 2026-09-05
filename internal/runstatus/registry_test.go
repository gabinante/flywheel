package runstatus

import (
	"context"
	"sync"
	"testing"
)

func TestNativeSessionIsVisibleBeforeCompletion(t *testing.T) {
	registry := New()
	var observed []Run
	registry.SetObserver(func(ctx context.Context, r Run) (string, error) {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		observed = append(observed, r)
		return "session-1", nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	ctx, finish := registry.Begin(ctx, Run{Kind: "executor", TicketID: "ticket-1"})
	Running(ctx)
	Session(ctx, "native-1")
	live := registry.Snapshot()
	if len(live) != 1 || live[0].SessionID != "session-1" || live[0].State != "running" {
		t.Fatalf("live=%+v", live)
	}
	Session(ctx, "native-1")
	if len(observed) != 1 {
		t.Fatal("duplicate native announcement recorded twice")
	}
	cancel()
	finish()
	finish()
	if len(registry.Snapshot()) != 0 || len(observed) != 2 || observed[1].FinishedAt == nil {
		t.Fatalf("observations=%+v", observed)
	}
}
func TestConcurrentSnapshots(t *testing.T) {
	registry := New()
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				ctx, finish := registry.Begin(context.Background(), Run{})
				Running(ctx)
				Session(ctx, "native")
				_ = registry.Snapshot()
				finish()
			}
		}()
	}
	wg.Wait()
	if len(registry.Snapshot()) != 0 {
		t.Fatal("finished runs remained in registry")
	}
}
