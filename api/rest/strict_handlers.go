// Package rest: strict server implementation for the generated OpenAPI server interface.
// Implementations delegate to existing services and use CheckProjectAccess/CheckOrgAccess for auth.

package rest

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/entity"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/gitnotes"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workstream"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// StrictServer implements generated.StrictServerInterface by delegating to existing services.
type StrictServer struct {
	OrgSvc        *org.Service
	ProjectSvc    *project.Service
	WorkStreamSvc *workstream.Service
	TicketSvc     *ticket.Service
	QueueSvc      *queue.Service
	TraceSvc      *execution.Service
	ReviewSvc     *review.Service
	EntitySvc     *entity.Service
	AgentStore    agent.AgentStore
	CostSvc       *cost.Service
}

func (s *StrictServer) GetHealthz(ctx context.Context, req generated.GetHealthzRequestObject) (generated.GetHealthzResponseObject, error) {
	return generated.GetHealthz200Response{}, nil
}

func (s *StrictServer) GetMeStats(ctx context.Context, req generated.GetMeStatsRequestObject) (generated.GetMeStatsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetMeStats401JSONResponse(seToGen(err)), nil
	}
	agentID := GetAgentID(ctx)
	created, err := s.TicketSvc.CountTicketsCreatedBy(ctx, agentID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	approved, rejected, err := s.ReviewSvc.GetReviewCountsByReviewer(ctx, agentID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.GetMeStats200JSONResponse(generated.MeStats{
		TicketsCreated:   created,
		ReviewsApproved:  approved,
		ReviewsRejected:  rejected,
	}), nil
}

func (s *StrictServer) GetMeStatsHistory(ctx context.Context, req generated.GetMeStatsHistoryRequestObject) (generated.GetMeStatsHistoryResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetMeStatsHistory401JSONResponse(seToGen(err)), nil
	}
	days := 14
	if req.Params.Days != nil && *req.Params.Days > 0 {
		days = *req.Params.Days
		if days > 90 {
			days = 90
		}
	}
	agentID := GetAgentID(ctx)
	tickets, err := s.TicketSvc.CountTicketsCreatedByPerDay(ctx, agentID, days)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	approved, rejected, err := s.ReviewSvc.GetReviewCountsByReviewerPerDay(ctx, agentID, days)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	now := time.Now().UTC()
	daysSlice := make([]openapi_types.Date, days)
	for i := 0; i < days; i++ {
		t := now.AddDate(0, 0, -days+1+i)
		daysSlice[i] = openapi_types.Date{Time: t}
	}
	return generated.GetMeStatsHistory200JSONResponse(generated.MeStatsHistory{
		Days:            daysSlice,
		TicketsCreated:  tickets,
		ReviewsApproved: approved,
		ReviewsRejected: rejected,
	}), nil
}

func (s *StrictServer) ListOrgs(ctx context.Context, req generated.ListOrgsRequestObject) (generated.ListOrgsResponseObject, error) {
	if err := requireOAuthAgent(ctx, s.AgentStore); err != nil {
		return generated.ListOrgs401JSONResponse(seToGen(err)), nil
	}
	agentID := GetAgentID(ctx)
	a, _ := s.AgentStore.GetByID(ctx, agentID)
	list, err := s.OrgSvc.ListOrgsForUser(ctx, a.UserID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := make([]generated.Org, len(list))
	for i := range list {
		out[i] = orgToGen(list[i])
	}
	return generated.ListOrgs200JSONResponse(out), nil
}

func (s *StrictServer) CreateOrg(ctx context.Context, req generated.CreateOrgRequestObject) (generated.CreateOrgResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.CreateOrg401JSONResponse(seToGen(err)), nil
	}
	agentID := GetAgentID(ctx)
	a, _ := s.AgentStore.GetByID(ctx, agentID)
	name, slug := "", ""
	if req.Body != nil {
		if req.Body.Name != nil {
			name = *req.Body.Name
		}
		if req.Body.Slug != nil {
			slug = *req.Body.Slug
		}
	}
	var o *org.Org
	var err error
	if a.UserID != "" {
		o, err = s.OrgSvc.CreateOrgWithOwner(ctx, name, slug, a.UserID)
	} else {
		o, err = s.OrgSvc.CreateOrg(ctx, name, slug)
	}
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.CreateOrg201JSONResponse(orgToGen(o)), nil
}

