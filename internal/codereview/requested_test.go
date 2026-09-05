package codereview

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func requestedGitHub(t *testing.T) (*GitHub, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gh")
	script := `#!/bin/sh
cd "$(dirname "$0")"
case "$1 $2" in
  'api user') echo reviewer ;;
  'search prs') cat search.json ;;
  api\ repos/*/timeline*)
    [ "$3 $4" = '--paginate --slurp' ] || exit 2
    cat timeline.json ;;
  *) echo "Unexpected gh command" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return NewGitHub(bin), dir
}

func writeRequestedFixture(t *testing.T, dir string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "timeline.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitRequestTimeline(t *testing.T) {
	gh, dir := requestedGitHub(t)
	requested := func(id int, event, login string) map[string]any {
		return map[string]any{"id": id, "event": event, "requested_reviewer": map[string]string{"login": login}, "created_at": "2026-09-05T09:00:00Z"}
	}
	pages := [][]map[string]any{{requested(1, "review_requested", "reviewer"), requested(2, "review_requested", "other")}, {requested(3, "review_requested", "REVIEWER")}}
	writeRequestedFixture(t, dir, pages)
	event, err := gh.LatestReviewRequest(context.Background(), "test/repo", 1, "reviewer")
	if err != nil || event == nil || event.ID != 3 {
		t.Fatalf("event=%+v err=%v", event, err)
	}
	pages[1] = append(pages[1], requested(4, "review_request_removed", "reviewer"))
	writeRequestedFixture(t, dir, pages)
	event, err = gh.LatestReviewRequest(context.Background(), "test/repo", 1, "reviewer")
	if err != nil || event != nil {
		t.Fatalf("removed request returned: %+v %v", event, err)
	}
}

func TestLegacyAttemptsInheritSelectedModel(t *testing.T) {
	cfg := Config{Harness: "codex", Model: "selected-model", Effort: "high"}
	r := &Request{Harness: "codex"}
	applyAttemptDefaults(r, cfg)
	if r.Model != cfg.Model || r.ReasoningEffort != cfg.Effort {
		t.Fatalf("defaults lost: %+v", r)
	}
	r.Model = "explicit-model"
	applyAttemptDefaults(r, cfg)
	if r.Model != "explicit-model" {
		t.Fatal("overwrote explicit model")
	}
	r = &Request{Harness: "claude"}
	applyAttemptDefaults(r, cfg)
	if r.Model != "" {
		t.Fatal("applied a Codex model to Claude")
	}
}

func reviewTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("FLYWHEEL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set FLYWHEEL_TEST_DATABASE_URL to an isolated migrated database")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return NewStore(pool), pool
}

func TestExplicitReRequestsQueueOnceAndSurviveActiveAttempt(t *testing.T) {
	store, pool := reviewTestStore(t)
	ctx := context.Background()
	gh, dir := requestedGitHub(t)
	repo := "review-test/" + mustUUID()
	r := &Request{Repo: repo, Number: 1, URL: PRURL(repo, 1), State: StateWatching, Harness: "codex", Watch: true}
	if err := store.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM code_review_requests WHERE id=$1", r.ID) })
	if _, err := pool.Exec(ctx, "UPDATE code_review_requests SET request_handled_at=now()-interval '1 hour' WHERE id=$1", r.ID); err != nil {
		t.Fatal(err)
	}
	search := fmt.Sprintf(`[{"number":1,"url":%q,"repository":{"nameWithOwner":%q},"author":{"login":"author"}}]`, r.URL, repo)
	if err := os.WriteFile(filepath.Join(dir, "search.json"), []byte(search), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := func(id int) {
		writeRequestedFixture(t, dir, [][]map[string]any{{{"id": id, "event": "review_requested", "requested_reviewer": map[string]string{"login": "reviewer"}, "created_at": time.Now().UTC().Format(time.RFC3339Nano)}}})
	}
	svc := New(store, gh, nil, nil, nil, Config{Enabled: true, WatchRequested: true})
	poll := func() {
		t.Helper()
		if err := svc.pollRequested(ctx); err != nil {
			t.Fatal(err)
		}
	}
	check := func(state State, attempt int) *Request {
		t.Helper()
		got, err := store.Get(ctx, r.ID)
		if err != nil || got.State != state || got.Attempt != attempt {
			t.Fatalf("want %s/%d got %+v err=%v", state, attempt, got, err)
		}
		return got
	}
	fixture(10)
	poll()
	check(StateQueued, 2)
	poll()
	check(StateQueued, 2)
	claimed, err := store.ClaimQueued(ctx, 100)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claimed=%v err=%v", claimed, err)
	}
	check(StateFetching, 2)
	fixture(11)
	poll()
	check(StateFetching, 2)
	// Simulate a restart of the poller and removal from search after this review posts.
	svc = New(store, gh, nil, nil, nil, Config{Enabled: true, WatchRequested: true})
	if err := os.WriteFile(filepath.Join(dir, "search.json"), []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	done := check(StateFetching, 2)
	done.State = StateWatching
	if err := store.Update(ctx, done); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimQueued(ctx, 100)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("pending request did not survive: %v %v", claimed, err)
	}
	check(StateFetching, 3)
	done = check(StateFetching, 3)
	done.State = StateFailed
	if err := store.Update(ctx, done); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "search.json"), []byte(search), 0600); err != nil {
		t.Fatal(err)
	}
	poll()
	check(StateFailed, 3) // Same request cannot loop forever after a failure.
	fixture(12)
	poll()
	check(StateQueued, 4)
	if err := store.Stop(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	poll()
	check(StateClosed, 4) // Stop consumes the outstanding request.
	fixture(13)
	poll()
	check(StateQueued, 5) // An explicit later request overrides stop.
}

func TestManualReviewQueueIsAtomicAndCachedCardsStayFresh(t *testing.T) {
	store, pool := reviewTestStore(t)
	ctx := context.Background()
	r := &Request{Repo: "review-test/" + mustUUID(), Number: 1, State: StateFailed, Harness: "codex"}
	if err := store.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM code_review_requests WHERE id=$1", r.ID) })
	r.SessionID = "old-session"
	r.Verdict = VerdictApprove
	if err := store.Update(ctx, r); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Requeue(ctx, r.ID, OriginPaste); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	cards := []PRCard{{PRDetail: PRDetail{Repo: r.Repo, Number: 1}, Review: r}}
	fresh, err := store.CurrentReviews(ctx, cards)
	if err != nil {
		t.Fatal(err)
	}
	if fresh[0].Review.State != StateQueued || fresh[0].Review.Attempt != 2 || fresh[0].Review.SessionID != "" || fresh[0].Review.Verdict != "" {
		t.Fatalf("stale attempt data: %+v", fresh[0].Review)
	}
	if cards[0].Review.State != StateFailed {
		t.Fatal("mutated shared GitHub cache")
	}
}

func TestConcurrentFirstRequestsShareOneQueueEntry(t *testing.T) {
	store, pool := reviewTestStore(t)
	ctx := context.Background()
	repo := "review-test/" + mustUUID()
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM code_review_requests WHERE repo=$1", repo) })
	var wg sync.WaitGroup
	ids := make(chan string, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := &Request{Repo: repo, Number: 1}
			if err := store.Create(ctx, r); err != nil {
				t.Error(err)
				return
			}
			ids <- r.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for got := range ids {
		if id != "" && got != id {
			t.Fatal("created duplicate queue entries")
		}
		id = got
	}
	r, err := store.GetByRepoNumber(ctx, repo, 1)
	if err != nil || r == nil || r.State != StateQueued || r.Attempt != 1 {
		t.Fatalf("request=%+v err=%v", r, err)
	}
}
