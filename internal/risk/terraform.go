package risk

import "strings"

// TerraformClassifier parses Terraform JSON plan output and classifies operations
// based on resource type and change action.
//
// Classification rules:
//   - destroy of stateful resources (databases, storage, clusters) → destructive
//   - replace (delete+create) of stateful resources → destructive
//   - destroy of stateless resources (IAM policies, security groups) → high
//   - replace of stateless resources → high
//   - update in-place → reversible
//   - create (additive) → low
//   - read/data sources → safe
//
// The classifier inspects:
//   - Operation.Action: "create", "update", "destroy", "replace", "read"
//   - Operation.Target: Terraform resource address (e.g. "aws_rds_instance.main")
//   - Operation.Details["resource_type"]: explicit resource type if provided
type TerraformClassifier struct{}

// statefulResourcePrefixes are Terraform resource types that hold persistent state.
// Destroying or replacing these is destructive.
var statefulResourcePrefixes = []string{
	"aws_rds",
	"aws_db_instance",
	"aws_db_cluster",
	"aws_dynamodb_table",
	"aws_s3_bucket",
	"aws_elasticache",
	"aws_elasticsearch",
	"aws_opensearch",
	"aws_efs_file_system",
	"aws_ebs_volume",
	"aws_kinesis_stream",
	"aws_sqs_queue",
	"aws_sns_topic",
	"aws_redshift",
	"google_sql_database",
	"google_storage_bucket",
	"google_redis_instance",
	"google_pubsub_topic",
	"google_bigtable",
	"google_spanner",
	"azurerm_sql",
	"azurerm_cosmosdb",
	"azurerm_storage",
	"azurerm_redis",
	"azurerm_servicebus",
	"kubernetes_persistent_volume",
	"kubernetes_stateful_set",
	"helm_release",
}

// Classify classifies a Terraform plan operation.
func (c *TerraformClassifier) Classify(op Operation) (RiskLevel, []RuleCitation) {
	action := strings.ToLower(strings.TrimSpace(op.Action))
	resourceType := extractResourceType(op)
	stateful := isStatefulResource(resourceType)

	switch action {
	case "destroy", "delete":
		if stateful {
			return RiskDestructive, []RuleCitation{{
				Rule:        "tf.destroy_stateful",
				Backend:     BackendTerraform,
				Level:       RiskDestructive,
				Description: "Terraform destroys stateful resource '" + resourceType + "' — potential data loss",
			}}
		}
		return RiskHigh, []RuleCitation{{
			Rule:        "tf.destroy_stateless",
			Backend:     BackendTerraform,
			Level:       RiskHigh,
			Description: "Terraform destroys resource '" + resourceType + "' — may disrupt service",
		}}

	case "replace", "delete-create", "create-delete":
		if stateful {
			return RiskDestructive, []RuleCitation{{
				Rule:        "tf.replace_stateful",
				Backend:     BackendTerraform,
				Level:       RiskDestructive,
				Description: "Terraform replaces stateful resource '" + resourceType + "' — destroys then recreates, data loss likely",
			}}
		}
		return RiskHigh, []RuleCitation{{
			Rule:        "tf.replace_stateless",
			Backend:     BackendTerraform,
			Level:       RiskHigh,
			Description: "Terraform replaces resource '" + resourceType + "' — brief outage possible",
		}}

	case "update", "modify":
		return RiskReversible, []RuleCitation{{
			Rule:        "tf.update_in_place",
			Backend:     BackendTerraform,
			Level:       RiskReversible,
			Description: "Terraform updates resource '" + resourceType + "' in place — reversible via plan revert",
		}}

	case "create", "add":
		return RiskLow, []RuleCitation{{
			Rule:        "tf.create",
			Backend:     BackendTerraform,
			Level:       RiskLow,
			Description: "Terraform creates new resource '" + resourceType + "' — additive, no existing resources affected",
		}}

	case "read", "data", "no-op", "noop":
		return RiskSafe, []RuleCitation{{
			Rule:        "tf.read",
			Backend:     BackendTerraform,
			Level:       RiskSafe,
			Description: "Terraform reads data source '" + resourceType + "' — no modifications",
		}}

	default:
		// Unknown Terraform action → maximum risk.
		return RiskDestructive, []RuleCitation{{
			Rule:        "tf.unknown_action",
			Backend:     BackendTerraform,
			Level:       RiskDestructive,
			Description: "Unknown Terraform action '" + action + "' on '" + resourceType + "' — defaults to maximum risk",
		}}
	}
}

// extractResourceType gets the Terraform resource type from Details or parses it from Target.
func extractResourceType(op Operation) string {
	// Prefer explicit resource_type in details.
	if rt, ok := op.Details["resource_type"].(string); ok && rt != "" {
		return rt
	}
	// Parse from target address: "aws_rds_instance.main" → "aws_rds_instance"
	target := op.Target
	if idx := strings.Index(target, "."); idx > 0 {
		return target[:idx]
	}
	return target
}

// isStatefulResource checks if a resource type prefix matches known stateful resources.
func isStatefulResource(resourceType string) bool {
	rt := strings.ToLower(resourceType)
	for _, prefix := range statefulResourcePrefixes {
		if strings.HasPrefix(rt, prefix) {
			return true
		}
	}
	return false
}