func (s *StrictServer) GetOrg(ctx context.Context, req generated.GetOrgRequestObject) (generated.GetOrgResponseObject, error) {
	if err := CheckOrgAccess(ctx, req.OrgID, s.AgentStore, s.OrgSvc); err != nil {
		return generated.GetOrg404JSONResponse(seToGen(err)), nil
	}
	o, err := s.OrgSvc.GetOrg(ctx, req.OrgID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.GetOrg200JSONResponse(orgToGen(o)), nil
}

func (s *StrictServer) ListProjectsByOrg(ctx context.Context, req generated.ListProjectsByOrgRequestObject) (generated.ListProjectsByOrgResponseObject, error) {
	if err := CheckOrgAccess(ctx, req.OrgID, s.AgentStore, s.OrgSvc); err != nil {
		log.Printf("ListProjectsByOrg: org=%s CheckOrgAccess denied: %s", req.OrgID, err.Message)
		return nil, err
	}
	status := "active"
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	list, err := s.ProjectSvc.ListByOrgID(ctx, req.OrgID, status)
	if err != nil {
		log.Printf("ListProjectsByOrg: org=%s status=%s ListByOrgID error: %v", req.OrgID, status, err)
		return nil, apierrors.MapError(err)
	}
	log.Printf("ListProjectsByOrg: org=%s status=%s count=%d", req.OrgID, status, len(list))
	out := make([]generated.Project, len(list))
	for i := range list {
		out[i] = projectToGen(&list[i])
	}
	return generated.ListProjectsByOrg200JSONResponse(out), nil
}

func (s *StrictServer) CreateProject(ctx context.Context, req generated.CreateProjectRequestObject) (generated.CreateProjectResponseObject, error) {
	if err := CheckOrgAccess(ctx, req.OrgID, s.AgentStore, s.OrgSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}
	name, slug, repoURL := "", "", ""
	var techStack []string
	if body.Name != nil {
		name = *body.Name
	}
	if body.Slug != nil {
		slug = *body.Slug
	}
	if body.RepoUrl != nil {
		repoURL = *body.RepoUrl
	}
	if body.TechStack != nil {
		techStack = *body.TechStack
	}
	p, err := s.ProjectSvc.CreateProject(ctx, req.OrgID, name, slug, repoURL, techStack)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.CreateProject201JSONResponse(projectToGen(p)), nil
}

func (s *StrictServer) GetCommitNotes(ctx context.Context, req generated.GetCommitNotesRequestObject) (generated.GetCommitNotesResponseObject, error) {
	repoPath := req.Params.RepoPath
	proj, err := s.ProjectSvc.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if proj == nil || proj.OrgID != req.OrgID {
		return generated.GetCommitNotes404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "project not found", false))), nil
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if !repoPathAccessible(repoPath) {
		return generated.GetCommitNotes501JSONResponse(generated.NotImplementedError{
			Code:  ptr(generated.NotImplementedErrorCodeNotImplemented),
			Error: ptr("repo_path query param required and must point to an accessible git repo (server does not have the repo)"),
		}), nil
	}
	noteType := ""
	if req.Params.Type != nil {
		noteType = string(*req.Params.Type)
	}
	out := generated.CommitNotesResponse{CommitSha: &req.CommitSha, Notes: &map[string]string{}}
	if noteType != "" {
		ref := gitnotes.RefForType(noteType)
		if ref == "" {
			return nil, apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false)
		}
		body, err := gitnotes.ShowNote(repoPath, ref, req.CommitSha)
		if err != nil {
			return nil, apierrors.MapError(err)
		}
		(*out.Notes)[filepath.Base(ref)] = body
	} else {
		for _, ref := range gitnotes.AllRefs() {
			body, _ := gitnotes.ShowNote(repoPath, ref, req.CommitSha)
			if body != "" {
				(*out.Notes)[filepath.Base(ref)] = body
			}
		}
	}
	return generated.GetCommitNotes200JSONResponse(out), nil
}

