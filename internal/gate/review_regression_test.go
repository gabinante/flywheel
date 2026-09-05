package gate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReviewUnavailableChecksMustNotPass(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\necho 'HTTP 401: Bad credentials' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	status, reason := ParseGHPRChecks("https://github.com/example/repo/pull/1")
	if status == ChecksPassed {
		t.Fatalf("authentication error satisfied CI gate: %s", reason)
	}
}
