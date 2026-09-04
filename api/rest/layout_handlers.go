package rest

import (
	"context"
	"strings"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/settings"
)

func layoutToGen(l settings.LayoutSettings) generated.Layout {
	out := generated.Layout{ProjectSections: []generated.ProjectSection{}}
	for _, sct := range l.ProjectSections {
		ids := sct.ProjectIDs
		if ids == nil {
			ids = []string{}
		}
		out.ProjectSections = append(out.ProjectSections, generated.ProjectSection{Id: sct.ID, Name: sct.Name, ProjectIds: ids, Collapsed: sct.Collapsed})
	}
	return out
}

// GetLayout returns the operator's UI arrangement.
func (s *StrictServer) GetLayout(ctx context.Context, _ generated.GetLayoutRequestObject) (generated.GetLayoutResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetLayout401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSettings(); err != nil {
		return nil, err
	}
	return generated.GetLayout200JSONResponse(layoutToGen(s.SettingsSvc.Current().Layout)), nil
}

// UpdateLayout replaces the operator's UI arrangement.
func (s *StrictServer) UpdateLayout(ctx context.Context, req generated.UpdateLayoutRequestObject) (generated.UpdateLayoutResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.UpdateLayout401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSettings(); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "request body is required", false)
	}
	var layout settings.LayoutSettings
	for _, sct := range req.Body.ProjectSections {
		if strings.TrimSpace(sct.Id) == "" {
			continue
		}
		layout.ProjectSections = append(layout.ProjectSections, settings.ProjectSection{
			ID: strings.TrimSpace(sct.Id), Name: strings.TrimSpace(sct.Name), ProjectIDs: cleanList(sct.ProjectIds), Collapsed: sct.Collapsed,
		})
	}
	saved, err := s.SettingsSvc.UpdateLayout(ctx, layout)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false)
	}
	return generated.UpdateLayout200JSONResponse(layoutToGen(saved)), nil
}
