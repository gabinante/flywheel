package environment

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Development", "development"},
		{"My Environment", "my-environment"},
		{"Staging (EU)", "staging-eu"},
		{"prod-v2", "prod-v2"},
		{"  Spaces  ", "spaces"},
		{"UPPERCASE", "uppercase"},
		{"Special!@#Chars", "specialchars"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
