-- Ephemeral UI invalidations. Payloads contain only a topic; NOTIFY is delivered
-- after commit (and suppressed on rollback), including writes by background jobs.
CREATE FUNCTION notify_flywheel_activity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  -- Ignore timestamp-only upserts from reconciliation/ingestion loops.
  IF TG_OP = 'UPDATE' AND (to_jsonb(OLD) - 'updated_at') = (to_jsonb(NEW) - 'updated_at') THEN
    RETURN NULL;
  END IF;
  PERFORM pg_notify('flywheel_activity', TG_ARGV[0]);
  RETURN NULL;
END;
$$;

CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON code_review_requests
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('reviews');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON code_review_findings
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('reviews');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON code_review_messages
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('reviews');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON pr_feedback_rounds
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('reviews');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON overview_cache
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('prs');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON agent_sessions
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('sessions');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON session_links
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('sessions');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON session_prompts
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('sessions');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON tickets
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('tickets');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON execution_steps
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('tickets');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON escalations
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('tickets');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON reviews
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('tickets');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON work_streams
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('tickets');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON ticket_external_refs
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('tickets');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON orgs
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('projects');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON projects
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('projects');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON project_repositories
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('projects');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON project_linear_links
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('projects');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON project_reports
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('projects');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON workflow_definitions
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('workflows');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON workflow_definition_versions
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('workflows');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON workflow_phase_completions
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('workflows');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON operator_settings
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('settings');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON orchestrator_runs
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('orchestrator');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON orchestrator_run_events
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('orchestrator');
CREATE TRIGGER flywheel_activity AFTER INSERT OR UPDATE OR DELETE ON orchestrator_messages
  FOR EACH ROW EXECUTE FUNCTION notify_flywheel_activity('orchestrator');
