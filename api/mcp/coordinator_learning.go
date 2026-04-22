package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gabinante/flywheel/internal/ticket"
)

// RegisterCoordinatorLearningTools registers MCP tools that enable the coordinator
// learning loop: querying prior work history, surfacing calibration metrics, and
// recording coordinator-feedback findings when tickets fail.
//
// These tools are read-mostly and designed for the coordinator's investigate phase.
// They query the ticket store and findings layer to provide cross-session continuity.
func RegisterCoordinatorLearningTools(s *mcp.Server, b *Backend) {
	if b == nil {
		return
	}

	wrap := func(f func(*Backend, context.Context, map[string]any) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			if req != nil && req.Session != nil {
				ctx = context.WithValue(ctx, sessionContextKey{}, req.Session)
			}
			return f(b, ctx, args)
		}
	}

	// coordinator_get_history — Query prior ticket history and findings for a project.
	mcp.AddTool(s, &mcp.Tool{
		Name: "coordinator_get_history",
		Description: "Query prior ticket history and coordinator-feedback findings for a project. " +
			"Use during the investigate phase to see what work has been done, what failed, and why. " +
			"Returns recent tickets with outcomes, coordinator-feedback findings, and a history summary. " +
			"This is the primary tool for cross-session continuity — call it at the start of every " +
			"coordinator session to load prior context before authoring new tickets.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID to query history for"},
				"include_closed": map[string]any{
					"type":        "boolean",
					"description": "Include closed/done tickets in history (default true)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max tickets to return (default 50)",
					"minimum":     1,
					"maximum":     200,
				},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(handleCoordinatorGetHistory))

	// coordinator_calibration — Surface calibration metrics for coordinator self-improvement.
	mcp.AddTool(s, &mcp.Tool{
		Name: "coordinator_calibration",
		Description: "Surface calibration metrics for a project: ticket success rates, common failure patterns, " +
			"rejection reasons, and authoring quality trends. Use this to sharpen ticket decomposition over time. " +
			"Example output: 'You authored 12 tickets touching payments; 3 had validation rebounds due to acceptance ambiguity.'",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"agent_id": map[string]any{
					"type":        "string",
					"description": "Filter to tickets created by this agent (optional, shows all if omitted)",
				},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(handleCoordinatorCalibration))

	// coordinator_record_feedback �� Manually record coordinator feedback for a ticket.
	mcp.AddTool(s, &mcp.Tool{
		Name: "coordinator_record_feedback",
		Description: "Record coordinator-feedback finding for a ticket that failed, was rejected, or had issues. " +
			"Use when reviewing outcomes of prior work to capture lessons learned. " +
			"Automated feedback is also generated on reject/fail/replan events, but this tool allows " +
			"the coordinator to add richer context and analysis.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"ticket_id":  map[string]any{"type": "string", "description": "Ticket ID the feedback is about"},
				"feedback":   map[string]any{"type": "string", "description": "What went wrong and what to do differently"},
				"category": map[string]any{
					"type":        "string",
					"description": "Category of the feedback",
					"enum":        []string{"acceptance_ambiguity", "scope_too_large", "missing_dependency", "wrong_decomposition", "missing_context", "other"},
				},
				"agent_id": map[string]any{
					"type":        "string",
					"description": "Agent ID recording the feedback (optional, inferred from auth)",
				},
			},
			"required":             []string{"project_id", "ticket_id", "feedback", "category"},
			"additionalProperties": false,
		},
	}, wrap(handleCoordinatorRecordFeedback))
}

// --------------------------------------------------------------------------
// Handler: coordinator_get_history
// --------------------------------------------------------------------------

