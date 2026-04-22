// Package hooks provides a lightweight client library for publishing change
// events to the Flywheel event bus. It implements spec v0.2 §2.4: the hook
// library for change event publication.
//
// Design principle: publishing a change must be easier than not publishing it.
//
// # Quick Start — 3 Lines
//
// The simplest integration is three lines of Go:
//
//	client := hooks.NewClient(bus)
//	err := client.Publish(ctx, hooks.Deploy("my-service", "v2.1.0"))
//	// handle err
//
// # With Context — Fluent Builder
//
// Add context using the fluent builder API:
//
//	err := client.Publish(ctx,
//	    hooks.Deploy("api-gateway", "v2.1.0").
//	        By("ci-pipeline").
//	        In("production").
//	        For("proj-abc").
//	        WithBefore("v2.0.3").
//	        Affecting("cache-layer", "cdn").
//	        WithMeta("pr", "#142"))
//
// # Client Defaults
//
// Set defaults once to avoid repeating common fields:
//
//	client := hooks.NewClient(bus,
//	    hooks.WithEnvironment("production"),
//	    hooks.WithInitiator("deploy-pipeline"),
//	    hooks.WithProjectID("proj-abc"),
//	)
//	// Now every Publish call inherits these defaults.
//	client.Publish(ctx, hooks.Deploy("api", "v2"))
//
// # Package-Level Convenience
//
// For scripts and cron jobs, use the package-level singleton:
//
//	hooks.Init(bus, hooks.WithEnvironment("production"))
//	hooks.Publish(ctx, hooks.Deploy("api", "v2"))
//
// # Common Change Types
//
// Built-in helpers for common scenarios:
//
//	hooks.Deploy("service", "version")           // Service deployment
//	hooks.ConfigUpdate("entity", "key")          // Configuration change
//	hooks.Failover("service")                    // Failover event
//	hooks.Migration("database", "migration-id")  // Database migration
//	hooks.Scale("entity")                        // Scale up/down
//	hooks.Rollback("service", "target-version")  // Rollback
//	hooks.Custom("type", "entity")               // Custom change type
//
// # Webhook Receiver
//
// Accept change events from external systems via HTTP:
//
//	mux.Handle("POST /api/v1/hooks/webhook/{source}",
//	    hooks.WebhookHandler(client))
//
// External systems POST JSON to /api/v1/hooks/webhook/github?project_id=proj-1.
// The payload can be a single Change, array of Changes, or arbitrary JSON
// (which gets wrapped as a "custom" change). Register custom transformers
// for platform-specific payloads:
//
//	hooks.WebhookHandler(client, hooks.WebhookConfig{
//	    Transformers: map[string]hooks.WebhookTransformer{
//	        "argocd": myArgoCDTransformer,
//	    },
//	})
//
// # Gap Detection
//
// The GapDetector watches for state changes that no change event explains.
// When it finds one, it auto-generates an unattributed change event:
//
//	detector := hooks.NewGapDetector(bus, client,
//	    hooks.WithGapWindow(5 * time.Minute),
//	)
//	go detector.Start(ctx)
//
// This ensures the change stream is always complete. Even when callers
// forget to publish a change event, the gap is captured and flagged.
//
// # Integration Examples
//
// Cron job:
//
//	func main() {
//	    bus := events.NewInProcessBus()
//	    client := hooks.NewClient(bus, hooks.WithInitiator("backup-cron"))
//	    // ... do backup work ...
//	    client.Publish(ctx, hooks.Custom("backup_completed", "prod-db").
//	        WithMeta("size_gb", 42.5))
//	}
//
// CI/CD pipeline:
//
//	client := hooks.NewClient(bus, hooks.WithInitiator("github-actions"))
//	client.Publish(ctx, hooks.Deploy(serviceName, gitSHA).
//	    In(targetEnv).
//	    WithMeta("workflow", os.Getenv("GITHUB_WORKFLOW")))
//
// Webhook from external platform (curl):
//
//	curl -X POST 'http://flywheel:8080/api/v1/hooks/webhook/argocd?project_id=proj-1' \
//	    -H 'Content-Type: application/json' \
//	    -d '{"change_type":"deploy","entity_id":"my-app","after":"v3.0"}'
package hooks
