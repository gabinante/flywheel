package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --------------------------------------------------------------------------
// Weaviate Findings Backend
//
// Production-grade FindingsProvider backed by Weaviate vector database.
// Uses Weaviate's REST/GraphQL API via net/http (no SDK dependency).
// Relies on Weaviate's built-in text2vec module for automatic vectorization
// of finding claims, enabling true semantic search.
//
// Schema auto-creation: on first use, the provider ensures the "Finding"
// collection exists with the correct properties and vectorizer config.
//
// Pluggable: operators can swap this for pgvector or Qdrant by implementing
// FindingsProvider. The MCP tools are backend-agnostic.
// --------------------------------------------------------------------------

const (
	weaviateClassName = "Finding"
	weaviateAPIV1     = "/v1"
)

// WeaviateFindingsConfig holds configuration for the Weaviate findings backend.
type WeaviateFindingsConfig struct {
	// URL is the Weaviate server URL (e.g. "http://localhost:8088").
	URL string

	// APIKey is the Weaviate API key for authentication (optional, for cloud).
	APIKey string

	// Vectorizer is the text2vec module to use (default: "text2vec-openai").
	// Other options: "text2vec-transformers", "text2vec-cohere", "none".
	Vectorizer string

	// VectorizerConfig holds extra config for the vectorizer module (optional).
	VectorizerConfig map[string]any
}

// WeaviateFindingsStore implements FindingsProvider using Weaviate as the vector backend.
type WeaviateFindingsStore struct {
	cfg           WeaviateFindingsConfig
	client        *http.Client
	schemaCreated bool
}

// NewWeaviateFindingsStore creates a new Weaviate-backed findings provider.
func NewWeaviateFindingsStore(cfg WeaviateFindingsConfig) *WeaviateFindingsStore {
	if cfg.Vectorizer == "" {
		cfg.Vectorizer = "text2vec-openai"
	}
	return &WeaviateFindingsStore{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ContractVersion returns the contract version this provider implements.
func (w *WeaviateFindingsStore) ContractVersion() ContractVersion {
	return ContractVersion{Major: 1, Minor: 0, Patch: 0}
}

// SaveFinding persists a finding to Weaviate with automatic vectorization.
func (w *WeaviateFindingsStore) SaveFinding(ctx context.Context, finding Finding) (*Finding, error) {
	if err := w.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensuring weaviate schema: %w", err)
	}

	if finding.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if finding.Claim == "" {
		return nil, fmt.Errorf("claim is required")
	}

	finding.ID = uuid.New().String()
	finding.Valid = true
	finding.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	obj := w.findingToObject(finding)
	body, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling finding: %w", err)
	}

	resp, err := w.doRequest(ctx, "POST", weaviateAPIV1+"/objects", body)
	if err != nil {
		return nil, fmt.Errorf("creating weaviate object: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("weaviate create failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return &finding, nil
}

// QueryFindings performs semantic search via Weaviate's nearText GraphQL query.
func (w *WeaviateFindingsStore) QueryFindings(ctx context.Context, projectID, query string, opts FindingsQueryOpts) ([]Finding, error) {
	if err := w.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensuring weaviate schema: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// Build where filter.
	filters := []string{
		fmt.Sprintf(`{path: ["project_id"], operator: Equal, valueText: %q}`, projectID),
	}
	if opts.ValidOnly {
		filters = append(filters, `{path: ["valid"], operator: Equal, valueBoolean: true}`)
	}
	if opts.FindingType != "" {
		filters = append(filters, fmt.Sprintf(`{path: ["finding_type"], operator: Equal, valueText: %q}`, opts.FindingType))
	}

	whereClause := ""
	if len(filters) == 1 {
		whereClause = fmt.Sprintf("where: %s", filters[0])
	} else {
		whereClause = fmt.Sprintf("where: {operator: And, operands: [%s]}", strings.Join(filters, ", "))
	}

	gql := fmt.Sprintf(`{
		Get {
			%s(
				nearText: {concepts: [%q]}
				%s
				limit: %d
			) {
				_additional { id distance }
				project_id claim finding_type confidence
				commit_sha function_body_hash source_type
				source_ticket_id source_agent_id
				symbol_refs ticket_refs file_refs tags
				summary valid invalidated_at invalidation_reason
				created_at
			}
		}
	}`, weaviateClassName, query, whereClause, limit)

	return w.executeGraphQL(ctx, gql)
}

// FindingsBySymbol returns findings referencing the given symbol via filtered search.
func (w *WeaviateFindingsStore) FindingsBySymbol(ctx context.Context, projectID, symbolRef string, limit int) ([]Finding, error) {
	if err := w.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensuring weaviate schema: %w", err)
	}

	if limit <= 0 {
		limit = 20
	}

	gql := fmt.Sprintf(`{
		Get {
			%s(
				where: {
					operator: And
					operands: [
						{path: ["project_id"], operator: Equal, valueText: %q},
						{path: ["symbol_refs"], operator: ContainsAny, valueText: [%q]}
					]
				}
				limit: %d
			) {
				_additional { id }
				project_id claim finding_type confidence
				commit_sha function_body_hash source_type
				source_ticket_id source_agent_id
				symbol_refs ticket_refs file_refs tags
				summary valid invalidated_at invalidation_reason
				created_at
			}
		}
	}`, weaviateClassName, projectID, symbolRef, limit)

	return w.executeGraphQL(ctx, gql)
}

// FindingsByTicket returns findings referencing the given ticket.
func (w *WeaviateFindingsStore) FindingsByTicket(ctx context.Context, projectID, ticketRef string, limit int) ([]Finding, error) {
	if err := w.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensuring weaviate schema: %w", err)
	}

	if limit <= 0 {
		limit = 20
	}

	gql := fmt.Sprintf(`{
		Get {
			%s(
				where: {
					operator: And
					operands: [
						{path: ["project_id"], operator: Equal, valueText: %q},
						{
							operator: Or
							operands: [
								{path: ["ticket_refs"], operator: ContainsAny, valueText: [%q]},
								{path: ["source_ticket_id"], operator: Equal, valueText: %q}
							]
						}
					]
				}
				limit: %d
			) {
				_additional { id }
				project_id claim finding_type confidence
				commit_sha function_body_hash source_type
				source_ticket_id source_agent_id
				symbol_refs ticket_refs file_refs tags
				summary valid invalidated_at invalidation_reason
				created_at
			}
		}
	}`, weaviateClassName, projectID, ticketRef, ticketRef, limit)

	return w.executeGraphQL(ctx, gql)
}

