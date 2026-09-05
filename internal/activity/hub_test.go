package activity

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestCoalescingPreservesMissedTopicsAndReadiness(t *testing.T) {
	h := New()
	h.SourceReady(true)
	ch, cancel := h.Subscribe()
	defer cancel()
	// Leave the initial snapshot unread; updates must preserve its resync flag.
	h.Publish(Reviews, Reviews, "secret/unknown")
	h.flush()
	h.Publish(Sessions)
	h.SourceReady(false)
	h.flush()
	e := <-ch
	if !e.Resync || e.Ready || !reflect.DeepEqual(e.Topics, []string{Reviews, Sessions}) {
		t.Fatalf("event = %+v", e)
	}
	h.SourceReady(true)
	h.flush()
	if e = <-ch; !e.Resync || !e.Ready {
		t.Fatalf("recovery = %+v", e)
	}
}

func TestConcurrentProducersAndSubscribers(t *testing.T) {
	h := New()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_, cancel := h.Subscribe()
				h.Publish(Runs)
				h.flush()
				cancel()
				cancel()
			}
		}()
	}
	wg.Wait()
	if len(h.clients) != 0 {
		t.Fatal("subscriber leak")
	}
}

func TestShutdownClosesStreams(t *testing.T) {
	h := New()
	ctx, cancel := context.WithCancel(context.Background())
	ch, unsub := h.Subscribe()
	<-ch
	go h.Run(ctx)
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed stream")
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not close")
	}
	unsub()
	later, _ := h.Subscribe()
	if _, ok := <-later; ok {
		t.Fatal("subscribed after shutdown")
	}
}
