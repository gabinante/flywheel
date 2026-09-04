package rest

import (
	"context"
	"strings"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/internal/codereview"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/gabinante/flywheel/internal/ticket"
)

// ListProjectPullRequests returns PRs in the project's repositories that matter to the
// project: the operator's own open PRs and any PR linked to one of its tickets (by
// recorded PR URL, by Linear identifier in the branch or title, or by ticket id in the
// branch). Linked PRs carry the ticket id so the UI can nest them under the ticket.
func (s *StrictServer) ListProjectPullRequests(ctx context.Context, req generated.ListProjectPullRequestsRequestObject) (generated.ListProjectPullRequestsResponseObject, error) {
	req.ProjectID = s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	proj, err := s.ProjectSvc.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	seen := map[string]bool{}
	var repos []string
	add := func(u string) {
		if r := sessions.RepoFromOriginURL(u); r != "" && strings.Contains(r, "/") && !seen[r] {
			seen[r] = true
			repos = append(repos, r)
		}
	}
	add(proj.RepoURL)
	if s.RepoSvc != nil {
		if list, err := s.RepoSvc.ListRepositories(ctx, proj.ID); err == nil {
			for _, r := range list {
				add(r.RepoURL)
			}
		}
	}
	force := req.Params.Refresh != nil && *req.Params.Refresh
	login, cards, err := s.CodeReviewSvc.ProjectPRs(ctx, repos, force)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInternal, "github: "+err.Error(), true)
	}

	tickets, _ := s.TicketSvc.ListTickets(ctx, proj.ID, "", "")
	byURL := map[string]*ticket.Ticket{}
	byIdent := map[string]*ticket.Ticket{}
	byIDPrefix := map[string]*ticket.Ticket{}
	for _, t := range tickets {
		if u, ok := t.Outputs["pr_url"].(string); ok && u != "" {
			byURL[strings.TrimSuffix(strings.TrimSpace(u), "/")] = t
		}
		if t.External != nil && t.External.Identifier != "" {
			byIdent[strings.ToUpper(t.External.Identifier)] = t
		}
		if len(t.ID) >= 8 {
			byIDPrefix[strings.ToLower(t.ID[:8])] = t
		}
	}
	match := func(c codereview.PRCard) *ticket.Ticket {
		if t := byURL[strings.TrimSuffix(c.URL, "/")]; t != nil {
			return t
		}
		for _, ref := range c.LinearRefs {
			if t := byIdent[ref]; t != nil {
				return t
			}
		}
		branch := strings.ToLower(c.HeadRef)
		for prefix, t := range byIDPrefix {
			if strings.Contains(branch, prefix) {
				return t
			}
		}
		return nil
	}

	out := generated.ProjectPullRequests{Login: login, Items: []generated.PullRequestCard{}}
	for _, c := range cards {
		t := match(c)
		mine := login != "" && strings.EqualFold(c.Author, login)
		if t == nil && !(mine && c.State == "OPEN") {
			continue
		}
		if t != nil {
			c.TicketID, c.TicketTitle = t.ID, t.Title
			if t.External != nil {
				c.TicketIdentifier = t.External.Identifier
			}
		}
		g := prCardToGen(c)
		if c.TicketID != "" {
			id, title := c.TicketID, c.TicketTitle
			g.TicketId, g.TicketTitle = &id, &title
			if c.TicketIdentifier != "" {
				ident := c.TicketIdentifier
				g.TicketIdentifier = &ident
			}
		}
		out.Items = append(out.Items, g)
	}
	out.FetchedAt = time.Now()
	return generated.ListProjectPullRequests200JSONResponse(out), nil
}
