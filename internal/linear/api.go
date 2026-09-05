package linear

import (
	"context"
	"fmt"
	"time"
)

const issueFields = `
	id identifier title description url priority branchName createdAt updatedAt completedAt canceledAt
	state { id name type position }
	team { id key name }
	assignee { id name email }
	project { id }
	labels { nodes { name } }
	attachments { nodes { url title sourceType } }`

type rawIssue struct {
	Issue
	LabelsConn struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	AttachmentsConn struct {
		Nodes []struct {
			URL        string `json:"url"`
			Title      string `json:"title"`
			SourceType string `json:"sourceType"`
		} `json:"nodes"`
	} `json:"attachments"`
}

func (r rawIssue) toIssue() Issue {
	is := r.Issue
	for _, l := range r.LabelsConn.Nodes {
		is.Labels = append(is.Labels, l.Name)
	}
	for _, a := range r.AttachmentsConn.Nodes {
		is.Attachments = append(is.Attachments, a)
	}
	return is
}

type rawProject struct {
	Project
	TeamsConn struct {
		Nodes []Team `json:"nodes"`
	} `json:"teams"`
}

func (r rawProject) toProject() Project {
	p := r.Project
	p.Teams = append(p.Teams, r.TeamsConn.Nodes...)
	return p
}

// Viewer returns the authenticated user.
func (c *Client) Viewer(ctx context.Context) (*Viewer, error) {
	var out struct {
		Viewer Viewer `json:"viewer"`
	}
	if err := c.Query(ctx, `query { viewer { id name email } }`, nil, &out); err != nil {
		return nil, err
	}
	return &out.Viewer, nil
}

// LedProjects returns projects the viewer leads (excluding completed/canceled ones).
func (c *Client) LedProjects(ctx context.Context) ([]Project, error) {
	var out struct {
		Projects struct {
			Nodes []rawProject `json:"nodes"`
		} `json:"projects"`
	}
	q := `query { projects(first: 100, filter: { lead: { isMe: { eq: true } } }) {
		nodes { id name url status { name type } lead { id } teams { nodes { id key name } } } } }`
	if err := c.Query(ctx, q, nil, &out); err != nil {
		return nil, err
	}
	var res []Project
	for _, rp := range out.Projects.Nodes {
		p := rp.toProject()
		switch p.Status.Type {
		case "completed", "canceled":
			continue
		}
		res = append(res, p)
	}
	return res, nil
}

