package claims

import (
	"github.com/gabinante/flywheel/internal/plan"
)

// ExtractTouches converts a plan's content into claims Touch objects.
// Each backend type maps its declared resources to the appropriate claim types.
// The environment parameter is used if the plan doesn't declare one explicitly.
func ExtractTouches(p *plan.Plan, environment string) []Touch {
	if p == nil {
		return nil
	}
	var touches []Touch

	switch p.Backend {
	case plan.BackendCode:
		touches = extractCodeTouches(p.Content, environment)
	case plan.BackendDatabase:
		touches = extractDatabaseTouches(p.Content, environment)
	case plan.BackendTerraform:
		touches = extractTerraformTouches(p.Content, environment)
	case plan.BackendShell:
		touches = extractShellTouches(p.Content, environment)
	case plan.BackendDeploy:
		touches = extractDeployTouches(p.Content, environment)
	}

	return touches
}

// extractCodeTouches generates file_write touches from code plan hunks.
func extractCodeTouches(c plan.Content, env string) []Touch {
	if c.Code == nil {
		return nil
	}
	return []Touch{
		{
			EntityID:    c.Code.FilePath,
			Environment: env,
			ClaimType:   ClaimFileWrite,
			Metadata: map[string]any{
				"language":    c.Code.Language,
				"before_hash": c.Code.BeforeHash,
				"after_hash":  c.Code.AfterHash,
			},
		},
	}
}

// extractDatabaseTouches generates schema touches from database plan DDL.
func extractDatabaseTouches(c plan.Content, env string) []Touch {
	if c.Database == nil {
		return nil
	}
	entityID := c.Database.MigrationName
	if c.Database.DatabaseName != "" {
		entityID = c.Database.DatabaseName + ":" + c.Database.MigrationName
	}
	return []Touch{
		{
			EntityID:    entityID,
			Environment: env,
			ClaimType:   ClaimSchema,
			Metadata: map[string]any{
				"direction":      c.Database.Direction,
				"schema_version": c.Database.SchemaVersion,
			},
		},
	}
}

// extractTerraformTouches generates resource touches from terraform plan changes.
func extractTerraformTouches(c plan.Content, env string) []Touch {
	if c.Terraform == nil {
		return nil
	}
	var touches []Touch
	for _, rc := range c.Terraform.ResourceChanges {
		touches = append(touches, Touch{
			EntityID:    rc.Address,
			Environment: env,
			ClaimType:   ClaimResource,
			Metadata: map[string]any{
				"type":          rc.Type,
				"name":          rc.Name,
				"change_action": rc.ChangeAction,
				"provider":      c.Terraform.Provider,
			},
		})
	}
	// If no resource changes, claim at the workspace level
	if len(touches) == 0 && c.Terraform.Workspace != "" {
		touches = append(touches, Touch{
			EntityID:    c.Terraform.Workspace,
			Environment: env,
			ClaimType:   ClaimResource,
			Metadata: map[string]any{
				"provider": c.Terraform.Provider,
			},
		})
	}
	return touches
}

// extractShellTouches generates touches from the shell plan's side effect manifest.
func extractShellTouches(c plan.Content, env string) []Touch {
	if c.Shell == nil {
		return nil
	}
	var touches []Touch
	manifest := c.Shell.SideEffectManifest

	// File operations → file_write claims (only writes and deletes)
	for _, fop := range manifest.FileOps {
		if fop.Action == "write" || fop.Action == "delete" {
			touches = append(touches, Touch{
				EntityID:    fop.Path,
				Environment: env,
				ClaimType:   ClaimFileWrite,
				Metadata: map[string]any{
					"action": fop.Action,
				},
			})
		}
	}

	// Network operations → service claims
	for _, nop := range manifest.NetworkOps {
		touches = append(touches, Touch{
			EntityID:    nop.Endpoint,
			Environment: env,
			ClaimType:   ClaimService,
			Metadata: map[string]any{
				"method":     nop.Method,
				"idempotent": nop.Idempotent,
			},
		})
	}

	return touches
}

// extractDeployTouches generates deploy_target touches from deploy plans.
func extractDeployTouches(c plan.Content, env string) []Touch {
	if c.Deploy == nil {
		return nil
	}
	return []Touch{
		{
			EntityID:    c.Deploy.Target,
			Environment: env,
			ClaimType:   ClaimDeployTarget,
			Metadata: map[string]any{
				"artifact_hash":   c.Deploy.ArtifactHash,
				"deploy_strategy": c.Deploy.DeployStrategy,
				"replicas":        c.Deploy.Replicas,
			},
		},
	}
}
