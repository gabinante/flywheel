package gitnotes

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AddNote adds a note to the given commit in the repo at repoPath.
// ref is the full notes ref (e.g. refs/notes/flywheel/decision).
// body is the note content (e.g. JSON); commitSHA is the commit to attach the note to (e.g. "HEAD" or full SHA).
func AddNote(repoPath, ref, commitSHA, body string) error {
	if repoPath == "" || ref == "" || commitSHA == "" {
		return fmt.Errorf("gitnotes: repoPath, ref, and commitSHA are required")
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("gitnotes: resolve repo path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return fmt.Errorf("gitnotes: not a git repo: %w", err)
	}

	tmp, err := os.CreateTemp("", "flywheel-git-note-*.txt")
	if err != nil {
		return fmt.Errorf("gitnotes: create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(body); err != nil {
		return fmt.Errorf("gitnotes: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("gitnotes: close temp file: %w", err)
	}

	cmd := exec.Command("git", "-C", abs, "notes", "--ref="+ref, "add", "-F", tmp.Name(), commitSHA)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git notes add: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

// ShowNote returns the note body for the given commit and ref, or empty string if no note exists.
func ShowNote(repoPath, ref, commitSHA string) (string, error) {
	if repoPath == "" || ref == "" || commitSHA == "" {
		return "", fmt.Errorf("gitnotes: repoPath, ref, and commitSHA are required")
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("gitnotes: resolve repo path: %w", err)
	}

	cmd := exec.Command("git", "-C", abs, "notes", "--ref="+ref, "show", commitSHA)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// No note for this object is not an error
		s := strings.ToLower(string(out))
		if strings.Contains(s, "no note found") || strings.Contains(s, "object not found") {
			return "", nil
		}
		return "", fmt.Errorf("git notes show: %w: %s", err, bytes.TrimSpace(out))
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// LogEntry is one commit with its note body for a given ref.
type LogEntry struct {
	CommitSHA string `json:"commit_sha"`
	Ref       string `json:"ref"`
	Body      string `json:"body"`
}

// Log returns up to limit (commit, note) pairs for the given ref.
// Order is that of `git notes list` (implementation-defined).
func Log(repoPath, ref string, limit int) ([]LogEntry, error) {
	if repoPath == "" || ref == "" {
		return nil, fmt.Errorf("gitnotes: repoPath and ref are required")
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("gitnotes: resolve repo path: %w", err)
	}

	cmd := exec.Command("git", "-C", abs, "notes", "--ref="+ref, "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git notes list: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	var entries []LogEntry
	for i, line := range lines {
		if limit >= 0 && (limit == 0 || i >= limit) {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// git notes list: "note_sha object_sha" (note object, then object the note is attached to = commit)
		commitSHA := strings.TrimSpace(fields[1])
		body, err := ShowNote(repoPath, ref, commitSHA)
		if err != nil {
			return nil, fmt.Errorf("gitnotes log: show %s: %w", commitSHA, err)
		}
		entries = append(entries, LogEntry{CommitSHA: commitSHA, Ref: ref, Body: body})
	}
	return entries, nil
}

// DiffEntry is a commit in base..head with its note body for a given ref.
type DiffEntry struct {
	CommitSHA string `json:"commit_sha"`
	Ref       string `json:"ref"`
	Body      string `json:"body"`
}

// Diff returns notes for commits in the range base..head for the given ref.
// base and head are commit-ish (e.g. "main", "HEAD").
func Diff(repoPath, ref, base, head string) ([]DiffEntry, error) {
	if repoPath == "" || ref == "" || base == "" || head == "" {
		return nil, fmt.Errorf("gitnotes: repoPath, ref, base, and head are required")
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("gitnotes: resolve repo path: %w", err)
	}

	cmd := exec.Command("git", "-C", abs, "log", "--format=%H", base+".."+head)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	var entries []DiffEntry
	for _, line := range lines {
		commitSHA := strings.TrimSpace(line)
		if commitSHA == "" {
			continue
		}
		body, err := ShowNote(repoPath, ref, commitSHA)
		if err != nil {
			return nil, fmt.Errorf("gitnotes diff: show %s: %w", commitSHA, err)
		}
		if body != "" {
			entries = append(entries, DiffEntry{CommitSHA: commitSHA, Ref: ref, Body: body})
		}
	}
	return entries, nil
}