func handleCoordinatorGetHistory(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	if projectID == "" {
		return nil, nil, fmt.Errorf("project_id is required")
	}

	limit := 50
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	includeClosed := true
	if v, ok := args["include_closed"].(bool); ok {
		includeClosed = v
	}

	// 1. Query all tickets for the project (across all states).
	allTickets, err := b.Ticket.ListTickets(ctx, projectID, "", "")
	if err != nil {
		return nil, nil, fmt.Errorf("listing tickets: %w", err)
	}

	// 2. Build ticket summaries with outcome classification.
	type ticketSummary struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Type      string `json:"type"`
		State     string `json:"state"`
		Priority  int    `json:"priority"`
		CreatedBy string `json:"created_by"`
		CreatedAt string `json:"created_at"`
		Outcome   string `json:"outcome"` // success, rejected, failed, in_progress, cancelled
		HasOutput bool   `json:"has_output"`
	}

	var summaries []ticketSummary
	for _, t := range allTickets {
		if !includeClosed && t.State == ticket.StateClosed {
			continue
		}
		if len(summaries) >= limit {
			break
		}

		outcome := classifyTicketOutcome(t)
		summaries = append(summaries, ticketSummary{
			ID:        t.ID,
			Title:     t.Title,
			Type:      string(t.Type),
			State:     string(t.State),
			Priority:  int(t.Priority),
			CreatedBy: t.CreatedBy,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
			Outcome:   outcome,
			HasOutput: len(t.Outputs) > 0,
		})
	}

	// 3. Query coordinator-feedback findings if findings layer is available.
	var feedbackFindings []Finding
	if b.Findings != nil {
		feedbackFindings, err = b.Findings.QueryFindings(ctx, projectID, "coordinator-feedback", FindingsQueryOpts{
			ValidOnly: false, // include invalidated feedback too — we want the learning
			Limit:     50,
		})
		if err != nil {
			// Non-fatal: continue without findings.
			feedbackFindings = nil
		}
	}

	// 4. Build history summary stats.
	stats := computeHistoryStats(allTickets)

	result := map[string]any{
		"project_id":         projectID,
		"tickets":            summaries,
		"ticket_count":       len(summaries),
		"feedback_findings":  feedbackFindings,
		"feedback_count":     len(feedbackFindings),
		"stats":              stats,
		"has_findings_layer": b.Findings != nil,
	}

	return nil, result, nil
}

// --------------------------------------------------------------------------
// Handler: coordinator_calibration
// --------------------------------------------------------------------------

func handleCoordinatorCalibration(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	if projectID == "" {
		return nil, nil, fmt.Errorf("project_id is required")
	}
	agentID := argStr(args, "agent_id")

	// 1. Get all tickets for the project.
	allTickets, err := b.Ticket.ListTickets(ctx, projectID, "", "")
	if err != nil {
		return nil, nil, fmt.Errorf("listing tickets: %w", err)
	}

	// 2. Filter by agent if specified.
	var tickets []*ticket.Ticket
	for _, t := range allTickets {
		if agentID == "" || t.CreatedBy == agentID {
			tickets = append(tickets, t)
		}
	}

	// 3. Compute calibration metrics.
	calibration := computeCalibration(tickets)

	// 4. Query failure-pattern findings if available.
	var failurePatterns []Finding
	if b.Findings != nil {
		failurePatterns, err = b.Findings.QueryFindings(ctx, projectID, "coordinator-feedback failure pattern", FindingsQueryOpts{
			ValidOnly: false,
			Limit:     30,
		})
		if err != nil {
			failurePatterns = nil
		}
	}

	// 5. Build category breakdown from feedback findings.
	categoryBreakdown := make(map[string]int)
	if b.Findings != nil {
		allFeedback, _ := b.Findings.QueryFindings(ctx, projectID, "coordinator-feedback", FindingsQueryOpts{
			ValidOnly: false,
			Limit:     200,
		})
		for _, f := range allFeedback {
			for _, tag := range f.Tags {
				if isCoordinatorFeedbackCategory(tag) {
					categoryBreakdown[tag]++
				}
			}
		}
	}

	result := map[string]any{
		"project_id":         projectID,
		"agent_id":           agentID,
		"calibration":        calibration,
		"failure_patterns":   failurePatterns,
		"category_breakdown": categoryBreakdown,
		"has_findings_layer": b.Findings != nil,
	}

	return nil, result, nil
}

