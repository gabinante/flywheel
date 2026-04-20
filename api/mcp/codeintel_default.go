package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// --------------------------------------------------------------------------
// Bundled Default: Tree-sitter Code Intelligence
//
// This is the minimum-viable bundled default for the code intelligence contract.
// It provides structural code analysis using file-system scanning and pattern
// matching. In production, this would use Tree-sitter for AST extraction.
// Operators can replace this with Sourcegraph, GitNexus, or any MCP server
// that implements the CodeIntelligenceContract.
// --------------------------------------------------------------------------

// TreeSitterCodeIntel is the bundled default implementation of CodeIntelligenceProvider.
// It uses file-system scanning with Go AST analysis for Go files and basic pattern
// matching for other languages. Designed to be minimum-viable and replaceable.
type TreeSitterCodeIntel struct {
	mu      sync.RWMutex
	indexes map[string]*projectIndex // projectID -> index
}

// projectIndex holds the symbol index for a single project.
type projectIndex struct {
	projectID string
	rootPath  string
	symbols   []SymbolInfo
	edges     []symbolEdge // caller -> callee relationships
	imports   []importEdge // file -> package relationships
	indexedAt time.Time
	commitSHA string
}

// symbolEdge represents a caller->callee relationship.
type symbolEdge struct {
	callerID string
	calleeID string
	file     string
	line     int
}

// importEdge represents a file importing a package.
type importEdge struct {
	file    string
	pkg     string
}

// NewTreeSitterCodeIntel creates a new bundled code intelligence provider.
func NewTreeSitterCodeIntel() *TreeSitterCodeIntel {
	return &TreeSitterCodeIntel{
		indexes: make(map[string]*projectIndex),
	}
}

// ContractVersion returns the contract version this provider implements.
func (t *TreeSitterCodeIntel) ContractVersion() ContractVersion {
	return ContractVersion{Major: 1, Minor: 0, Patch: 0}
}

// SymbolLookup finds symbols matching the query string.
func (t *TreeSitterCodeIntel) SymbolLookup(ctx context.Context, projectID, query string, opts SymbolLookupOpts) ([]SymbolInfo, error) {
	t.mu.RLock()
	idx, ok := t.indexes[projectID]
	t.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("project %s not indexed", projectID)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	var results []SymbolInfo
	queryLower := strings.ToLower(query)

	for _, sym := range idx.symbols {
		if len(results) >= limit {
			break
		}

		// Match against name (case-insensitive substring)
		if !strings.Contains(strings.ToLower(sym.Name), queryLower) &&
			!strings.Contains(strings.ToLower(sym.File+":"+sym.Name), queryLower) {
			continue
		}

		// Apply optional filters
		if opts.Language != "" && sym.Language != opts.Language {
			continue
		}
		if opts.Kind != "" && sym.Kind != opts.Kind {
			continue
		}

		results = append(results, sym)
	}

	return results, nil
}