// InvalidateFindings marks findings as invalid when their referenced files change.
func (w *WeaviateFindingsStore) InvalidateFindings(ctx context.Context, projectID, commitSHA string, changedFiles []string) (int, error) {
	if err := w.ensureSchema(ctx); err != nil {
		return 0, fmt.Errorf("ensuring weaviate schema: %w", err)
	}

	// First, find all valid findings that reference the changed files.
	fileQuotes := make([]string, len(changedFiles))
	for i, f := range changedFiles {
		fileQuotes[i] = fmt.Sprintf("%q", f)
	}

	gql := fmt.Sprintf(`{
		Get {
			%s(
				where: {
					operator: And
					operands: [
						{path: ["project_id"], operator: Equal, valueText: %q},
						{path: ["valid"], operator: Equal, valueBoolean: true},
						{path: ["file_refs"], operator: ContainsAny, valueText: [%s]}
					]
				}
				limit: 1000
			) {
				_additional { id }
				file_refs
			}
		}
	}`, weaviateClassName, projectID, strings.Join(fileQuotes, ", "))

	findings, err := w.executeGraphQL(ctx, gql)
	if err != nil {
		return 0, fmt.Errorf("querying findings to invalidate: %w", err)
	}

	// Batch update each found finding.
	now := time.Now().UTC().Format(time.RFC3339)
	count := 0
	for _, f := range findings {
		patchBody := map[string]any{
			"class": weaviateClassName,
			"properties": map[string]any{
				"valid":               false,
				"invalidated_at":      now,
				"invalidation_reason": fmt.Sprintf("files changed at commit %s", commitSHA),
			},
		}
		body, err := json.Marshal(patchBody)
		if err != nil {
			continue
		}
		resp, err := w.doRequest(ctx, "PATCH", fmt.Sprintf("%s/objects/%s/%s", weaviateAPIV1, weaviateClassName, f.ID), body)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			count++
		}
	}

	return count, nil
}

