package rest

import (
	"encoding/json"
	"net/http"
	"time"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/plan"
)

// PlansHandler handles plan CRUD and lifecycle operations.
type PlansHandler struct {
	PlanSvc *plan.Service
}

// createPlanRequest is the request body for creating a plan.
type createPlanRequest struct {
	TicketID       string           `json:"ticket_id"`
	Backend        string           `json:"backend"`
	Content        json.RawMessage  `json:"content"`
	CreatedBy      string           `json:"created_by"`
	FreshnessStamp *time.Time       `json:"freshness_stamp,omitempty"`
	ExpiresAt      *time.Time       `json:"expires_at,omitempty"`
}

// updateContentRequest is the request body for updating plan content.
type updateContentRequest struct {
	Content   json.RawMessage `json:"content"`
	UpdatedBy string          `json:"updated_by"`
}

// transitionRequest is the request body for plan state transitions.
type transitionRequest struct {
	Action string `json:"action"`
}

func (h *PlansHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}

	if body.TicketID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "ticket_id required", false))
		return
	}
	if body.Backend == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "backend required", false))
		return
	}
	if body.CreatedBy == "" {
		// Try to use authenticated agent ID
		if agentID := GetAgentID(r.Context()); agentID != "" {
			body.CreatedBy = agentID
		} else {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "created_by required", false))
			return
		}
	}

	backend := plan.Backend(body.Backend)
	if !plan.IsValidBackend(backend) {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid backend: must be one of database, terraform, code, shell, deploy", false))
		return
	}

	// Parse content based on backend type
	content, err := parseContent(backend, body.Content)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}

	freshnessStamp := time.Now().UTC()
	if body.FreshnessStamp != nil {
		freshnessStamp = *body.FreshnessStamp
	}

	p, err := h.PlanSvc.CreatePlan(r.Context(), body.TicketID, backend, content, body.CreatedBy, freshnessStamp, body.ExpiresAt)
	if err != nil {
		if ve, ok := err.(*plan.ValidationError); ok {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, ve.Error(), false))
			return
		}
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *PlansHandler) get(w http.ResponseWriter, r *http.Request) {
	planID := PathParam(r, "planID")
	p, err := h.PlanSvc.GetPlan(r.Context(), planID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *PlansHandler) listByTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	backend := r.URL.Query().Get("backend")

	var plans []*plan.Plan
	var err error
	if backend != "" {
		b := plan.Backend(backend)
		if !plan.IsValidBackend(b) {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid backend filter", false))
			return
		}
		plans, err = h.PlanSvc.ListPlansByTicketAndBackend(r.Context(), ticketID, b)
	} else {
		plans, err = h.PlanSvc.ListPlansByTicket(r.Context(), ticketID)
	}
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if plans == nil {
		plans = []*plan.Plan{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plans)
}

func (h *PlansHandler) updateContent(w http.ResponseWriter, r *http.Request) {
	planID := PathParam(r, "planID")

	var body updateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}
	if body.UpdatedBy == "" {
		if agentID := GetAgentID(r.Context()); agentID != "" {
			body.UpdatedBy = agentID
		} else {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "updated_by required", false))
			return
		}
	}

	// Get existing plan to know the backend
	p, err := h.PlanSvc.GetPlan(r.Context(), planID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	content, err := parseContent(p.Backend, body.Content)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}

	if err := h.PlanSvc.UpdateContent(r.Context(), planID, content, body.UpdatedBy); err != nil {
		if ve, ok := err.(*plan.ValidationError); ok {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, ve.Error(), false))
			return
		}
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PlansHandler) transition(w http.ResponseWriter, r *http.Request) {
	planID := PathParam(r, "planID")

	var body transitionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}
	if body.Action == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "action required", false))
		return
	}

	var err error
	switch body.Action {
	case "submit":
		err = h.PlanSvc.SubmitPlan(r.Context(), planID)
	case "classify":
		err = h.PlanSvc.ClassifyPlan(r.Context(), planID)
	case "approve":
		err = h.PlanSvc.ApprovePlan(r.Context(), planID)
	case "apply":
		err = h.PlanSvc.ApplyPlan(r.Context(), planID)
	case "reject":
		err = h.PlanSvc.RejectPlan(r.Context(), planID)
	default:
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "action must be one of: submit, classify, approve, apply, reject", false))
		return
	}

	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	// Return the updated plan
	p, err := h.PlanSvc.GetPlan(r.Context(), planID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *PlansHandler) versions(w http.ResponseWriter, r *http.Request) {
	planID := PathParam(r, "planID")
	versions, err := h.PlanSvc.GetVersions(r.Context(), planID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if versions == nil {
		versions = []*plan.PlanVersion{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(versions)
}

func (h *PlansHandler) freshness(w http.ResponseWriter, r *http.Request) {
	planID := PathParam(r, "planID")
	fresh, err := h.PlanSvc.IsFresh(r.Context(), planID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"plan_id": planID, "fresh": fresh})
}

// parseContent unmarshals raw JSON into the correct Content struct based on backend.
func parseContent(backend plan.Backend, raw json.RawMessage) (plan.Content, error) {
	var content plan.Content
	switch backend {
	case plan.BackendDatabase:
		var db plan.DatabasePlan
		if err := json.Unmarshal(raw, &db); err != nil {
			return content, &plan.ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for database plan: " + err.Error()}
		}
		content.Database = &db
	case plan.BackendTerraform:
		var tf plan.TerraformPlan
		if err := json.Unmarshal(raw, &tf); err != nil {
			return content, &plan.ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for terraform plan: " + err.Error()}
		}
		content.Terraform = &tf
	case plan.BackendCode:
		var code plan.CodePlan
		if err := json.Unmarshal(raw, &code); err != nil {
			return content, &plan.ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for code plan: " + err.Error()}
		}
		content.Code = &code
	case plan.BackendShell:
		var shell plan.ShellPlan
		if err := json.Unmarshal(raw, &shell); err != nil {
			return content, &plan.ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for shell plan: " + err.Error()}
		}
		content.Shell = &shell
	case plan.BackendDeploy:
		var deploy plan.DeployPlan
		if err := json.Unmarshal(raw, &deploy); err != nil {
			return content, &plan.ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for deploy plan: " + err.Error()}
		}
		content.Deploy = &deploy
	}
	return content, nil
}