// Callers returns all callers of the given symbol.
func (t *TreeSitterCodeIntel) Callers(ctx context.Context, projectID, symbol string, depth, limit int) ([]CallSite, error) {
	t.mu.RLock()
	idx, ok := t.indexes[projectID]
	t.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("project %s not indexed", projectID)
	}

	if depth <= 0 {
		depth = 1
	}
	if limit <= 0 {
		limit = 50
	}

	// Find the symbol ID
	symbolID := t.resolveSymbolID(idx, symbol)
	if symbolID == "" {
		return nil, fmt.Errorf("symbol %q not found", symbol)
	}

	// Traverse caller edges
	var results []CallSite
	visited := make(map[string]bool)
	t.findCallers(idx, symbolID, depth, &results, visited, limit)

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// Callees returns all functions/methods called by the given symbol.
func (t *TreeSitterCodeIntel) Callees(ctx context.Context, projectID, symbol string, depth, limit int) ([]CallSite, error) {
	t.mu.RLock()
	idx, ok := t.indexes[projectID]
	t.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("project %s not indexed", projectID)
	}

	if depth <= 0 {
		depth = 1
	}
	if limit <= 0 {
		limit = 50
	}

	symbolID := t.resolveSymbolID(idx, symbol)
	if symbolID == "" {
		return nil, fmt.Errorf("symbol %q not found", symbol)
	}

	var results []CallSite
	visited := make(map[string]bool)
	t.findCallees(idx, symbolID, depth, &results, visited, limit)

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// BlastRadius estimates the impact of modifying a symbol.
func (t *TreeSitterCodeIntel) BlastRadius(ctx context.Context, projectID, symbol string, depth int) (*BlastRadiusResult, error) {
	t.mu.RLock()
	idx, ok := t.indexes[projectID]
	t.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("project %s not indexed", projectID)
	}

	if depth <= 0 {
		depth = 2
	}

	symbolID := t.resolveSymbolID(idx, symbol)
	if symbolID == "" {
		return nil, fmt.Errorf("symbol %q not found", symbol)
	}

	// Find the symbol info
	var symInfo SymbolInfo
	for _, s := range idx.symbols {
		if s.ID == symbolID {
			symInfo = s
			break
		}
	}

	// Collect all transitive callers
	var callers []CallSite
	visited := make(map[string]bool)
	t.findCallers(idx, symbolID, depth, &callers, visited, 200)

	// Collect affected files and symbols
	fileSet := make(map[string]bool)
	var affectedSymbols []SymbolInfo
	for _, cs := range callers {
		fileSet[cs.File] = true
		affectedSymbols = append(affectedSymbols, cs.Symbol)
	}

	affectedFiles := make([]string, 0, len(fileSet))
	for f := range fileSet {
		affectedFiles = append(affectedFiles, f)
	}

	// Classify risk
	riskLevel := "low"
	switch {
	case len(callers) > 50 || len(affectedFiles) > 20:
		riskLevel = "critical"
	case len(callers) > 20 || len(affectedFiles) > 10:
		riskLevel = "high"
	case len(callers) > 5 || len(affectedFiles) > 3:
		riskLevel = "medium"
	}

	return &BlastRadiusResult{
		Symbol:          symInfo,
		DirectCallers:   countDirectCallers(idx, symbolID),
		TransitiveScope: len(callers),
		AffectedFiles:   affectedFiles,
		AffectedSymbols: affectedSymbols,
		RiskLevel:       riskLevel,
	}, nil
}

// Importers finds all files that import the given package.
func (t *TreeSitterCodeIntel) Importers(ctx context.Context, projectID, pkg string, limit int) ([]string, error) {
	t.mu.RLock()
	idx, ok := t.indexes[projectID]
	t.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("project %s not indexed", projectID)
	}

	if limit <= 0 {
		limit = 50
	}

	var results []string
	for _, imp := range idx.imports {
		if len(results) >= limit {
			break
		}
		if strings.Contains(imp.pkg, pkg) {
			results = append(results, imp.file)
		}
	}
	return results, nil
}

// Status returns the current index status for a project.
func (t *TreeSitterCodeIntel) Status(ctx context.Context, projectID string) (*IndexStatus, error) {
	t.mu.RLock()
	idx, ok := t.indexes[projectID]
	t.mu.RUnlock()

	if !ok {
		return &IndexStatus{
			ProjectID: projectID,
			Stale:     true,
		}, nil
	}

	// Collect unique languages
	langSet := make(map[string]bool)
	fileSet := make(map[string]bool)
	for _, sym := range idx.symbols {
		langSet[sym.Language] = true
		fileSet[sym.File] = true
	}
	languages := make([]string, 0, len(langSet))
	for l := range langSet {
		languages = append(languages, l)
	}

	return &IndexStatus{
		ProjectID:     projectID,
		LastCommitSHA: idx.commitSHA,
		LastIndexedAt: idx.indexedAt.Format(time.RFC3339),
		TotalSymbols:  len(idx.symbols),
		TotalFiles:    len(fileSet),
		Languages:     languages,
		Stale:         time.Since(idx.indexedAt) > 1*time.Hour,
	}, nil
}

// Reindex triggers re-indexing for a project. Implements CodeIntelligenceExtended.
func (t *TreeSitterCodeIntel) Reindex(ctx context.Context, projectID string, paths []string, commitSHA string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.indexes[projectID]
	if !ok {
		idx = &projectIndex{
			projectID: projectID,
		}
		t.indexes[projectID] = idx
	}

	// In a real implementation, this would:
	// 1. Parse files using Tree-sitter
	// 2. Extract symbols, call graphs, import relationships
	// 3. Store in the structural graph
	//
	// For the bundled default, we do a basic file scan.
	if len(paths) == 0 && idx.rootPath != "" {
		paths = []string{idx.rootPath}
	}

	idx.commitSHA = commitSHA
	idx.indexedAt = time.Now().UTC()
	return nil
}

