// Package jira implements the mirror.Adapter for Jira issue tracking.
// It provides one-way ticket mirroring from Warrant to Jira Cloud/Server.
package jira

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

const adapterName = "jira"

// Client wraps the Jira REST API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	email      string
	apiToken   string
}

// NewClient creates a new Jira API client.
func NewClient(baseURL, email, apiToken string) *Client {
	return &Client{
		httpClient: &http.Client{},
		baseURL:    strings.TrimRight(baseURL, "/"),
		email:      email,
		apiToken:   apiToken,
	}
}

// Adapter implements mirror.Adapter for Jira.
// It uses per-project config (project key, issue type) from the mirror.Config
// passed to each method, allowing a single adapter to serve multiple projects.
type Adapter struct {
	client *Client
}

// NewAdapter creates a new Jira mirror adapter.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{
		client: client,
	}
}

// Name returns the adapter identifier.
func (a *Adapter) Name() string {
	return adapterName
}

// projectKey extracts the Jira project key from per-project config.
func projectKey(cfg *mirror.Config) string {
	if cfg.Jira == nil {
		return ""
	}
	return cfg.Jira.ProjectKey
}

// issueType extracts the Jira issue type from per-project config.
func issueType(cfg *mirror.Config) string {
	if cfg.Jira == nil || cfg.Jira.IssueType == "" {
		return "Task"
	}
	return cfg.Jira.IssueType
}

// CreateTicket creates a new Jira issue mirroring the given ticket data.
func (a *Adapter) CreateTicket(ctx context.Context, cfg *mirror.Config, data mirror.TicketData) (string, error) {
	pKey := projectKey(cfg)
	if pKey == "" {
		return "", fmt.Errorf("jira: project_key not configured")
	}

	description := formatDescription(data)

	payload := map[string]any{
		"fields": map[string]any{
			"project": map[string]string{
				"key": pKey,
			},
			"summary":   fmt.Sprintf("[%s] %s", data.ID, data.Title),
			"issuetype": map[string]string{"name": issueType(cfg)},
			"description": map[string]any{
				"type":    "doc",
				"version": 1,
				"content": []map[string]any{
					{
						"type": "paragraph",
						"content": []map[string]any{
							{"type": "text", "text": description},
						},
					},
				},
			},
			"priority": map[string]string{"name": jiraPriority(data.Priority)},
		},
	}

	resp, err := a.client.request(ctx, http.MethodPost, "/rest/api/3/issue", payload)
	if err != nil {
		return "", fmt.Errorf("jira: create issue: %w", err)
	}

	key, _ := resp["key"].(string)
	if key == "" {
		id, _ := resp["id"].(string)
		return id, nil
	}
	return key, nil
}

// UpdateState transitions the Jira issue to a new status.
func (a *Adapter) UpdateState(ctx context.Context, _ *mirror.Config, externalID string, newState string, _ mirror.TicketData) error {
	// Find the transition ID for the target state.
	transitionID, err := a.resolveTransitionID(ctx, externalID, newState)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"transition": map[string]string{
			"id": transitionID,
		},
	}

	_, err = a.client.request(ctx, http.MethodPost,
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", externalID), payload)
	if err != nil {
		return fmt.Errorf("jira: transition issue: %w", err)
	}
	return nil
}

// UpdateFields updates custom fields on the Jira issue.
func (a *Adapter) UpdateFields(ctx context.Context, _ *mirror.Config, externalID string, fields mirror.CustomFields) error {
	if fields.RiskClassification == "" && fields.EnvironmentTarget == "" && len(fields.LinkedServices) == 0 {
		return nil
	}

	// Build update payload with custom field mappings.
	// Uses Jira's standard fields where available, falls back to labels for metadata.
	updateFields := map[string]any{}

	// Use labels to store structured metadata (common Jira pattern).
	var lbls []string
	if fields.RiskClassification != "" {
		lbls = append(lbls, fmt.Sprintf("risk:%s", fields.RiskClassification))
	}
	if fields.EnvironmentTarget != "" {
		lbls = append(lbls, fmt.Sprintf("env:%s", fields.EnvironmentTarget))
		// Also set the environment field if available.
		updateFields["environment"] = map[string]string{"name": fields.EnvironmentTarget}
	}
	for _, svc := range fields.LinkedServices {
		lbls = append(lbls, fmt.Sprintf("service:%s", svc))
	}

	if len(lbls) > 0 {
		updateFields["labels"] = lbls
	}

	if len(updateFields) == 0 {
		return nil
	}

	payload := map[string]any{
		"fields": updateFields,
	}

	_, err := a.client.request(ctx, http.MethodPut,
		fmt.Sprintf("/rest/api/3/issue/%s", externalID), payload)
	if err != nil {
		return fmt.Errorf("jira: update fields: %w", err)
	}
	return nil
}

