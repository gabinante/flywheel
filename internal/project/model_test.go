package project

import "testing"

func TestDispatchConfigNormalizedCustomRoles(t *testing.T) {
	cfg := DispatchConfig{
		MaxActiveWorkers: -1,
		Roles: []DispatchWorkerRole{
			{ID: " Security Review ", Name: " Security Review ", Description: " check auth ", BaseType: "review"},
			{ID: "executor", Name: "Reserved", BaseType: "planner"},
			{ID: "security_review", Name: "Duplicate", BaseType: "planner"},
			{Name: "Release Captain", BaseType: "deployment"},
		},
	}

	got := cfg.Normalized()
	if got.MaxActiveWorkers != 0 {
		t.Fatalf("MaxActiveWorkers = %d, want 0", got.MaxActiveWorkers)
	}
	if len(got.Roles) != 2 {
		t.Fatalf("len(Roles) = %d, want 2: %#v", len(got.Roles), got.Roles)
	}
	if got.Roles[0].ID != "security_review" {
		t.Fatalf("first role ID = %q, want security_review", got.Roles[0].ID)
	}
	if got.Roles[0].Description != "check auth" {
		t.Fatalf("first role description = %q, want trimmed description", got.Roles[0].Description)
	}
	if got.Roles[0].BaseType != "validator" {
		t.Fatalf("first role base = %q, want validator", got.Roles[0].BaseType)
	}
	if got.Roles[1].ID != "release_captain" {
		t.Fatalf("second role ID = %q, want release_captain", got.Roles[1].ID)
	}
	if got.Roles[1].BaseType != "deployer" {
		t.Fatalf("second role base = %q, want deployer", got.Roles[1].BaseType)
	}
}
