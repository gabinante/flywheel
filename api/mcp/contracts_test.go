package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestContractVersion_String(t *testing.T) {
	v := ContractVersion{Major: 1, Minor: 2, Patch: 3}
	if got := v.String(); got != "1.2.3" {
		t.Errorf("String() = %q, want %q", got, "1.2.3")
	}
}

func TestContractVersion_Compatible(t *testing.T) {
	tests := []struct {
		name     string
		contract ContractVersion
		provider ContractVersion
		want     bool
	}{
		{"exact match", ContractVersion{1, 0, 0}, ContractVersion{1, 0, 0}, true},
		{"provider newer minor", ContractVersion{1, 0, 0}, ContractVersion{1, 1, 0}, true},
		{"provider newer patch", ContractVersion{1, 0, 0}, ContractVersion{1, 0, 1}, true},
		{"provider older minor", ContractVersion{1, 1, 0}, ContractVersion{1, 0, 0}, false},
		{"different major", ContractVersion{2, 0, 0}, ContractVersion{1, 0, 0}, false},
		{"provider major ahead", ContractVersion{1, 0, 0}, ContractVersion{2, 0, 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.contract.Compatible(tt.provider); got != tt.want {
				t.Errorf("Compatible(%v, %v) = %v, want %v", tt.contract, tt.provider, got, tt.want)
			}
		})
	}
}

func TestPluginContract_RequiredTools(t *testing.T) {
	contract := CodeIntelligenceContract
	required := contract.RequiredTools()

	// Code intelligence has 5 required tools
	if len(required) != 5 {
		t.Errorf("CodeIntelligence RequiredTools() count = %d, want 5", len(required))
	}

	// Verify all are marked required
	for _, tool := range required {
		if !tool.Required {
			t.Errorf("RequiredTools() returned non-required tool: %s", tool.Name)
		}
	}
}

func TestPluginContract_OptionalTools(t *testing.T) {
	contract := CodeIntelligenceContract
	optional := contract.OptionalTools()

	// Code intelligence has 2 optional tools
	if len(optional) != 2 {
		t.Errorf("CodeIntelligence OptionalTools() count = %d, want 2", len(optional))
	}

	for _, tool := range optional {
		if tool.Required {
			t.Errorf("OptionalTools() returned required tool: %s", tool.Name)
		}
	}
}

func TestPluginContract_RequiredResources(t *testing.T) {
	contract := CodeIntelligenceContract
	required := contract.RequiredResources()

	if len(required) != 1 {
		t.Errorf("CodeIntelligence RequiredResources() count = %d, want 1", len(required))
	}
}

func TestAllContracts(t *testing.T) {
	contracts := AllContracts()

	if len(contracts) != 6 {
		t.Fatalf("AllContracts() returned %d contracts, want 6", len(contracts))
	}

	expectedNames := []string{
		"code_intelligence",
		"catalog",
		"state_index",
		"findings",
		"signal_ingestion",
		"notification",
	}

	for i, name := range expectedNames {
		if contracts[i].Name != name {
			t.Errorf("AllContracts()[%d].Name = %q, want %q", i, contracts[i].Name, name)
		}
	}

	// Verify each contract has at least one required tool
	for _, c := range contracts {
		if len(c.RequiredTools()) == 0 {
			t.Errorf("Contract %q has no required tools", c.Name)
		}
	}

	// Verify all contracts have version 1.0.0
	for _, c := range contracts {
		if c.Version.Major != 1 || c.Version.Minor != 0 || c.Version.Patch != 0 {
			t.Errorf("Contract %q version = %s, want 1.0.0", c.Name, c.Version)
		}
	}
}

func TestAllContracts_UniqueToolNames(t *testing.T) {
	// All tool names across all contracts should be unique
	seen := make(map[string]string) // tool name -> contract name
	for _, contract := range AllContracts() {
		for _, tool := range contract.Tools {
			if prev, exists := seen[tool.Name]; exists {
				t.Errorf("Duplicate tool name %q in contracts %q and %q", tool.Name, prev, contract.Name)
			}
			seen[tool.Name] = contract.Name
		}
	}
}