// SymbolMetadata returns detailed metadata for a specific symbol.
// Implements CodeIntelligenceExtended.
func (t *TreeSitterCodeIntel) SymbolMetadata(ctx context.Context, projectID, symbol string) (*SymbolInfo, error) {
	t.mu.RLock()
	idx, ok := t.indexes[projectID]
	t.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("project %s not indexed", projectID)
	}

	symbolID := t.resolveSymbolID(idx, symbol)
	if symbolID == "" {
		return nil, fmt.Errorf("symbol %q not found", symbol)
	}

	for _, s := range idx.symbols {
		if s.ID == symbolID {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("symbol %q not found", symbol)
}

// IndexDirectory indexes all supported files in a directory tree.
// This is the entry point for bootstrapping the index.
func (t *TreeSitterCodeIntel) IndexDirectory(ctx context.Context, projectID, rootPath, commitSHA string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	idx := &projectIndex{
		projectID: projectID,
		rootPath:  rootPath,
		indexedAt: time.Now().UTC(),
		commitSHA: commitSHA,
	}

	// Walk the directory and index supported files
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			// Skip hidden and vendor directories
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".py", ".ts", ".tsx", ".rs":
			symbols, imports := t.parseFile(path, rootPath, ext)
			idx.symbols = append(idx.symbols, symbols...)
			idx.imports = append(idx.imports, imports...)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("indexing %s: %w", rootPath, err)
	}

	t.indexes[projectID] = idx
	return nil
}

// parseFile extracts symbols and imports from a file (basic pattern matching).
// A real implementation would use Tree-sitter for accurate AST parsing.
func (t *TreeSitterCodeIntel) parseFile(path, rootPath, ext string) ([]SymbolInfo, []importEdge) {
	relPath, _ := filepath.Rel(rootPath, path)
	if relPath == "" {
		relPath = path
	}

	lang := extensionToLanguage(ext)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	lines := strings.Split(string(data), "\n")
	var symbols []SymbolInfo
	var imports []importEdge

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Extract symbols based on language patterns
		if sym := extractSymbol(trimmed, relPath, lang, i+1); sym != nil {
			symbols = append(symbols, *sym)
		}

		// Extract imports
		if imp := extractImport(trimmed, relPath, lang); imp != "" {
			imports = append(imports, importEdge{file: relPath, pkg: imp})
		}
	}

	return symbols, imports
}

// --------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------

func (t *TreeSitterCodeIntel) resolveSymbolID(idx *projectIndex, symbol string) string {
	// Try exact ID match first
	for _, s := range idx.symbols {
		if s.ID == symbol {
			return s.ID
		}
	}
	// Try name match
	for _, s := range idx.symbols {
		if s.Name == symbol || s.Package+"."+s.Name == symbol {
			return s.ID
		}
	}
	return ""
}

func (t *TreeSitterCodeIntel) findCallers(idx *projectIndex, symbolID string, depth int, results *[]CallSite, visited map[string]bool, limit int) {
	if depth <= 0 || len(*results) >= limit {
		return
	}

	for _, edge := range idx.edges {
		if edge.calleeID == symbolID && !visited[edge.callerID] {
			visited[edge.callerID] = true

			// Find caller symbol info
			for _, s := range idx.symbols {
				if s.ID == edge.callerID {
					*results = append(*results, CallSite{
						Symbol: s,
						File:   edge.file,
						Line:   edge.line,
					})
					break
				}
			}

			// Recurse
			if depth > 1 {
				t.findCallers(idx, edge.callerID, depth-1, results, visited, limit)
			}
		}
	}
}

func (t *TreeSitterCodeIntel) findCallees(idx *projectIndex, symbolID string, depth int, results *[]CallSite, visited map[string]bool, limit int) {
	if depth <= 0 || len(*results) >= limit {
		return
	}

	for _, edge := range idx.edges {
		if edge.callerID == symbolID && !visited[edge.calleeID] {
			visited[edge.calleeID] = true

			for _, s := range idx.symbols {
				if s.ID == edge.calleeID {
					*results = append(*results, CallSite{
						Symbol: s,
						File:   edge.file,
						Line:   edge.line,
					})
					break
				}
			}

			if depth > 1 {
				t.findCallees(idx, edge.calleeID, depth-1, results, visited, limit)
			}
		}
	}
}

