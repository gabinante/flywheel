CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tickets_project_workflow_phase_status
  ON tickets (project_id, workflow_phase_status)
  WHERE workflow_phase_status != '';
