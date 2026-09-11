package codereview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewDiscussionAllPagesAndThreadState(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
cd "$(dirname "$0")"
case "$*" in
 *--paginate*) ;;
 *) exit 9 ;;
esac
case "$*" in
 *issues/*) echo '[{"id":1,"body":"Intentional tradeoff"}]' ;;
 *reviews\?*) echo '[{"id":2,"state":"CHANGES_REQUESTED","body":"Original review"}]' ;;
 *comments\?*) printf '%s\n' '[{"id":3,"node_id":"root","body":"Concern"}]' '[{"id":4,"in_reply_to_id":3,"body":"The invariant prevents this","user":{"login":"author"}}]' ;;
 *graphql*) cat threads.json ;;
 *) exit 8 ;;
esac
`
	bin := filepath.Join(dir, "gh")
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	threads := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"id":"thread","isResolved":true,"isOutdated":true,"comments":{"nodes":[{"id":"root"}]}}]}}}}}`
	if err := os.WriteFile(filepath.Join(dir, "threads.json"), []byte(threads+"\n"+`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	gh := NewGitHub(bin)
	d, err := gh.ReviewDiscussion(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Inline) != 2 || d.Inline[1].ReplyTo != 3 || d.Inline[1].Body != "The invariant prevents this" {
		t.Fatalf("Lost paginated reply: %+v", d.Inline)
	}
	if len(d.Threads) != 1 || !d.Threads[0].IsResolved || !d.Threads[0].IsOutdated || d.Threads[0].Comments.Nodes[0].ID != d.Inline[0].NodeID {
		t.Fatalf("Lost thread state: %+v", d.Threads)
	}
	if len(d.Comments) != 1 || len(d.Reviews) != 1 {
		t.Fatal("Missing conversation/review bodies")
	}
	if err := os.WriteFile(filepath.Join(dir, "threads.json"), []byte(`{"errors":[{"message":"Unavailable"}],"data":{"repository":null}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := gh.ReviewDiscussion(context.Background(), "owner/repo", 1); err == nil {
		t.Fatal("Partial GitHub response accepted")
	}
}

func TestReviewPromptReassessesReplies(t *testing.T) {
	prompt := BuildReviewPrompt(&PR{Repo: "owner/repo", Number: 1}, "/tmp/diff", nil, []Finding{{Title: "Race", Body: "Detailed original reasoning", GitHubCommentID: 42}}, "/tmp/discussion.json")
	for _, part := range []string{"/tmp/discussion.json", "Detailed original reasoning", "GitHub comment 42", "convincing evidence", "untrusted evidence", "Resolved/outdated status alone"} {
		if !strings.Contains(prompt, part) {
			t.Errorf("missing %q", part)
		}
	}
}
