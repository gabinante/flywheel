package mcp

import (
	"context"
	"testing"
)

// --------------------------------------------------------------------------
// Tests for the findings layer: model, in-memory provider, MCP tool handlers
// --------------------------------------------------------------------------

func TestMemoryFindingsStore_SaveAndGet(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	finding := Finding{
		ProjectID:   "proj-1",
		Claim:       "This function has O(n²) complexity",
		FindingType: "performance",
		Confidence:  0.9,
		CommitSHA:   "abc123",
		SourceType:  "agent_analysis",
		Summary:     "Nested loops in processItems",
		SymbolRefs:  []string{"pkg:processItems"},
		FileRefs:    []string{"internal/processor.go"},
		Tags:        []string{"performance", "optimization"},
	}

	saved, err := store.SaveFinding(ctx, finding)
	if err != nil {
		t.Fatalf("SaveFinding: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if !saved.Valid {
		t.Fatal("expected Valid=true for new finding")
	}
	if saved.CreatedAt == "" {
		t.Fatal("expected non-empty CreatedAt")
	}

	// GetFinding
	got, err := store.GetFinding(ctx, "proj-1", saved.ID)
	if err != nil {
		t.Fatalf("GetFinding: %v", err)
	}
	if got.Claim != "This function has O(n²) complexity" {
		t.Errorf("expected claim to match, got %q", got.Claim)
	}
	if got.FindingType != "performance" {
		t.Errorf("expected finding_type=performance, got %q", got.FindingType)
	}
}

func TestMemoryFindingsStore_SaveValidation(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	// Missing project_id
	_, err := store.SaveFinding(ctx, Finding{Claim: "test"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}

	// Missing claim
	_, err = store.SaveFinding(ctx, Finding{ProjectID: "proj-1"})
	if err == nil {
		t.Fatal("expected error for missing claim")
	}
}

func TestMemoryFindingsStore_QueryFindings(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	// Seed some findings
	findings := []Finding{
		{ProjectID: "proj-1", Claim: "SQL injection vulnerability in login handler", FindingType: "security", Confidence: 0.95, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "SQL injection risk"},
		{ProjectID: "proj-1", Claim: "Nested loops cause O(n²) performance", FindingType: "performance", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "Performance bottleneck"},
		{ProjectID: "proj-1", Claim: "Missing error handling in API layer", FindingType: "code_quality", Confidence: 0.7, CommitSHA: "abc", SourceType: "review_comment", Summary: "Error handling gap"},
		{ProjectID: "proj-2", Claim: "Different project finding", FindingType: "architecture", Confidence: 0.5, CommitSHA: "def", SourceType: "agent_analysis", Summary: "Architecture concern"},
	}

	for _, f := range findings {
		if _, err := store.SaveFinding(ctx, f); err != nil {
			t.Fatalf("SaveFinding: %v", err)
		}
	}

	// Query by text
	results, err := store.QueryFindings(ctx, "proj-1", "SQL injection", FindingsQueryOpts{})
	if err != nil {
		t.Fatalf("QueryFindings: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'SQL injection', got %d", len(results))
	}
	if results[0].FindingType != "security" {
		t.Errorf("expected security finding, got %q", results[0].FindingType)
	}

	// Query with type filter
	results, err = store.QueryFindings(ctx, "proj-1", "performance", FindingsQueryOpts{FindingType: "performance"})
	if err != nil {
		t.Fatalf("QueryFindings: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for performance type, got %d", len(results))
	}

	// Query with limit
	results, err = store.QueryFindings(ctx, "proj-1", "a", FindingsQueryOpts{Limit: 2})
	if err != nil {
		t.Fatalf("QueryFindings: %v", err)
	}
	if len(results) > 2 {
		t.Fatalf("expected at most 2 results, got %d", len(results))
	}

	// Query different project returns nothing
	results, err = store.QueryFindings(ctx, "proj-2", "SQL", FindingsQueryOpts{})
	if err != nil {
		t.Fatalf("QueryFindings: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for proj-2 SQL query, got %d", len(results))
	}
}

func TestMemoryFindingsStore_FindingsBySymbol(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	f1 := Finding{ProjectID: "proj-1", Claim: "claim 1", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s1", SymbolRefs: []string{"pkg:Foo", "pkg:Bar"}}
	f2 := Finding{ProjectID: "proj-1", Claim: "claim 2", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s2", SymbolRefs: []string{"pkg:Bar", "pkg:Baz"}}
	f3 := Finding{ProjectID: "proj-1", Claim: "claim 3", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s3", SymbolRefs: []string{"pkg:Foo"}}

	store.SaveFinding(ctx, f1)
	store.SaveFinding(ctx, f2)
	store.SaveFinding(ctx, f3)

	results, err := store.FindingsBySymbol(ctx, "proj-1", "pkg:Foo", 10)
	if err != nil {
		t.Fatalf("FindingsBySymbol: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 findings for pkg:Foo, got %d", len(results))
	}

	results, err = store.FindingsBySymbol(ctx, "proj-1", "pkg:Baz", 10)
	if err != nil {
		t.Fatalf("FindingsBySymbol: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 finding for pkg:Baz, got %d", len(results))
	}
}

func TestMemoryFindingsStore_FindingsByTicket(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	f1 := Finding{ProjectID: "proj-1", Claim: "claim 1", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s1", TicketRefs: []string{"ticket-1", "ticket-2"}}
	f2 := Finding{ProjectID: "proj-1", Claim: "claim 2", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s2", SourceTicketID: "ticket-1"}
	f3 := Finding{ProjectID: "proj-1", Claim: "claim 3", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s3", TicketRefs: []string{"ticket-3"}}

	store.SaveFinding(ctx, f1)
	store.SaveFinding(ctx, f2)
	store.SaveFinding(ctx, f3)

	results, err := store.FindingsByTicket(ctx, "proj-1", "ticket-1", 10)
	if err != nil {
		t.Fatalf("FindingsByTicket: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 findings for ticket-1, got %d", len(results))
	}

	results, err = store.FindingsByTicket(ctx, "proj-1", "ticket-3", 10)
	if err != nil {
		t.Fatalf("FindingsByTicket: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 finding for ticket-3, got %d", len(results))
	}
}

func TestMemoryFindingsStore_InvalidateFindings(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	f1 := Finding{ProjectID: "proj-1", Claim: "claim 1", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s1", FileRefs: []string{"internal/foo.go", "internal/bar.go"}}
	f2 := Finding{ProjectID: "proj-1", Claim: "claim 2", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s2", FileRefs: []string{"internal/baz.go"}}
	f3 := Finding{ProjectID: "proj-1", Claim: "claim 3", FindingType: "code_quality", Confidence: 0.8, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s3", FileRefs: []string{"internal/foo.go"}}

	saved1, _ := store.SaveFinding(ctx, f1)
	saved2, _ := store.SaveFinding(ctx, f2)
	saved3, _ := store.SaveFinding(ctx, f3)

	// Invalidate findings touching foo.go
	count, err := store.InvalidateFindings(ctx, "proj-1", "def456", []string{"internal/foo.go"})
	if err != nil {
		t.Fatalf("InvalidateFindings: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 invalidated, got %d", count)
	}

	// Verify f1 and f3 are invalid
	got1, _ := store.GetFinding(ctx, "proj-1", saved1.ID)
	if got1.Valid {
		t.Error("expected f1 to be invalidated")
	}
	if got1.InvalidatedAt == "" {
		t.Error("expected f1 InvalidatedAt to be set")
	}
	if got1.InvalidationReason == "" {
		t.Error("expected f1 InvalidationReason to be set")
	}

	got2, _ := store.GetFinding(ctx, "proj-1", saved2.ID)
	if !got2.Valid {
		t.Error("expected f2 to still be valid")
	}

	got3, _ := store.GetFinding(ctx, "proj-1", saved3.ID)
	if got3.Valid {
		t.Error("expected f3 to be invalidated")
	}

	// ValidOnly query should exclude invalidated
	results, _ := store.QueryFindings(ctx, "proj-1", "claim", FindingsQueryOpts{ValidOnly: true})
	if len(results) != 1 {
		t.Fatalf("expected 1 valid finding, got %d", len(results))
	}
	if results[0].ID != saved2.ID {
		t.Errorf("expected the valid finding to be f2, got ID %s", results[0].ID)
	}
}

func TestMemoryFindingsStore_DeleteFinding(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	saved, _ := store.SaveFinding(ctx, Finding{ProjectID: "proj-1", Claim: "test claim", FindingType: "code_quality", Confidence: 0.5, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "test"})

	err := store.DeleteFinding(ctx, "proj-1", saved.ID)
	if err != nil {
		t.Fatalf("DeleteFinding: %v", err)
	}

	_, err = store.GetFinding(ctx, "proj-1", saved.ID)
	if err == nil {
		t.Fatal("expected error after deletion")
	}

	// Delete non-existent
	err = store.DeleteFinding(ctx, "proj-1", "nonexistent")
	if err == nil {
		t.Fatal("expected error deleting non-existent finding")
	}
}

func TestMemoryFindingsStore_FindingsByFile(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	store.SaveFinding(ctx, Finding{ProjectID: "proj-1", Claim: "c1", FindingType: "code_quality", Confidence: 0.5, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s1", FileRefs: []string{"a.go", "b.go"}})
	store.SaveFinding(ctx, Finding{ProjectID: "proj-1", Claim: "c2", FindingType: "code_quality", Confidence: 0.5, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s2", FileRefs: []string{"b.go", "c.go"}})
	store.SaveFinding(ctx, Finding{ProjectID: "proj-1", Claim: "c3", FindingType: "code_quality", Confidence: 0.5, CommitSHA: "abc", SourceType: "agent_analysis", Summary: "s3", FileRefs: []string{"a.go"}})

	results, err := store.FindingsByFile(ctx, "proj-1", "a.go", 10)
	if err != nil {
		t.Fatalf("FindingsByFile: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 findings for a.go, got %d", len(results))
	}
}

func TestMemoryFindingsStore_GetNotFound(t *testing.T) {
	store := NewMemoryFindingsStore()
	ctx := context.Background()

	_, err := store.GetFinding(ctx, "proj-1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent finding")
	}
}

func TestMemoryFindingsStore_ContractVersion(t *testing.T) {
	store := NewMemoryFindingsStore()
	v := store.ContractVersion()
	if v.Major != 1 || v.Minor != 0 || v.Patch != 0 {
		t.Errorf("expected 1.0.0, got %s", v)
	}
}

func TestFindingsContract(t *testing.T) {
	c := FindingsContract
	if c.Name != "findings" {
		t.Errorf("expected name 'findings', got %q", c.Name)
	}
	if c.Layer != 4 {
		t.Errorf("expected layer 4, got %d", c.Layer)
	}
	if c.Version.Major != 1 {
		t.Errorf("expected major version 1, got %d", c.Version.Major)
	}

	required := c.RequiredTools()
	if len(required) != 6 {
		t.Errorf("expected 6 required tools, got %d", len(required))
	}

	optional := c.OptionalTools()
	if len(optional) != 2 {
		t.Errorf("expected 2 optional tools, got %d", len(optional))
	}

	requiredResources := c.RequiredResources()
	if len(requiredResources) != 1 {
		t.Errorf("expected 1 required resource, got %d", len(requiredResources))
	}
}

func TestFindingsPluginRegistration(t *testing.T) {
	registry := NewPluginRegistry()

	store := NewMemoryFindingsStore()
	err := registry.RegisterFindings(store)
	if err != nil {
		t.Fatalf("RegisterFindings: %v", err)
	}

	if registry.Findings() == nil {
		t.Fatal("expected registered findings provider")
	}

	// Verify the provider is the one we registered
	if registry.Findings().ContractVersion().Major != 1 {
		t.Errorf("expected version major 1")
	}
}

func TestAllContractsIncludesFindings(t *testing.T) {
	contracts := AllContracts()
	found := false
	for _, c := range contracts {
		if c.Name == "findings" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("AllContracts() should include findings contract")
	}
}

// --------------------------------------------------------------------------
// MCP Tool handler tests (using in-memory backend)
// --------------------------------------------------------------------------

func newTestBackendWithFindings() *Backend {
	return &Backend{
		Findings: NewMemoryFindingsStore(),
	}
}

func TestHandleFindingsSave(t *testing.T) {
	b := newTestBackendWithFindings()
	ctx := context.Background()

	args := map[string]any{
		"project_id":   "proj-1",
		"claim":        "Function processItems has O(n²) complexity",
		"finding_type": "performance",
		"confidence":   0.9,
		"commit_sha":   "abc123def",
		"source_type":  "agent_analysis",
		"summary":      "Performance issue in processItems",
		"symbol_refs":  []any{"pkg:processItems"},
		"file_refs":    []any{"internal/processor.go"},
		"tags":         []any{"perf", "hot-path"},
	}

	_, payload, err := handleFindingsSave(b, ctx, args)
	if err != nil {
		t.Fatalf("handleFindingsSave: %v", err)
	}

	saved, ok := payload.(*Finding)
	if !ok {
		t.Fatalf("expected *Finding payload, got %T", payload)
	}
	if saved.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if saved.Claim != "Function processItems has O(n²) complexity" {
		t.Errorf("unexpected claim: %q", saved.Claim)
	}
	if saved.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", saved.Confidence)
	}
	if len(saved.SymbolRefs) != 1 || saved.SymbolRefs[0] != "pkg:processItems" {
		t.Errorf("unexpected symbol_refs: %v", saved.SymbolRefs)
	}
}

func TestHandleFindingsQuery(t *testing.T) {
	b := newTestBackendWithFindings()
	ctx := context.Background()

	// Seed a finding
	handleFindingsSave(b, ctx, map[string]any{
		"project_id":   "proj-1",
		"claim":        "SQL injection risk in login endpoint",
		"finding_type": "security",
		"confidence":   0.95,
		"commit_sha":   "abc",
		"source_type":  "agent_analysis",
		"summary":      "SQL injection vulnerability",
	})

	args := map[string]any{
		"project_id": "proj-1",
		"query":      "SQL injection",
	}

	_, payload, err := handleFindingsQuery(b, ctx, args)
	if err != nil {
		t.Fatalf("handleFindingsQuery: %v", err)
	}

	result, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", payload)
	}
	if result["count"].(int) != 1 {
		t.Fatalf("expected 1 result, got %d", result["count"].(int))
	}
}

func TestHandleFindingsBySymbol(t *testing.T) {
	b := newTestBackendWithFindings()
	ctx := context.Background()

	handleFindingsSave(b, ctx, map[string]any{
		"project_id":   "proj-1",
		"claim":        "test claim",
		"finding_type": "code_quality",
		"confidence":   0.8,
		"commit_sha":   "abc",
		"source_type":  "agent_analysis",
		"summary":      "test",
		"symbol_refs":  []any{"pkg:MyFunc"},
	})

	_, payload, err := handleFindingsBySymbol(b, ctx, map[string]any{
		"project_id": "proj-1",
		"symbol_ref": "pkg:MyFunc",
	})
	if err != nil {
		t.Fatalf("handleFindingsBySymbol: %v", err)
	}

	result := payload.(map[string]any)
	if result["count"].(int) != 1 {
		t.Fatalf("expected 1 result, got %d", result["count"].(int))
	}
}

func TestHandleFindingsByTicket(t *testing.T) {
	b := newTestBackendWithFindings()
	ctx := context.Background()

	handleFindingsSave(b, ctx, map[string]any{
		"project_id":   "proj-1",
		"claim":        "test claim",
		"finding_type": "code_quality",
		"confidence":   0.8,
		"commit_sha":   "abc",
		"source_type":  "agent_analysis",
		"summary":      "test",
		"ticket_refs":  []any{"warrant-42"},
	})

	_, payload, err := handleFindingsByTicket(b, ctx, map[string]any{
		"project_id": "proj-1",
		"ticket_ref": "warrant-42",
	})
	if err != nil {
		t.Fatalf("handleFindingsByTicket: %v", err)
	}

	result := payload.(map[string]any)
	if result["count"].(int) != 1 {
		t.Fatalf("expected 1 result, got %d", result["count"].(int))
	}
}

func TestHandleFindingsInvalidate(t *testing.T) {
	b := newTestBackendWithFindings()
	ctx := context.Background()

	handleFindingsSave(b, ctx, map[string]any{
		"project_id":   "proj-1",
		"claim":        "test claim",
		"finding_type": "code_quality",
		"confidence":   0.8,
		"commit_sha":   "abc",
		"source_type":  "agent_analysis",
		"summary":      "test",
		"file_refs":    []any{"internal/foo.go"},
	})

	_, payload, err := handleFindingsInvalidate(b, ctx, map[string]any{
		"project_id":    "proj-1",
		"commit_sha":    "def456",
		"changed_files": []any{"internal/foo.go"},
	})
	if err != nil {
		t.Fatalf("handleFindingsInvalidate: %v", err)
	}

	result := payload.(map[string]any)
	if result["invalidated_count"].(int) != 1 {
		t.Fatalf("expected 1 invalidated, got %d", result["invalidated_count"].(int))
	}
}

func TestHandleFindingsGet(t *testing.T) {
	b := newTestBackendWithFindings()
	ctx := context.Background()

	_, savePayload, _ := handleFindingsSave(b, ctx, map[string]any{
		"project_id":   "proj-1",
		"claim":        "test claim",
		"finding_type": "code_quality",
		"confidence":   0.8,
		"commit_sha":   "abc",
		"source_type":  "agent_analysis",
		"summary":      "test",
	})

	saved := savePayload.(*Finding)

	_, payload, err := handleFindingsGet(b, ctx, map[string]any{
		"project_id": "proj-1",
		"finding_id": saved.ID,
	})
	if err != nil {
		t.Fatalf("handleFindingsGet: %v", err)
	}

	got := payload.(*Finding)
	if got.ID != saved.ID {
		t.Errorf("expected ID %s, got %s", saved.ID, got.ID)
	}
	if got.Claim != "test claim" {
		t.Errorf("unexpected claim: %q", got.Claim)
	}
}

func TestWeaviateFindingsStore_ContractVersion(t *testing.T) {
	store := NewWeaviateFindingsStore(WeaviateFindingsConfig{URL: "http://localhost:8088"})
	v := store.ContractVersion()
	if v.Major != 1 || v.Minor != 0 || v.Patch != 0 {
		t.Errorf("expected 1.0.0, got %s", v)
	}
}

func TestWeaviateFindingsStore_DefaultVectorizer(t *testing.T) {
	store := NewWeaviateFindingsStore(WeaviateFindingsConfig{URL: "http://localhost:8088"})
	if store.cfg.Vectorizer != "text2vec-openai" {
		t.Errorf("expected default vectorizer 'text2vec-openai', got %q", store.cfg.Vectorizer)
	}
}

func TestWeaviateFindingsStore_BuildSchema(t *testing.T) {
	store := NewWeaviateFindingsStore(WeaviateFindingsConfig{
		URL:        "http://localhost:8088",
		Vectorizer: "text2vec-transformers",
	})
	schema := store.buildSchema()

	className, ok := schema["class"].(string)
	if !ok || className != "Finding" {
		t.Errorf("expected class 'Finding', got %v", schema["class"])
	}

	vectorizer, ok := schema["vectorizer"].(string)
	if !ok || vectorizer != "text2vec-transformers" {
		t.Errorf("expected vectorizer 'text2vec-transformers', got %v", schema["vectorizer"])
	}

	properties, ok := schema["properties"].([]map[string]any)
	if !ok {
		t.Fatal("expected properties array")
	}
	if len(properties) < 15 {
		t.Errorf("expected at least 15 properties, got %d", len(properties))
	}

	// Check that claim is vectorized (no skip)
	for _, prop := range properties {
		if prop["name"] == "claim" {
			modCfg, ok := prop["moduleConfig"]
			if ok {
				// Claim should NOT have skip:true
				if cfg, ok := modCfg.(map[string]any); ok {
					if vecCfg, ok := cfg["text2vec-transformers"].(map[string]any); ok {
						if skip, ok := vecCfg["skip"].(bool); ok && skip {
							t.Error("claim should not have skip=true in vectorizer config")
						}
					}
				}
			}
			break
		}
	}

	// Check that project_id has skip=true
	for _, prop := range properties {
		if prop["name"] == "project_id" {
			modCfg := prop["moduleConfig"].(map[string]any)
			vecCfg := modCfg["text2vec-transformers"].(map[string]any)
			if skip, ok := vecCfg["skip"].(bool); !ok || !skip {
				t.Error("project_id should have skip=true in vectorizer config")
			}
			break
		}
	}
}

func TestToStringSlice(t *testing.T) {
	// nil
	if result := toStringSlice(nil); result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	// []string
	result := toStringSlice([]string{"a", "b"})
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("unexpected result: %v", result)
	}

	// []any
	result = toStringSlice([]any{"x", "y", "z"})
	if len(result) != 3 || result[0] != "x" {
		t.Errorf("unexpected result: %v", result)
	}

	// unsupported type
	if result := toStringSlice(42); result != nil {
		t.Errorf("expected nil for unsupported type, got %v", result)
	}
}

func TestArgHelpers(t *testing.T) {
	args := map[string]any{
		"str_val":   "hello",
		"arr_val":   []any{"a", "b"},
		"int_val":   42,
		"nil_val":   nil,
	}

	if v := argStr(args, "str_val"); v != "hello" {
		t.Errorf("expected 'hello', got %q", v)
	}
	if v := argStr(args, "missing"); v != "" {
		t.Errorf("expected empty string, got %q", v)
	}
	if v := argStr(args, "int_val"); v != "" {
		t.Errorf("expected empty string for int, got %q", v)
	}

	if v := argStrSlice(args, "arr_val"); len(v) != 2 || v[0] != "a" {
		t.Errorf("unexpected slice: %v", v)
	}
	if v := argStrSlice(args, "missing"); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
	if v := argStrSlice(args, "nil_val"); v != nil {
		t.Errorf("expected nil for nil val, got %v", v)
	}
}