func (s *StrictServer) GetGitNotesLog(ctx context.Context, req generated.GetGitNotesLogRequestObject) (generated.GetGitNotesLogResponseObject, error) {
	repoPath := req.Params.RepoPath
	proj, err := s.ProjectSvc.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if proj == nil || proj.OrgID != req.OrgID {
		return generated.GetGitNotesLog404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "project not found", false))), nil
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if !repoPathAccessible(repoPath) {
		return generated.GetGitNotesLog501JSONResponse(generated.NotImplementedError{
			Code:  ptr(generated.NotImplementedErrorCodeNotImplemented),
			Error: ptr("repo_path query param required and must point to an accessible git repo (server does not have the repo)"),
		}), nil
	}
	noteType := gitnotes.TypeDecision
	if req.Params.Type != nil {
		noteType = string(*req.Params.Type)
	}
	limit := 20
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
		if limit <= 0 {
			limit = 20
		}
	}
	ref := gitnotes.RefForType(noteType)
	if ref == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false)
	}
	entries, err := gitnotes.Log(repoPath, ref, limit)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := make([]generated.GitNotesLogEntry, len(entries))
	for i := range entries {
		out[i] = generated.GitNotesLogEntry{
			CommitSha: &entries[i].CommitSHA,
			Ref:       &entries[i].Ref,
			Body:      &entries[i].Body,
		}
	}
	return generated.GetGitNotesLog200JSONResponse(generated.GitNotesLogResponse{Entries: &out}), nil
}

func (s *StrictServer) GetProject(ctx context.Context, req generated.GetProjectRequestObject) (generated.GetProjectResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	p, err := s.ProjectSvc.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.GetProject200JSONResponse(projectToGen(p)), nil
}

func (s *StrictServer) UpdateProject(ctx context.Context, req generated.UpdateProjectRequestObject) (generated.UpdateProjectResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if req.Body == nil || (req.Body.Status == nil && req.Body.RepoUrl == nil && req.Body.Name == nil && req.Body.Slug == nil && req.Body.DefaultBranch == nil && req.Body.DispatchEnabled == nil) {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "at least one of status, repo_url, name, slug, default_branch, dispatch_enabled required", false)
	}
	if req.Body.Status != nil {
		if err := s.ProjectSvc.UpdateStatus(ctx, req.ProjectID, string(*req.Body.Status)); err != nil {
			return nil, apierrors.MapError(err)
		}
	}
	if req.Body.RepoUrl != nil {
		if err := s.ProjectSvc.UpdateRepoURL(ctx, req.ProjectID, *req.Body.RepoUrl); err != nil {
			return nil, apierrors.MapError(err)
		}
	}
	if req.Body.Name != nil && strings.TrimSpace(*req.Body.Name) != "" {
		if err := s.ProjectSvc.UpdateName(ctx, req.ProjectID, strings.TrimSpace(*req.Body.Name)); err != nil {
			return nil, apierrors.MapError(err)
		}
	}
	if req.Body.Slug != nil {
		slug := strings.TrimSpace(*req.Body.Slug)
		if err := s.ProjectSvc.UpdateSlug(ctx, req.ProjectID, slug); err != nil {
			return nil, apierrors.MapError(err)
		}
	}
	if req.Body.DefaultBranch != nil {
		if err := s.ProjectSvc.UpdateDefaultBranch(ctx, req.ProjectID, strings.TrimSpace(*req.Body.DefaultBranch)); err != nil {
			return nil, apierrors.MapError(err)
		}
	}
	if req.Body.DispatchEnabled != nil {
		if err := s.ProjectSvc.UpdateDispatchEnabled(ctx, req.ProjectID, *req.Body.DispatchEnabled); err != nil {
			return nil, apierrors.MapError(err)
		}
	}
	p, err := s.ProjectSvc.GetProject(ctx, req.ProjectID)
	if err != nil {
		log.Printf("UpdateProject: project=%s GetProject after update: %v", req.ProjectID, err)
		return nil, apierrors.MapError(err)
	}
	return generated.UpdateProject200JSONResponse(projectToGen(p)), nil
}

func (s *StrictServer) ListWorkStreams(ctx context.Context, req generated.ListWorkStreamsRequestObject) (generated.ListWorkStreamsResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	statusFilter := "active"
	if req.Params.Status != nil {
		statusFilter = string(*req.Params.Status)
	}
	list, err := s.WorkStreamSvc.ListWorkStreams(ctx, req.ProjectID, statusFilter)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := make([]generated.WorkStream, len(list))
	for i := range list {
		out[i] = workStreamToGen(&list[i])
	}
	return generated.ListWorkStreams200JSONResponse(out), nil
}

