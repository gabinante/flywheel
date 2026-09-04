package rest

import (
	"context"
	"errors"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/schedule"
)

func actionToGen(a schedule.Action) generated.ScheduledAction {
	out := generated.ScheduledAction{
		Id: a.ID, Name: a.Name, Description: a.Description, Kind: generated.ScheduledActionKind(a.Kind), Enabled: a.Enabled,
		LastRunAt: a.LastRunAt, NextRunAt: a.NextRunAt, Runnable: a.Runnable, Outward: a.Outward,
	}
	if a.IntervalSeconds > 0 {
		v := a.IntervalSeconds
		out.IntervalSeconds = &v
	}
	if a.LastError != "" {
		v := a.LastError
		out.LastError = &v
	}
	if a.Detail != "" {
		v := a.Detail
		out.Detail = &v
	}
	if a.Count > 0 {
		v := a.Count
		out.Count = &v
	}
	if a.ProjectID != "" {
		v := a.ProjectID
		out.ProjectId = &v
	}
	if a.SettingsSection != "" {
		v := a.SettingsSection
		out.SettingsSection = &v
	}
	return out
}

func (s *StrictServer) requireSchedule() *apierrors.StructuredError {
	if s.ScheduleSvc == nil {
		return apierrors.New(apierrors.CodeInternal, "schedule is not configured", false)
	}
	return nil
}

// ListScheduledActions returns every automation with its timing.
func (s *StrictServer) ListScheduledActions(ctx context.Context, _ generated.ListScheduledActionsRequestObject) (generated.ListScheduledActionsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.ListScheduledActions401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSchedule(); err != nil {
		return nil, err
	}
	items := s.ScheduleSvc.List(ctx)
	out := generated.ScheduledActions{Now: time.Now(), Items: make([]generated.ScheduledAction, 0, len(items))}
	for _, a := range items {
		out.Items = append(out.Items, actionToGen(a))
	}
	return generated.ListScheduledActions200JSONResponse(out), nil
}

// RunScheduledAction triggers one action now.
func (s *StrictServer) RunScheduledAction(ctx context.Context, req generated.RunScheduledActionRequestObject) (generated.RunScheduledActionResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return nil, err
	}
	if err := s.requireSchedule(); err != nil {
		return nil, err
	}
	a, err := s.ScheduleSvc.Run(ctx, req.ActionID)
	if err != nil {
		if errors.Is(err, schedule.ErrUnknownAction) {
			return generated.RunScheduledAction404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "unknown scheduled action", false))), nil
		}
		return generated.RunScheduledAction400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), true))), nil
	}
	return generated.RunScheduledAction200JSONResponse(actionToGen(*a)), nil
}
