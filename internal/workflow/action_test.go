package workflow

import (
	"context"
	"sort"
	"testing"
)

func TestActionRegistryExecute(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register("greet", func(ctx context.Context, ac ActionContext) ActionResult {
		name, _ := ac.Params["name"].(string)
		return ActionResult{
			Outcome:  "success",
			Metadata: map[string]any{"greeting": "hello " + name},
		}
	})

	result := reg.Execute(context.Background(), "greet", ActionContext{
		TicketID: "t-1",
		Params:   map[string]any{"name": "world"},
	})
	if result.Outcome != "success" {
		t.Fatalf("expected success, got %s", result.Outcome)
	}
	if result.Metadata["greeting"] != "hello world" {
		t.Fatalf("expected 'hello world', got %v", result.Metadata["greeting"])
	}
}

func TestActionRegistryUnknown(t *testing.T) {
	reg := NewActionRegistry()
	result := reg.Execute(context.Background(), "nonexistent", ActionContext{})
	if result.Outcome != "failed" {
		t.Fatalf("expected failed, got %s", result.Outcome)
	}
	if result.Error == "" {
		t.Fatal("expected error message for unknown handler")
	}
}

func TestActionRegistryPanicRecovery(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register("panic", func(ctx context.Context, ac ActionContext) ActionResult {
		panic("something went wrong")
	})

	result := reg.Execute(context.Background(), "panic", ActionContext{})
	if result.Outcome != "failed" {
		t.Fatalf("expected failed, got %s", result.Outcome)
	}
	if result.Error == "" {
		t.Fatal("expected error message from panic recovery")
	}
}

func TestActionRegistryDuplicatePanics(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register("action1", func(ctx context.Context, ac ActionContext) ActionResult {
		return ActionResult{Outcome: "success"}
	})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	reg.Register("action1", func(ctx context.Context, ac ActionContext) ActionResult {
		return ActionResult{Outcome: "success"}
	})
}

func TestActionRegistryHasAndNames(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register("a", func(ctx context.Context, ac ActionContext) ActionResult {
		return ActionResult{Outcome: "success"}
	})
	reg.Register("b", func(ctx context.Context, ac ActionContext) ActionResult {
		return ActionResult{Outcome: "success"}
	})

	if !reg.Has("a") {
		t.Error("expected Has('a') = true")
	}
	if !reg.Has("b") {
		t.Error("expected Has('b') = true")
	}
	if reg.Has("c") {
		t.Error("expected Has('c') = false")
	}

	names := reg.Names()
	sort.Strings(names)
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("expected [a b], got %v", names)
	}
}

func TestParseActionConfig(t *testing.T) {
	cfg, err := ParseActionConfig(map[string]any{
		"action": "notify_slack",
		"params": map[string]any{"channel": "#ops"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Action != "notify_slack" {
		t.Errorf("expected action 'notify_slack', got %q", cfg.Action)
	}
	if cfg.Params["channel"] != "#ops" {
		t.Errorf("expected channel '#ops', got %v", cfg.Params["channel"])
	}
}

func TestPhaseActionValid(t *testing.T) {
	if !IsValidPhaseType(PhaseAction) {
		t.Error("PhaseAction should be valid")
	}

	primary := PrimaryPhaseTypes()
	found := false
	for _, pt := range primary {
		if pt == PhaseAction {
			found = true
			break
		}
	}
	if !found {
		t.Error("PhaseAction should be in PrimaryPhaseTypes")
	}
}
