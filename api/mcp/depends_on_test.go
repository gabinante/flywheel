package mcp

import (
	"reflect"
	"testing"
)

func TestGetStringSlice_NativeArray(t *testing.T) {
	// MCP clients typically send depends_on as a native JSON array,
	// which the SDK deserializes to []interface{}.
	args := map[string]any{
		"depends_on": []interface{}{"ticket-1", "ticket-2"},
	}
	got := getStringSlice(args, "depends_on")
	want := []string{"ticket-1", "ticket-2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("getStringSlice(native array) = %v, want %v", got, want)
	}
}

func TestGetStringSlice_JSONString(t *testing.T) {
	// Backward compatibility: some clients may send a JSON-encoded string.
	args := map[string]any{
		"depends_on": `["ticket-a", "ticket-b"]`,
	}
	got := getStringSlice(args, "depends_on")
	want := []string{"ticket-a", "ticket-b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("getStringSlice(JSON string) = %v, want %v", got, want)
	}
}

func TestGetStringSlice_EmptyArray(t *testing.T) {
	args := map[string]any{
		"depends_on": []interface{}{},
	}
	got := getStringSlice(args, "depends_on")
	if got == nil || len(got) != 0 {
		t.Errorf("getStringSlice(empty array) = %v, want empty non-nil slice", got)
	}
}

func TestGetStringSlice_EmptyJSONString(t *testing.T) {
	args := map[string]any{
		"depends_on": `[]`,
	}
	got := getStringSlice(args, "depends_on")
	if got == nil || len(got) != 0 {
		t.Errorf("getStringSlice(empty JSON string) = %v, want empty non-nil slice", got)
	}
}

func TestGetStringSlice_Missing(t *testing.T) {
	args := map[string]any{
		"other_key": "value",
	}
	got := getStringSlice(args, "depends_on")
	if got != nil {
		t.Errorf("getStringSlice(missing key) = %v, want nil", got)
	}
}

func TestGetStringSlice_Nil(t *testing.T) {
	args := map[string]any{
		"depends_on": nil,
	}
	got := getStringSlice(args, "depends_on")
	if got != nil {
		t.Errorf("getStringSlice(nil value) = %v, want nil", got)
	}
}

func TestGetStringSlice_NilArgs(t *testing.T) {
	got := getStringSlice(nil, "depends_on")
	if got != nil {
		t.Errorf("getStringSlice(nil args) = %v, want nil", got)
	}
}

func TestGetStringSlice_EmptyString(t *testing.T) {
	args := map[string]any{
		"depends_on": "",
	}
	got := getStringSlice(args, "depends_on")
	if got != nil {
		t.Errorf("getStringSlice(empty string) = %v, want nil", got)
	}
}

func TestGetStringSlice_InvalidJSONString(t *testing.T) {
	args := map[string]any{
		"depends_on": "not-json",
	}
	got := getStringSlice(args, "depends_on")
	if got != nil {
		t.Errorf("getStringSlice(invalid JSON) = %v, want nil", got)
	}
}

func TestGetStringSlice_SingleElement(t *testing.T) {
	args := map[string]any{
		"depends_on": []interface{}{"only-one"},
	}
	got := getStringSlice(args, "depends_on")
	want := []string{"only-one"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("getStringSlice(single element) = %v, want %v", got, want)
	}
}
