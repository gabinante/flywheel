package codereview

import (
	"context"
	"testing"
)

func TestPriorityQueuePersistsAndDoesNotDuplicate(t *testing.T) {
	store, pool := reviewTestStore(t)
	ctx := context.Background()
	repo := "priority-test/" + mustUUID()
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM code_review_requests WHERE repo=$1", repo) })
	normal := &Request{Repo: repo, Number: 1, State: StateQueued, Harness: "codex"}
	urgent := &Request{Repo: repo, Number: 2, State: StateQueued, Harness: "codex"}
	for _, r := range []*Request{normal, urgent} {
		if err := store.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE code_review_requests SET retry_at=now()+interval '1 hour' WHERE id=$1", urgent.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Prioritize(ctx, urgent.ID); err != nil {
		t.Fatal(err)
	}
	r, err := store.Get(ctx, urgent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.PriorityAt == nil || r.RetryAt != nil {
		t.Fatalf("priority/backoff not persisted: %+v", r)
	}
	timestamp := *r.PriorityAt
	if err := store.Prioritize(ctx, urgent.ID); err != nil {
		t.Fatal(err)
	}
	r, _ = store.Get(ctx, urgent.ID)
	if !r.PriorityAt.Equal(timestamp) {
		t.Fatal("repeated click changed FIFO order")
	}
	// A new store represents reload/restart; extra capacity must only claim priority work.
	reloaded := NewStore(pool)
	picked, err := reloaded.claimQueue(ctx, 1, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(picked) != 1 || picked[0].ID != urgent.ID {
		t.Fatalf("priority claim: %+v", picked)
	}
	if err := store.Prioritize(ctx, urgent.ID); err != nil {
		t.Fatal(err)
	}
	picked, err = store.claimQueue(ctx, 1, nil, true)
	if err != nil || len(picked) != 0 {
		t.Fatalf("duplicate or normal review in extra slot: %v %+v", err, picked)
	}
	// Normal capacity still makes progress.
	picked, err = store.ClaimQueued(ctx, 1)
	if err != nil || len(picked) != 1 || picked[0].ID != normal.ID {
		t.Fatalf("normal claim: %v %+v", err, picked)
	}
	if err := store.Stop(ctx, urgent.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Prioritize(ctx, urgent.ID); err != nil {
		t.Fatal(err)
	}
	r, _ = store.Get(ctx, urgent.ID)
	if r.State != StateClosed {
		t.Fatal("priority revived stopped review")
	}
	if err := store.Requeue(ctx, urgent.ID, OriginPaste); err != nil {
		t.Fatal(err)
	}
	r, _ = store.Get(ctx, urgent.ID)
	if r.PriorityAt != nil {
		t.Fatal("priority leaked to a new manual attempt")
	}
	// Ensure active-ID exclusion also applies to priority retries.
	if err := store.Prioritize(ctx, urgent.ID); err != nil {
		t.Fatal(err)
	}
	picked, err = store.claimQueue(ctx, 1, []string{urgent.ID}, true)
	if err != nil || len(picked) != 0 {
		t.Fatal("claimed attempt still owned by running worker")
	}
}
