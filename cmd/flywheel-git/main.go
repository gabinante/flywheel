package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gabinante/flywheel/internal/gitnotes"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	repoPath := "."
	if d := os.Getenv("GIT_DIR"); d != "" {
		// GIT_DIR is .git when in repo; repo is parent
		repoPath = filepath.Dir(d)
	} else if wd, err := os.Getwd(); err == nil {
		repoPath = wd
	}

	switch os.Args[1] {
	case "note":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "add":
			runNoteAdd(repoPath, os.Args[3:])
		case "show":
			runNoteShow(repoPath, os.Args[3:])
		case "log":
			runNoteLog(repoPath, os.Args[3:])
		case "diff":
			runNoteDiff(repoPath, os.Args[3:])
		default:
			printUsage()
			os.Exit(1)
		}
	case "sync":
		runSync(repoPath, os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `flywheel-git — Flywheel Git Notes CLI

Usage:
  flywheel-git note add  -t <type> -m <message> [-c commit]   Add a note (type: decision|trace|intent)
  flywheel-git note show [-t type] [-c commit]                Show note(s) for a commit
  flywheel-git note log   [-t type] [-n limit]                List commits with notes
  flywheel-git note diff  [-t type] <base> <head>             Notes in base..head
  flywheel-git sync       [push|pull|both]                     Push/pull refs/notes/flywheel/*

Defaults: repo=., commit=HEAD, type=decision, limit=20.
`)
}

