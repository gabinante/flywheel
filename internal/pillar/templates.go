package pillar

// Template holds the default template for a pillar type: suggested strategy
// prompts, common claim patterns, and typical gaps to consider.
type Template struct {
	PillarType       PillarType `json:"pillar_type"`
	Description      string     `json:"description"`
	StrategyPrompt   string     `json:"strategy_prompt"`
	ExampleClaims    []string   `json:"example_claims"`
	CommonGaps       []string   `json:"common_gaps"`
	DefaultCadence   string     `json:"default_cadence"`
}

// DefaultTemplates returns the seven default pillar templates.
func DefaultTemplates() []Template {
	return []Template{
		{
			PillarType:     PillarObservability,
			Description:    "The ability to understand system state through external outputs: logs, metrics, traces, and alerts.",
			StrategyPrompt: "How do we ensure this entity's internal state is observable in production? What signals do operators need?",
			ExampleClaims: []string{
				"Structured logging with correlation IDs is enabled for all request paths",
				"Latency p50/p95/p99 metrics are exported to the metrics backend",
				"Distributed traces span all downstream service calls",
				"Alerting covers error rate and latency SLO breaches",
			},
			CommonGaps: []string{
				"No alerting on degraded-but-not-down states",
				"Logs lack structured context for debugging",
				"No distributed tracing across async boundaries",
				"Dashboard coverage incomplete for new features",
			},
			DefaultCadence: "monthly",
		},
		{
			PillarType:     PillarMutability,
			Description:    "The ability to change the system safely: deployability, rollback capability, feature flags, and schema migration safety.",
			StrategyPrompt: "How do we ensure changes to this entity can be made safely and rolled back quickly?",
			ExampleClaims: []string{
				"Zero-downtime deployments via rolling update strategy",
				"Database migrations are backward-compatible and reversible",
				"Feature flags gate all new user-facing behavior",
				"Rollback to previous version completes in under 5 minutes",
			},
			CommonGaps: []string{
				"No automated rollback on health check failure",
				"Schema migrations are not reversible",
				"Feature flag cleanup process is undefined",
				"Deploy pipeline lacks canary stage",
			},
			DefaultCadence: "monthly",
		},
		{
			PillarType:     PillarScalability,
			Description:    "The ability to handle growth: horizontal scaling, resource limits, bottleneck identification, and capacity planning.",
			StrategyPrompt: "What are the scaling dimensions and limits for this entity? How do we plan for growth?",
			ExampleClaims: []string{
				"Horizontally scalable: adding replicas increases throughput linearly",
				"Resource limits (CPU, memory) are set and tested under load",
				"Connection pool sizing matches expected concurrency",
				"Capacity planning review occurs quarterly with growth projections",
			},
			CommonGaps: []string{
				"No load testing in pre-production",
				"Database connection pool not tuned for peak",
				"No autoscaling policy defined",
				"Capacity planning is reactive, not proactive",
			},
			DefaultCadence: "quarterly",
		},
		{
			PillarType:     PillarAvailability,
			Description:    "The ability to remain operational: uptime targets, redundancy, failover, health checks, and dependency resilience.",
			StrategyPrompt: "What is the availability target for this entity? How do we achieve and measure it?",
			ExampleClaims: []string{
				"99.9% uptime SLO defined and measured via synthetic probes",
				"Health check endpoint covers all critical dependencies",
				"Multi-AZ deployment provides zone-level redundancy",
				"Graceful degradation when non-critical dependencies fail",
			},
			CommonGaps: []string{
				"No SLO defined or measured",
				"Single point of failure in data layer",
				"Health check is shallow (returns 200 without checking deps)",
				"No chaos testing validates failover paths",
			},
			DefaultCadence: "monthly",
		},
		{
			PillarType:     PillarSecurity,
			Description:    "Protection of data and systems: authentication, authorization, encryption, vulnerability management, and audit trails.",
			StrategyPrompt: "How do we protect this entity's data and access? What is the threat model?",
			ExampleClaims: []string{
				"All API endpoints require authentication via JWT or API key",
				"Data at rest is encrypted (AES-256 or equivalent)",
				"Data in transit is encrypted (TLS 1.2+)",
				"Dependency vulnerabilities scanned weekly with automated alerts",
			},
			CommonGaps: []string{
				"No formal threat model documented",
				"Secrets rotation is manual",
				"Audit logging does not capture all state mutations",
				"No penetration testing schedule",
			},
			DefaultCadence: "monthly",
		},
		{
			PillarType:     PillarResiliency,
			Description:    "The ability to recover from failure: circuit breakers, retries, timeouts, bulkheads, and disaster recovery.",
			StrategyPrompt: "How does this entity behave under failure? What recovery mechanisms are in place?",
			ExampleClaims: []string{
				"Circuit breaker on all external service calls with configurable thresholds",
				"Retry with exponential backoff for transient failures",
				"Request timeouts set on all outbound calls",
				"Disaster recovery runbook tested quarterly",
			},
			CommonGaps: []string{
				"No circuit breaker on database calls",
				"Retry storms possible under sustained failure",
				"No bulkhead isolation between workloads",
				"DR runbook untested or outdated",
			},
			DefaultCadence: "quarterly",
		},
		{
			PillarType:     PillarCost,
			Description:    "Understanding and managing operational cost: resource efficiency, cost attribution, waste identification, and budget alignment.",
			StrategyPrompt: "How do we understand and manage the cost of running this entity? Where is waste likely?",
			ExampleClaims: []string{
				"Cloud resource costs are tagged and attributed to this service",
				"Idle resources are identified and right-sized monthly",
				"Cost anomaly alerts fire on 20%+ daily spend increase",
				"Reserved capacity covers baseline load; spot/preemptible for burst",
			},
			CommonGaps: []string{
				"No cost tagging on infrastructure resources",
				"Over-provisioned resources not reviewed",
				"No cost anomaly detection",
				"Cost allocation model does not cover shared infrastructure",
			},
			DefaultCadence: "quarterly",
		},
	}
}

// TemplateByType returns the default template for a given pillar type, or nil.
func TemplateByType(pt PillarType) *Template {
	for _, t := range DefaultTemplates() {
		if t.PillarType == pt {
			return &t
		}
	}
	return nil
}