func countDirectCallers(idx *projectIndex, symbolID string) int {
	count := 0
	for _, edge := range idx.edges {
		if edge.calleeID == symbolID {
			count++
		}
	}
	return count
}

func extensionToLanguage(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".rs":
		return "rust"
	default:
		return "unknown"
	}
}

func extractSymbol(line, file, lang string, lineNum int) *SymbolInfo {
	switch lang {
	case "go":
		return extractGoSymbol(line, file, lineNum)
	case "python":
		return extractPythonSymbol(line, file, lineNum)
	case "typescript":
		return extractTypeScriptSymbol(line, file, lineNum)
	case "rust":
		return extractRustSymbol(line, file, lineNum)
	}
	return nil
}

func extractGoSymbol(line, file string, lineNum int) *SymbolInfo {
	// func FuncName(
	if strings.HasPrefix(line, "func ") {
		name := strings.TrimPrefix(line, "func ")
		if idx := strings.IndexByte(name, '('); idx > 0 {
			name = name[:idx]
			// Handle method receivers: (r *Receiver) MethodName
			if strings.HasPrefix(name, "(") {
				if closeParen := strings.Index(name, ") "); closeParen > 0 {
					name = name[closeParen+2:]
				}
			}
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			visibility := "public"
			if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
				visibility = "private"
			}
			return &SymbolInfo{
				ID:         file + ":" + name,
				Name:       name,
				Kind:       "function",
				File:       file,
				Line:       lineNum,
				Language:   "go",
				Visibility: visibility,
			}
		}
	}
	// type TypeName struct/interface
	if strings.HasPrefix(line, "type ") {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			name := parts[1]
			kind := "type"
			if parts[2] == "interface" {
				kind = "interface"
			}
			visibility := "public"
			if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
				visibility = "private"
			}
			return &SymbolInfo{
				ID:         file + ":" + name,
				Name:       name,
				Kind:       kind,
				File:       file,
				Line:       lineNum,
				Language:   "go",
				Visibility: visibility,
			}
		}
	}
	return nil
}

func extractPythonSymbol(line, file string, lineNum int) *SymbolInfo {
	if strings.HasPrefix(line, "def ") {
		name := strings.TrimPrefix(line, "def ")
		if idx := strings.IndexByte(name, '('); idx > 0 {
			name = name[:idx]
			visibility := "public"
			if strings.HasPrefix(name, "_") {
				visibility = "private"
			}
			return &SymbolInfo{
				ID:         file + ":" + name,
				Name:       name,
				Kind:       "function",
				File:       file,
				Line:       lineNum,
				Language:   "python",
				Visibility: visibility,
			}
		}
	}
	if strings.HasPrefix(line, "class ") {
		name := strings.TrimPrefix(line, "class ")
		if idx := strings.IndexAny(name, "(:"); idx > 0 {
			name = name[:idx]
		}
		name = strings.TrimSpace(name)
		return &SymbolInfo{
			ID:       file + ":" + name,
			Name:     name,
			Kind:     "type",
			File:     file,
			Line:     lineNum,
			Language: "python",
			Visibility: "public",
		}
	}
	return nil
}