// NotePayload is the schema we validate for add (matches design doc).
type NotePayload struct {
	V         int             `json:"v"`
	Type      string          `json:"type"`
	Message   string          `json:"message"`
	AgentID   string          `json:"agent_id,omitempty"`
	TicketID  string          `json:"ticket_id,omitempty"`
	ProjectID string          `json:"project_id,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

func parseFlags(args []string) (map[string]string, []string) {
	flags := make(map[string]string)
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if (a == "-t" || a == "-m" || a == "-c" || a == "-n") && i+1 < len(args) {
			key := a[1:]
			flags[key] = args[i+1]
			i++
			continue
		}
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			// -tvalue
			for j := 1; j < len(a); j++ {
				if j+1 < len(a) && a[j+1] != '=' {
					continue
				}
				if j+1 < len(a) {
					flags[string(a[j])] = a[j+2:]
				}
				break
			}
			continue
		}
		rest = append(rest, a)
	}
	return flags, rest
}

func runNoteAdd(repoPath string, args []string) {
	flags, _ := parseFlags(args)
	noteType := flags["t"]
	if noteType == "" {
		noteType = gitnotes.TypeDecision
	}
	if gitnotes.RefForType(noteType) == "" {
		fmt.Fprintf(os.Stderr, "flywheel-git: type must be decision, trace, or intent\n")
		os.Exit(1)
	}
	message := flags["m"]
	if message == "" {
		fmt.Fprintf(os.Stderr, "flywheel-git: -m <message> required\n")
		os.Exit(1)
	}
	commit := flags["c"]
	if commit == "" {
		commit = "HEAD"
	}

	payload := NotePayload{
		V:         1,
		Type:      noteType,
		Message:   message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
		os.Exit(1)
	}

	ref := gitnotes.RefForType(noteType)
	if err := gitnotes.AddNote(repoPath, ref, commit, string(body)); err != nil {
		fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Added %s note to %s\n", noteType, commit)
}

func runNoteShow(repoPath string, args []string) {
	flags, _ := parseFlags(args)
	noteType := flags["t"]
	commit := flags["c"]
	if commit == "" {
		commit = "HEAD"
	}

	if noteType != "" {
		ref := gitnotes.RefForType(noteType)
		if ref == "" {
			fmt.Fprintf(os.Stderr, "flywheel-git: type must be decision, trace, or intent\n")
			os.Exit(1)
		}
		body, err := gitnotes.ShowNote(repoPath, ref, commit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
			os.Exit(1)
		}
		if body == "" {
			fmt.Printf("(no %s note for %s)\n", noteType, commit)
			return
		}
		fmt.Println(body)
		return
	}

	for _, ref := range gitnotes.AllRefs() {
		body, err := gitnotes.ShowNote(repoPath, ref, commit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
			os.Exit(1)
		}
		if body != "" {
			fmt.Printf("--- %s ---\n%s\n", filepath.Base(ref), body)
		}
	}
}

func runNoteLog(repoPath string, args []string) {
	flags, _ := parseFlags(args)
	noteType := flags["t"]
	if noteType == "" {
		noteType = gitnotes.TypeDecision
	}
	ref := gitnotes.RefForType(noteType)
	if ref == "" {
		fmt.Fprintf(os.Stderr, "flywheel-git: type must be decision, trace, or intent\n")
		os.Exit(1)
	}
	limit := 20
	if n := flags["n"]; n != "" {
		if i, err := strconv.Atoi(n); err == nil && i > 0 {
			limit = i
		}
	}

	entries, err := gitnotes.Log(repoPath, ref, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
		os.Exit(1)
	}
	for _, e := range entries {
		fmt.Printf("%s\n%s\n---\n", e.CommitSHA, e.Body)
	}
}

func runNoteDiff(repoPath string, args []string) {
	flags, rest := parseFlags(args)
	noteType := flags["t"]
	if noteType == "" {
		noteType = gitnotes.TypeDecision
	}
	ref := gitnotes.RefForType(noteType)
	if ref == "" {
		fmt.Fprintf(os.Stderr, "flywheel-git: type must be decision, trace, or intent\n")
		os.Exit(1)
	}
	if len(rest) < 2 {
		fmt.Fprintf(os.Stderr, "flywheel-git: diff requires <base> <head>\n")
		os.Exit(1)
	}
	base, head := rest[0], rest[1]

	entries, err := gitnotes.Diff(repoPath, ref, base, head)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
		os.Exit(1)
	}
	for _, e := range entries {
		fmt.Printf("%s\n%s\n---\n", e.CommitSHA, e.Body)
	}
}

func runSync(repoPath string, args []string) {
	dir := "."
	if abs, err := filepath.Abs(repoPath); err == nil {
		dir = abs
	}
	direction := "both"
	if len(args) > 0 {
		direction = args[0]
	}
	switch direction {
	case "push":
		if err := pushNotes(dir); err != nil {
			fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
			os.Exit(1)
		}
	case "pull":
		if err := pullNotes(dir); err != nil {
			fmt.Fprintf(os.Stderr, "flywheel-git: %v\n", err)
			os.Exit(1)
		}
	case "both":
		if err := pullNotes(dir); err != nil {
			fmt.Fprintf(os.Stderr, "flywheel-git pull: %v\n", err)
			os.Exit(1)
		}
		if err := pushNotes(dir); err != nil {
			fmt.Fprintf(os.Stderr, "flywheel-git push: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "flywheel-git: sync expects push, pull, or both\n")
		os.Exit(1)
	}
	fmt.Println("Sync done.")
}

func pushNotes(repoPath string) error {
	for _, ref := range gitnotes.AllRefs() {
		cmd := exec.Command("git", "-C", repoPath, "push", "origin", ref)
		if out, err := cmd.CombinedOutput(); err != nil {
			// Ignore "everything up-to-date" or no remote
			if len(out) > 0 {
				return fmt.Errorf("push %s: %w: %s", ref, err, out)
			}
			return fmt.Errorf("push %s: %w", ref, err)
		}
	}
	return nil
}

func pullNotes(repoPath string) error {
	for _, ref := range gitnotes.AllRefs() {
		cmd := exec.Command("git", "-C", repoPath, "fetch", "origin", ref+":"+ref)
		if out, err := cmd.CombinedOutput(); err != nil {
			// Ref might not exist on remote yet
			if bytes.Contains(out, []byte("couldn't find")) {
				continue
			}
			return fmt.Errorf("fetch %s: %w: %s", ref, err, out)
		}
	}
	return nil
}
