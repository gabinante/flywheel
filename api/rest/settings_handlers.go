package rest

import (
	"context"
	"encoding/json"
	"github.com/gabinante/flywheel/internal/project"
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
			Enabled: s.Review.Enabled, RoleId: optStr(s.Review.RoleID), Harness: s.Review.Harness, Model: s.Review.Model, ReasoningEffort: s.Review.ReasoningEffort,
			Publish: s.Review.Publish, WatchRequested: s.Review.WatchRequested, WatchAuthored: s.Review.WatchAuthored, SkipDrafts: s.Review.SkipDrafts,
			MaxConcurrent: s.Review.MaxConcurrent, PollIntervalSeconds: s.Review.PollIntervalSeconds, RepoRoot: s.Review.RepoRoot,
			ReReviewQuietMinutes: s.Review.ReReviewQuietMinutes, ReReviewMinGapMinutes: s.Review.ReReviewMinGapMinutes, WatchScope: s.Review.WatchScope,
		},
		Feedback: generated.FeedbackSettings{RoleId: optStr(s.Feedback.RoleID), Harness: s.Feedback.Harness, Model: s.Feedback.Model, ReasoningEffort: s.Feedback.ReasoningEffort, AutoAddress: s.Feedback.AutoAddress},
		Report: generated.ReportSettings{
			ProjectUpdatesEnabled: s.Report.ProjectUpdatesEnabled, ProjectUpdateIntervalHours: s.Report.ProjectUpdateIntervalHours,
			WeeklyEnabled: s.Report.WeeklyEnabled, WeeklyDay: s.Report.WeeklyDay, WeeklyHour: s.Report.WeeklyHour,
			RoundupDocumentId: s.Report.RoundupDocumentID, RoundupProjectId: s.Report.RoundupProjectID, DefaultHealth: s.Report.DefaultHealth,
		},
		Workers: dispatchConfigToGen(s.Workers),
		Harnesses: generated.HarnessSettings{
			Claude: generated.HarnessDefaults{Bin: s.Harness.Claude.Bin, Model: s.Harness.Claude.Model, ReasoningEffort: s.Harness.Claude.ReasoningEffort},
			Codex:  generated.HarnessDefaults{Bin: s.Harness.Codex.Bin, Model: s.Harness.Codex.Model, ReasoningEffort: s.Harness.Codex.ReasoningEffort},
		},
		Dispatch: generated.DispatchSettings{
			Enabled: s.Dispatch.Enabled, MaxWorkers: s.Dispatch.MaxWorkers, Driver: s.Dispatch.Driver, Model: s.Dispatch.Model,
			ReasoningEffort: s.Dispatch.ReasoningEffort, WorktreeDir: s.Dispatch.WorktreeDir, WorkerKeySet: s.Dispatch.WorkerAPIKey != "",
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
			Enabled: b.Review.Enabled, RoleID: derefStr(b.Review.RoleId), Harness: strings.ToLower(strings.TrimSpace(b.Review.Harness)), Model: strings.TrimSpace(b.Review.Model),
			ReasoningEffort: strings.TrimSpace(b.Review.ReasoningEffort), Publish: b.Review.Publish, WatchRequested: b.Review.WatchRequested,
			WatchAuthored: b.Review.WatchAuthored, SkipDrafts: b.Review.SkipDrafts, MaxConcurrent: b.Review.MaxConcurrent,
			PollIntervalSeconds: b.Review.PollIntervalSeconds, RepoRoot: strings.TrimSpace(b.Review.RepoRoot),
			ReReviewQuietMinutes: b.Review.ReReviewQuietMinutes, ReReviewMinGapMinutes: b.Review.ReReviewMinGapMinutes, WatchScope: strings.ToLower(strings.TrimSpace(b.Review.WatchScope)),
		},
		Feedback: settings.FeedbackSettings{
			RoleID: derefStr(b.Feedback.RoleId), Harness: strings.ToLower(strings.TrimSpace(b.Feedback.Harness)), Model: strings.TrimSpace(b.Feedback.Model),
			ReasoningEffort: strings.TrimSpace(b.Feedback.ReasoningEffort), AutoAddress: b.Feedback.AutoAddress,
		},
		Report: settings.ReportSettings{
			ProjectUpdatesEnabled: b.Report.ProjectUpdatesEnabled, ProjectUpdateIntervalHours: b.Report.ProjectUpdateIntervalHours,
			WeeklyEnabled: b.Report.WeeklyEnabled, WeeklyDay: strings.TrimSpace(b.Report.WeeklyDay), WeeklyHour: b.Report.WeeklyHour,
			RoundupDocumentID: strings.TrimSpace(b.Report.RoundupDocumentId), RoundupProjectID: strings.TrimSpace(b.Report.RoundupProjectId),
			DefaultHealth: strings.TrimSpace(b.Report.DefaultHealth),
		},
	}
	next.Prompts = cur.Prompts
	next.Layout = cur.Layout // the settings page never edits the layout
	next.Workers = cur.Workers
	if b.Workers != nil {
		next.Workers = dispatchConfigFromGen(*b.Workers)
	}
	next.Harness = cur.Harness
	if b.Harnesses != nil {
		next.Harness = settings.HarnessSettings{
			Claude: settings.HarnessDefaults{Bin: strings.TrimSpace(b.Harnesses.Claude.Bin), Model: strings.TrimSpace(b.Harnesses.Claude.Model), ReasoningEffort: strings.TrimSpace(b.Harnesses.Claude.ReasoningEffort)},
			Codex:  settings.HarnessDefaults{Bin: strings.TrimSpace(b.Harnesses.Codex.Bin), Model: strings.TrimSpace(b.Harnesses.Codex.Model), ReasoningEffort: strings.TrimSpace(b.Harnesses.Codex.ReasoningEffort)},
		}
	}
	next.Dispatch = cur.Dispatch // keep the worker key and anything the client did not send
	if b.Dispatch != nil {
		next.Dispatch.Enabled = b.Dispatch.Enabled
		next.Dispatch.MaxWorkers = b.Dispatch.MaxWorkers
		next.Dispatch.Driver = strings.ToLower(strings.TrimSpace(b.Dispatch.Driver))
		next.Dispatch.Model = strings.TrimSpace(b.Dispatch.Model)
		next.Dispatch.ReasoningEffort = strings.TrimSpace(b.Dispatch.ReasoningEffort)
		if wd := strings.TrimSpace(b.Dispatch.WorktreeDir); wd != "" {
			next.Dispatch.WorktreeDir = wd
		}
	}
	if b.Linear.ClearApiKey != nil && *b.Linear.ClearApiKey {
		next.Linear.APIKey = ""
	}
	if b.Linear.ApiKey != nil && strings.TrimSpace(*b.Linear.ApiKey) != "" {
		next.Linear.APIKey = strings.TrimSpace(*b.Linear.ApiKey)
	}
	saved, err := s.SettingsSvc.UpdateOperational(ctx, next)
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

func optStr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

// dispatchConfigToGen / dispatchConfigFromGen round-trip through JSON: the generated
// type mirrors the domain type's json tags.
func dispatchConfigToGen(c project.DispatchConfig) generated.DispatchConfig {
	var out generated.DispatchConfig
	raw, _ := json.Marshal(c.Normalized())
	_ = json.Unmarshal(raw, &out)
	return out
}

func dispatchConfigFromGen(c generated.DispatchConfig) project.DispatchConfig {
	var out project.DispatchConfig
	raw, _ := json.Marshal(c)
	_ = json.Unmarshal(raw, &out)
	return out.Normalized()
}
