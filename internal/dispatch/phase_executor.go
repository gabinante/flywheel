package dispatch

// phase_executor.go contains the workflow phase processing subsystem. These are
// Dispatcher methods extracted from dispatcher.go for maintainability. The
// dispatcher delegates all phase routing, gate rechecking, and timeout checking
// to functions in this file.

import (
	"context"
	"log/slog"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/gate"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workflow"
)

func (d *Dispatcher) processReadyPhase(ctx context.Context, t *ticket.Ticket) {
	if t.WorkflowID == "" || t.WorkflowPhase == "" || d.workflowEngine == nil {
		return
	}

	pos, err := d.workflowEngine.GetPosition(ctx, t.ID, t.WorkflowID, t.WorkflowPhase, t.WorkflowVersion)
	if err != nil || pos == nil || pos.CurrentPhase == nil {
		slog.Error("dispatch: get workflow position failed", "ticket", t.ID, "error", err)
		return
	}

	phase := pos.CurrentPhase
	if phase.Type == workflow.PhaseAgent {
		d.mu.Lock()
		_, worker := d.active[t.ID]
		_, reviewer := d.active["review:"+t.ID]
		d.mu.Unlock()
		if worker || reviewer {
			return
		}
		cfg, _ := workflow.ParseAgentConfig(phase.Config)
		if cfg.Role == "validator" {
			d.spawnReviewer(ctx, t)
			return
		}
		if t.State != ticket.StateDraft {
			if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerWorkflowContinue, ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}, nil); err != nil {
				return
			}
			t, err = d.tickets.GetTicket(ctx, t.ID)
			if err != nil {
				return
			}
		}
		d.spawn(ctx, t)
		return
	}
	if d.workflowPhaseUpdater != nil {
		ok, err := d.claimPhase(ctx, t)
		if err != nil || !ok {
			return
		}
	}

	switch phase.Type {
	case workflow.PhaseGate:
		// Check gate conditions. Met → advance. Not met → set blocked, emit event.
		if d.checkerRegistry != nil {
			gateCfg, _ := workflow.ParseGateConfig(phase.Config)
			if gateCfg != nil {
				conditions := gateCfg.EffectiveConditions()
				for _, condition := range conditions {
					if condition.Type == "webhook" && d.externalExecutor != nil && d.outputPatcher != nil {
						marker, _ := t.Outputs["_gate_callback_"+phase.ID].(map[string]any)
						if marker["entered_at"] != phaseEnteredAt(t) {
							url, err := d.externalExecutor.GateCallback(ctx, t.ID, t.WorkflowID, phase.ID)
							if err != nil {
								d.advanceWorkflowIfNeeded(ctx, t, "failed")
								return
							}
							if err := d.outputPatcher.PatchOutputs(ctx, t.ID, map[string]any{"_gate_callback_" + phase.ID: map[string]any{"url": url, "entered_at": phaseEnteredAt(t)}}); err != nil {
								return
							}
						}
					}
				}
				if len(conditions) > 0 {
					var reqs []gate.GateRequirement
					for _, c := range conditions {
						reqs = append(reqs, gate.GateRequirement{Type: gate.GateRequirementType(c.Type), Config: c.Config})
					}
					prURL, _ := t.Outputs["pr_url"].(string)
					statuses := d.checkerRegistry.CheckAll(ctx, reqs, gate.CheckContext{
						TicketID: t.ID, ProjectID: t.ProjectID, PRURL: prURL,
						PhaseID: phase.ID, Outputs: t.Outputs, PhaseEnteredAt: phaseEnteredAt(t),
					})
					if gate.AnyFailed(statuses) {
						d.advanceWorkflowIfNeeded(ctx, t, "failed")
						return
					}
					if len(gate.Unsatisfied(statuses)) == 0 {
						// All conditions met — advance.
						next := d.advanceWorkflowIfNeeded(ctx, t, "success")
						if next == nil {
							actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
							_ = actor
							d.completeWorkflow(ctx, t)
						}
						return
					}
				}
			}
		}
		// Conditions not met — set blocked and emit gate event.
		if d.workflowPhaseUpdater != nil {
			_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
		}
		gateCfg, _ := workflow.ParseGateConfig(phase.Config)
		var conditionTypes []string
		if gateCfg != nil {
			for _, c := range gateCfg.EffectiveConditions() {
				conditionTypes = append(conditionTypes, c.Type)
			}
		}
		_ = d.bus.Publish(ctx, events.Event{
			Type: events.EventWorkflowGateReached,
			Payload: map[string]any{
				"ticket_id":  t.ID,
				"project_id": t.ProjectID,
				"phase_id":   phase.ID,
				"phase_name": phase.Name,
				"conditions": conditionTypes,
			},
		})
		slog.Info("dispatch: workflow gate reached, waiting for conditions", "ticket", t.ID, "phase", phase.Name)

	case workflow.PhaseExternal:
		if d.externalExecutor != nil {
			cfg, parseErr := workflow.ParseExternalConfig(phase.Config)
			if parseErr == nil && (cfg.URL != "" || cfg.PollURL != "") {
				switch cfg.Mode {
				case "sync":
					outcome, _, execErr := d.externalExecutor.ExecuteSync(ctx, cfg, t.ID, t.WorkflowID, phase.ID)
					if execErr != nil {
						slog.Error("dispatch: external phase sync failed", "ticket", t.ID, "phase", phase.ID, "error", execErr)
					}
					next := d.advanceWorkflowIfNeeded(ctx, t, outcome)
					_ = d.bus.Publish(ctx, events.Event{
						Type: events.EventWorkflowExternalResult,
						Payload: map[string]any{
							"ticket_id":  t.ID,
							"project_id": t.ProjectID,
							"phase_id":   phase.ID,
							"outcome":    outcome,
						},
					})
					if next == nil {
						d.completeWorkflow(ctx, t)
					}
					return

				case "async":
					if d.workflowPhaseUpdater != nil {
						_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
					}
					_, asyncErr := d.externalExecutor.InitiateAsync(ctx, cfg, t.ID, t.WorkflowID, phase.ID)
					if asyncErr != nil {
						slog.Error("dispatch: external phase async initiate failed", "ticket", t.ID, "phase", phase.ID, "error", asyncErr)
						d.advanceWorkflowIfNeeded(ctx, t, "failed")
						return
					}
					_ = d.bus.Publish(ctx, events.Event{
						Type: events.EventWorkflowExternalFired,
						Payload: map[string]any{
							"ticket_id":  t.ID,
							"project_id": t.ProjectID,
							"phase_id":   phase.ID,
							"mode":       "async",
						},
					})
					return // wait for callback

				case "poll":
					ready, err := d.externalExecutor.PollOnce(ctx, cfg)
					if err == nil && ready {
						if d.advanceWorkflowIfNeeded(ctx, t, "success") == nil {
							d.completeWorkflow(ctx, t)
						}
						return
					}
					if d.workflowPhaseUpdater != nil {
						_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
					}
					return
				default: // unknown
					if d.workflowPhaseUpdater != nil {
						_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
					}
					return
				}
			}
		}
		_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "failed")

	case workflow.PhaseAction:
		cfg, _ := workflow.ParseActionConfig(phase.Config)
		if cfg.Action == "merge_pr" {
			if t.State == ticket.StateAwaitingValidation {
				if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerWorkflowValidate, ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}, nil); err != nil {
					return
				}
				t.State = ticket.StateValidated
			}
			pr, _ := t.Outputs["pr_url"].(string)
			d.autoMergePR(ctx, t, pr)
			fresh, err := d.tickets.GetTicket(ctx, t.ID)
			if err == nil && fresh.WorkflowPhase == phase.ID && fresh.WorkflowPhaseStatus != "failed" {
				_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "ready")
			}
			return
		}

		if d.actionRegistry != nil {
			actionCfg, parseErr := workflow.ParseActionConfig(phase.Config)
			if parseErr == nil && actionCfg.Action != "" {
				ac := workflow.ActionContext{
					TicketID:   t.ID,
					ProjectID:  t.ProjectID,
					PhaseID:    phase.ID,
					WorkflowID: t.WorkflowID,
					Params:     actionCfg.Params,
					Outputs:    t.Outputs,
					Inputs:     t.Inputs,
				}
				result := d.actionRegistry.Execute(ctx, actionCfg.Action, ac)
				if result.Metadata != nil && d.outputPatcher != nil {
					_ = d.outputPatcher.PatchOutputs(ctx, t.ID, result.Metadata)
				}
				_ = d.bus.Publish(ctx, events.Event{
					Type: events.EventWorkflowActionResult,
					Payload: map[string]any{
						"ticket_id":  t.ID,
						"project_id": t.ProjectID,
						"phase_id":   phase.ID,
						"action":     actionCfg.Action,
						"outcome":    result.Outcome,
					},
				})
				outcome := result.Outcome
				if outcome == "" {
					outcome = "success"
				}
				next := d.advanceWorkflowIfNeeded(ctx, t, outcome)
				if next == nil {
					actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
					_ = actor
					d.completeWorkflow(ctx, t)
				}
				return
			}
		}
		_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "failed")

	case workflow.PhaseAgent, workflow.PhaseManual:
		// Phases that need agent dispatch or human action — set blocked and wait.
		if d.workflowPhaseUpdater != nil {
			_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
		}

	default:
		_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "failed")
	}
}