func (s *StrictServer) CreateWorkStream(ctx context.Context, req generated.CreateWorkStreamRequestObject) (generated.CreateWorkStreamResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil || body.Name == "" {
		return generated.CreateWorkStream400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "name required", false))), nil
	}
	name := body.Name
	slug, plan := "", ""
	if body.Slug != nil {
		slug = *body.Slug
	}
	if body.Plan != nil {
		plan = *body.Plan
	}
	w, err := s.WorkStreamSvc.CreateWorkStream(ctx, req.ProjectID, name, slug, plan)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.CreateWorkStream201JSONResponse(workStreamToGen(w)), nil
}

func (s *StrictServer) GetWorkStream(ctx context.Context, req generated.GetWorkStreamRequestObject) (generated.GetWorkStreamResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	w, err := s.WorkStreamSvc.GetWorkStream(ctx, req.WorkStreamID)
	if err != nil {
		if errors.Is(err, workstream.ErrWorkStreamNotFound) {
			return generated.GetWorkStream404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))), nil
		}
		return nil, apierrors.MapError(err)
	}
	if w.ProjectID != req.ProjectID {
		return generated.GetWorkStream404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))), nil
	}
	return generated.GetWorkStream200JSONResponse(workStreamToGen(w)), nil
}

func (s *StrictServer) UpdateWorkStream(ctx context.Context, req generated.UpdateWorkStreamRequestObject) (generated.UpdateWorkStreamResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	w, err := s.WorkStreamSvc.GetWorkStream(ctx, req.WorkStreamID)
	if err != nil {
		if errors.Is(err, workstream.ErrWorkStreamNotFound) {
			return generated.UpdateWorkStream404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))), nil
		}
		return nil, apierrors.MapError(err)
	}
	if w.ProjectID != req.ProjectID {
		return generated.UpdateWorkStream404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))), nil
	}
	body := req.Body
	if body == nil {
		return generated.UpdateWorkStream400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "body required", false))), nil
	}
	name, plan, branch, status := w.Name, w.Plan, w.Branch, w.Status
	if body.Name != nil {
		name = *body.Name
	}
	if body.Plan != nil {
		plan = *body.Plan
	}
	if body.Branch != nil {
		branch = *body.Branch
	}
	if body.Status != nil {
		status = string(*body.Status)
	}
	if err := s.WorkStreamSvc.UpdateWorkStream(ctx, req.WorkStreamID, name, plan, branch, status); err != nil {
		if errors.Is(err, workstream.ErrInvalidStatus) {
			return generated.UpdateWorkStream400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))), nil
		}
		return nil, apierrors.MapError(err)
	}
	updated, _ := s.WorkStreamSvc.GetWorkStream(ctx, req.WorkStreamID)
	return generated.UpdateWorkStream200JSONResponse(workStreamToGen(updated)), nil
}

func (s *StrictServer) ListTickets(ctx context.Context, req generated.ListTicketsRequestObject) (generated.ListTicketsResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	workStreamID := ""
	if req.Params.WorkStreamId != nil {
		workStreamID = *req.Params.WorkStreamId
	}
	state := ticket.State("")
	if req.Params.State != nil {
		state = ticket.MapLegacyState(ticket.State(*req.Params.State))
	}
	list, err := s.TicketSvc.ListTickets(ctx, req.ProjectID, workStreamID, state)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := make([]generated.Ticket, len(list))
	for i := range list {
		out[i] = ticketToGen(list[i])
	}
	return generated.ListTickets200JSONResponse(out), nil
}

