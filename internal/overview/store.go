// Package overview builds the operator's global work and attention tray.
package overview

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/runstatus"
	"github.com/gabinante/flywheel/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID          string              `json:"id"`
	Kind        string              `json:"kind"`
	Title       string              `json:"title"`
	Ref         string              `json:"ref,omitempty"`
	Href        string              `json:"href"`
	ProjectName string              `json:"project_name,omitempty"`
	ProjectID   string              `json:"project_id,omitempty"`
	TicketID    string              `json:"ticket_id,omitempty"`
	ReviewID    string              `json:"review_id,omitempty"`
	Progress    *runstatus.Progress `json:"progress,omitempty"`
	Harness     string              `json:"harness,omitempty"`
	Worker      string              `json:"worker,omitempty"`
	Status      string              `json:"status"`
	Reason      string              `json:"reason,omitempty"`
	Action      string              `json:"action,omitempty"`
	SessionHref string              `json:"session_href,omitempty"`
	StartedAt   time.Time           `json:"started_at"`
}
type Snapshot struct {
	InFlight  []Item    `json:"in_flight"`
	Attention []Item    `json:"attention"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type project struct{ id, name, path string }
type candidate struct {
	item                                              Item
	state, workflowID, phaseStatus, phaseID, question string
	enteredAt                                         *time.Time
	outputs                                           map[string]any
	phase                                             *workflow.Phase
}

// AttentionReason mirrors the decisions accepted by the workflow engine. Automated
// gates and already-recorded approvals are not presented as human work.
func attentionReason(c candidate, working bool) (string, string) {
	if c.state == "closed" {
		return "", ""
	}
	if c.state == "awaiting_input" {
		if c.question != "" {
			return c.question, "Answer question"
		}
		return "A worker needs your input.", "Provide input"
	}
	if c.phaseStatus == "failed" {
		return "The workflow phase failed.", "Retry phase"
	}
	if working {
		return "", ""
	}
	if c.workflowID == "" {
		if c.state == "awaiting_validation" {
			return "Implementation is ready for review.", "Review ticket"
		}
		return "", ""
	}
	p := c.phase
	if p == nil || c.enteredAt == nil {
		return "", ""
	}
	if p.Type == workflow.PhaseManual {
		return p.Name + " needs your approval.", "Approve phase"
	}
	if p.Type == workflow.PhaseGate {
		cfg, err := workflow.ParseGateConfig(p.Config)
		if err != nil || cfg == nil {
			return "", ""
		}
		for _, condition := range cfg.EffectiveConditions() {
			if condition.Type == "human_approval" {
				value, _ := c.outputs["_human_approval_"+c.phaseID].(string)
				approved, err := time.Parse(time.RFC3339Nano, value)
				if err == nil && approved.Equal(*c.enteredAt) {
					return "", ""
				}
				return p.Name + " needs your approval.", "Approve phase"
			}
		}
	}
	if p.Type == workflow.PhaseAgent && c.phaseStatus == "blocked" {
		cfg, err := workflow.ParseAgentConfig(p.Config)
		if err == nil && cfg != nil && cfg.AutoAdvance != nil && !*cfg.AutoAdvance {
			return p.Name + " finished and is waiting to continue.", "Continue workflow"
		}
	}
	return "", ""
}

func (s *Store) Get(ctx context.Context, runs []runstatus.Run, activeTicketIDs, activeReviewIDs []string) (Snapshot, error) {
	out := Snapshot{InFlight: []Item{}, Attention: []Item{}, UpdatedAt: time.Now().UTC()}
	projects := map[string]project{}
	repos := map[string]project{}
	rows, err := s.pool.Query(ctx, `SELECT p.id,p.name,o.slug,p.slug,COALESCE(r.repo_url,p.repo_url,'') FROM projects p JOIN orgs o ON o.id=p.org_id LEFT JOIN LATERAL (SELECT repo_url FROM project_repositories WHERE project_id=p.id UNION SELECT p.repo_url) r ON true ORDER BY p.name,p.id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var p project
		var orgSlug, projectSlug, repo string
		if err = rows.Scan(&p.id, &p.name, &orgSlug, &projectSlug, &repo); err != nil {
			rows.Close()
			return out, err
		}
		p.path = "/orgs/" + url.PathEscape(orgSlug) + "/projects/" + url.PathEscape(projectSlug)
		projects[p.id] = p
		if repo != "" {
			key := repoKey(repo)
			if _, ok := repos[key]; !ok {
				repos[key] = p
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	active := map[string]bool{}
	runTickets := map[string]bool{}
	runReviews := map[string]bool{}
	for _, id := range activeTicketIDs {
		active[id] = true
	}
	for _, r := range runs {
		if r.TicketID != "" {
			active[r.TicketID] = true
			runTickets[r.TicketID] = true
		}
		if r.ReviewID != "" {
			runReviews[r.ReviewID] = true
		}
	}
	ids := make([]string, 0, len(active))
	for id := range active {
		ids = append(ids, id)
	}
	rows, err = s.pool.Query(ctx, `SELECT t.id,t.project_id,t.title,COALESCE(e.identifier,''),t.state,COALESCE(t.workflow_id,''),COALESCE(t.workflow_phase,''),t.workflow_phase_status,t.workflow_phase_entered_at,t.outputs,t.updated_at,
 COALESCE((SELECT COALESCE(NULLIF(question,''),reason) FROM escalations WHERE ticket_id=t.id AND resolved_at IS NULL ORDER BY created_at DESC LIMIT 1),''),
 (SELECT value FROM jsonb_array_elements(CASE WHEN COALESCE(t.workflow_version,0)>0 AND t.workflow_version<>w.version THEN COALESCE(v.phases,'[]'::jsonb) ELSE COALESCE(w.phases,'[]'::jsonb) END) WHERE value->>'id'=t.workflow_phase LIMIT 1)
 FROM tickets t LEFT JOIN ticket_external_refs e ON e.ticket_id=t.id LEFT JOIN workflow_definitions w ON w.id=t.workflow_id LEFT JOIN workflow_definition_versions v ON v.workflow_id=t.workflow_id AND v.version=t.workflow_version
 WHERE t.id=ANY($1) OR (t.state<>'closed' AND (t.state IN ('awaiting_input','awaiting_validation') OR t.workflow_phase_status IN ('ready','blocked','failed','running'))) ORDER BY t.priority,t.updated_at,t.id`, ids)
	if err != nil {
		return out, err
	}
	tickets := map[string]candidate{}
	for rows.Next() {
		var c candidate
		var phaseJSON, outputsJSON []byte
		if err = rows.Scan(&c.item.TicketID, &c.item.ProjectID, &c.item.Title, &c.item.Ref, &c.state, &c.workflowID, &c.phaseID, &c.phaseStatus, &c.enteredAt, &outputsJSON, &c.item.StartedAt, &c.question, &phaseJSON); err != nil {
			rows.Close()
			return out, err
		}
		if len(phaseJSON) > 0 {
			if err = json.Unmarshal(phaseJSON, &c.phase); err != nil {
				rows.Close()
				return out, err
			}
		}
		if len(outputsJSON) > 0 {
			var value any
			if err = json.Unmarshal(outputsJSON, &value); err != nil {
				rows.Close()
				return out, err
			}
			// Older tickets may contain array-valued outputs, with no approval markers.
			c.outputs, _ = value.(map[string]any)
		}
		p := projects[c.item.ProjectID]
		c.item.ProjectName = p.name
		c.item.Href = p.path + "/tickets/" + url.PathEscape(c.item.TicketID)
		c.item.ID = "ticket:" + c.item.TicketID
		c.item.Kind = "ticket"
		if c.item.Ref == "" {
			c.item.Ref = c.item.TicketID
		}
		tickets[c.item.TicketID] = c
		if reason, action := attentionReason(c, active[c.item.TicketID]); reason != "" {
			item := c.item
			item.Reason = reason
			item.Action = action
			item.Status = c.phaseStatus
			out.Attention = append(out.Attention, item)
		}
		if active[c.item.TicketID] && !runTickets[c.item.TicketID] {
			item := c.item
			item.Status = "Preparing worker"
			out.InFlight = append(out.InFlight, item)
		} else if !active[c.item.TicketID] && c.phaseStatus == "running" && c.phase != nil && c.phase.Type == workflow.PhaseExternal {
			item := c.item
			item.Kind = "external"
			item.Status = c.phase.Name
			out.InFlight = append(out.InFlight, item)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, r := range runs {
		p := projects[r.ProjectID]
		if p.id == "" && r.Ref != "" {
			p = repos[repoKey(strings.Split(r.Ref, "#")[0])]
		}
		item := Item{ID: r.ID, Kind: r.Kind, Title: r.Title, Ref: r.Ref, ProjectID: p.id, ProjectName: p.name, Harness: r.Harness, Worker: r.Worker, Status: r.State, StartedAt: r.StartedAt, Href: p.path + "/command"}
		progress := r.Progress.At(out.UpdatedAt)
		item.Progress, item.ReviewID = &progress, r.ReviewID
		if r.TicketID != "" {
			if c, ok := tickets[r.TicketID]; ok {
				item.Title = c.item.Title
				item.Ref = c.item.Ref
				item.TicketID = r.TicketID
				item.Href = c.item.Href
				item.ProjectID = c.item.ProjectID
				item.ProjectName = c.item.ProjectName
			}
		}
		if r.ReviewID != "" {
			item.Href = "/code-reviews/" + url.PathEscape(r.ReviewID)
		} else if r.Ref != "" && r.Kind == "feedback" {
			item.Href = "/code-reviews"
		}
		if item.Title == "" {
			item.Title = "Harness run"
		}
		if r.SessionID != "" {
			item.SessionHref = "/sessions/" + url.PathEscape(r.SessionID)
			if item.Href == "/command" {
				item.Href = item.SessionHref
			}
		}
		if item.Href == "/command" {
			item.Href = "/sessions"
		}
		out.InFlight = append(out.InFlight, item)
	}
	// Review preparation/publication also counts as work even between harness turns.
	rows, err = s.pool.Query(ctx, `SELECT id,title,repo,number,state,error,session_id,updated_at FROM code_review_requests WHERE state IN ('failed','fetching','reviewing','publishing') ORDER BY updated_at,id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, title, repo, state, reason, sess string
		var number int
		var at time.Time
		if err = rows.Scan(&id, &title, &repo, &number, &state, &reason, &sess, &at); err != nil {
			rows.Close()
			return out, err
		}
		p := repos[repoKey(repo)]
		item := Item{ID: "review:" + id, ReviewID: id, Kind: "code_review", Title: title, Ref: fmt.Sprintf("%s#%d", repo, number), Href: "/code-reviews/" + url.PathEscape(id), ProjectID: p.id, ProjectName: p.name, Status: state, StartedAt: at}
		if state == "failed" {
			item.Action = "Retry review"
			item.Reason = reason
			if reason == "" {
				item.Reason = "The code review failed."
			}
			if sess != "" {
				item.SessionHref = "/sessions/" + url.PathEscape(sess)
			}
			out.Attention = append(out.Attention, item)
		} else if !runReviews[id] {
			owned := false
			for _, activeID := range activeReviewIDs {
				if id == activeID {
					owned = true
					break
				}
			}
			if owned || out.UpdatedAt.Sub(at) < 15*time.Second {
				item.Progress = &runstatus.Progress{WorkerState: "preparing", Health: "preparing"}
				out.InFlight = append(out.InFlight, item)
			} else {
				item.Progress = &runstatus.Progress{WorkerState: "untracked", Health: "untracked"}
				item.Reason = "Saved as " + state + ", but this server has no worker for this review."
				item.Action = "Inspect review"
				out.Attention = append(out.Attention, item)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = s.pool.Query(ctx, `SELECT id,title,repo,number,reviewer,observed_at FROM pr_feedback_rounds WHERE state='new' ORDER BY observed_at,id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, title, repo, reviewer string
		var number int
		var at time.Time
		if err = rows.Scan(&id, &title, &repo, &number, &reviewer, &at); err != nil {
			rows.Close()
			return out, err
		}
		p := repos[repoKey(repo)]
		ref := fmt.Sprintf("%s#%d", repo, number)
		if title == "" {
			title = ref
		}
		reason := "New PR feedback needs a decision."
		if reviewer != "" {
			reason = "New feedback from " + reviewer + "."
		}
		out.Attention = append(out.Attention, Item{ID: "feedback:" + id, Kind: "feedback", Title: title, Ref: ref, Href: "/code-reviews", ProjectID: p.id, ProjectName: p.name, Status: "new", Reason: reason, Action: "Address feedback", StartedAt: at})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	sort.SliceStable(out.InFlight, func(i, j int) bool { return out.InFlight[i].StartedAt.Before(out.InFlight[j].StartedAt) })
	return out, nil
}
func repoKey(repo string) string {
	repo = strings.TrimSpace(repo)
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "git@github.com:", "ssh://git@github.com/"} {
		repo = strings.TrimPrefix(repo, prefix)
	}
	return strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git"))
}
