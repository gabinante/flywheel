// Package linear implements the mirror.Adapter for Linear issue tracking.
// It provides one-way ticket mirroring from Warrant to Linear.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gabinante/flywheel/internal/mirror"
)

const (
	linearAPIURL = "https://api.linear.app/graphql"
	adapterName  = "linear"
)

// Client wraps the Linear GraphQL API.
type Client struct {
	httpClient *http.Client
	apiKey     string
}

// NewClient creates a new Linear API client.
func NewClient(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{},
		apiKey:     apiKey,
	}
}

// Adapter implements mirror.Adapter for Linear.
// It uses per-project config (team ID, labels) from the mirror.Config
// passed to each method, allowing a single adapter to serve multiple projects.
type Adapter struct {
	client *Client
}

// NewAdapter creates a new Linear mirror adapter.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{
		client: client,
	}
}

// Name returns the adapter identifier.
func (a *Adapter) Name() string {
	return adapterName
}

// teamID extracts the Linear team ID from project config.
func teamID(cfg *mirror.Config) string {
	if cfg.Linear == nil {
		return ""
	}
	return cfg.Linear.TeamID
}

// labels extracts the Linear labels from project config.
func labels(cfg *mirror.Config) []string {
	if cfg.Linear == nil {
		return nil
	}
	return cfg.Linear.Labels
}

// CreateTicket creates a new Linear issue mirroring the given ticket data.
func (a *Adapter) CreateTicket(ctx context.Context, cfg *mirror.Config, data mirror.TicketData) (string, error) {
	tid := teamID(cfg)
	if tid == "" {
		return "", fmt.Errorf("linear: team_id not configured")
	}

	description := formatDescription(data)

	variables := map[string]any{
		"teamId":      tid,
		"title":       fmt.Sprintf("[%s] %s", data.ID, data.Title),
		"description": description,
		"priority":    linearPriority(data.Priority),
	}
	if lbls := labels(cfg); len(lbls) > 0 {
		variables["labelIds"] = lbls
	}

	query := `mutation CreateIssue($teamId: String!, $title: String!, $description: String, $priority: Int, $labelIds: [String!]) {
		issueCreate(input: {
			teamId: $teamId,
			title: $title,
			description: $description,
			priority: $priority,
			labelIds: $labelIds
		}) {
			success
			issue {
				id
				identifier
			}
		}
	}`

	resp, err := a.client.graphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("linear: create issue: %w", err)
	}

	issueCreate, ok := resp["issueCreate"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("linear: unexpected response shape")
	}
	success, _ := issueCreate["success"].(bool)
	if !success {
		return "", fmt.Errorf("linear: issue creation failed")
	}
	issue, ok := issueCreate["issue"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("linear: no issue in response")
	}
	identifier, _ := issue["identifier"].(string)
	if identifier == "" {
		identifier, _ = issue["id"].(string)
	}
	return identifier, nil
}

// UpdateState updates the Linear issue's workflow state.
func (a *Adapter) UpdateState(ctx context.Context, cfg *mirror.Config, externalID string, newState string, _ mirror.TicketData) error {
	tid := teamID(cfg)
	if tid == "" {
		return fmt.Errorf("linear: team_id not configured")
	}

	// Resolve the state ID from the state name.
	stateID, err := a.resolveStateID(ctx, tid, newState)
	if err != nil {
		return err
	}

	issueID, err := a.resolveIssueID(ctx, externalID)
	if err != nil {
		return err
	}

	query := `mutation UpdateIssueState($issueId: String!, $stateId: String!) {
		issueUpdate(id: $issueId, input: { stateId: $stateId }) {
			success
		}
	}`

	resp, err := a.client.graphQL(ctx, query, map[string]any{
		"issueId": issueID,
		"stateId": stateID,
	})
	if err != nil {
		return fmt.Errorf("linear: update state: %w", err)
	}
	issueUpdate, ok := resp["issueUpdate"].(map[string]any)
	if !ok {
		return fmt.Errorf("linear: unexpected response shape")
	}
	if success, _ := issueUpdate["success"].(bool); !success {
		return fmt.Errorf("linear: state update failed")
	}
	return nil
}

// UpdateFields adds a comment with updated custom field data to the Linear issue.
// Linear doesn't have arbitrary custom fields in the same way Jira does,
// so we use issue comments for structured data.
func (a *Adapter) UpdateFields(ctx context.Context, _ *mirror.Config, externalID string, fields mirror.CustomFields) error {
	if fields.RiskClassification == "" && fields.EnvironmentTarget == "" && len(fields.LinkedServices) == 0 {
		return nil // nothing to update
	}

	issueID, err := a.resolveIssueID(ctx, externalID)
	if err != nil {
		return err
	}

	var parts []string
	parts = append(parts, "**Warrant Metadata Update**")
	if fields.RiskClassification != "" {
		parts = append(parts, fmt.Sprintf("- Risk: %s", fields.RiskClassification))
	}
	if fields.EnvironmentTarget != "" {
		parts = append(parts, fmt.Sprintf("- Environment: %s", fields.EnvironmentTarget))
	}
	if len(fields.LinkedServices) > 0 {
		parts = append(parts, fmt.Sprintf("- Services: %s", strings.Join(fields.LinkedServices, ", ")))
	}

	body := strings.Join(parts, "\n")
	query := `mutation CreateComment($issueId: String!, $body: String!) {
		commentCreate(input: { issueId: $issueId, body: $body }) {
			success
		}
	}`
	_, err = a.client.graphQL(ctx, query, map[string]any{
		"issueId": issueID,
		"body":    body,
	})
	if err != nil {
		return fmt.Errorf("linear: update fields comment: %w", err)
	}
	return nil
}

