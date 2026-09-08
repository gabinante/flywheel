package sessions

import (
	"reflect"
	"testing"
)

func TestExtractRefs(t *testing.T) {
	text := "review https://github.com/example-org/example-app/pull/20888 in depth; relates to ENG-465 and " +
		"https://linear.app/example-org/issue/OPS-3488/foo. Use UTF-8 and SHA-256. P1 findings only. See ADR-008 at 10:06 GMT-7."
	got := ExtractRefs(text)
	want := []Link{
		{Kind: LinkPR, Ref: "example-org/example-app#20888", Source: LinkSourceInferred},
		{Kind: LinkLinearIssue, Ref: "OPS-3488", Source: LinkSourceInferred},
		{Kind: LinkLinearIssue, Ref: "ENG-465", Source: LinkSourceInferred},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractRefs = %+v, want %+v", got, want)
	}
}

func TestRefsFromBranch(t *testing.T) {
	cases := map[string]string{
		"ops-3488-review-fixes":               "OPS-3488",
		"developer/eng-465-sdg-fix":    "ENG-465",
		"gabrielabinante/bulk-rollouts-latest": "",
		"main":                                 "",
		"codex/pr-336-publication-taxonomy":    "", // "pr" is denylisted
	}
	for branch, want := range cases {
		got := RefsFromBranch(branch)
		if want == "" {
			if len(got) != 0 {
				t.Errorf("RefsFromBranch(%q) = %+v, want none", branch, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Ref != want {
			t.Errorf("RefsFromBranch(%q) = %+v, want %s", branch, got, want)
		}
	}
}

func TestRepoFromOriginURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:example-org/example-app.git":   "example-org/example-app",
		"https://github.com/gabinante/flywheel":      "gabinante/flywheel",
		"https://github.com/gabinante/flywheel.git/": "gabinante/flywheel",
		"ssh://git@gitlab.com/org/tool.git":          "tool",
		"":                                           "",
	}
	for in, want := range cases {
		if got := RepoFromOriginURL(in); got != want {
			t.Errorf("RepoFromOriginURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRepoFromPath(t *testing.T) {
	cases := map[string]string{
		"/Users/g/git/example-app":                          "example-app",
		"/Users/g/git/rle-dp-worktrees/deid-validation": "rle-dp",
		"/Users/g/git/example-app-worktrees/corpus-stats":   "example-app",
		"/Users/g/git/example-app/.claude/worktrees/x":      "example-app",
		"/Users/g": "",
	}
	for in, want := range cases {
		if got := RepoFromPath(in); got != want {
			t.Errorf("RepoFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}