// --------------------------------------------------------------------------
// Handler: coordinator_record_feedback
// --------------------------------------------------------------------------

func handleCoordinatorRecordFeedback(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	ticketID := argStr(args, "ticket_id")
	feedback := argStr(args, "feedback")
	category := argStr(args, "category")
	agentID := argStr(args, "agent_id")

	if projectID == "" || ticketID == "" || feedback == "" || category == "" {
		return nil, nil, fmt.Errorf("project_id, ticket_id, feedback, and category are all required")
	}

	// Validate the ticket exists and get context.
	t, err := b.Ticket.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting ticket: %w", err)
	}

	if b.Findings == nil {
		// Without findings layer, return a descriptive message.
		return nil, map[string]any{
			"recorded":          false,
			"reason":            "findings layer not configured — feedback not persisted",
			"ticket_id":         ticketID,
			"feedback":          feedback,
			"category":          category,
			"recommendation":    "Configure a FindingsProvider (Weaviate or in-memory) to enable persistent coordinator feedback",
		}, nil
	}

	finding := Finding{
		ProjectID:      projectID,
		Claim:          fmt.Sprintf("Coordinator feedback for %s: %s", ticketID, feedback),
		FindingType:    "investigation", // closest existing type for coordinator meta-findings
		Confidence:     0.9,
		SourceType:     "review_comment",
		SourceTicketID: ticketID,
		SourceAgentID:  agentID,
		TicketRefs:     []string{ticketID},
		Tags:           []string{"coordinator-feedback", category, "author_role:coordinator"},
		Summary:        fmt.Sprintf("[%s] %s — %s", category, t.Title, feedback),
	}

	saved, err := b.Findings.SaveFinding(ctx, finding)
	if err != nil {
		return nil, nil, fmt.Errorf("saving feedback finding: %w", err)
	}

	return nil, map[string]any{
		"recorded":   true,
		"finding_id": saved.ID,
		"ticket_id":  ticketID,
		"category":   category,
		"feedback":   feedback,
	}, nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// classifyTicketOutcome determines the outcome category of a ticket based on its state
// and prior attempts.
func classifyTicketOutcome(t *ticket.Ticket) string {
	switch t.State {
	case ticket.StateClosed:
		return "success"
	case ticket.StateDraft:
		if len(t.Context.PriorAttempts) > 0 {
			return "failed"
		}
		return "pending"
	case ticket.StateAwaitingValidation:
		return "awaiting_review"
	case ticket.StateAwaitingInput:
		return "needs_input"
	case ticket.StateExecuting, ticket.StatePlanning:
		return "in_progress"
	default:
		return string(t.State)
	}
}

// historyStats holds summary statistics for a project's ticket history.
type historyStats struct {
	Total          int `json:"total"`
	Closed         int `json:"closed"`
	InProgress     int `json:"in_progress"`
	Failed         int `json:"failed"`
	AwaitingReview int `json:"awaiting_review"`
	SuccessRate    string `json:"success_rate"` // e.g. "85.7%"
}

// computeHistoryStats computes summary statistics from a list of tickets.
func computeHistoryStats(tickets []*ticket.Ticket) historyStats {
	stats := historyStats{Total: len(tickets)}
	var terminal int // tickets that reached a terminal outcome

	for _, t := range tickets {
		switch t.State {
		case ticket.StateClosed:
			stats.Closed++
			terminal++
		case ticket.StateExecuting, ticket.StatePlanning, ticket.StateSpecced:
			stats.InProgress++
		case ticket.StateAwaitingValidation:
			stats.AwaitingReview++
		case ticket.StateDraft:
			if len(t.Context.PriorAttempts) > 0 {
				stats.Failed++
				terminal++
			}
		}
	}

	if terminal > 0 {
		rate := float64(stats.Closed) / float64(terminal) * 100
		stats.SuccessRate = fmt.Sprintf("%.1f%%", rate)
	} else {
		stats.SuccessRate = "N/A"
	}

	return stats
}