// CloseTicket marks the Linear issue as done with a closing comment.
func (a *Adapter) CloseTicket(ctx context.Context, cfg *mirror.Config, externalID string, contextURL string, _ mirror.TicketData) error {
	tid := teamID(cfg)

	issueID, err := a.resolveIssueID(ctx, externalID)
	if err != nil {
		return err
	}

	// Add closing comment with context link.
	comment := fmt.Sprintf("**Closed in Warrant**\n\nThis ticket was completed in Warrant. Full execution context and trace available at:\n%s", contextURL)
	commentQuery := `mutation CreateComment($issueId: String!, $body: String!) {
		commentCreate(input: { issueId: $issueId, body: $body }) {
			success
		}
	}`
	_, _ = a.client.graphQL(ctx, commentQuery, map[string]any{
		"issueId": issueID,
		"body":    comment,
	})

	// Resolve the "Done" state and transition.
	if tid == "" {
		return nil
	}
	stateID, err := a.resolveDoneStateID(ctx, tid)
	if err != nil {
		// If we can't find a Done state, just leave the comment.
		return nil
	}

	query := `mutation UpdateIssueState($issueId: String!, $stateId: String!) {
		issueUpdate(id: $issueId, input: { stateId: $stateId }) {
			success
		}
	}`
	_, err = a.client.graphQL(ctx, query, map[string]any{
		"issueId": issueID,
		"stateId": stateID,
	})
	if err != nil {
		return fmt.Errorf("linear: close issue: %w", err)
	}
	return nil
}

// resolveStateID finds the workflow state ID by name for the team.
func (a *Adapter) resolveStateID(ctx context.Context, tid string, stateName string) (string, error) {
	query := `query TeamStates($teamId: String!) {
		team(id: $teamId) {
			states {
				nodes {
					id
					name
				}
			}
		}
	}`
	resp, err := a.client.graphQL(ctx, query, map[string]any{"teamId": tid})
	if err != nil {
		return "", fmt.Errorf("linear: resolve state: %w", err)
	}

	team, ok := resp["team"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("linear: no team in response")
	}
	states, ok := team["states"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("linear: no states in response")
	}
	nodes, ok := states["nodes"].([]any)
	if !ok {
		return "", fmt.Errorf("linear: no state nodes in response")
	}

	for _, node := range nodes {
		n, ok := node.(map[string]any)
		if !ok {
			continue
		}
		name, _ := n["name"].(string)
		if strings.EqualFold(name, stateName) {
			id, _ := n["id"].(string)
			return id, nil
		}
	}
	return "", fmt.Errorf("linear: state %q not found for team %s", stateName, tid)
}

// resolveDoneStateID finds a "Done" or "Completed" state for the team.
func (a *Adapter) resolveDoneStateID(ctx context.Context, tid string) (string, error) {
	for _, name := range []string{"Done", "Completed", "Closed"} {
		if id, err := a.resolveStateID(ctx, tid, name); err == nil {
			return id, nil
		}
	}
	return "", fmt.Errorf("linear: no done/completed/closed state found")
}

// resolveIssueID resolves an issue identifier (e.g. "ENG-123") to its internal UUID.
func (a *Adapter) resolveIssueID(ctx context.Context, identifier string) (string, error) {
	query := `query GetIssue($identifier: String!) {
		issue(id: $identifier) {
			id
		}
	}`
	resp, err := a.client.graphQL(ctx, query, map[string]any{"identifier": identifier})
	if err != nil {
		// The identifier might already be the UUID.
		return identifier, nil
	}
	issue, ok := resp["issue"].(map[string]any)
	if !ok {
		return identifier, nil
	}
	id, _ := issue["id"].(string)
	if id == "" {
		return identifier, nil
	}
	return id, nil
}

// graphQL executes a GraphQL query against the Linear API.
func (c *Client) graphQL(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	body := map[string]any{
		"query":     query,
		"variables": variables,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, linearAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linear API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("linear: unmarshal response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("linear GraphQL error: %s", result.Errors[0].Message)
	}
	return result.Data, nil
}

// formatDescription creates a markdown description for the Linear issue.
func formatDescription(data mirror.TicketData) string {
	var b strings.Builder
	b.WriteString(data.Description)
	b.WriteString("\n\n---\n")
	b.WriteString(fmt.Sprintf("**Warrant Ticket:** %s\n", data.ID))
	b.WriteString(fmt.Sprintf("**Type:** %s\n", data.Type))
	b.WriteString(fmt.Sprintf("**Priority:** P%d\n", data.Priority))
	if data.Environment != "" {
		b.WriteString(fmt.Sprintf("**Environment:** %s\n", data.Environment))
	}
	if data.URL != "" {
		b.WriteString(fmt.Sprintf("\n[View in Warrant](%s)\n", data.URL))
	}
	return b.String()
}

// linearPriority maps our 0-3 priority to Linear's 1-4 scale.
// Linear: 0=No priority, 1=Urgent, 2=High, 3=Medium, 4=Low
func linearPriority(p int) int {
	switch p {
	case 0:
		return 1 // P0 -> Urgent
	case 1:
		return 2 // P1 -> High
	case 2:
		return 3 // P2 -> Medium
	case 3:
		return 4 // P3 -> Low
	default:
		return 3 // default Medium
	}
}