func TestAllContracts_ToolSchemasValid(t *testing.T) {
	for _, contract := range AllContracts() {
		for _, tool := range contract.Tools {
			schema := tool.InputSchema
			if schema == nil {
				t.Errorf("Contract %q tool %q has nil InputSchema", contract.Name, tool.Name)
				continue
			}
			// Must be a JSON Schema object
			if typ, ok := schema["type"]; !ok || typ != "object" {
				t.Errorf("Contract %q tool %q schema type = %v, want \"object\"", contract.Name, tool.Name, typ)
			}
			// Must have properties
			if _, ok := schema["properties"]; !ok {
				t.Errorf("Contract %q tool %q schema missing properties", contract.Name, tool.Name)
			}
			// Must have additionalProperties: false
			if ap, ok := schema["additionalProperties"]; !ok || ap != false {
				t.Errorf("Contract %q tool %q schema additionalProperties = %v, want false", contract.Name, tool.Name, ap)
			}
			// Must have required array (at minimum project_id)
			if req, ok := schema["required"]; !ok || req == nil {
				t.Errorf("Contract %q tool %q schema missing required fields", contract.Name, tool.Name)
			}
		}
	}
}

func TestAllContracts_Layers(t *testing.T) {
	expectedLayers := map[string]int{
		"code_intelligence": 3,
		"findings":          4,
		"catalog":           14,
		"state_index":       10,
		"signal_ingestion":  11,
		"notification":      12,
	}

	for _, c := range AllContracts() {
		expected, ok := expectedLayers[c.Name]
		if !ok {
			t.Errorf("Unexpected contract %q", c.Name)
			continue
		}
		if c.Layer != expected {
			t.Errorf("Contract %q layer = %d, want %d", c.Name, c.Layer, expected)
		}
	}
}

// --------------------------------------------------------------------------
// Plugin Registry Tests
// --------------------------------------------------------------------------

func TestPluginRegistry_RegisterCodeIntelligence(t *testing.T) {
	reg := NewPluginRegistry()
	provider := NewTreeSitterCodeIntel()

	if err := reg.RegisterCodeIntelligence(provider); err != nil {
		t.Fatalf("RegisterCodeIntelligence() error = %v", err)
	}

	if reg.CodeIntelligence() == nil {
		t.Error("CodeIntelligence() returned nil after registration")
	}
}

func TestPluginRegistry_VersionMismatch(t *testing.T) {
	reg := NewPluginRegistry()
	provider := &mockIncompatibleProvider{}

	err := reg.RegisterCodeIntelligence(provider)
	if err == nil {
		t.Error("RegisterCodeIntelligence() should fail with incompatible version")
	}
}

type mockIncompatibleProvider struct{ TreeSitterCodeIntel }

func (m *mockIncompatibleProvider) ContractVersion() ContractVersion {
	return ContractVersion{Major: 2, Minor: 0, Patch: 0} // Major mismatch
}

// --------------------------------------------------------------------------
// Tree-sitter Default Implementation Tests
// --------------------------------------------------------------------------

func TestTreeSitterCodeIntel_ContractVersion(t *testing.T) {
	ci := NewTreeSitterCodeIntel()
	v := ci.ContractVersion()
	if v.Major != 1 || v.Minor != 0 || v.Patch != 0 {
		t.Errorf("ContractVersion() = %s, want 1.0.0", v)
	}
}

func TestTreeSitterCodeIntel_StatusUnindexed(t *testing.T) {
	ci := NewTreeSitterCodeIntel()
	ctx := context.Background()

	status, err := ci.Status(ctx, "test-project")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if !status.Stale {
		t.Error("Status() should report stale for unindexed project")
	}
	if status.ProjectID != "test-project" {
		t.Errorf("Status().ProjectID = %q, want %q", status.ProjectID, "test-project")
	}
}