// processReadyWorkflowPhases queries tickets with workflow_phase_status='ready'
// and processes each one. Called from scanPending during reconciliation.
func (d *Dispatcher) processReadyWorkflowPhases(ctx context.Context) {
	if d.workflowPhaseUpdater == nil || d.workflowEngine == nil {
		return
	}
	tickets, err := d.workflowPhaseUpdater.ListByWorkflowPhaseStatus(ctx, d.config().ProjectID, "ready")
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("dispatch: list ready workflow phases failed", "error", err)
		return
	}
	if limit := d.scanLimitValue(); len(tickets) > limit {
		tickets = tickets[:limit]
	}
	for _, t := range tickets {
		if ctx.Err() != nil {
			return
		}
		if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
			continue
		}
		d.processReadyPhase(ctx, t)
	}
}

// recheckBlockedGates re-evaluates gate conditions on blocked tickets.
// This enables polling for http_check and github_checks conditions that may
// become satisfied between reconcile loops.
func (d *Dispatcher) recheckBlockedGates(ctx context.Context) {
	if d.workflowPhaseUpdater == nil || d.workflowEngine == nil || d.checkerRegistry == nil {
		return
	}
	tickets, err := d.workflowPhaseUpdater.ListByWorkflowPhaseStatus(ctx, d.config().ProjectID, "blocked")
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("dispatch: list blocked gates failed", "error", err)
		return
	}
	if limit := d.scanLimitValue(); len(tickets) > limit {
		tickets = tickets[:limit]
	}
	for _, t := range tickets {
		if ctx.Err() != nil {
			return
		}
		if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
			continue
		}
		pos, err := d.workflowEngine.GetPosition(ctx, t.ID, t.WorkflowID, t.WorkflowPhase, t.WorkflowVersion)
		if err != nil || pos == nil || pos.CurrentPhase == nil {
			continue
		}
		phase := pos.CurrentPhase
		if phase.Type == workflow.PhaseExternal {
			cfg, err := workflow.ParseExternalConfig(phase.Config)
			if err != nil || cfg.Mode != "poll" {
				continue
			}
			interval := 30 * time.Second
			if parsed, err := time.ParseDuration(cfg.PollInterval); err == nil && parsed > 0 {
				interval = parsed
			}
			key := "_poll_at_" + phase.ID
			if raw, ok := t.Outputs[key].(string); ok {
				if last, err := time.Parse(time.RFC3339Nano, raw); err == nil && time.Since(last) < interval {
					continue
				}
			}
			if d.outputPatcher != nil {
				_ = d.outputPatcher.PatchOutputs(ctx, t.ID, map[string]any{key: time.Now().Format(time.RFC3339Nano)})
			}
			timeout := 30 * time.Minute
			if parsed, err := time.ParseDuration(cfg.PollTimeout); err == nil && parsed > 0 {
				timeout = parsed
			}
			if t.WorkflowPhaseEnteredAt != nil && time.Since(*t.WorkflowPhaseEnteredAt) > timeout {
				d.advanceWorkflowIfNeeded(ctx, t, "failed")
				continue
			}
			ready, err := d.externalExecutor.PollOnce(ctx, cfg)
			if err == nil && ready {
				if d.advanceWorkflowIfNeeded(ctx, t, "success") == nil {
					d.completeWorkflow(ctx, t)
				}
			}
			continue
		}
		if phase.Type != workflow.PhaseGate {
			continue
		}
		gateCfg, _ := workflow.ParseGateConfig(phase.Config)
		if gateCfg == nil {
			continue
		}
		conditions := gateCfg.EffectiveConditions()
		if len(conditions) == 0 {
			continue
		}
		var reqs []gate.GateRequirement
		for _, c := range conditions {
			reqs = append(reqs, gate.GateRequirement{Type: gate.GateRequirementType(c.Type), Config: c.Config})
		}
		prURL, _ := t.Outputs["pr_url"].(string)
		statuses := d.checkerRegistry.CheckAll(ctx, reqs, gate.CheckContext{
			TicketID: t.ID, ProjectID: t.ProjectID, PRURL: prURL,
			PhaseID: phase.ID, Outputs: t.Outputs, PhaseEnteredAt: phaseEnteredAt(t),
		})
		if gate.AnyFailed(statuses) {
			d.advanceWorkflowIfNeeded(ctx, t, "failed")
			continue
		}
		if len(gate.Unsatisfied(statuses)) == 0 {
			// All conditions now satisfied — reset to ready so processReadyPhase advances.
			_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "ready")
			slog.Info("dispatch: blocked gate now satisfied, advancing", "ticket", t.ID, "phase", phase.Name)
		}
	}
}

