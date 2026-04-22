package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// --------------------------------------------------------------------------
// Bundled Default: In-Memory Findings Store
//
// Minimum-viable bundled default for the findings contract. Stores findings
// in memory with basic substring-based "semantic" search. Suitable for
// development, testing, and small deployments. For production semantic search,
// swap to the Weaviate backend or another vector store implementation.
//
// Operators can replace this with a Weaviate, pgvector, or Qdrant MCP server
// that implements the FindingsContract.
// --------------------------------------------------------------------------

// MemoryFindingsStore is the bundled default implementation of FindingsProvider.
// It uses in-memory storage with substring matching for query. Designed to be
// minimum-viable and replaceable.
type MemoryFindingsStore struct {
	mu       sync.RWMutex
	findings map[string][]Finding // projectID -> findings
}

// NewMemoryFindingsStore creates a new in-memory findings provider.
func NewMemoryFindingsStore() *MemoryFindingsStore {
	return &MemoryFindingsStore{
		findings: make(map[string][]Finding),
	}
}

// ContractVersion returns the contract version this provider implements.
func (m *MemoryFindingsStore) ContractVersion() ContractVersion {
	return ContractVersion{Major: 1, Minor: 0, Patch: 0}
}

// SaveFinding persists a finding in memory with a generated UUID.
func (m *MemoryFindingsStore) SaveFinding(_ context.Context, finding Finding) (*Finding, error) {
	if finding.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if finding.Claim == "" {
		return nil, fmt.Errorf("claim is required")
	}

	finding.ID = uuid.New().String()
	finding.Valid = true
	finding.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	m.mu.Lock()
	m.findings[finding.ProjectID] = append(m.findings[finding.ProjectID], finding)
	m.mu.Unlock()

	return &finding, nil
}

// QueryFindings performs substring-based search across claim and summary fields.
// In a real vector store this would be semantic similarity search.
func (m *MemoryFindingsStore) QueryFindings(_ context.Context, projectID, query string, opts FindingsQueryOpts) ([]Finding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	queryLower := strings.ToLower(query)
	var results []Finding

	for _, f := range m.findings[projectID] {
		if len(results) >= limit {
			break
		}
		if opts.ValidOnly && !f.Valid {
			continue
		}
		if opts.FindingType != "" && f.FindingType != opts.FindingType {
			continue
		}
		if opts.SymbolRef != "" && !containsStr(f.SymbolRefs, opts.SymbolRef) {
			continue
		}
		if opts.FileRef != "" && !containsStr(f.FileRefs, opts.FileRef) {
			continue
		}
		// Basic substring match as a stand-in for semantic search.
		if strings.Contains(strings.ToLower(f.Claim), queryLower) ||
			strings.Contains(strings.ToLower(f.Summary), queryLower) ||
			matchesTags(f.Tags, queryLower) {
			results = append(results, f)
		}
	}

	return results, nil
}

// FindingsBySymbol returns findings that reference the given symbol.
func (m *MemoryFindingsStore) FindingsBySymbol(_ context.Context, projectID, symbolRef string, limit int) ([]Finding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var results []Finding
	for _, f := range m.findings[projectID] {
		if len(results) >= limit {
			break
		}
		if containsStr(f.SymbolRefs, symbolRef) {
			results = append(results, f)
		}
	}

	return results, nil
}

// FindingsByTicket returns findings that reference the given ticket.
func (m *MemoryFindingsStore) FindingsByTicket(_ context.Context, projectID, ticketRef string, limit int) ([]Finding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var results []Finding
	for _, f := range m.findings[projectID] {
		if len(results) >= limit {
			break
		}
		// Check both ticket_refs and source_ticket_id.
		if containsStr(f.TicketRefs, ticketRef) || f.SourceTicketID == ticketRef {
			results = append(results, f)
		}
	}

	return results, nil
}

// InvalidateFindings marks findings as invalid when their referenced files have changed.
func (m *MemoryFindingsStore) InvalidateFindings(_ context.Context, projectID, commitSHA string, changedFiles []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	changedSet := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	count := 0
	now := time.Now().UTC().Format(time.RFC3339)

	findings := m.findings[projectID]
	for i := range findings {
		if !findings[i].Valid {
			continue
		}
		// Invalidate if any file ref overlaps with changed files.
		for _, ref := range findings[i].FileRefs {
			if changedSet[ref] {
				findings[i].Valid = false
				findings[i].InvalidatedAt = now
				findings[i].InvalidationReason = fmt.Sprintf("file %s changed at commit %s", ref, commitSHA)
				count++
				break
			}
		}
	}

	return count, nil
}

// GetFinding returns a single finding by ID.
func (m *MemoryFindingsStore) GetFinding(_ context.Context, projectID, findingID string) (*Finding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, f := range m.findings[projectID] {
		if f.ID == findingID {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("finding %q not found in project %s", findingID, projectID)
}

// DeleteFinding permanently removes a finding. Implements FindingsExtended.
func (m *MemoryFindingsStore) DeleteFinding(_ context.Context, projectID, findingID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	findings := m.findings[projectID]
	for i, f := range findings {
		if f.ID == findingID {
			m.findings[projectID] = append(findings[:i], findings[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("finding %q not found in project %s", findingID, projectID)
}

// FindingsByFile returns findings whose file references include the given path.
// Implements FindingsExtended.
func (m *MemoryFindingsStore) FindingsByFile(_ context.Context, projectID, filePath string, limit int) ([]Finding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var results []Finding
	for _, f := range m.findings[projectID] {
		if len(results) >= limit {
			break
		}
		if containsStr(f.FileRefs, filePath) {
			results = append(results, f)
		}
	}

	return results, nil
}

// helpers

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func matchesTags(tags []string, query string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}
	return false
}

// Verify interface compliance at compile time.
var _ FindingsProvider = (*MemoryFindingsStore)(nil)
var _ FindingsExtended = (*MemoryFindingsStore)(nil)
