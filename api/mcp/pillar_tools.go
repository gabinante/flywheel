package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gabinante/flywheel/internal/pillar"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// registerPillarTools registers all pillar layer (Layer 15) MCP tools.
func registerPillarTools(s *mcp.Server, b *Backend, wrap wrapFn) {
	if b.Pillar == nil {
		return
	}

	// ── create_pillar_entry ──────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_pillar_entry",
		Description: "Create a pillar strategy entry for a project-map entity. The seven pillars are: observability, mutability, scalability, availability, security, resiliency, cost. Each entry holds a strategy statement, known gaps, and a review cadence.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":     map[string]any{"type": "string", "description": "Project ID"},
				"entity_id":      map[string]any{"type": "string", "description": "Catalog entity ID this pillar applies to"},
				"entity_type":    map[string]any{"type": "string", "description": "Entity type (service, datastore, etc.)"},
				"pillar_type":    map[string]any{"type": "string", "description": "Pillar type", "enum": []string{"observability", "mutability", "scalability", "availability", "security", "resiliency", "cost"}},
				"strategy":       map[string]any{"type": "string", "description": "Strategy statement for this pillar"},
				"gaps":           map[string]any{"type": "array", "description": "Known gaps", "items": map[string]any{"type": "object", "properties": map[string]any{"description": map[string]any{"type": "string"}, "severity": map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}}, "mitigation": map[string]any{"type": "string"}}}},
				"review_cadence": map[string]any{"type": "string", "description": "How often to review", "enum": []string{"weekly", "biweekly", "monthly", "quarterly"}},
				"created_by":     map[string]any{"type": "string", "description": "Creator agent/user ID"},
			},
			"required":             []string{"project_id", "entity_id", "pillar_type"},
			"additionalProperties": false,
		},
	}, wrap(createPillarEntryHandler))

	// ── get_pillar_entry ────────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_pillar_entry",
		Description: "Get a pillar entry by ID. Returns strategy, gaps, review schedule, and metadata.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entry_id": map[string]any{"type": "string", "description": "Pillar entry ID"},
			},
			"required":             []string{"entry_id"},
			"additionalProperties": false,
		},
	}, wrap(getPillarEntryHandler))

	// ── list_pillar_entries ──────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_pillar_entries",
		Description: "List pillar entries for a project. Filter by entity_id or pillar_type.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":  map[string]any{"type": "string", "description": "Project ID"},
				"entity_id":   map[string]any{"type": "string", "description": "Filter by entity ID (optional)"},
				"pillar_type": map[string]any{"type": "string", "description": "Filter by pillar type (optional)"},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(listPillarEntriesHandler))

	// ── update_pillar_entry ──────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_pillar_entry",
		Description: "Update a pillar entry's strategy, gaps, or review cadence.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entry_id":       map[string]any{"type": "string", "description": "Pillar entry ID"},
				"strategy":       map[string]any{"type": "string", "description": "Updated strategy statement"},
				"gaps":           map[string]any{"type": "array", "description": "Updated gaps", "items": map[string]any{"type": "object", "properties": map[string]any{"description": map[string]any{"type": "string"}, "severity": map[string]any{"type": "string"}, "mitigation": map[string]any{"type": "string"}}}},
				"review_cadence": map[string]any{"type": "string", "description": "Updated review cadence", "enum": []string{"weekly", "biweekly", "monthly", "quarterly"}},
			},
			"required":             []string{"entry_id"},
			"additionalProperties": false,
		},
	}, wrap(updatePillarEntryHandler))

	// ── create_pillar_claim ──────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_pillar_claim",
		Description: "Add a structured claim to a pillar entry. Claims cite a project-map entity as evidence for a pillar strategy.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pillar_entry_id": map[string]any{"type": "string", "description": "Pillar entry ID"},
				"statement":       map[string]any{"type": "string", "description": "What is being claimed"},
				"entity_ref_id":   map[string]any{"type": "string", "description": "Catalog entity ID cited as evidence"},
				"entity_ref_type": map[string]any{"type": "string", "description": "Type of the referenced entity"},
				"evidence":        map[string]any{"type": "string", "description": "Supporting evidence or proof"},
			},
			"required":             []string{"pillar_entry_id", "statement", "entity_ref_id"},
			"additionalProperties": false,
		},
	}, wrap(createPillarClaimHandler))

	// ── list_pillar_claims ──────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_pillar_claims",
		Description: "List all claims for a pillar entry.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pillar_entry_id": map[string]any{"type": "string", "description": "Pillar entry ID"},
			},
			"required":             []string{"pillar_entry_id"},
			"additionalProperties": false,
		},
	}, wrap(listPillarClaimsHandler))

	// ── update_pillar_claim ──────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_pillar_claim",
		Description: "Update a claim's verification status and/or evidence.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claim_id": map[string]any{"type": "string", "description": "Claim ID"},
				"status":   map[string]any{"type": "string", "description": "New status", "enum": []string{"verified", "unverified", "stale", "invalid"}},
				"evidence": map[string]any{"type": "string", "description": "Updated evidence"},
			},
			"required":             []string{"claim_id"},
			"additionalProperties": false,
		},
	}, wrap(updatePillarClaimHandler))

	// ── evaluate_pillar_entry ────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "evaluate_pillar_entry",
		Description: "Run the continuous evaluation loop on a pillar entry. Checks: evidence intact, claims match reality, strategy appropriate. Returns evaluation results.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entry_id": map[string]any{"type": "string", "description": "Pillar entry ID to evaluate"},
			},
			"required":             []string{"entry_id"},
			"additionalProperties": false,
		},
	}, wrap(evaluatePillarEntryHandler))

	// ── list_pillar_templates ────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_pillar_templates",
		Description: "Get the default templates for all seven pillar types. Templates include strategy prompts, example claims, and common gaps.",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":          map[string]any{},
			"additionalProperties": false,
		},
	}, wrap(listPillarTemplatesHandler))

	// ── list_pillars_due_for_review ──────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_pillars_due_for_review",
		Description: "List pillar entries that are due for review (next_review_at <= now).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(listPillarsDueForReviewHandler))

	// ── mark_pillar_reviewed ─────────────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{
		Name:        "mark_pillar_reviewed",
		Description: "Mark a pillar entry as reviewed. Updates last_reviewed_at and computes next_review_at based on cadence.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entry_id": map[string]any{"type": "string", "description": "Pillar entry ID"},
			},
			"required":             []string{"entry_id"},
			"additionalProperties": false,
		},
	}, wrap(markPillarReviewedHandler))
}