// checkPhaseTimeouts checks for phases that have exceeded their configured timeout.
// Phases with a Timeout field that have been running/blocked longer than the timeout
// are advanced with outcome="failed" and metadata={"reason": "phase_timeout"}.
func (d *Dispatcher) checkPhaseTimeouts(ctx context.Context) {
	if d.workflowPhaseUpdater == nil || d.workflowEngine == nil {
		return
	}
	// Check both running and blocked phases for timeouts.
	for _, status := range []string{"running", "blocked"} {
		tickets, err := d.workflowPhaseUpdater.ListByWorkflowPhaseStatus(ctx, d.config().ProjectID, status)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("dispatch: list phases for timeout check failed", "status", status, "error", err)
			continue
		}
		if limit := d.scanLimitValue(); len(tickets) > limit {
			tickets = tickets[:limit]
		}
		for _, t := range tickets {
			if ctx.Err() != nil {
				return
			}
			if t.WorkflowID == "" || t.WorkflowPhase == "" || t.WorkflowPhaseEnteredAt == nil {
				continue
			}
			pos, err := d.workflowEngine.GetPosition(ctx, t.ID, t.WorkflowID, t.WorkflowPhase, t.WorkflowVersion)
			if err != nil || pos == nil || pos.CurrentPhase == nil {
				continue
			}
			if pos.CurrentPhase.Timeout == "" {
				continue
			}
			timeout, err := time.ParseDuration(pos.CurrentPhase.Timeout)
			if err != nil || timeout <= 0 {
				continue
			}
			if time.Since(*t.WorkflowPhaseEnteredAt) > timeout {
				slog.Warn("dispatch: phase timeout exceeded", "ticket", t.ID, "phase", t.WorkflowPhase, "timeout", pos.CurrentPhase.Timeout)
				d.advanceWorkflowIfNeeded(ctx, t, "failed")
			}
		}
	}
}

func phaseEnteredAt(t *ticket.Ticket) string {
	if t.WorkflowPhaseEnteredAt == nil {
		return ""
	}
	return t.WorkflowPhaseEnteredAt.Format(time.RFC3339Nano)
}

func (d *Dispatcher) claimPhase(ctx context.Context, t *ticket.Ticket) (bool, error) {
	if st, ok := d.workflowPhaseUpdater.(interface {
		CASWorkflowAttempt(context.Context, *ticket.Ticket, string, string) (bool, error)
	}); ok {
		return st.CASWorkflowAttempt(ctx, t, "ready", "running")
	}
	return d.workflowPhaseUpdater.CASWorkflowPhaseStatus(ctx, t.ID, "ready", "running")
}
