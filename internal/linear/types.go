package linear

import "time"

// Viewer is the authenticated Linear user.
type Viewer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Team is a Linear team.
type Team struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Project is a Linear project.
type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"status"`
	Lead *struct {
		ID string `json:"id"`
	} `json:"lead"`
	Teams []Team `json:"-"`
}

// WorkflowState is one column of a team's workflow.
type WorkflowState struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"` // triage, backlog, unstarted, started, completed, canceled
	Position float64 `json:"position"`
}

// Issue is a Linear issue with the fields Flywheel projects.
type Issue struct {
	ID          string        `json:"id"`
	Identifier  string        `json:"identifier"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	URL         string        `json:"url"`
	Priority    int           `json:"priority"` // 0 none, 1 urgent, 2 high, 3 normal, 4 low
	BranchName  string        `json:"branchName"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	CompletedAt *time.Time    `json:"completedAt"`
	CanceledAt  *time.Time    `json:"canceledAt"`
	State       WorkflowState `json:"state"`
	Team        Team          `json:"team"`
	Assignee    *struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"assignee"`
	Project *struct {
		ID string `json:"id"`
	} `json:"project"`
	Labels      []string `json:"-"`
	Attachments []struct {
		URL        string `json:"url"`
		Title      string `json:"title"`
		SourceType string `json:"sourceType"`
	} `json:"-"`
}

// Comment is a Linear issue comment.
type Comment struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}
