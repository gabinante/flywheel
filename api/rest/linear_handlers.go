package rest

import (
	"context"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/linear"
	"github.com/gabinante/flywheel/internal/ticket"
)

func externalRefToGen(r *ticket.ExternalRef) *generated.TicketExternalRef {
	if r == nil {
		return nil
	}
	out := &generated.TicketExternalRef{
		Provider:   r.Provider,
		ExternalId: r.ExternalID,
		Identifier: r.Identifier,
		Priority:   r.Priority,
		SyncedAt:   r.SyncedAt,
		UpdatedAt:  r.UpdatedAt,
	}
	if r.URL != "" {
		out.Url = &r.URL
	}
	if r.StateName != "" {
		out.StateName = &r.StateName
	}
	if r.StateType != "" {
		out.StateType = &r.StateType
	}
	if r.Assignee != "" {
		out.Assignee = &r.Assignee
	}
	if r.TeamKey != "" {
		out.TeamKey = &r.TeamKey
	}
	if r.BranchName != "" {
		out.BranchName = &r.BranchName
	}
	if len(r.Labels) > 0 {
		labels := r.Labels
		out.Labels = &labels
	}
	return out
}

func (s *StrictServer) projectLinkToGen(ctx context.Context, projectID string, l *linear.ProjectLink) generated.ProjectLinearLink {
	out := generated.ProjectLinearLink{ProjectId: projectID, Linked: l != nil, TeamKeys: []string{}}
	if l == nil {
		return out
	}
	out.LinearProjectId = &l.LinearProjectID
	out.LinearProjectName = &l.LinearProjectName
	out.LinearProjectUrl = &l.LinearProjectURL
	if l.TeamKeys != nil {
		out.TeamKeys = l.TeamKeys
	}
	out.SyncedAt = l.SyncedAt
	if l.LastError != "" {
		e := l.LastError
		out.LastError = &e
	}
	if s.LinearSvc != nil {
		if n, err := s.LinearSvc.TicketCount(ctx, projectID); err == nil {
			out.TicketCount = n
		}
	}
	return out
}

func (s *StrictServer) GetLinearStatus(ctx context.Context, req generated.GetLinearStatusRequestObject) (generated.GetLinearStatusResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetLinearStatus401JSONResponse(seToGen(err)), nil
	}
	out := generated.LinearStatus{Links: []generated.ProjectLinearLink{}}
	if s.LinearSvc == nil {
		return generated.GetLinearStatus200JSONResponse(out), nil
	}
	st := s.LinearSvc.Status(ctx)
	out.Enabled = st.Enabled
	out.ProjectsLinked = st.ProjectsLinked
	out.TicketsLinked = st.TicketsLinked
	out.LastRunAt = st.LastRunAt
	out.LastDurationMs = st.LastDurationMS
	if st.ViewerName != "" {
		out.ViewerName = &st.ViewerName
	}
	if st.ViewerEmail != "" {
		out.ViewerEmail = &st.ViewerEmail
	}
	if st.LastError != "" {
		e := st.LastError
		out.LastError = &e
	}
	links, err := s.LinearSvc.Links(ctx)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	for _, l := range links {
		out.Links = append(out.Links, s.projectLinkToGen(ctx, l.ProjectID, l))
	}
	return generated.GetLinearStatus200JSONResponse(out), nil
}

func (s *StrictServer) GetProjectLinearLink(ctx context.Context, req generated.GetProjectLinearLinkRequestObject) (generated.GetProjectLinearLinkResponseObject, error) {
	req.ProjectID = s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if s.LinearSvc == nil {
		return generated.GetProjectLinearLink200JSONResponse(s.projectLinkToGen(ctx, req.ProjectID, nil)), nil
	}
	l, err := s.LinearSvc.Link(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.GetProjectLinearLink200JSONResponse(s.projectLinkToGen(ctx, req.ProjectID, l)), nil
}

func (s *StrictServer) SyncProjectLinear(ctx context.Context, req generated.SyncProjectLinearRequestObject) (generated.SyncProjectLinearResponseObject, error) {
	req.ProjectID = s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if s.LinearSvc == nil || !s.LinearSvc.Enabled() {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "Linear sync is not configured (set LINEAR_API_KEY)", false)
	}
	l, err := s.LinearSvc.Link(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if l == nil {
		return generated.SyncProjectLinear404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "project is not linked to a Linear project", false))), nil
	}
	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	l, err = s.LinearSvc.SyncProject(syncCtx, req.ProjectID)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInternal, "linear sync failed: "+err.Error(), true)
	}
	return generated.SyncProjectLinear200JSONResponse(s.projectLinkToGen(ctx, req.ProjectID, l)), nil
}

func (s *StrictServer) SetProjectLinearLink(ctx context.Context, req generated.SetProjectLinearLinkRequestObject) (generated.SetProjectLinearLinkResponseObject, error) {
	req.ProjectID = s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if s.LinearSvc == nil {
		return nil, apierrors.New(apierrors.CodeInternal, "Linear integration is not available", false)
	}
	if req.Body == nil || req.Body.LinearProject == "" {
		return generated.SetProjectLinearLink400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "linear_project is required", false))), nil
	}
	l, err := s.LinearSvc.LinkProject(ctx, req.ProjectID, req.Body.LinearProject)
	if err != nil {
		return generated.SetProjectLinearLink400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))), nil
	}
	return generated.SetProjectLinearLink200JSONResponse(s.projectLinkToGen(ctx, req.ProjectID, l)), nil
}

func (s *StrictServer) DeleteProjectLinearLink(ctx context.Context, req generated.DeleteProjectLinearLinkRequestObject) (generated.DeleteProjectLinearLinkResponseObject, error) {
	req.ProjectID = s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if s.LinearSvc == nil {
		return nil, apierrors.New(apierrors.CodeInternal, "Linear integration is not available", false)
	}
	if err := s.LinearSvc.UnlinkProject(ctx, req.ProjectID); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.DeleteProjectLinearLink204Response{}, nil
}
