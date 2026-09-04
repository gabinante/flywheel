package rest

import (
	"context"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/report"
)

func reportToGen(r *report.Report) generated.Report {
	out := generated.Report{Id: r.ID, Kind: r.Kind, WindowStart: r.WindowStart, WindowEnd: r.WindowEnd, Body: r.Body, Posted: r.Posted, CreatedAt: r.CreatedAt}
	if r.ProjectID != "" {
		v := r.ProjectID
		out.ProjectId = &v
	}
	if r.Health != "" {
		v := r.Health
		out.Health = &v
	}
	if r.URL != "" {
		v := r.URL
		out.Url = &v
	}
	return out
}

func (s *StrictServer) requireReports() *apierrors.StructuredError {
	if s.ReportSvc == nil {
		return apierrors.New(apierrors.CodeInternal, "reports are not configured", false)
	}
	return nil
}

func parseWeekOf(v *string) time.Time {
	if v == nil || *v == "" {
		return time.Now()
	}
	if t, err := time.ParseInLocation("2006-01-02", *v, time.Local); err == nil {
		return t
	}
	return time.Now()
}

func (s *StrictServer) ListReports(ctx context.Context, req generated.ListReportsRequestObject) (generated.ListReportsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.ListReports401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireReports(); err != nil {
		return nil, err
	}
	pid, kind, limit := "", "", 50
	if req.Params.ProjectId != nil {
		pid = s.resolveProject(ctx, *req.Params.ProjectId)
	}
	if req.Params.Kind != nil {
		kind = *req.Params.Kind
	}
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	list, err := s.ReportSvc.List(ctx, pid, kind, limit)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := generated.ReportListResponse{Reports: make([]generated.Report, 0, len(list))}
	for _, r := range list {
		out.Reports = append(out.Reports, reportToGen(r))
	}
	return generated.ListReports200JSONResponse(out), nil
}

func (s *StrictServer) PreviewWeeklyRoundup(ctx context.Context, req generated.PreviewWeeklyRoundupRequestObject) (generated.PreviewWeeklyRoundupResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.PreviewWeeklyRoundup401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireReports(); err != nil {
		return nil, err
	}
	r, err := s.ReportSvc.ComposeWeeklyRoundup(ctx, parseWeekOf(req.Params.WeekOf))
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.PreviewWeeklyRoundup200JSONResponse(reportToGen(r)), nil
}

func (s *StrictServer) PostWeeklyRoundup(ctx context.Context, req generated.PostWeeklyRoundupRequestObject) (generated.PostWeeklyRoundupResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.PostWeeklyRoundup401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireReports(); err != nil {
		return nil, err
	}
	weekOf := time.Now()
	body := ""
	if req.Body != nil {
		weekOf = parseWeekOf(req.Body.WeekOf)
		if req.Body.Body != nil {
			body = *req.Body.Body
		}
	}
	r, err := s.ReportSvc.PostWeeklyRoundup(ctx, weekOf, body)
	if err != nil {
		return generated.PostWeeklyRoundup400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))), nil
	}
	return generated.PostWeeklyRoundup200JSONResponse(reportToGen(r)), nil
}

func (s *StrictServer) PreviewProjectUpdate(ctx context.Context, req generated.PreviewProjectUpdateRequestObject) (generated.PreviewProjectUpdateResponseObject, error) {
	req.ProjectID = s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if err := s.requireReports(); err != nil {
		return nil, err
	}
	r, err := s.ReportSvc.ComposeProjectUpdate(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.PreviewProjectUpdate200JSONResponse(reportToGen(r)), nil
}

func (s *StrictServer) PostProjectUpdate(ctx context.Context, req generated.PostProjectUpdateRequestObject) (generated.PostProjectUpdateResponseObject, error) {
	req.ProjectID = s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if err := s.requireReports(); err != nil {
		return nil, err
	}
	body, health := "", ""
	if req.Body != nil {
		if req.Body.Body != nil {
			body = *req.Body.Body
		}
		if req.Body.Health != nil {
			health = string(*req.Body.Health)
		}
	}
	r, err := s.ReportSvc.PostProjectUpdate(ctx, req.ProjectID, body, health)
	if err != nil {
		return generated.PostProjectUpdate400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))), nil
	}
	return generated.PostProjectUpdate200JSONResponse(reportToGen(r)), nil
}