// calibrationMetrics holds computed calibration data for coordinator self-assessment.
type calibrationMetrics struct {
	TotalAuthored       int     `json:"total_authored"`
	SuccessCount        int     `json:"success_count"`
	RejectedCount       int     `json:"rejected_count"`
	FailedCount         int     `json:"failed_count"`
	InProgressCount     int     `json:"in_progress_count"`
	SuccessRate         string  `json:"success_rate"`
	AvgAttemptsPerClose float64 `json:"avg_attempts_per_close"`
	TicketsWithRetries  int     `json:"tickets_with_retries"`
	Summary             string  `json:"summary"`
}

// computeCalibration computes calibration metrics from a set of tickets.
func computeCalibration(tickets []*ticket.Ticket) calibrationMetrics {
	m := calibrationMetrics{
		TotalAuthored: len(tickets),
	}

	var totalAttempts int
	var terminal int

	for _, t := range tickets {
		attempts := len(t.Context.PriorAttempts) + 1 // current attempt counts as 1
		if len(t.Context.PriorAttempts) > 0 {
			m.TicketsWithRetries++
		}

		switch t.State {
		case ticket.StateClosed:
			m.SuccessCount++
			terminal++
			totalAttempts += attempts
		case ticket.StateDraft:
			if len(t.Context.PriorAttempts) > 0 {
				m.FailedCount++
				terminal++
			}
		case ticket.StateExecuting, ticket.StatePlanning, ticket.StateSpecced,
			ticket.StateAwaitingValidation, ticket.StateValidated,
			ticket.StateDeploying, ticket.StateObserving:
			m.InProgressCount++
		}

		// Count rejected attempts from prior_attempts.
		for _, attempt := range t.Context.PriorAttempts {
			if attempt.Outcome == "rejected" {
				m.RejectedCount++
			}
		}
	}

	if terminal > 0 {
		rate := float64(m.SuccessCount) / float64(terminal) * 100
		m.SuccessRate = fmt.Sprintf("%.1f%%", rate)
	} else {
		m.SuccessRate = "N/A"
	}

	if m.SuccessCount > 0 {
		m.AvgAttemptsPerClose = float64(totalAttempts) / float64(m.SuccessCount)
	}

	// Build human-readable summary.
	m.Summary = buildCalibrationSummary(m)

	return m
}

// buildCalibrationSummary generates a human-readable calibration summary.
func buildCalibrationSummary(m calibrationMetrics) string {
	if m.TotalAuthored == 0 {
		return "No tickets authored yet — no calibration data available."
	}

	summary := fmt.Sprintf("Authored %d tickets", m.TotalAuthored)

	if m.SuccessCount > 0 {
		summary += fmt.Sprintf("; %d closed successfully (%s success rate)", m.SuccessCount, m.SuccessRate)
	}

	if m.RejectedCount > 0 {
		summary += fmt.Sprintf("; %d rejections across %d tickets", m.RejectedCount, m.TicketsWithRetries)
	}

	if m.FailedCount > 0 {
		summary += fmt.Sprintf("; %d failed", m.FailedCount)
	}

	if m.InProgressCount > 0 {
		summary += fmt.Sprintf("; %d still in progress", m.InProgressCount)
	}

	if m.AvgAttemptsPerClose > 1.1 {
		summary += fmt.Sprintf(". Avg %.1f attempts per successful close — consider tightening acceptance criteria.", m.AvgAttemptsPerClose)
	}

	return summary + "."
}

// isCoordinatorFeedbackCategory checks if a tag is a known feedback category.
func isCoordinatorFeedbackCategory(tag string) bool {
	categories := map[string]bool{
		"acceptance_ambiguity":  true,
		"scope_too_large":      true,
		"missing_dependency":   true,
		"wrong_decomposition":  true,
		"missing_context":      true,
		"other":                true,
	}
	return categories[tag]
}
