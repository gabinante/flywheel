package rest

import (
	"context"
	"errors"
	"strings"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/project"
)

// MergeProject folds the path project into the `into` project and deletes it.
func (s *StrictServer) MergeProject(ctx context.Context, req generated.MergeProjectRequestObject) (generated.MergeProjectResponseObject, error) {
	sourceID := s.resolveProject(ctx, req.ProjectID)
	if err := CheckProjectAccess(ctx, sourceID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Into) == "" {
		return generated.MergeProject400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "into (target project id or slug) is required", false))), nil
	}
	targetID := s.resolveProject(ctx, strings.TrimSpace(req.Body.Into))
	if err := CheckProjectAccess(ctx, targetID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	res, err := s.ProjectSvc.MergeProjects(ctx, sourceID, targetID)
	if err != nil {
		switch {
		case errors.Is(err, project.ErrMergeBothLinked):
			return generated.MergeProject409JSONResponse(seToGen(apierrors.New(apierrors.CodeConflict, err.Error(), false))), nil
		case errors.Is(err, project.ErrMergeSameProject), errors.Is(err, project.ErrMergeDifferentOrg):
			return generated.MergeProject400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))), nil
		case errors.Is(err, project.ErrProjectNotFound):
			return generated.MergeProject404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "project not found", false))), nil
		}
		return nil, apierrors.MapError(err)
	}
	target, err := s.ProjectSvc.GetProject(ctx, targetID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	touched := map[string]int{}
	for k, v := range res.TablesTouched {
		touched[k] = int(v)
	}
	return generated.MergeProject200JSONResponse(generated.ProjectMergeResult{
		Target: projectToGen(target), SourceId: res.SourceID, SourceName: res.SourceName,
		TicketsMoved: int(res.TicketsMoved), WorkStreamsMoved: int(res.WorkStreamsMoved), ReposMoved: int(res.ReposMoved),
		LinearLinkMoved: res.LinearLinkMoved, TablesTouched: touched,
	}), nil
}