func (s *StrictServer) CreateTicket(ctx context.Context, req generated.CreateTicketRequestObject) (generated.CreateTicketResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil || body.Title == "" || body.CreatedBy == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "title and created_by required", false)
	}
	typ := ticket.TypeTask
	if body.Type != nil {
		typ = ticket.TicketType(string(*body.Type))
	}
	prio := ticket.P2
	if body.Priority != nil && *body.Priority >= 0 && *body.Priority <= 3 {
		prio = ticket.Priority(*body.Priority)
	}
	dependsOn := []string{}
	if body.DependsOn != nil {
		dependsOn = *body.DependsOn
	}
	var obj ticket.Objective
	if body.Objective != nil {
		obj = objectiveFromGen(body.Objective)
	}
	var ctxPack ticket.TicketContext
	if body.TicketContext != nil {
		ctxPack = ticketContextFromGen(body.TicketContext)
	}
	idemKey := ""
	if body.IdempotencyKey != nil {
		idemKey = *body.IdempotencyKey
	}
	workStreamID := ""
	if body.WorkStreamId != nil && *body.WorkStreamId != "" {
		workStreamID = *body.WorkStreamId
		// Validate work stream exists and belongs to project
		ws, err := s.WorkStreamSvc.GetWorkStream(ctx, workStreamID)
		if err != nil || ws == nil || ws.ProjectID != req.ProjectID {
			return generated.CreateTicket404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "work stream not found or does not belong to project", false))), nil
		}
	}
	t, err := s.TicketSvc.CreateTicket(ctx, req.ProjectID, body.Title, typ, prio, body.CreatedBy, dependsOn, workStreamID, obj, ctxPack, idemKey)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.CreateTicket201JSONResponse(ticketToGen(t)), nil
}

func (s *StrictServer) GetTicket(ctx context.Context, req generated.GetTicketRequestObject) (generated.GetTicketResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	return generated.GetTicket200JSONResponse(ticketToGen(t)), nil
}

func (s *StrictServer) UpdateTicket(ctx context.Context, req generated.UpdateTicketRequestObject) (generated.UpdateTicketResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	dependsOn := []string{}
	if req.Body != nil && req.Body.DependsOn != nil {
		dependsOn = *req.Body.DependsOn
	}
	if err := s.TicketSvc.UpdateDependsOn(ctx, req.TicketID, dependsOn); err != nil {
		return nil, apierrors.MapError(err)
	}
	if req.Body != nil {
		var desc *string
		var success *[]string
		var accept *string
		o := req.Body.Objective
		hasObj := o != nil && (o.Description != nil || o.SuccessCriteria != nil || o.AcceptanceTest != nil)
		if o != nil {
			desc = o.Description
			success = o.SuccessCriteria
			accept = o.AcceptanceTest
		}
		if req.Body.Title != nil || hasObj {
			if err := s.TicketSvc.PatchTicketMetadata(ctx, req.TicketID, req.Body.Title, desc, success, accept); err != nil {
				return nil, apierrors.MapError(err)
			}
		}
	}
	return generated.UpdateTicket204Response{}, nil
}

func (s *StrictServer) ListPendingReviews(ctx context.Context, req generated.ListPendingReviewsRequestObject) (generated.ListPendingReviewsResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	ids, err := s.ReviewSvc.ListPendingReviews(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	tickets := make([]generated.Ticket, 0, len(ids))
	for _, id := range ids {
		t, _ := s.TicketSvc.GetTicket(ctx, id)
		if t != nil {
			tickets = append(tickets, ticketToGen(t))
		}
	}
	return generated.ListPendingReviews200JSONResponse(generated.PendingReviewsResponse{Tickets: &tickets}), nil
}

func (s *StrictServer) CreateReview(ctx context.Context, req generated.CreateReviewRequestObject) (generated.CreateReviewResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}
	reviewerID := "api"
	if body.ReviewerId != nil {
		reviewerID = *body.ReviewerId
	}
	// Attribute review to authenticated agent when client sends placeholder (TUI, API, or empty).
	if reviewerID == "" || reviewerID == "api" || reviewerID == "tui" {
		if agentID := GetAgentID(ctx); agentID != "" {
			reviewerID = agentID
		}
	}
	notes := ""
	if body.Notes != nil {
		notes = *body.Notes
	}
	switch body.Decision {
	case generated.Approved:
		if err := s.ReviewSvc.ApproveTicket(ctx, req.TicketID, reviewerID, notes); err != nil {
			return nil, apierrors.MapError(err)
		}
	case generated.Rejected:
		if err := s.ReviewSvc.RejectTicket(ctx, req.TicketID, reviewerID, notes); err != nil {
			return nil, apierrors.MapError(err)
		}
	case generated.Reopened:
		if err := s.ReviewSvc.ReopenTicketForReview(ctx, req.TicketID, reviewerID, notes); err != nil {
			return nil, apierrors.MapError(err)
		}
	default:
		return nil, apierrors.New(apierrors.CodeInvalidInput, "decision must be approved, rejected, or reopened", false)
	}
	return generated.CreateReview200Response{}, nil
}

