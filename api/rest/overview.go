package rest

import (
	"encoding/json"
	"github.com/gabinante/flywheel/internal/auth"
	"github.com/gabinante/flywheel/internal/codereview"
	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/overview"
	"github.com/gabinante/flywheel/internal/runstatus"
	"log/slog"
	"net/http"
)

type OverviewHandler struct {
	Store       *overview.Store
	Dispatcher  *dispatch.Dispatcher
	CodeReviews *codereview.Service
}

func (h *OverviewHandler) get(w http.ResponseWriter, r *http.Request) {
	if !auth.IsOperator(r.Context()) {
		http.Error(w, "local operator required", http.StatusUnauthorized)
		return
	}
	status := h.Dispatcher.Summary()
	var reviewIDs []string
	if h.CodeReviews != nil {
		reviewIDs = h.CodeReviews.ActiveReviewIDs()
	}
	snapshot, err := h.Store.Get(r.Context(), runstatus.Default.Snapshot(), status.ActiveTicketIDs, reviewIDs)
	if err != nil {
		slog.Error("global overview failed", "error", err)
		http.Error(w, "Could not load current work. Try again shortly.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		overview.Snapshot
		Dispatch dispatch.Status `json:"dispatch"`
	}{snapshot, status})
}
