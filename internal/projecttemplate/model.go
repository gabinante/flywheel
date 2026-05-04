package projecttemplate

import "time"

type WorkstreamTemplate struct {
	ID          string           `json:"id"`
	OrgID       string           `json:"org_id,omitempty"`
	Name        string           `json:"name"`
	Slug        string           `json:"slug"`
	Description string           `json:"description"`
	Plan        string           `json:"plan"`
	Tickets     []TicketTemplate `json:"tickets"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type TicketTemplate struct {
	Title           string   `json:"title"`
	Type            string   `json:"type"`
	Priority        int      `json:"priority"`
	Description     string   `json:"description"`
	SuccessCriteria []string `json:"success_criteria,omitempty"`
}

type ProjectTemplate struct {
	ID                    string    `json:"id"`
	OrgID                 string    `json:"org_id,omitempty"`
	Name                  string    `json:"name"`
	Slug                  string    `json:"slug"`
	Description           string    `json:"description"`
	WorkstreamTemplateIDs []string  `json:"workstream_template_ids"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ProjectTemplateExpanded struct {
	ProjectTemplate
	WorkstreamTemplates []WorkstreamTemplate `json:"workstream_templates"`
}

type SeedResult struct {
	WorkstreamsCreated int      `json:"workstreams_created"`
	TicketsCreated     int      `json:"tickets_created"`
	WorkstreamIDs      []string `json:"workstream_ids"`
	TicketIDs          []string `json:"ticket_ids"`
}