func (s *StrictServer) GetTrace(ctx context.Context, req generated.GetTraceRequestObject) (generated.GetTraceResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	trace, err := s.TraceSvc.GetTrace(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.GetTrace200JSONResponse(execTraceToGen(trace)), nil
}

func (s *StrictServer) LogStep(ctx context.Context, req generated.LogStepRequestObject) (generated.LogStepResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil || body.LeaseToken == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "lease_token required", false)
	}
	if body.Step.Type == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "step.type required", false)
	}
	payload := make(map[string]any)
	if body.Step.Payload != nil {
		payload = *body.Step.Payload
	}
	step := execution.Step{Type: execution.StepType(body.Step.Type), Payload: payload}
	if err := s.TraceSvc.LogStep(ctx, req.TicketID, body.LeaseToken, step); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.LogStep204Response{}, nil
}

func (s *StrictServer) TransitionTicket(ctx context.Context, req generated.TransitionTicketRequestObject) (generated.TransitionTicketResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil || body.Trigger == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "trigger required", false)
	}
	actorID := "api"
	if body.ActorId != nil {
		actorID = *body.ActorId
	}
	actorType := ticket.ActorAgent
	if body.Actor != nil {
		switch *body.Actor {
		case "human":
			actorType = ticket.ActorHuman
		case "system":
			actorType = ticket.ActorSystem
		}
	}
	payload := make(map[string]any)
	if body.Payload != nil {
		payload = *body.Payload
	}
	actor := ticket.Actor{ID: actorID, Type: actorType}
	if err := s.TicketSvc.TransitionTicket(ctx, req.TicketID, body.Trigger, actor, payload); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.TransitionTicket200Response{}, nil
}

func (s *StrictServer) ListEscalations(ctx context.Context, req generated.ListEscalationsRequestObject) (generated.ListEscalationsResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	list, err := s.ReviewSvc.ListEscalations(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := make([]generated.Escalation, len(list))
	for i := range list {
		out[i] = escalationToGen(&list[i])
	}
	return generated.ListEscalations200JSONResponse(out), nil
}

func (s *StrictServer) ResolveEscalation(ctx context.Context, req generated.ResolveEscalationRequestObject) (generated.ResolveEscalationResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}
	reviewerID := "api"
	if body.ReviewerId != nil {
		reviewerID = *body.ReviewerId
	}
	if err := s.ReviewSvc.ResolveEscalation(ctx, req.TicketID, req.EscalationID, reviewerID, body.Answer); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.ResolveEscalation200Response{}, nil
}

func (s *StrictServer) ClaimTicket(ctx context.Context, req generated.ClaimTicketRequestObject) (generated.ClaimTicketResponseObject, error) {
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil || body.AgentId == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "agent_id required", false)
	}
	idemKey := ""
	if body.IdempotencyKey != nil {
		idemKey = *body.IdempotencyKey
	}
	prioPtr := (*int)(nil)
	if body.Priority != nil && *body.Priority >= 0 && *body.Priority <= 3 {
		p := *body.Priority
		prioPtr = &p
	}
	t, lease, err := s.QueueSvc.ClaimTicket(ctx, body.AgentId, req.ProjectID, prioPtr, idemKey)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.ClaimTicket200JSONResponse(generated.ClaimResponseBody{
		Ticket: ptr(ticketToGen(t)),
		Lease:  ptr(leaseToGen(lease)),
	}), nil
}

func (s *StrictServer) RenewLease(ctx context.Context, req generated.RenewLeaseRequestObject) (generated.RenewLeaseResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	body := req.Body
	if body == nil || body.LeaseToken == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "lease_token required", false)
	}
	exp, err := s.QueueSvc.RenewLease(ctx, req.TicketID, body.LeaseToken)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.RenewLease200JSONResponse(generated.RenewLeaseResponseBody{ExpiresAt: &exp}), nil
}

