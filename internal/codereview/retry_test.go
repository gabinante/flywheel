package codereview

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryableReviewErrors(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{"git fetch: early EOF", true}, {"harness: stream disconnected", true},
		{"check PR before publication: HTTP 502", true}, {"invalid review output", true},
		{"unsupported model", false}, {"executable file not found", false},
		{"post review: timeout", false}, {"Server stopped; inspect GitHub before rerunning", false},
	} {
		if got := retryableReviewError(errors.New(tc.message)); got != tc.want {
			t.Errorf("%q: retryable=%v", tc.message, got)
		}
	}
	if retryableReviewError(context.Canceled) {
		t.Fatal("cancellation should not retry within the stopped service")
	}
}

func TestUnpublishedAttemptsDoNotSuppressRetryFindings(t *testing.T) {
	finding := Finding{Path: "worker.go", Title: "Lost update", Body: "The result is overwritten."}
	for _, status := range []string{"pending", "withheld", "posted", "in_body", "repeat"} {
		prior := finding
		prior.Status = status
		want := status == "posted" || status == "in_body" || status == "repeat"
		if isRepeatFinding(finding, []Finding{prior}) != want {
			t.Errorf("prior finding status %s incorrectly affected retry publication", status)
		}
	}
}

func TestRetriesPersistAndRespectBudgetStopAndOwnership(t *testing.T) {
	store, pool := reviewTestStore(t)
	ctx := context.Background()
	r := &Request{Repo: "retry/" + mustUUID(), Number: 1, State: StateReviewing, Harness: "codex"}
	if err := store.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM code_review_requests WHERE id=$1`, r.ID) })
	get := func() *Request {
		t.Helper()
		r, err := store.Get(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for count, delay := range retryDelays {
		r = get()
		r.State, r.SessionID, r.Summary, r.Verdict = StateReviewing, "previous-session", "old result", "approve"
		if err := store.Update(ctx, r); err != nil {
			t.Fatal(err)
		}
		before := time.Now()
		ok, err := store.scheduleRetry(ctx, r, "temporary network failure", false)
		if err != nil || !ok {
			t.Fatalf("scheduled=%v err=%v", ok, err)
		}
		next := get()
		if next.State != StateQueued || next.RetryCount != count+1 || next.Attempt != r.Attempt+1 || next.RetryAt == nil || next.RetryAt.Before(before.Add(delay)) || next.SessionID != "" || next.Summary != "" || next.Verdict != "" {
			t.Fatalf("invalid retry state: %+v", next)
		}
		// A late update from the previous worker cannot undo its scheduled successor.
		r.State = StateFailed
		if err := store.Update(ctx, r); err != nil {
			t.Fatal(err)
		}
		if get().State != StateQueued {
			t.Fatal("stale worker overwrote retry")
		}
		// A fresh store (as after restart) respects the durable deadline.
		claims, err := NewStore(pool).ClaimQueued(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, claim := range claims {
			if claim.ID == r.ID {
				t.Fatal("retry launched before its deadline")
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE code_review_requests SET retry_at=now()-interval '1 second' WHERE id=$1`, r.ID); err != nil {
			t.Fatal(err)
		}
		claims, err = store.claimQueued(ctx, 100, []string{r.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, claim := range claims {
			if claim.ID == r.ID {
				t.Fatal("retry overlapped a worker still cleaning up")
			}
		}
		claims, err = store.ClaimQueued(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, claim := range claims {
			if claim.ID == r.ID {
				found = claim.RetryAt == nil && claim.RetryCount == count+1
			}
		}
		if !found {
			t.Fatal("due retry did not launch")
		}
	}
	r = get()
	if ok, err := store.scheduleRetry(ctx, r, "network failure", false); err != nil || ok {
		t.Fatalf("exhausted retry scheduled=%v err=%v", ok, err)
	}
	if ok, err := store.scheduleRetry(ctx, r, "PR head changed", true); err != nil || !ok {
		t.Fatalf("head update scheduled=%v err=%v", ok, err)
	}
	if get().RetryCount != 3 {
		t.Fatal("head changes altered the failure budget")
	}
	if err := store.Stop(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := store.scheduleRetry(ctx, get(), "late failure", true); err != nil || ok {
		t.Fatalf("stopped retry scheduled=%v err=%v", ok, err)
	}
	if get().RetryAt != nil {
		t.Fatal("stop retained a pending retry")
	}
	if err := store.Requeue(ctx, r.ID, OriginPaste); err != nil {
		t.Fatal(err)
	}
	if next := get(); next.RetryCount != 0 || next.RetryAt != nil {
		t.Fatal("manual rerun did not reset the recovery budget")
	}
}

func TestRecoveryRetriesInterruptedReadsButNotAmbiguousPublication(t *testing.T) {
	store, pool := reviewTestStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		state  State
		reason string
		want   State
	}{
		{StateReviewing, "", StateQueued}, {StatePublishing, "", StateFailed},
		{StateFailed, "PR head changed while preparing the review; rerun on the current head", StateQueued},
		{StateFailed, "prepare worktree: git fetch: early EOF", StateQueued},
		{StateFailed, "post review: connection lost", StateFailed},
		{StateFailed, "unsupported model", StateFailed},
	} {
		r := &Request{Repo: "recovery/" + mustUUID(), Number: 1, State: tc.state, Error: tc.reason}
		if err := store.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM code_review_requests WHERE id=$1`, r.ID) })
		if err := store.Update(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := store.RecoverInterrupted(ctx); err != nil {
			t.Fatal(err)
		}
		next, err := store.Get(ctx, r.ID)
		if err != nil || next.State != tc.want {
			t.Fatalf("%s/%s: %+v %v", tc.state, tc.reason, next, err)
		}
		attempt := next.Attempt
		if err := store.RecoverInterrupted(ctx); err != nil {
			t.Fatal(err)
		}
		next, err = store.Get(ctx, r.ID)
		if err != nil || next.Attempt != attempt {
			t.Fatal("restart scheduled the same recovery twice")
		}
	}
}
