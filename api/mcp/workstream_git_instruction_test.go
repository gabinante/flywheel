package mcp

import (
	"testing"

	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/workstream"
)

func TestWorkStreamGitInstruction(t *testing.T) {
	t.Parallel()
	projWithRepo := &project.Project{RepoURL: "git@github.com:org/repo.git"}
	projNoRepo := &project.Project{}

	t.Run("no repo", func(t *testing.T) {
		t.Parallel()
		ws := &workstream.WorkStream{Slug: "foo", Branch: ""}
		if workStreamGitInstruction(projNoRepo, ws) != nil {
			t.Fatal("expected nil without repo_url")
		}
	})

	t.Run("nil project", func(t *testing.T) {
		t.Parallel()
		ws := &workstream.WorkStream{Slug: "foo", Branch: "main"}
		if workStreamGitInstruction(nil, ws) != nil {
			t.Fatal("expected nil project")
		}
	})

	t.Run("checkout when branch set", func(t *testing.T) {
		t.Parallel()
		ws := &workstream.WorkStream{Slug: "foo", Branch: "feature/bar"}
		gi := workStreamGitInstruction(projWithRepo, ws)
		if gi == nil {
			t.Fatal("expected instruction")
		}
		if gi["action"] != "checkout_branch" {
			t.Fatalf("action = %v", gi["action"])
		}
		if gi["suggested_branch"] != "feature/bar" {
			t.Fatalf("suggested_branch = %v", gi["suggested_branch"])
		}
	})

	t.Run("create_or_set when branch empty", func(t *testing.T) {
		t.Parallel()
		ws := &workstream.WorkStream{Slug: "web-ui", Branch: ""}
		gi := workStreamGitInstruction(projWithRepo, ws)
		if gi == nil {
			t.Fatal("expected instruction")
		}
		if gi["action"] != "create_or_set_branch" {
			t.Fatalf("action = %v", gi["action"])
		}
		if gi["suggested_branch"] != "feature/web-ui" {
			t.Fatalf("suggested_branch = %v", gi["suggested_branch"])
		}
	})

	t.Run("empty slug no instruction", func(t *testing.T) {
		t.Parallel()
		ws := &workstream.WorkStream{Slug: "", Branch: ""}
		if workStreamGitInstruction(projWithRepo, ws) != nil {
			t.Fatal("expected nil without slug for create_or_set")
		}
	})
}
