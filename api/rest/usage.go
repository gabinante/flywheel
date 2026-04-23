package rest

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/cost"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

type UsageHandler struct {
	CostSvc    *cost.Service
	ProjectSvc ProjectGetterForAccess
	OrgSvc     OrgMemberLister
	AgentStore AgentGetter
}

type usageBreakdownItem struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	Calls         int     `json:"calls"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
	CostMillicent int64   `json:"cost_millicent"`
	CostDollars   float64 `json:"cost_dollars"`
	Share         float64 `json:"share"`
}

type usageDailyPoint struct {
	Date          string  `json:"date"`
	Calls         int     `json:"calls"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
	CostMillicent int64   `json:"cost_millicent"`
	CostDollars   float64 `json:"cost_dollars"`
}

type usageSummary struct {
	Calls         int     `json:"calls"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
	CostMillicent int64   `json:"cost_millicent"`
	CostDollars   float64 `json:"cost_dollars"`
}

func (h *UsageHandler) getProjectUsage(w http.ResponseWriter, r *http.Request) {
	if h.CostSvc == nil || h.CostSvc.Tracker == nil {
		writeUsageJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":     "usage tracking not enabled",
			"code":      "service_unavailable",
			"retriable": false,
		})
		return
	}

	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			switch {
			case parsed < 1:
				days = 1
			case parsed > 180:
				days = 180
			default:
				days = parsed
			}
		}
	}

	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days+1)
	to := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)

	calls, err := h.CostSvc.Tracker.GetProjectCalls(r.Context(), projectID, "")
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	daily := make(map[string]*usageDailyPoint, days)
	for i := 0; i < days; i++ {
		day := from.AddDate(0, 0, i).Format("2006-01-02")
		daily[day] = &usageDailyPoint{Date: day}
	}

	byTaskType := make(map[string]*usageBreakdownItem)
	byWorkerGroup := make(map[string]*usageBreakdownItem)
	byAPI := make(map[string]*usageBreakdownItem)
	byProvider := make(map[string]*usageBreakdownItem)
	byModel := make(map[string]*usageBreakdownItem)
	summary := usageSummary{}

	for _, call := range calls {
		if call == nil || call.Timestamp.Before(from) || call.Timestamp.After(to) {
			continue
		}
		tokens := call.InputTokens + call.OutputTokens
		costMilli := int64(call.CostMillicent)
		summary.Calls++
		summary.InputTokens += call.InputTokens
		summary.OutputTokens += call.OutputTokens
		summary.TotalTokens += tokens
		summary.CostMillicent += costMilli

		dayKey := call.Timestamp.UTC().Format("2006-01-02")
		if point := daily[dayKey]; point != nil {
			point.Calls++
			point.InputTokens += call.InputTokens
			point.OutputTokens += call.OutputTokens
			point.TotalTokens += tokens
			point.CostMillicent += costMilli
		}

		taskType := normalizeTaskType(call.WorkerRole)
		accumulateUsage(byTaskType, taskType, taskType, call, tokens, costMilli)

		workerGroup := workerGroupForRole(call.WorkerRole)
		accumulateUsage(byWorkerGroup, workerGroup, workerGroup, call, tokens, costMilli)

		apiLabel := cost.APILabel(call.Provider, call.Model)
		accumulateUsage(byAPI, apiLabel, apiLabel, call, tokens, costMilli)

		provider := strings.TrimSpace(call.Provider)
		if provider == "" {
			provider = "unknown"
		}
		accumulateUsage(byProvider, provider, provider, call, tokens, costMilli)

		model := strings.TrimSpace(call.Model)
		if model == "" {
			model = "unknown"
		}
		accumulateUsage(byModel, model, model, call, tokens, costMilli)
	}

	summary.CostDollars = cost.Unit(summary.CostMillicent).ToDollars()

	var budgetStatus *cost.BudgetStatus
	if status, err := h.CostSvc.GetProjectStatus(r.Context(), projectID); err == nil {
		budgetStatus = status
	}

	writeUsageJSON(w, http.StatusOK, map[string]any{
		"project_id": projectID,
		"estimated":  true,
		"window": map[string]any{
			"days": days,
			"from": from.Format(time.RFC3339),
			"to":   to.Format(time.RFC3339),
		},
		"summary":         summary,
		"daily":           sortedDailyUsage(daily),
		"by_task_type":    sortedBreakdown(byTaskType, summary.TotalTokens),
		"by_worker_group": sortedBreakdown(byWorkerGroup, summary.TotalTokens),
		"by_api":          sortedBreakdown(byAPI, summary.TotalTokens),
		"by_provider":     sortedBreakdown(byProvider, summary.TotalTokens),
		"by_model":        sortedBreakdown(byModel, summary.TotalTokens),
		"budget_status":   budgetStatus,
	})
}

func accumulateUsage(dst map[string]*usageBreakdownItem, key, label string, call *cost.LLMCallRecord, totalTokens, costMilli int64) {
	item := dst[key]
	if item == nil {
		item = &usageBreakdownItem{Key: key, Label: label}
		dst[key] = item
	}
	item.Calls++
	item.InputTokens += call.InputTokens
	item.OutputTokens += call.OutputTokens
	item.TotalTokens += totalTokens
	item.CostMillicent += costMilli
}

func sortedDailyUsage(points map[string]*usageDailyPoint) []usageDailyPoint {
	keys := make([]string, 0, len(points))
	for key := range points {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]usageDailyPoint, 0, len(keys))
	for _, key := range keys {
		point := points[key]
		point.CostDollars = cost.Unit(point.CostMillicent).ToDollars()
		out = append(out, *point)
	}
	return out
}

func sortedBreakdown(items map[string]*usageBreakdownItem, totalTokens int64) []usageBreakdownItem {
	out := make([]usageBreakdownItem, 0, len(items))
	for _, item := range items {
		item.CostDollars = cost.Unit(item.CostMillicent).ToDollars()
		if totalTokens > 0 {
			item.Share = float64(item.TotalTokens) / float64(totalTokens)
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalTokens == out[j].TotalTokens {
			return out[i].Label < out[j].Label
		}
		return out[i].TotalTokens > out[j].TotalTokens
	})
	return out
}

func normalizeTaskType(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "", "unknown":
		return "unknown"
	case "implementation", "planning", "review", "deployment", "investigation", "orchestrator", "conflict_resolution":
		return role
	default:
		return role
	}
}

func workerGroupForRole(role string) string {
	switch normalizeTaskType(role) {
	case "orchestrator":
		return "orchestrator"
	case "review":
		return "reviewer"
	default:
		return "worker"
	}
}

func writeUsageJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