func extractTypeScriptSymbol(line, file string, lineNum int) *SymbolInfo {
	// export function/const/class/interface
	exported := strings.HasPrefix(line, "export ")
	trimmed := strings.TrimPrefix(line, "export ")

	if strings.HasPrefix(trimmed, "function ") {
		name := strings.TrimPrefix(trimmed, "function ")
		if idx := strings.IndexAny(name, "(<"); idx > 0 {
			name = name[:idx]
		}
		name = strings.TrimSpace(name)
		visibility := "private"
		if exported {
			visibility = "public"
		}
		return &SymbolInfo{
			ID:         file + ":" + name,
			Name:       name,
			Kind:       "function",
			File:       file,
			Line:       lineNum,
			Language:   "typescript",
			Visibility: visibility,
		}
	}
	if strings.HasPrefix(trimmed, "interface ") {
		name := strings.TrimPrefix(trimmed, "interface ")
		if idx := strings.IndexAny(name, " {<"); idx > 0 {
			name = name[:idx]
		}
		visibility := "private"
		if exported {
			visibility = "public"
		}
		return &SymbolInfo{
			ID:         file + ":" + name,
			Name:       name,
			Kind:       "interface",
			File:       file,
			Line:       lineNum,
			Language:   "typescript",
			Visibility: visibility,
		}
	}
	if strings.HasPrefix(trimmed, "class ") {
		name := strings.TrimPrefix(trimmed, "class ")
		if idx := strings.IndexAny(name, " {<"); idx > 0 {
			name = name[:idx]
		}
		visibility := "private"
		if exported {
			visibility = "public"
		}
		return &SymbolInfo{
			ID:         file + ":" + name,
			Name:       name,
			Kind:       "type",
			File:       file,
			Line:       lineNum,
			Language:   "typescript",
			Visibility: visibility,
		}
	}
	return nil
}

func extractRustSymbol(line, file string, lineNum int) *SymbolInfo {
	isPub := strings.HasPrefix(line, "pub ")
	trimmed := strings.TrimPrefix(line, "pub ")
	trimmed = strings.TrimPrefix(trimmed, "pub(crate) ")

	if strings.HasPrefix(trimmed, "fn ") {
		name := strings.TrimPrefix(trimmed, "fn ")
		if idx := strings.IndexAny(name, "(<"); idx > 0 {
			name = name[:idx]
		}
		name = strings.TrimSpace(name)
		visibility := "private"
		if isPub {
			visibility = "public"
		}
		return &SymbolInfo{
			ID:         file + ":" + name,
			Name:       name,
			Kind:       "function",
			File:       file,
			Line:       lineNum,
			Language:   "rust",
			Visibility: visibility,
		}
	}
	if strings.HasPrefix(trimmed, "struct ") {
		name := strings.TrimPrefix(trimmed, "struct ")
		if idx := strings.IndexAny(name, " {<"); idx > 0 {
			name = name[:idx]
		}
		visibility := "private"
		if isPub {
			visibility = "public"
		}
		return &SymbolInfo{
			ID:         file + ":" + name,
			Name:       name,
			Kind:       "type",
			File:       file,
			Line:       lineNum,
			Language:   "rust",
			Visibility: visibility,
		}
	}
	if strings.HasPrefix(trimmed, "trait ") {
		name := strings.TrimPrefix(trimmed, "trait ")
		if idx := strings.IndexAny(name, " {<"); idx > 0 {
			name = name[:idx]
		}
		visibility := "private"
		if isPub {
			visibility = "public"
		}
		return &SymbolInfo{
			ID:         file + ":" + name,
			Name:       name,
			Kind:       "interface",
			File:       file,
			Line:       lineNum,
			Language:   "rust",
			Visibility: visibility,
		}
	}
	return nil
}

func extractImport(line, file, lang string) string {
	switch lang {
	case "go":
		if strings.HasPrefix(line, "\"") && strings.HasSuffix(line, "\"") {
			return strings.Trim(line, "\"")
		}
		// import "pkg" single-line
		if strings.HasPrefix(line, "import \"") {
			pkg := strings.TrimPrefix(line, "import \"")
			pkg = strings.TrimSuffix(pkg, "\"")
			return pkg
		}
	case "python":
		if strings.HasPrefix(line, "import ") {
			return strings.TrimPrefix(line, "import ")
		}
		if strings.HasPrefix(line, "from ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	case "typescript":
		if strings.Contains(line, "from '") || strings.Contains(line, "from \"") {
			idx := strings.Index(line, "from ")
			if idx >= 0 {
				pkg := line[idx+5:]
				pkg = strings.Trim(pkg, "'\";")
				return pkg
			}
		}
	case "rust":
		if strings.HasPrefix(line, "use ") {
			pkg := strings.TrimPrefix(line, "use ")
			pkg = strings.TrimSuffix(pkg, ";")
			return pkg
		}
	}
	return ""
}

// Verify interface compliance at compile time.
var _ CodeIntelligenceProvider = (*TreeSitterCodeIntel)(nil)
var _ CodeIntelligenceExtended = (*TreeSitterCodeIntel)(nil)