func (s *StrictServer) ReleaseLease(ctx context.Context, req generated.ReleaseLeaseRequestObject) (generated.ReleaseLeaseResponseObject, error) {
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	token := ""
	if req.Params.LeaseToken != nil {
		token = *req.Params.LeaseToken
	}
	if token == "" && req.Body != nil && req.Body.LeaseToken != nil {
		token = *req.Body.LeaseToken
	}
	if err := s.QueueSvc.ReleaseLease(ctx, req.TicketID, token); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.ReleaseLease204Response{}, nil
}

// Helpers

func requireAgent(ctx context.Context, store agent.AgentStore) *apierrors.StructuredError {
	agentID := GetAgentID(ctx)
	if agentID == "" {
		return apierrors.New(apierrors.CodeUnauthorized, "authentication required", false)
	}
	a, err := store.GetByID(ctx, agentID)
	if err != nil || a == nil {
		return apierrors.New(apierrors.CodeUnauthorized, "agent not found", false)
	}
	return nil
}

func requireOAuthAgent(ctx context.Context, store agent.AgentStore) *apierrors.StructuredError {
	if err := requireAgent(ctx, store); err != nil {
		return err
	}
	agentID := GetAgentID(ctx)
	a, _ := store.GetByID(ctx, agentID)
	if a.UserID == "" {
		return apierrors.New(apierrors.CodeUnauthorized, "OAuth required (agent must be linked to a user)", false)
	}
	return nil
}

func seToGen(e *apierrors.StructuredError) generated.StructuredError {
	return generated.StructuredError{
		Error:     e.Message,
		Code:      generated.StructuredErrorCode(e.Code),
		Retriable: e.Retriable,
	}
}

func ptr[T any](v T) *T { return &v }

func orgToGen(o *org.Org) generated.Org {
	ca := o.CreatedAt
	return generated.Org{
		Id:        &o.ID,
		Name:      &o.Name,
		Slug:      &o.Slug,
		CreatedAt: &ca,
	}
}

func workStreamToGen(w *workstream.WorkStream) generated.WorkStream {
	ca := w.CreatedAt
	st := generated.WorkStreamStatus(w.Status)
	out := generated.WorkStream{
		Id:        &w.ID,
		ProjectId: &w.ProjectID,
		Name:      &w.Name,
		Slug:      &w.Slug,
		Status:    &st,
		CreatedAt: &ca,
	}
	if w.Plan != "" {
		out.Plan = &w.Plan
	}
	if w.Branch != "" {
		out.Branch = &w.Branch
	}
	return out
}

func projectToGen(p *project.Project) generated.Project {
	ca := p.CreatedAt
	st := generated.ProjectStatus(p.Status)
	cp := map[string]interface{}{}
	if len(p.ContextPack.Conventions) > 0 || len(p.ContextPack.KeyFiles) > 0 {
		cp["conventions"] = p.ContextPack.Conventions
		// key_files etc. as needed
	}
	db := p.DefaultBranch
	if db == "" {
		db = "main"
	}
	de := p.DispatchEnabled
	return generated.Project{
		Id:              &p.ID,
		OrgId:           &p.OrgID,
		Name:            &p.Name,
		Slug:            &p.Slug,
		RepoUrl:         &p.RepoURL,
		DefaultBranch:   &db,
		TechStack:       &p.TechStack,
		Status:          &st,
		DispatchEnabled: &de,
		CreatedAt:       &ca,
		ContextPack:     &cp,
	}
}

func ticketToGen(t *ticket.Ticket) generated.Ticket {
	ca := t.CreatedAt
	ua := t.UpdatedAt
	ver := t.Version
	prio := int(t.Priority)
	out := generated.Ticket{
		Id:        &t.ID,
		ProjectId: &t.ProjectID,
		Title:     &t.Title,
		Type:      (*generated.TicketType)(&t.Type),
		Priority:  &prio,
		State:     (*generated.TicketState)(&t.State),
		Version:   &ver,
		CreatedAt: &ca,
		UpdatedAt: &ua,
		CreatedBy: &t.CreatedBy,
		AssignedTo: &t.AssignedTo,
		DependsOn: &t.DependsOn,
		Inputs:    &t.Inputs,
		Outputs:   &t.Outputs,
		Objective:     objectiveToGenPtr(t.Objective),
		TicketContext: ticketContextToGenPtr(t.Context),
	}
	if t.WorkStreamID != "" {
		out.WorkStreamId = &t.WorkStreamID
	}
	return out
}