func TestTreeSitterCodeIntel_IndexAndLookup(t *testing.T) {
	ci := NewTreeSitterCodeIntel()
	ctx := context.Background()

	// Create temp directory with test files
	tmpDir := t.TempDir()

	goFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(goFile, []byte(`package main

import "fmt"

func HelloWorld() {
	fmt.Println("hello")
}

func privateHelper() {
}

type MyService struct {
	Name string
}

type Runnable interface {
	Run()
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	pyFile := filepath.Join(tmpDir, "app.py")
	if err := os.WriteFile(pyFile, []byte(`import os
from pathlib import Path

def process_data(input):
    pass

class DataProcessor:
    pass

def _internal():
    pass
`), 0644); err != nil {
		t.Fatal(err)
	}

	tsFile := filepath.Join(tmpDir, "server.ts")
	if err := os.WriteFile(tsFile, []byte(`import { Router } from 'express'

export function createServer() {}

export interface Config {
  port: number
}

function internalHelper() {}

export class Application {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Index the directory
	if err := ci.IndexDirectory(ctx, "test-project", tmpDir, "abc123"); err != nil {
		t.Fatalf("IndexDirectory() error = %v", err)
	}

	// Verify status
	status, err := ci.Status(ctx, "test-project")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.TotalSymbols == 0 {
		t.Error("Expected symbols after indexing")
	}
	if status.LastCommitSHA != "abc123" {
		t.Errorf("LastCommitSHA = %q, want %q", status.LastCommitSHA, "abc123")
	}

	// Test SymbolLookup - find Go function
	results, err := ci.SymbolLookup(ctx, "test-project", "HelloWorld", SymbolLookupOpts{})
	if err != nil {
		t.Fatalf("SymbolLookup() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SymbolLookup('HelloWorld') returned no results")
	}
	if results[0].Name != "HelloWorld" {
		t.Errorf("SymbolLookup result name = %q, want %q", results[0].Name, "HelloWorld")
	}
	if results[0].Kind != "function" {
		t.Errorf("SymbolLookup result kind = %q, want %q", results[0].Kind, "function")
	}
	if results[0].Visibility != "public" {
		t.Errorf("SymbolLookup result visibility = %q, want %q", results[0].Visibility, "public")
	}

	// Test language filter
	results, err = ci.SymbolLookup(ctx, "test-project", "process", SymbolLookupOpts{Language: "python"})
	if err != nil {
		t.Fatalf("SymbolLookup(python) error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SymbolLookup with python filter returned no results")
	}
	if results[0].Language != "python" {
		t.Errorf("Expected python language, got %q", results[0].Language)
	}

	// Test kind filter
	results, err = ci.SymbolLookup(ctx, "test-project", "", SymbolLookupOpts{Kind: "interface"})
	if err != nil {
		t.Fatalf("SymbolLookup(interface) error = %v", err)
	}
	// Should find Runnable (Go) and Config (TS)
	if len(results) < 2 {
		t.Errorf("Expected at least 2 interfaces, got %d", len(results))
	}

	// Test private visibility detection
	results, err = ci.SymbolLookup(ctx, "test-project", "privateHelper", SymbolLookupOpts{})
	if err != nil {
		t.Fatalf("SymbolLookup(private) error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SymbolLookup('privateHelper') returned no results")
	}
	if results[0].Visibility != "private" {
		t.Errorf("privateHelper visibility = %q, want %q", results[0].Visibility, "private")
	}
}

func TestTreeSitterCodeIntel_SymbolLookup_NotIndexed(t *testing.T) {
	ci := NewTreeSitterCodeIntel()
	ctx := context.Background()

	_, err := ci.SymbolLookup(ctx, "nonexistent", "foo", SymbolLookupOpts{})
	if err == nil {
		t.Error("SymbolLookup() should error for non-indexed project")
	}
}

func TestTreeSitterCodeIntel_Importers(t *testing.T) {
	ci := NewTreeSitterCodeIntel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(goFile, []byte(`package main

import "fmt"
import "os"
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ci.IndexDirectory(ctx, "test-project", tmpDir, "abc123"); err != nil {
		t.Fatalf("IndexDirectory() error = %v", err)
	}

	importers, err := ci.Importers(ctx, "test-project", "fmt", 10)
	if err != nil {
		t.Fatalf("Importers() error = %v", err)
	}
	if len(importers) == 0 {
		t.Error("Expected to find importers of fmt")
	}
}

func TestTreeSitterCodeIntel_InterfaceCompliance(t *testing.T) {
	// Compile-time check that TreeSitterCodeIntel implements both interfaces
	var _ CodeIntelligenceProvider = (*TreeSitterCodeIntel)(nil)
	var _ CodeIntelligenceExtended = (*TreeSitterCodeIntel)(nil)
}

func TestTreeSitterCodeIntel_SkipsHiddenDirs(t *testing.T) {
	ci := NewTreeSitterCodeIntel()
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create a file in the root
	rootFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(rootFile, []byte(`package main
func Visible() {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create hidden dir with a file
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	os.MkdirAll(hiddenDir, 0755)
	hiddenFile := filepath.Join(hiddenDir, "secret.go")
	if err := os.WriteFile(hiddenFile, []byte(`package hidden
func ShouldNotAppear() {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ci.IndexDirectory(ctx, "test-project", tmpDir, "abc123"); err != nil {
		t.Fatalf("IndexDirectory() error = %v", err)
	}

	results, err := ci.SymbolLookup(ctx, "test-project", "ShouldNotAppear", SymbolLookupOpts{})
	if err != nil {
		t.Fatalf("SymbolLookup() error = %v", err)
	}
	if len(results) != 0 {
		t.Error("Should not index files in hidden directories")
	}
}