// ── Handler implementations ──────────────────────────────────────────────────

func createPillarEntryHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	pillarType, err := requireString(args, "pillar_type")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityType := getString(args, "entity_type", "")
	strategy := getString(args, "strategy", "")
	cadence := getString(args, "review_cadence", "")
	createdBy := getString(args, "created_by", "")
	if createdBy == "" {
		createdBy, _ = getAgentIDFromArgs(ctx, args)
		if createdBy == "" {
			createdBy = "mcp"
		}
	}

	var gaps []pillar.Gap
	if gapsRaw, ok := args["gaps"]; ok && gapsRaw != nil {
		gapsJSON, _ := json.Marshal(gapsRaw)
		_ = json.Unmarshal(gapsJSON, &gaps)
	}

	entry, err := b.Pillar.CreateEntry(ctx, projectID, entityID, entityType, pillarType, strategy, createdBy, gaps, cadence)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(entry)
}

func getPillarEntryHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	entryID, err := requireString(args, "entry_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entry, err := b.Pillar.GetEntry(ctx, entryID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(entry)
}

func listPillarEntriesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityID := getString(args, "entity_id", "")
	pillarType := getString(args, "pillar_type", "")
	entries, err := b.Pillar.ListEntries(ctx, projectID, entityID, pillarType, 50)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"entries": entries})
}

func updatePillarEntryHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	entryID, err := requireString(args, "entry_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	strategy := getString(args, "strategy", "")
	cadence := getString(args, "review_cadence", "")

	var gaps []pillar.Gap
	if gapsRaw, ok := args["gaps"]; ok && gapsRaw != nil {
		gapsJSON, _ := json.Marshal(gapsRaw)
		_ = json.Unmarshal(gapsJSON, &gaps)
	}

	entry, err := b.Pillar.UpdateEntry(ctx, entryID, strategy, gaps, cadence)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(entry)
}

func createPillarClaimHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	pillarEntryID, err := requireString(args, "pillar_entry_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	statement, err := requireString(args, "statement")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityRefID, err := requireString(args, "entity_ref_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityRefType := getString(args, "entity_ref_type", "")
	evidence := getString(args, "evidence", "")

	claim, err := b.Pillar.CreateClaim(ctx, pillarEntryID, statement, entityRefID, entityRefType, evidence)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(claim)
}

func listPillarClaimsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	pillarEntryID, err := requireString(args, "pillar_entry_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	claims, err := b.Pillar.ListClaims(ctx, pillarEntryID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"claims": claims})
}

func updatePillarClaimHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	claimID, err := requireString(args, "claim_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	status := getString(args, "status", "")
	evidence := getString(args, "evidence", "")

	claim, err := b.Pillar.UpdateClaimStatus(ctx, claimID, status, evidence)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(claim)
}

func evaluatePillarEntryHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	entryID, err := requireString(args, "entry_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	evals, err := b.Pillar.EvaluateEntry(ctx, entryID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"evaluations": evals})
}

func listPillarTemplatesHandler(b *Backend, _ context.Context, _ map[string]any) (*mcp.CallToolResult, any, error) {
	return jsonResult(map[string]any{"templates": b.Pillar.GetTemplates()})
}

func listPillarsDueForReviewHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entries, err := b.Pillar.ListDueForReview(ctx, projectID, time.Now().UTC(), 50)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"entries": entries})
}

func markPillarReviewedHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	entryID, err := requireString(args, "entry_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entry, err := b.Pillar.MarkReviewed(ctx, entryID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(entry)
}
