package mcp

import (
	"testing"
)

func TestGetStringArray(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		key  string
		want []string
	}{
		{
			name: "nil args",
			args: nil,
			key:  "depends_on",
			want: nil,
		},
		{
			name: "missing key",
			args: map[string]any{"other": "val"},
			key:  "depends_on",
			want: nil,
		},
		{
			name: "nil value",
			args: map[string]any{"depends_on": nil},
			key:  "depends_on",
			want: nil,
		},
		{
			name: "native JSON array ([]interface{}) - typical MCP client",
			args: map[string]any{
				"depends_on": []interface{}{"ticket-1", "ticket-2"},
			},
			key:  "depends_on",
			want: []string{"ticket-1", "ticket-2"},
		},
		{
			name: "native JSON array with single element",
			args: map[string]any{
				"depends_on": []interface{}{"lightning-talks-ai-adjacent-topics-21"},
			},
			key:  "depends_on",
			want: []string{"lightning-talks-ai-adjacent-topics-21"},
		},
		{
			name: "empty native array",
			args: map[string]any{
				"depends_on": []interface{}{},
			},
			key:  "depends_on",
			want: []string{},
		},
		{
			name: "JSON-encoded string array - legacy format",
			args: map[string]any{
				"depends_on": `["ticket-1","ticket-2"]`,
			},
			key:  "depends_on",
			want: []string{"ticket-1", "ticket-2"},
		},
		{
			name: "JSON-encoded string with single element",
			args: map[string]any{
				"depends_on": `["lightning-talks-ai-adjacent-topics-21"]`,
			},
			key:  "depends_on",
			want: []string{"lightning-talks-ai-adjacent-topics-21"},
		},
		{
			name: "empty JSON-encoded array",
			args: map[string]any{
				"depends_on": `[]`,
			},
			key:  "depends_on",
			want: []string{},
		},
		{
			name: "[]string value (rare but possible)",
			args: map[string]any{
				"depends_on": []string{"a", "b"},
			},
			key:  "depends_on",
			want: []string{"a", "b"},
		},
		{
			name: "empty string value",
			args: map[string]any{
				"depends_on": "",
			},
			key:  "depends_on",
			want: nil,
		},
		{
			name: "invalid JSON string",
			args: map[string]any{
				"depends_on": "not-json",
			},
			key:  "depends_on",
			want: nil,
		},
		{
			name: "non-string elements in native array are skipped",
			args: map[string]any{
				"depends_on": []interface{}{"ticket-1", 42, "ticket-2"},
			},
			key:  "depends_on",
			want: []string{"ticket-1", "ticket-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStringArray(tt.args, tt.key)
			if tt.want == nil {
				if got != nil {
					t.Errorf("getStringArray() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("getStringArray() len = %d, want %d; got %v", len(got), len(tt.want), got)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("getStringArray()[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}
