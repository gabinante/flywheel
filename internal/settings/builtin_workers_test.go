package settings

import (
	"context"
	"github.com/gabinante/flywheel/config"
	"github.com/gabinante/flywheel/internal/project"
	"testing"
)

func TestBuiltinWorkersKeepEditsAndLegacyClients(t *testing.T) {
	defaults := FromConfig(&config.Config{})
	svc, err := Load(context.Background(), nil, defaults)
	if err != nil {
		t.Fatal(err)
	}
	next := svc.Current()
	if len(next.Workers.Workers) != 4 {
		t.Fatalf("got %d built-ins", len(next.Workers.Workers))
	}
	for i := range next.Workers.Workers {
		w := &next.Workers.Workers[i]
		if w.ID == project.BuiltinReview {
			w.Name = "My reviewer"
			w.Model = "custom-model"
			w.SystemPrompt = "Be pragmatic"
		}
	}
	next, err = svc.Update(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	review, _ := next.ReviewConfig()
	if review.Model != "custom-model" || review.PromptPrefix != "Be pragmatic" {
		t.Fatalf("worker edits not applied: %+v", review)
	}
	next.Review.Model = "legacy-model"
	next, err = svc.Update(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if w := next.builtin(project.BuiltinReview); w.Model != "legacy-model" || w.Name != "My reviewer" || w.SystemPrompt != "Be pragmatic" {
		t.Fatalf("lost edits: %+v", w)
	}
	again, err := Load(context.Background(), nil, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Current().Workers.Workers) != 4 {
		t.Fatal("duplicated built-ins")
	}
}

func TestBuiltinWorkersDoNotCaptureLegacyRoles(t *testing.T) {
	s := FromConfig(&config.Config{})
	if _, ok := s.ResolveRole("executor"); ok {
		t.Fatal("built-ins captured implicit custom-worker routing")
	}
	s.Workers.Workers = append(s.Workers.Workers, project.DispatchWorkerProfile{ID: "custom", Enabled: true, Driver: "codex"})
	if w, ok := s.ResolveRole("executor"); !ok || w.WorkerID != "custom" {
		t.Fatalf("custom routing changed: %+v", w)
	}
}
