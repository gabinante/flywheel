package rest

import (
	"context"
	"strings"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/settings"
)

func settingsToGen(s settings.Settings, saved bool) generated.OperatorSettings {
	ids := s.Linear.ProjectIDs
	if ids == nil {
		ids = []string{}
	}
	return generated.OperatorSettings{
		Saved: saved,
		Linear: generated.LinearSettings{
			ApiKeySet: s.Linear.APIKey != "", ApiKeyHint: settings.KeyHint(s.Linear.APIKey), Enabled: s.Linear.Enabled,
			ProjectIds: ids, DefaultTeamKey: s.Linear.DefaultTeamKey, SyncIntervalSeconds: s.Linear.SyncIntervalSeconds,
		},
		Review: generated.ReviewSettings{
			Enabled: s.Review.Enabled, Harness: s.Review.Harness, Model: s.Review.Model, ReasoningEffort: s.Review.ReasoningEffort,
			Publish: s.Review.Publish, WatchRequested: s.Review.WatchRequested, WatchAuthored: s.Review.WatchAuthored, SkipDrafts: s.Review.SkipDrafts,
			MaxConcurrent: s.Review.MaxConcurrent, PollIntervalSeconds: s.Review.PollIntervalSeconds, RepoRoot: s.Review.RepoRoot,
		},
		Feedback: generated.FeedbackSettings{Harness: s.Feedback.Harness, Model: s.Feedback.Model, ReasoningEffort: s.Feedback.ReasoningEffort, AutoAddress: s.Feedback.AutoAddress},
		Report: generated.ReportSettings{
			ProjectUpdatesEnabled: s.Report.ProjectUpdatesEnabled, ProjectUpdateIntervalHours: s.Report.ProjectUpdateIntervalHours,
			WeeklyEnabled: s.Report.WeeklyEnabled, WeeklyDay: s.Report.WeeklyDay, WeeklyHour: s.Report.WeeklyHour,
			RoundupDocumentId: s.Report.RoundupDocumentID, RoundupProjectId: s.Report.RoundupProjectID, DefaultHealth: s.Report.DefaultHealth,
		},
	}
}

func (s *StrictServer) requireSettings() *apierrors.StructuredError {
	if s.SettingsSvc == nil {
		return apierrors.New(apierrors.CodeInternal, "settings are not configured", false)
	}
	return nil
}

// GetOperatorSettings returns the effective settings with the Linear key masked.
func (s *StrictServer) GetOperatorSettings(ctx context.Context, _ generated.GetOperatorSettingsRequestObject) (generated.GetOperatorSettingsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetOperatorSettings401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSettings(); err != nil {
		return nil, err
	}
	return generated.GetOperatorSettings200JSONResponse(settingsToGen(s.SettingsSvc.Current(), s.SettingsSvc.Saved())), nil
}

// UpdateOperatorSettings replaces the settings and applies them to the running services.
func (s *StrictServer) UpdateOperatorSettings(ctx context.Context, req generated.UpdateOperatorSettingsRequestObject) (generated.UpdateOperatorSettingsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.UpdateOperatorSettings401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSettings(); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return generated.UpdateOperatorSettings400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "request body is required", false))), nil
	}
	b := req.Body
	cur := s.SettingsSvc.Current()
	next := settings.Settings{
		Linear: settings.LinearSettings{
			APIKey: cur.Linear.APIKey, Enabled: b.Linear.Enabled, ProjectIDs: cleanList(b.Linear.ProjectIds),
			DefaultTeamKey: strings.TrimSpace(b.Linear.DefaultTeamKey), SyncIntervalSeconds: b.Linear.SyncIntervalSeconds,
		},
		Review: settings.ReviewSettings{
			Enabled: b.Review.Enabled, Harness: strings.ToLower(strings.TrimSpace(b.Review.Harness)), Model: strings.TrimSpace(b.Review.Model),
			ReasoningEffort: strings.TrimSpace(b.Review.ReasoningEffort), Publish: b.Review.Publish, WatchRequested: b.Review.WatchRequested,
			WatchAuthored: b.Review.WatchAuthored, SkipDrafts: b.Review.SkipDrafts, MaxConcurrent: b.Review.MaxConcurrent,
			PollIntervalSeconds: b.Review.PollIntervalSeconds, RepoRoot: strings.TrimSpace(b.Review.RepoRoot),
		},
		Feedback: settings.FeedbackSettings{
			Harness: strings.ToLower(strings.TrimSpace(b.Feedback.Harness)), Model: strings.TrimSpace(b.Feedback.Model),
			ReasoningEffort: strings.TrimSpace(b.Feedback.ReasoningEffort), AutoAddress: b.Feedback.AutoAddress,
		},
		Report: settings.ReportSettings{
			ProjectUpdatesEnabled: b.Report.ProjectUpdatesEnabled, ProjectUpdateIntervalHours: b.Report.ProjectUpdateIntervalHours,
			WeeklyEnabled: b.Report.WeeklyEnabled, WeeklyDay: strings.TrimSpace(b.Report.WeeklyDay), WeeklyHour: b.Report.WeeklyHour,
			RoundupDocumentID: strings.TrimSpace(b.Report.RoundupDocumentId), RoundupProjectID: strings.TrimSpace(b.Report.RoundupProjectId),
			DefaultHealth: strings.TrimSpace(b.Report.DefaultHealth),
		},
	}
	if b.Linear.ClearApiKey != nil && *b.Linear.ClearApiKey {
		next.Linear.APIKey = ""
	}
	if b.Linear.ApiKey != nil && strings.TrimSpace(*b.Linear.ApiKey) != "" {
		next.Linear.APIKey = strings.TrimSpace(*b.Linear.ApiKey)
	}
	saved, err := s.SettingsSvc.Update(ctx, next)
	if err != nil {
		return generated.UpdateOperatorSettings400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))), nil
	}
	return generated.UpdateOperatorSettings200JSONResponse(settingsToGen(saved, true)), nil
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
