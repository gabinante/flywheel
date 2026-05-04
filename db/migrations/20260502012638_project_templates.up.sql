-- Standalone workstream templates (the composable library)
CREATE TABLE workstream_templates (
    id TEXT PRIMARY KEY,
    org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE,  -- NULL = system-level
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    plan TEXT NOT NULL DEFAULT '',
    tickets JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_wst_slug ON workstream_templates (COALESCE(org_id, ''), slug);

-- Named project templates (combinations of workstream templates)
CREATE TABLE project_templates (
    id TEXT PRIMARY KEY,
    org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE,  -- NULL = system-level
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    workstream_template_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_pt_slug ON project_templates (COALESCE(org_id, ''), slug);

-- Seed system-level workstream templates
INSERT INTO workstream_templates (id, org_id, name, slug, description, plan, tickets) VALUES
('wst-production-readiness', NULL, 'Production Readiness', 'production-readiness',
 'Health checks, structured logging, graceful shutdown, metrics, and runbook',
 '## Production Readiness\n\nEnsure the service meets production-grade standards before launch.\n\n- Health check endpoints\n- Structured logging with correlation IDs\n- Graceful shutdown handling\n- Prometheus metrics\n- Operational runbook',
 '[
   {"title": "Add health check endpoints", "type": "task", "priority": 1, "description": "Implement /healthz and /readyz endpoints with dependency checks", "success_criteria": ["GET /healthz returns 200 with JSON body", "GET /readyz checks DB and Redis connectivity", "Failing dependency returns 503"]},
   {"title": "Implement structured logging", "type": "task", "priority": 1, "description": "Replace fmt.Print calls with structured slog logging including correlation IDs", "success_criteria": ["All log lines use slog with JSON output", "Request correlation ID propagated through context", "Log levels used appropriately (info, warn, error)"]},
   {"title": "Add graceful shutdown handling", "type": "task", "priority": 2, "description": "Handle SIGTERM/SIGINT with connection draining and cleanup", "success_criteria": ["Server stops accepting new connections on signal", "In-flight requests complete within timeout", "Background workers stopped cleanly"]},
   {"title": "Add Prometheus metrics", "type": "task", "priority": 2, "description": "Instrument key paths with Prometheus counters, histograms, and gauges", "success_criteria": ["Request duration histogram by route", "Error rate counter by status code", "Active connections gauge", "/metrics endpoint exposed"]},
   {"title": "Write operational runbook", "type": "task", "priority": 3, "description": "Document common operational procedures: deploy, rollback, debug, scale", "success_criteria": ["Runbook covers deploy and rollback procedures", "Troubleshooting section with common failure modes", "On-call escalation path documented"]}
 ]'),

('wst-backups-and-resilience', NULL, 'Backups & Resilience', 'backups-and-resilience',
 'Automated DB backups, restore testing, circuit breakers, and disaster recovery plan',
 '## Backups & Resilience\n\nProtect against data loss and service outages.\n\n- Automated database backups\n- Restore testing procedures\n- Circuit breakers for external dependencies\n- Disaster recovery plan',
 '[
   {"title": "Set up automated database backups", "type": "task", "priority": 1, "description": "Configure pg_dump or WAL archiving on a schedule with retention policy", "success_criteria": ["Backups run on schedule (daily minimum)", "Backup retention policy enforced (30 days)", "Backup success/failure alerts configured"]},
   {"title": "Test backup restore procedure", "type": "task", "priority": 1, "description": "Verify backups can be restored and document the procedure", "success_criteria": ["Restore tested on a staging environment", "Restore procedure documented step-by-step", "RTO and RPO measured and documented"]},
   {"title": "Implement circuit breakers", "type": "task", "priority": 2, "description": "Add circuit breakers for external service calls (APIs, databases, caches)", "success_criteria": ["Circuit breaker wraps all external HTTP calls", "Open/half-open/closed states with configurable thresholds", "Fallback behavior defined for each dependency"]},
   {"title": "Write disaster recovery plan", "type": "spike", "priority": 2, "description": "Document DR scenarios, recovery procedures, and communication plan", "success_criteria": ["DR plan covers database failure, region outage, and data corruption", "Recovery time objectives defined per scenario", "Communication plan with stakeholder contacts"]}
 ]'),

('wst-marketing-landing-page', NULL, 'Marketing Landing Page', 'marketing-landing-page',
 'Landing page design, copy, analytics integration, and SEO optimization',
 '## Marketing Landing Page\n\nCreate a compelling landing page that converts visitors.\n\n- Design and layout\n- Copywriting\n- Analytics integration\n- SEO optimization',
 '[
   {"title": "Design landing page layout", "type": "task", "priority": 1, "description": "Create responsive landing page with hero, features, social proof, and CTA sections", "success_criteria": ["Mobile-first responsive design", "Hero section with clear value proposition", "Feature grid with icons and descriptions", "CTA button above the fold"]},
   {"title": "Write landing page copy", "type": "task", "priority": 1, "description": "Write compelling headlines, feature descriptions, and calls to action", "success_criteria": ["Headline communicates core value in under 10 words", "Feature descriptions focus on benefits not features", "CTA copy is action-oriented"]},
   {"title": "Integrate analytics tracking", "type": "task", "priority": 2, "description": "Add page view, scroll depth, and CTA click tracking", "success_criteria": ["Page views tracked with referrer attribution", "CTA click events captured", "Scroll depth milestones tracked (25%, 50%, 75%, 100%)"]},
   {"title": "SEO optimization", "type": "task", "priority": 2, "description": "Optimize meta tags, structured data, and page speed", "success_criteria": ["Title and meta description optimized for target keywords", "Open Graph and Twitter card meta tags set", "Lighthouse performance score above 90"]}
 ]'),

('wst-authentik-integration', NULL, 'Authentik Integration', 'authentik-integration',
 'OIDC provider config, SSO login flow, group provisioning, and session management',
 '## Authentik Integration\n\nIntegrate with Authentik for SSO and identity management.\n\n- OIDC provider configuration\n- SSO login flow\n- Group/role provisioning\n- Session management',
 '[
   {"title": "Configure OIDC provider in Authentik", "type": "task", "priority": 1, "description": "Set up OAuth2/OIDC application in Authentik with correct redirect URIs and scopes", "success_criteria": ["OIDC application created in Authentik", "Redirect URIs configured for all environments", "Required scopes (openid, profile, email) enabled"]},
   {"title": "Implement SSO login flow", "type": "task", "priority": 1, "description": "Add OIDC authorization code flow with PKCE for browser login", "success_criteria": ["Login redirects to Authentik authorization endpoint", "Callback handles code exchange and token validation", "User session created after successful authentication"]},
   {"title": "Set up group provisioning", "type": "task", "priority": 2, "description": "Map Authentik groups to application roles for RBAC", "success_criteria": ["Authentik groups synced to local roles on login", "Admin group maps to admin role", "New users get default role from group membership"]},
   {"title": "Implement session management", "type": "task", "priority": 2, "description": "Handle token refresh, session expiry, and logout with Authentik", "success_criteria": ["Access tokens refreshed before expiry", "Session timeout enforced with configurable duration", "Logout clears local session and Authentik session"]}
 ]'),

('wst-ci-cd-pipeline', NULL, 'CI/CD Pipeline', 'ci-cd-pipeline',
 'CI pipeline setup, automated tests, build caching, and deploy automation',
 '## CI/CD Pipeline\n\nAutomate build, test, and deployment workflows.\n\n- CI pipeline configuration\n- Automated test execution\n- Build caching\n- Deploy automation',
 '[
   {"title": "Set up CI pipeline", "type": "task", "priority": 1, "description": "Configure GitHub Actions (or equivalent) for build and test on push/PR", "success_criteria": ["Pipeline triggers on push to main and PR creation", "Build step compiles/bundles the application", "Pipeline status reported on PRs"]},
   {"title": "Add automated test execution", "type": "task", "priority": 1, "description": "Run unit and integration tests in CI with coverage reporting", "success_criteria": ["Unit tests run in CI on every push", "Integration tests run against test database", "Coverage report generated and posted to PR"]},
   {"title": "Configure build caching", "type": "task", "priority": 2, "description": "Cache dependencies and build artifacts to speed up CI runs", "success_criteria": ["Dependency cache restored between runs", "Build artifact cache reduces build time by 50%+", "Cache invalidation works correctly on dependency changes"]},
   {"title": "Automate deployments", "type": "task", "priority": 2, "description": "Set up automated deployment to staging on merge to main, production on tag", "success_criteria": ["Merge to main triggers staging deploy", "Git tag triggers production deploy", "Deploy includes health check verification"]}
 ]'),

('wst-security-hardening', NULL, 'Security Hardening', 'security-hardening',
 'Dependency scanning, secret rotation, RBAC audit, and TLS configuration',
 '## Security Hardening\n\nStrengthen the security posture of the application.\n\n- Dependency vulnerability scanning\n- Secret rotation procedures\n- RBAC audit\n- TLS configuration',
 '[
   {"title": "Set up dependency scanning", "type": "task", "priority": 1, "description": "Add automated vulnerability scanning for dependencies in CI", "success_criteria": ["Dependabot or equivalent configured", "CI fails on critical/high vulnerabilities", "Weekly scan report generated"]},
   {"title": "Implement secret rotation", "type": "task", "priority": 1, "description": "Document and automate rotation for API keys, database credentials, and tokens", "success_criteria": ["Secret inventory documented", "Rotation procedure for each secret type", "Zero-downtime rotation verified"]},
   {"title": "Audit RBAC configuration", "type": "spike", "priority": 2, "description": "Review and document role-based access control for all endpoints", "success_criteria": ["All endpoints mapped to required roles", "Principle of least privilege verified", "Audit findings documented with remediation plan"]},
   {"title": "Configure TLS and security headers", "type": "task", "priority": 2, "description": "Ensure TLS 1.2+ and set security headers (HSTS, CSP, X-Frame-Options)", "success_criteria": ["TLS 1.2 minimum enforced", "HSTS header with 1-year max-age", "Content-Security-Policy configured", "Security headers score A+ on securityheaders.com"]}
 ]'),

('wst-api-documentation', NULL, 'API Documentation', 'api-documentation',
 'OpenAPI spec, endpoint documentation, examples, and SDK generation',
 '## API Documentation\n\nCreate comprehensive API documentation for consumers.\n\n- OpenAPI specification\n- Endpoint documentation\n- Request/response examples\n- SDK generation',
 '[
   {"title": "Write OpenAPI specification", "type": "task", "priority": 1, "description": "Create or update OpenAPI 3.x spec covering all public endpoints", "success_criteria": ["All public endpoints documented in OpenAPI spec", "Request/response schemas defined with examples", "Spec validates with no errors"]},
   {"title": "Add endpoint documentation", "type": "task", "priority": 1, "description": "Write descriptions, parameter docs, and error responses for each endpoint", "success_criteria": ["Each endpoint has a summary and description", "All parameters documented with types and constraints", "Error responses documented with status codes and bodies"]},
   {"title": "Create usage examples", "type": "task", "priority": 2, "description": "Add curl and SDK examples for common API workflows", "success_criteria": ["Authentication flow example", "CRUD operations example for primary resource", "Error handling example with retry logic"]},
   {"title": "Set up SDK generation", "type": "spike", "priority": 3, "description": "Evaluate and configure automatic SDK generation from OpenAPI spec", "success_criteria": ["TypeScript client generated from spec", "Generated client tested against live API", "Generation integrated into CI pipeline"]}
 ]'),

('wst-monitoring-and-alerting', NULL, 'Monitoring & Alerting', 'monitoring-and-alerting',
 'Dashboards, alert rules, on-call rotation, and incident response procedures',
 '## Monitoring & Alerting\n\nSet up observability and incident response.\n\n- Monitoring dashboards\n- Alert rules\n- On-call rotation\n- Incident response procedures',
 '[
   {"title": "Create monitoring dashboards", "type": "task", "priority": 1, "description": "Build Grafana dashboards for key service metrics: latency, throughput, errors, saturation", "success_criteria": ["Overview dashboard with RED metrics", "Database performance dashboard", "Infrastructure resource utilization dashboard"]},
   {"title": "Configure alert rules", "type": "task", "priority": 1, "description": "Set up alerts for SLO breaches, error spikes, and resource exhaustion", "success_criteria": ["P1 alert for error rate above 1% for 5 minutes", "P2 alert for p99 latency above 2 seconds", "P3 alert for disk usage above 80%", "Alerts route to appropriate channel (PagerDuty, Slack)"]},
   {"title": "Set up on-call rotation", "type": "task", "priority": 2, "description": "Configure on-call schedule with escalation policies", "success_criteria": ["Weekly rotation schedule configured", "Escalation policy with 15-minute timeout", "Backup on-call defined"]},
   {"title": "Write incident response playbook", "type": "task", "priority": 2, "description": "Document incident classification, response procedures, and post-mortem template", "success_criteria": ["Incident severity levels defined (SEV1-SEV4)", "Response procedures for each severity level", "Post-mortem template with timeline and action items"]}
 ]');

-- Seed system-level project templates
INSERT INTO project_templates (id, org_id, name, slug, description, workstream_template_ids) VALUES
('pt-saas-application', NULL, 'SaaS Application', 'saas-application',
 'Full-stack SaaS application with all production infrastructure workstreams',
 ARRAY['wst-production-readiness', 'wst-backups-and-resilience', 'wst-marketing-landing-page', 'wst-authentik-integration', 'wst-ci-cd-pipeline', 'wst-security-hardening', 'wst-api-documentation', 'wst-monitoring-and-alerting']),

('pt-internal-tool', NULL, 'Internal Tool', 'internal-tool',
 'Internal tool with core infrastructure workstreams (no marketing or public API docs)',
 ARRAY['wst-production-readiness', 'wst-backups-and-resilience', 'wst-ci-cd-pipeline', 'wst-monitoring-and-alerting']),

('pt-api-service', NULL, 'API Service', 'api-service',
 'Backend API service with security, documentation, and infrastructure workstreams',
 ARRAY['wst-production-readiness', 'wst-backups-and-resilience', 'wst-ci-cd-pipeline', 'wst-api-documentation', 'wst-security-hardening', 'wst-monitoring-and-alerting']);
