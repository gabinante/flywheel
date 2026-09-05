package errors

import (
	"fmt"
	"testing"
)

func TestMapErrorPreservesStructuredError(t *testing.T) {
	want := New(CodeConflict, "workflow attempt changed", true)
	for _, err := range []error{want, fmt.Errorf("decision: %w", want)} {
		got := MapError(err)
		if got != want || got.HTTPStatus() != 409 {
			t.Fatalf("lost structured error: %#v", got)
		}
	}
}
