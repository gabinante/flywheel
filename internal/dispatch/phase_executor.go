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

	// CAS guard: only process if status is 'ready', atomically set to 'running'.
	if d.workflowPhaseUpdater != nil {
		ok, err := d.workflowPhaseUpdater.CASWorkflowPhaseStatus(ctx, t.ID, "ready", "running")
		if err != nil {
			slog.Error("dispatch: CAS workflow phase status failed", "ticket", t.ID, "error", err)
			return
		}
		if !ok {
			return // another processor got it
		}
	}

	pos, err := d.workflowEngine.GetPosition(ctx, t.ID, t.WorkflowID, t.WorkflowPhase, t.WorkflowVersion)
	if err != nil || pos == nil || pos.CurrentPhase == nil {
		slog.Error("dispatch: get workflow position failed", "ticket", t.ID, "error", err)
		return
	}

	phase := pos.CurrentPhase

	switch phase.Type {
	case workflow.PhaseGate:
		// Check gate conditions. Met → advance. Not met → set blocked, emit event.
		if d.checkerRegistry != nil {
			gateCfg, _ := workflow.ParseGateConfig(phase.Config)
			if gateCfg != nil {
				conditions := gateCfg.EffectiveConditions()
				if len(conditions) > 0 {
					var reqs []gate.GateRequirement
					for _, c := range conditions {
						reqs = append(reqs, gate.GateRequirement{Type: gate.GateRequirementType(c.Type), Config: c.Config})
					}
					prURL, _ := t.Outputs["pr_url"].(string)
					statuses := d.checkerRegistry.CheckAll(ctx, reqs, gate.CheckContext{
						TicketID: t.ID, ProjectID: t.ProjectID, PRURL: prURL,
						PhaseID: phase.ID, Outputs: t.Outputs,
					})
					if len(gate.Unsatisfied(statuses)) == 0 {
						// All conditions met — advance.
						next := d.advanceWorkflowIfNeeded(ctx, t, "success")
						if next == nil {
							actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
							_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil)
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
			if parseErr == nil && cfg.URL != "" {
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
						actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
						_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil)
					}
					return

				case "async":
					if d.workflowPhaseUpdater != nil {
						_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
					}
					_, asyncErr := d.externalExecutor.InitiateAsync(ctx, cfg, t.ID, t.WorkflowID, phase.ID)
					if asyncErr != nil {
						slog.Error("dispatch: external phase async initiate failed", "ticket", t.ID, "phase", phase.ID, "error", asyncErr)
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

				default: // poll or unknown
					if d.workflowPhaseUpdater != nil {
						_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
					}
					return
				}
			}
		}
		// No executor or no URL — auto-advance (backward compat).
		next := d.advanceWorkflowIfNeeded(ctx, t, "success")
		if next == nil {
			actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
			_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil)
		}

	case workflow.PhaseAction:
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
					_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil)
				}
				return
			}
		}
		// No registry or no action — auto-advance (backward compat).
		next := d.advanceWorkflowIfNeeded(ctx, t, "success")
		if next == nil {
			actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
			_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil)
		}

	case workflow.PhaseAgent, workflow.PhaseManual:
		// Phases that need agent dispatch or human action — set blocked and wait.
		if d.workflowPhaseUpdater != nil {
			_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "blocked")
		}

	default:
		// Legacy types (deploy, observe, automated) — auto-advance.
		next := d.advanceWorkflowIfNeeded(ctx, t, "success")
		if next == nil {
			actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
			_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil)
		}
	}
}

// processReadyWorkflowPhases queries tickets with workflow_phase_status='ready'
// and processes each one. Called from scanPending during reconciliation.
func (d *Dispatcher) processReadyWorkflowPhases(ctx context.Context) {
	if d.workflowPhaseUpdater == nil || d.workflowEngine == nil {
		return
	}
	tickets, err := d.workflowPhaseUpdater.ListByWorkflowPhaseStatus(ctx, d.cfg.ProjectID, "ready")
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
	tickets, err := d.workflowPhaseUpdater.ListByWorkflowPhaseStatus(ctx, d.cfg.ProjectID, "blocked")
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
			PhaseID: phase.ID, Outputs: t.Outputs,
		})
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
		tickets, err := d.workflowPhaseUpdater.ListByWorkflowPhaseStatus(ctx, d.cfg.ProjectID, status)
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