// CloseTicket transitions the Jira issue to Done/Closed with a comment.
func (a *Adapter) CloseTicket(ctx context.Context, _ *mirror.Config, externalID string, contextURL string, _ mirror.TicketData) error {
	// Add closing comment with context link.
	comment := fmt.Sprintf("Closed in Warrant. Full execution context and trace available at: %s", contextURL)
	a.addComment(ctx, externalID, comment)

	// Try to transition to Done/Closed.
	for _, targetState := range []string{"Done", "Closed", "Resolved"} {
		transitionID, err := a.resolveTransitionID(ctx, externalID, targetState)
		if err != nil {
			continue
		}
		payload := map[string]any{
			"transition": map[string]string{"id": transitionID},
		}
		_, err = a.client.request(ctx, http.MethodPost,
			fmt.Sprintf("/rest/api/3/issue/%s/transitions", externalID), payload)
		if err == nil {
			return nil
		}
	}

	// If no closing transition found, the comment is still added.
	return nil
}

// resolveTransitionID finds the Jira transition ID for the target status name.
func (a *Adapter) resolveTransitionID(ctx context.Context, issueKey string, targetStatus string) (string, error) {
	resp, err := a.client.request(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey), nil)
	if err != nil {
		return "", fmt.Errorf("jira: get transitions: %w", err)
	}

	transitions, ok := resp["transitions"].([]any)
	if !ok {
		return "", fmt.Errorf("jira: no transitions in response")
	}

	for _, t := range transitions {
		tr, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tr["name"].(string)
		if strings.EqualFold(name, targetStatus) {
			id, _ := tr["id"].(string)
			return id, nil
		}
		// Also check the "to" status name.
		if to, ok := tr["to"].(map[string]any); ok {
			toName, _ := to["name"].(string)
			if strings.EqualFold(toName, targetStatus) {
				id, _ := tr["id"].(string)
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("jira: transition to %q not available for %s", targetStatus, issueKey)
}

// addComment adds a comment to a Jira issue.
func (a *Adapter) addComment(ctx context.Context, issueKey string, body string) {
	payload := map[string]any{
		"body": map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []map[string]any{
				{
					"type": "paragraph",
					"content": []map[string]any{
						{"type": "text", "text": body},
					},
				},
			},
		},
	}
	_, _ = a.client.request(ctx, http.MethodPost,
		fmt.Sprintf("/rest/api/3/issue/%s/comment", issueKey), payload)
}

// request executes an HTTP request against the Jira API.
func (c *Client) request(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.email, c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Some Jira endpoints return 204 No Content.
	if len(respBody) == 0 {
		return map[string]any{}, nil
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("jira: unmarshal response: %w", err)
	}
	return result, nil
}

// formatDescription creates a plain text description for the Jira issue.
func formatDescription(data mirror.TicketData) string {
	var b strings.Builder
	b.WriteString(data.Description)
	b.WriteString("\n\n---\n")
	b.WriteString(fmt.Sprintf("Warrant Ticket: %s\n", data.ID))
	b.WriteString(fmt.Sprintf("Type: %s\n", data.Type))
	b.WriteString(fmt.Sprintf("Priority: P%d\n", data.Priority))
	if data.Environment != "" {
		b.WriteString(fmt.Sprintf("Environment: %s\n", data.Environment))
	}
	if data.URL != "" {
		b.WriteString(fmt.Sprintf("\nView in Warrant: %s\n", data.URL))
	}
	return b.String()
}

// jiraPriority maps our 0-3 priority to Jira priority names.
func jiraPriority(p int) string {
	switch p {
	case 0:
		return "Highest"
	case 1:
		return "High"
	case 2:
		return "Medium"
	case 3:
		return "Low"
	default:
		return "Medium"
	}
}
