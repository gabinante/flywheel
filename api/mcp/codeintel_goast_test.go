package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGoAST(t *testing.T) {
	// Create a temp Go file to parse.
	dir := t.TempDir()
	goFile := filepath.Join(dir, "example.go")
	src := `package example

import (
	"context"
	"fmt"
)

// Service handles requests.
type Service struct {
	name string
}

// Processor defines the processing contract.
type Processor interface {
	Process(ctx context.Context) error
}

// NewService creates a new Service.
func NewService(name string) *Service {
	fmt.Println("creating", name)
	return &Service{name: name}
}

// Run starts the service.
func (s *Service) Run(ctx context.Context) error {
	helper()
	return nil
}

func helper() {
	fmt.Println("helping")
}
`
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	symbols, edges, imports := parseGoAST(goFile, dir)

	// Check symbols.
	symbolNames := make(map[string]string) // name -> kind
	for _, s := range symbols {
		symbolNames[s.Name] = s.Kind
	}

	wantSymbols := map[string]string{
		"Service":    "type",
		"Processor":  "interface",
		"NewService": "function",
		"Run":        "method",
		"helper":     "function",
	}
	for name, wantKind := range wantSymbols {
		kind, ok := symbolNames[name]
		if !ok {
			t.Errorf("symbol %q not found", name)
			continue
		}
		if kind != wantKind {
			t.Errorf("symbol %q kind = %q, want %q", name, kind, wantKind)
		}
	}

	// Check call edges: NewService calls fmt.Println, Run calls helper, helper calls fmt.Println.
	if len(edges) < 2 {
		t.Errorf("edges count = %d, want >= 2", len(edges))
	}

	// Check that at least one edge from NewService exists.
	foundNewServiceEdge := false
	foundRunEdge := false
	for _, e := range edges {
		if e.callerID == "example.go:NewService" {
			foundNewServiceEdge = true
		}
		if e.callerID == "example.go:Run" {
			foundRunEdge = true
		}
	}
	if !foundNewServiceEdge {
		t.Error("no edge from NewService")
	}
	if !foundRunEdge {
		t.Error("no edge from Run")
	}

	// Check imports.
	if len(imports) < 2 {
		t.Errorf("imports count = %d, want >= 2", len(imports))
	}
	foundCtx := false
	foundFmt := false
	for _, imp := range imports {
		if imp.pkg == "context" {
			foundCtx = true
		}
		if imp.pkg == "fmt" {
			foundFmt = true
		}
	}
	if !foundCtx {
		t.Error("missing 'context' import")
	}
	if !foundFmt {
		t.Error("missing 'fmt' import")
	}

	// Check symbol metadata.
	for _, s := range symbols {
		if s.Name == "NewService" {
			if s.Visibility != "public" {
				t.Errorf("NewService visibility = %q, want public", s.Visibility)
			}
			if s.Language != "go" {
				t.Errorf("NewService language = %q, want go", s.Language)
			}
			if s.Package != "example" {
				t.Errorf("NewService package = %q, want example", s.Package)
			}
			if s.Signature == "" {
				t.Error("NewService signature should not be empty")
			}
			if s.DocComment == "" {
				t.Error("NewService should have doc comment")
			}
		}
		if s.Name == "helper" {
			if s.Visibility != "private" {
				t.Errorf("helper visibility = %q, want private", s.Visibility)
			}
		}
	}
}

func TestParseGoAST_InvalidFile(t *testing.T) {
	// Non-existent file should return empty results.
	symbols, edges, imports := parseGoAST("/nonexistent/file.go", "/nonexistent")
	if len(symbols) != 0 || len(edges) != 0 || len(imports) != 0 {
		t.Error("expected empty results for non-existent file")
	}
}

func TestIndexDirectory_Integration(t *testing.T) {
	// Create a mini project with Go and Python files.
	dir := t.TempDir()

	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	pyFile := filepath.Join(dir, "util.py")
	if err := os.WriteFile(pyFile, []byte(`import os

def process():
    pass

class Handler:
    pass
`), 0644); err != nil {
		t.Fatal(err)
	}

	ci := NewTreeSitterCodeIntel()
	if err := ci.IndexDirectory(nil, "test-proj", dir, "abc123"); err != nil {
		t.Fatalf("IndexDirectory: %v", err)
	}

	// Verify symbols were indexed.
	symbols, err := ci.SymbolLookup(nil, "test-proj", "", SymbolLookupOpts{Limit: 100})
	if err != nil {
		t.Fatalf("SymbolLookup: %v", err)
	}
	if len(symbols) < 3 {
		t.Errorf("symbols count = %d, want >= 3 (main, process, Handler)", len(symbols))
	}

	// Verify status.
	status, err := ci.Status(nil, "test-proj")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.TotalSymbols < 3 {
		t.Errorf("TotalSymbols = %d, want >= 3", status.TotalSymbols)
	}
	if status.LastCommitSHA != "abc123" {
		t.Errorf("LastCommitSHA = %q, want abc123", status.LastCommitSHA)
	}

	// Verify call edges were extracted (Go AST should find fmt.Println call).
	ci.mu.RLock()
	idx := ci.indexes["test-proj"]
	ci.mu.RUnlock()
	if len(idx.edges) == 0 {
		t.Error("expected at least one call edge from Go AST parsing")
	}

	// Verify imports.
	importers, err := ci.Importers(nil, "test-proj", "fmt", 10)
	if err != nil {
		t.Fatalf("Importers: %v", err)
	}
	if len(importers) != 1 {
		t.Errorf("importers of 'fmt' = %d, want 1", len(importers))
	}
}