// ProjectByID returns one project.
func (c *Client) ProjectByID(ctx context.Context, id string) (*Project, error) {
	var out struct {
		Project rawProject `json:"project"`
	}
	q := `query($id: String!) { project(id: $id) { id name url status { name type } lead { id } teams { nodes { id key name } } } }`
	if err := c.Query(ctx, q, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	p := out.Project.toProject()
	return &p, nil
}

// IssuesUpdatedSince pages through a project's issues updated after since (all issues when zero).
func (c *Client) IssuesUpdatedSince(ctx context.Context, projectID string, since time.Time, visit func(Issue) error) error {
	filter := map[string]any{"project": map[string]any{"id": map[string]any{"eq": projectID}}}
	if !since.IsZero() {
		filter["updatedAt"] = map[string]any{"gt": since.UTC().Format(time.RFC3339Nano)}
	}
	var after *string
	for {
		var out struct {
			Issues struct {
				Nodes    []rawIssue `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		}
		q := `query($filter: IssueFilter, $after: String) {
			issues(first: 100, after: $after, orderBy: updatedAt, filter: $filter, includeArchived: true) {
				nodes {` + issueFields + `}
				pageInfo { hasNextPage endCursor }
			} }`
		vars := map[string]any{"filter": filter}
		if after != nil {
			vars["after"] = *after
		}
		if err := c.Query(ctx, q, vars, &out); err != nil {
			return err
		}
		for _, ri := range out.Issues.Nodes {
			if err := visit(ri.toIssue()); err != nil {
				return err
			}
		}
		if !out.Issues.PageInfo.HasNextPage || out.Issues.PageInfo.EndCursor == "" {
			return nil
		}
		cursor := out.Issues.PageInfo.EndCursor
		after = &cursor
	}
}

// IssueByIdentifier fetches one issue by identifier (e.g. RLETD-465) or id.
func (c *Client) IssueByIdentifier(ctx context.Context, identifier string) (*Issue, error) {
	var out struct {
		Issue rawIssue `json:"issue"`
	}
	q := `query($id: String!) { issue(id: $id) {` + issueFields + `} }`
	if err := c.Query(ctx, q, map[string]any{"id": identifier}, &out); err != nil {
		return nil, err
	}
	is := out.Issue.toIssue()
	return &is, nil
}

// TeamStates returns a team's workflow states ordered by position.
func (c *Client) TeamStates(ctx context.Context, teamID string) ([]WorkflowState, error) {
	var out struct {
		Team struct {
			States struct {
				Nodes []WorkflowState `json:"nodes"`
			} `json:"states"`
		} `json:"team"`
	}
	q := `query($id: String!) { team(id: $id) { states { nodes { id name type position } } } }`
	if err := c.Query(ctx, q, map[string]any{"id": teamID}, &out); err != nil {
		return nil, err
	}
	return out.Team.States.Nodes, nil
}

// CreateIssueInput is the subset of IssueCreateInput Flywheel uses.
type CreateIssueInput struct {
	ID          string
	TeamID      string
	ProjectID   string
	Title       string
	Description string
	Priority    int // Linear scale 0-4
	StateID     string
	AssigneeID  string
}

// CreateIssue creates an issue and returns it.
func (c *Client) CreateIssue(ctx context.Context, in CreateIssueInput) (*Issue, error) {
	input := map[string]any{"teamId": in.TeamID, "title": in.Title}
	if in.ID != "" {
		input["id"] = in.ID
	}
	if in.ProjectID != "" {
		input["projectId"] = in.ProjectID
	}
	if in.Description != "" {
		input["description"] = in.Description
	}
	if in.Priority > 0 {
		input["priority"] = in.Priority
	}
	if in.StateID != "" {
		input["stateId"] = in.StateID
	}
	if in.AssigneeID != "" {
		input["assigneeId"] = in.AssigneeID
	}
	var out struct {
		IssueCreate struct {
			Success bool     `json:"success"`
			Issue   rawIssue `json:"issue"`
		} `json:"issueCreate"`
	}
	q := `mutation($input: IssueCreateInput!) { issueCreate(input: $input) { success issue {` + issueFields + `} } }`
	if err := c.Query(ctx, q, map[string]any{"input": input}, &out); err != nil {
		if in.ID != "" {
			if existing, lookupErr := c.IssueByID(ctx, in.ID); lookupErr == nil && existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}
	is := out.IssueCreate.Issue.toIssue()
	return &is, nil
}

// UpdateIssueState moves an issue to a workflow state.
func (c *Client) UpdateIssueState(ctx context.Context, issueID, stateID string) (*Issue, error) {
	var out struct {
		IssueUpdate struct {
			Success bool     `json:"success"`
			Issue   rawIssue `json:"issue"`
		} `json:"issueUpdate"`
	}
	q := `mutation($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success issue {` + issueFields + `} } }`
	if err := c.Query(ctx, q, map[string]any{"id": issueID, "input": map[string]any{"stateId": stateID}}, &out); err != nil {
		return nil, err
	}
	is := out.IssueUpdate.Issue.toIssue()
	return &is, nil
}

// CreateComment posts a comment on an issue.
func (c *Client) CreateComment(ctx context.Context, issueID, body string) (*Comment, error) {
	return c.CreateCommentOnce(ctx, issueID, body, "")
}
func (c *Client) CreateCommentOnce(ctx context.Context, issueID, body, id string) (*Comment, error) {
	var out struct {
		CommentCreate struct {
			Success bool    `json:"success"`
			Comment Comment `json:"comment"`
		} `json:"commentCreate"`
	}
	q := `mutation($input: CommentCreateInput!) { commentCreate(input: $input) { success comment { id url } } }`
	input := map[string]any{"issueId": issueID, "body": body}
	if id != "" {
		input["id"] = id
	}
	if err := c.Query(ctx, q, map[string]any{"input": input}, &out); err != nil {
		if id != "" {
			var existing struct {
				Comment *Comment `json:"comment"`
			}
			if lookupErr := c.Query(ctx, `query($id: String!){comment(id:$id){id url}}`, map[string]any{"id": id}, &existing); lookupErr == nil && existing.Comment != nil && existing.Comment.ID == id {
				return existing.Comment, nil
			}
		}
		return nil, err
	}
	if !out.CommentCreate.Success {
		return nil, fmt.Errorf("Linear did not create the comment")
	}
	return &out.CommentCreate.Comment, nil
}

// AttachPullRequest links a GitHub PR to an issue (Linear renders it as a PR attachment).
func (c *Client) AttachPullRequest(ctx context.Context, issueID, prURL string) error {
	var out struct {
		Result struct {
			Success bool `json:"success"`
		} `json:"attachmentLinkGitHubPR"`
	}
	q := `mutation($issueId: String!, $url: String!) { attachmentLinkGitHubPR(issueId: $issueId, url: $url) { success } }`
	return c.Query(ctx, q, map[string]any{"issueId": issueID, "url": prURL}, &out)
}

// CreateProjectUpdate posts a project status update. health is onTrack, atRisk, or offTrack.
func (c *Client) CreateProjectUpdate(ctx context.Context, projectID, body, health string) (string, error) {
	input := map[string]any{"projectId": projectID, "body": body}
	if health != "" {
		input["health"] = health
	}
	var out struct {
		ProjectUpdateCreate struct {
			Success       bool `json:"success"`
			ProjectUpdate struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"projectUpdate"`
		} `json:"projectUpdateCreate"`
	}
	q := `mutation($input: ProjectUpdateCreateInput!) { projectUpdateCreate(input: $input) { success projectUpdate { id url } } }`
	if err := c.Query(ctx, q, map[string]any{"input": input}, &out); err != nil {
		return "", err
	}
	return out.ProjectUpdateCreate.ProjectUpdate.URL, nil
}

// Document is a Linear document (used for the rolling weekly roundup).
type Document struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// GetDocument fetches a document by id or slug.
func (c *Client) GetDocument(ctx context.Context, id string) (*Document, error) {
	var out struct {
		Document Document `json:"document"`
	}
	q := `query($id: String!) { document(id: $id) { id title url content } }`
	if err := c.Query(ctx, q, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return &out.Document, nil
}

// UpdateDocumentContent replaces a document's markdown content.
func (c *Client) UpdateDocumentContent(ctx context.Context, id, content string) (*Document, error) {
	var out struct {
		DocumentUpdate struct {
			Success  bool     `json:"success"`
			Document Document `json:"document"`
		} `json:"documentUpdate"`
	}
	q := `mutation($id: String!, $input: DocumentUpdateInput!) { documentUpdate(id: $id, input: $input) { success document { id title url content } } }`
	if err := c.Query(ctx, q, map[string]any{"id": id, "input": map[string]any{"content": content}}, &out); err != nil {
		return nil, err
	}
	return &out.DocumentUpdate.Document, nil
}

// CreateDocument creates a project document.
func (c *Client) CreateDocument(ctx context.Context, projectID, title, content string) (*Document, error) {
	var out struct {
		DocumentCreate struct {
			Success  bool     `json:"success"`
			Document Document `json:"document"`
		} `json:"documentCreate"`
	}
	q := `mutation($input: DocumentCreateInput!) { documentCreate(input: $input) { success document { id title url content } } }`
	if err := c.Query(ctx, q, map[string]any{"input": map[string]any{"projectId": projectID, "title": title, "content": content}}, &out); err != nil {
		return nil, err
	}
	return &out.DocumentCreate.Document, nil
}

// ProjectBySlugID finds a project by the short id at the end of its URL slug.
func (c *Client) ProjectBySlugID(ctx context.Context, slugID string) (*Project, error) {
	var out struct {
		Projects struct {
			Nodes []rawProject `json:"nodes"`
		} `json:"projects"`
	}
	q := `query($slug: String!) { projects(first: 1, filter: { slugId: { eq: $slug } }) {
		nodes { id name url status { name type } lead { id } teams { nodes { id key name } } } } }`
	if err := c.Query(ctx, q, map[string]any{"slug": slugID}, &out); err != nil {
		return nil, err
	}
	if len(out.Projects.Nodes) == 0 {
		return nil, fmt.Errorf("linear: no project with slug id %q", slugID)
	}
	p := out.Projects.Nodes[0].toProject()
	return &p, nil
}

func (c *Client) IssueByID(ctx context.Context, id string) (*Issue, error) {
	var out struct {
		Issue *rawIssue `json:"issue"`
	}
	if err := c.Query(ctx, `query($id:String!){issue(id:$id){`+issueFields+`}}`, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	if out.Issue == nil {
		return nil, fmt.Errorf("issue not found")
	}
	is := out.Issue.toIssue()
	return &is, nil
}
