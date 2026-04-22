package entity

import "testing"

func TestIsValidType(t *testing.T) {
	tests := []struct {
		input Type
		want  bool
	}{
		{TypeService, true},
		{TypeDatastore, true},
		{TypeIntegration, true},
		{TypeTicket, true},
		{TypeFinding, true},
		{Type("unknown"), false},
		{Type(""), false},
	}
	for _, tc := range tests {
		got := IsValidType(tc.input)
		if got != tc.want {
			t.Errorf("IsValidType(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestAllTypes(t *testing.T) {
	types := AllTypes()
	if len(types) != 5 {
		t.Fatalf("expected 5 types, got %d", len(types))
	}
	// Verify all are valid
	for _, tp := range types {
		if !IsValidType(tp) {
			t.Errorf("AllTypes() returned invalid type: %q", tp)
		}
	}
}

func TestEntityIsRetired(t *testing.T) {
	e := &Entity{}
	if e.IsRetired() {
		t.Error("new entity should not be retired")
	}
}

func TestInstanceIsRetired(t *testing.T) {
	i := &Instance{}
	if i.IsRetired() {
		t.Error("new instance should not be retired")
	}
}