func objectiveToGenPtr(o ticket.Objective) *generated.Objective {
	if o.Description == "" && o.AcceptanceTest == "" && len(o.SuccessCriteria) == 0 {
		return nil
	}
	out := &generated.Objective{}
	if o.Description != "" {
		out.Description = &o.Description
	}
	if o.AcceptanceTest != "" {
		out.AcceptanceTest = &o.AcceptanceTest
	}
	if len(o.SuccessCriteria) > 0 {
		out.SuccessCriteria = &o.SuccessCriteria
	}
	return out
}

func objectiveFromGen(o *generated.Objective) ticket.Objective {
	if o == nil {
		return ticket.Objective{}
	}
	out := ticket.Objective{}
	if o.Description != nil {
		out.Description = *o.Description
	}
	if o.AcceptanceTest != nil {
		out.AcceptanceTest = *o.AcceptanceTest
	}
	if o.SuccessCriteria != nil {
		out.SuccessCriteria = *o.SuccessCriteria
	}
	return out
}

func ticketContextToGenPtr(t ticket.TicketContext) *generated.TicketContext {
	var priorAttempts []map[string]interface{}
	for _, a := range t.PriorAttempts {
		priorAttempts = append(priorAttempts, map[string]interface{}{
			"agent_id": a.AgentID, "outcome": a.Outcome, "summary": a.Summary, "created_at": a.CreatedAt,
		})
	}
	return &generated.TicketContext{
		RelevantFiles: &t.RelevantFiles,
		Constraints:   &t.Constraints,
		PriorAttempts: &priorAttempts,
		HumanAnswers:  &t.HumanAnswers,
	}
}

func ticketContextFromGen(t *generated.TicketContext) ticket.TicketContext {
	if t == nil {
		return ticket.TicketContext{}
	}
	out := ticket.TicketContext{}
	if t.RelevantFiles != nil {
		out.RelevantFiles = *t.RelevantFiles
	}
	if t.Constraints != nil {
		out.Constraints = *t.Constraints
	}
	if t.PriorAttempts != nil {
		for _, m := range *t.PriorAttempts {
			a := ticket.AttemptSummary{}
			if v, ok := m["agent_id"].(string); ok {
				a.AgentID = v
			}
			if v, ok := m["outcome"].(string); ok {
				a.Outcome = v
			}
			if v, ok := m["summary"].(string); ok {
				a.Summary = v
			}
			if v, ok := m["created_at"].(string); ok {
				a.CreatedAt = v
			}
			out.PriorAttempts = append(out.PriorAttempts, a)
		}
	}
	if t.HumanAnswers != nil {
		out.HumanAnswers = *t.HumanAnswers
	}
	return out
}

func execTraceToGen(tr *execution.ExecutionTrace) generated.ExecutionTrace {
	out := generated.ExecutionTrace{
		TicketId: &tr.TicketID,
		AgentId:  &tr.AgentID,
	}
	if len(tr.Steps) > 0 {
		steps := make([]generated.TraceStep, len(tr.Steps))
		for i := range tr.Steps {
			steps[i] = generated.TraceStep{
				Id:        &tr.Steps[i].ID,
				Type:      (*generated.TraceStepType)(&tr.Steps[i].Type),
				Payload:   &tr.Steps[i].Payload,
				CreatedAt: &tr.Steps[i].CreatedAt,
			}
		}
		out.Steps = &steps
	}
	return out
}

func escalationToGen(e *review.Escalation) generated.Escalation {
	out := generated.Escalation{
		Id:        &e.ID,
		TicketId:  &e.TicketID,
		AgentId:   &e.AgentID,
		Reason:    &e.Reason,
		Question:  &e.Question,
		CreatedAt: &e.CreatedAt,
	}
	if e.Answer != "" {
		out.Answer = &e.Answer
	}
	if e.ResolvedBy != "" {
		out.ResolvedBy = &e.ResolvedBy
	}
	if e.ResolvedAt != nil {
		out.ResolvedAt = e.ResolvedAt
	}
	return out
}

func leaseToGen(l *queue.Lease) generated.Lease {
	exp := l.ExpiresAt
	return generated.Lease{
		TicketId:   &l.TicketID,
		AgentId:    &l.AgentID,
		Token:      &l.Token,
		ExpiresAt:  &exp,
		Renewable:  &l.Renewable,
	}
}