// GetFinding returns a single finding by ID from Weaviate.
func (w *WeaviateFindingsStore) GetFinding(ctx context.Context, projectID, findingID string) (*Finding, error) {
	if err := w.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensuring weaviate schema: %w", err)
	}

	resp, err := w.doRequest(ctx, "GET", fmt.Sprintf("%s/objects/%s/%s", weaviateAPIV1, weaviateClassName, findingID), nil)
	if err != nil {
		return nil, fmt.Errorf("getting weaviate object: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("finding %q not found", findingID)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("weaviate get failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID         string         `json:"id"`
		Class      string         `json:"class"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding weaviate response: %w", err)
	}

	finding := w.objectToFinding(result.ID, result.Properties)
	if finding.ProjectID != projectID {
		return nil, fmt.Errorf("finding %q not found in project %s", findingID, projectID)
	}

	return &finding, nil
}

// DeleteFinding permanently removes a finding from Weaviate. Implements FindingsExtended.
func (w *WeaviateFindingsStore) DeleteFinding(ctx context.Context, projectID, findingID string) error {
	// Verify the finding belongs to this project first.
	_, err := w.GetFinding(ctx, projectID, findingID)
	if err != nil {
		return err
	}

	resp, err := w.doRequest(ctx, "DELETE", fmt.Sprintf("%s/objects/%s/%s", weaviateAPIV1, weaviateClassName, findingID), nil)
	if err != nil {
		return fmt.Errorf("deleting weaviate object: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weaviate delete failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// FindingsByFile returns findings whose file references include the given path.
// Implements FindingsExtended.
func (w *WeaviateFindingsStore) FindingsByFile(ctx context.Context, projectID, filePath string, limit int) ([]Finding, error) {
	if err := w.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensuring weaviate schema: %w", err)
	}

	if limit <= 0 {
		limit = 20
	}

	gql := fmt.Sprintf(`{
		Get {
			%s(
				where: {
					operator: And
					operands: [
						{path: ["project_id"], operator: Equal, valueText: %q},
						{path: ["file_refs"], operator: ContainsAny, valueText: [%q]}
					]
				}
				limit: %d
			) {
				_additional { id }
				project_id claim finding_type confidence
				commit_sha function_body_hash source_type
				source_ticket_id source_agent_id
				symbol_refs ticket_refs file_refs tags
				summary valid invalidated_at invalidation_reason
				created_at
			}
		}
	}`, weaviateClassName, projectID, filePath, limit)

	return w.executeGraphQL(ctx, gql)
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// ensureSchema creates the Weaviate collection if it doesn't exist.
func (w *WeaviateFindingsStore) ensureSchema(ctx context.Context) error {
	if w.schemaCreated {
		return nil
	}

	// Check if the class already exists.
	resp, err := w.doRequest(ctx, "GET", weaviateAPIV1+"/schema/"+weaviateClassName, nil)
	if err != nil {
		return fmt.Errorf("checking schema: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		w.schemaCreated = true
		return nil
	}

	// Create the collection.
	schema := w.buildSchema()
	body, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshaling schema: %w", err)
	}

	resp, err = w.doRequest(ctx, "POST", weaviateAPIV1+"/schema", body)
	if err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		// Class might already exist (race condition) — treat as success.
		if strings.Contains(string(respBody), "already exists") {
			w.schemaCreated = true
			return nil
		}
		return fmt.Errorf("weaviate schema creation failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	w.schemaCreated = true
	return nil
}

// buildSchema returns the Weaviate class definition for findings.
func (w *WeaviateFindingsStore) buildSchema() map[string]any {
	vectorizerConfig := map[string]any{}
	if w.cfg.VectorizerConfig != nil {
		vectorizerConfig = w.cfg.VectorizerConfig
	}

	// Determine vectorizer module config key.
	vectorizer := w.cfg.Vectorizer
	if vectorizer == "" {
		vectorizer = "none"
	}

	return map[string]any{
		"class":       weaviateClassName,
		"description": "Semantic findings: claims with provenance, symbol references, and ticket references.",
		"vectorizer":  vectorizer,
		"moduleConfig": map[string]any{
			vectorizer: vectorizerConfig,
		},
		"properties": []map[string]any{
			{"name": "project_id", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "claim", "dataType": []string{"text"}, "indexSearchable": true,
				"description": "The finding's claim text — primary field for vectorization"},
			{"name": "finding_type", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "confidence", "dataType": []string{"number"}},
			{"name": "commit_sha", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "function_body_hash", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "source_type", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "source_ticket_id", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "source_agent_id", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "symbol_refs", "dataType": []string{"text[]"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "ticket_refs", "dataType": []string{"text[]"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "file_refs", "dataType": []string{"text[]"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "tags", "dataType": []string{"text[]"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "summary", "dataType": []string{"text"}, "indexSearchable": true,
				"description": "Short summary — included in vectorization for richer embeddings"},
			{"name": "valid", "dataType": []string{"boolean"}, "indexFilterable": true},
			{"name": "invalidated_at", "dataType": []string{"text"}, "indexFilterable": false, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "invalidation_reason", "dataType": []string{"text"}, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
			{"name": "created_at", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false,
				"moduleConfig": map[string]any{vectorizer: map[string]any{"skip": true}}},
		},
	}
}

// findingToObject converts a Finding to a Weaviate object payload.
func (w *WeaviateFindingsStore) findingToObject(f Finding) map[string]any {
	return map[string]any{
		"id":    f.ID,
		"class": weaviateClassName,
		"properties": map[string]any{
			"project_id":          f.ProjectID,
			"claim":               f.Claim,
			"finding_type":        f.FindingType,
			"confidence":          f.Confidence,
			"commit_sha":          f.CommitSHA,
			"function_body_hash":  f.FunctionBodyHash,
			"source_type":         f.SourceType,
			"source_ticket_id":    f.SourceTicketID,
			"source_agent_id":     f.SourceAgentID,
			"symbol_refs":         f.SymbolRefs,
			"ticket_refs":         f.TicketRefs,
			"file_refs":           f.FileRefs,
			"tags":                f.Tags,
			"summary":             f.Summary,
			"valid":               f.Valid,
			"invalidated_at":      f.InvalidatedAt,
			"invalidation_reason": f.InvalidationReason,
			"created_at":          f.CreatedAt,
		},
	}
}

// objectToFinding converts Weaviate object properties to a Finding.
func (w *WeaviateFindingsStore) objectToFinding(id string, props map[string]any) Finding {
	f := Finding{ID: id}

	if v, ok := props["project_id"].(string); ok {
		f.ProjectID = v
	}
	if v, ok := props["claim"].(string); ok {
		f.Claim = v
	}
	if v, ok := props["finding_type"].(string); ok {
		f.FindingType = v
	}
	if v, ok := props["confidence"].(float64); ok {
		f.Confidence = v
	}
	if v, ok := props["commit_sha"].(string); ok {
		f.CommitSHA = v
	}
	if v, ok := props["function_body_hash"].(string); ok {
		f.FunctionBodyHash = v
	}
	if v, ok := props["source_type"].(string); ok {
		f.SourceType = v
	}
	if v, ok := props["source_ticket_id"].(string); ok {
		f.SourceTicketID = v
	}
	if v, ok := props["source_agent_id"].(string); ok {
		f.SourceAgentID = v
	}
	f.SymbolRefs = toStringSlice(props["symbol_refs"])
	f.TicketRefs = toStringSlice(props["ticket_refs"])
	f.FileRefs = toStringSlice(props["file_refs"])
	f.Tags = toStringSlice(props["tags"])
	if v, ok := props["summary"].(string); ok {
		f.Summary = v
	}
	if v, ok := props["valid"].(bool); ok {
		f.Valid = v
	}
	if v, ok := props["invalidated_at"].(string); ok {
		f.InvalidatedAt = v
	}
	if v, ok := props["invalidation_reason"].(string); ok {
		f.InvalidationReason = v
	}
	if v, ok := props["created_at"].(string); ok {
		f.CreatedAt = v
	}

	return f
}

// executeGraphQL sends a GraphQL query to Weaviate and parses the result.
func (w *WeaviateFindingsStore) executeGraphQL(ctx context.Context, gql string) ([]Finding, error) {
	payload := map[string]string{"query": gql}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling graphql: %w", err)
	}

	resp, err := w.doRequest(ctx, "POST", weaviateAPIV1+"/graphql", body)
	if err != nil {
		return nil, fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("weaviate graphql failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var gqlResp struct {
		Data struct {
			Get map[string][]map[string]any `json:"Get"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("decoding graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("weaviate graphql error: %s", gqlResp.Errors[0].Message)
	}

	objects := gqlResp.Data.Get[weaviateClassName]
	var findings []Finding
	for _, obj := range objects {
		id := ""
		if additional, ok := obj["_additional"].(map[string]any); ok {
			if v, ok := additional["id"].(string); ok {
				id = v
			}
		}
		finding := w.objectToFinding(id, obj)
		findings = append(findings, finding)
	}

	return findings, nil
}

// doRequest performs an HTTP request to the Weaviate API.
func (w *WeaviateFindingsStore) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := strings.TrimRight(w.cfg.URL, "/") + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if w.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)
	}

	return w.client.Do(req)
}

// toStringSlice converts an interface{} to []string. Handles both []any and []string.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []string:
		return arr
	case []any:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// Verify interface compliance at compile time.
var _ FindingsProvider = (*WeaviateFindingsStore)(nil)
var _ FindingsExtended = (*WeaviateFindingsStore)(nil)
