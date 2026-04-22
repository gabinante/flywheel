package stateindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SteampipeProvider implements StateProvider using Steampipe's PostgreSQL FDW.
// Steampipe exposes cloud resources as SQL tables (e.g. aws_ec2_instance,
// gcp_compute_instance). This provider executes SQL queries against Steampipe's
// local Postgres and normalizes the results into ObservedResource format.
type SteampipeProvider struct {
	// db is a connection to Steampipe's Postgres (default: localhost:9193).
	db *sql.DB

	// queryMap maps resource types to Steampipe SQL queries.
	queryMap map[ResourceType]string
}

// SteampipeConfig holds connection settings for the Steampipe provider.
type SteampipeConfig struct {
	// ConnectionString is the Postgres connection string for Steampipe.
	// Default: "postgresql://steampipe@localhost:9193/steampipe"
	ConnectionString string

	// CustomQueries allows operators to override default resource queries.
	CustomQueries map[ResourceType]string
}

// DefaultSteampipeConfig returns the default configuration.
func DefaultSteampipeConfig() SteampipeConfig {
	return SteampipeConfig{
		ConnectionString: "postgresql://steampipe@localhost:9193/steampipe",
	}
}

// NewSteampipeProvider creates a Steampipe-based state provider.
// Pass nil for config to use defaults.
func NewSteampipeProvider(db *sql.DB) *SteampipeProvider {
	p := &SteampipeProvider{
		db:       db,
		queryMap: defaultSteampipeQueries(),
	}
	return p
}

// SetQueryMap overrides the default query map with custom queries.
func (p *SteampipeProvider) SetQueryMap(queries map[ResourceType]string) {
	for k, v := range queries {
		p.queryMap[k] = v
	}
}

// Name returns the provider identifier.
func (p *SteampipeProvider) Name() string {
	return "steampipe"
}

// SupportedResourceTypes returns all resource types this provider can query.
func (p *SteampipeProvider) SupportedResourceTypes() []ResourceType {
	types := make([]ResourceType, 0, len(p.queryMap))
	for rt := range p.queryMap {
		types = append(types, rt)
	}
	return types
}

// Healthy checks if the Steampipe Postgres is reachable.
func (p *SteampipeProvider) Healthy(ctx context.Context) error {
	if p.db == nil {
		return fmt.Errorf("steampipe: no database connection")
	}
	return p.db.PingContext(ctx)
}

// QueryState executes a Steampipe SQL query and returns normalized resources.
func (p *SteampipeProvider) QueryState(ctx context.Context, projectID string, resourceType ResourceType, environment string) ([]*ObservedResource, error) {
	query, ok := p.queryMap[resourceType]
	if !ok {
		return nil, fmt.Errorf("steampipe: no query defined for resource type %s", resourceType)
	}

	if p.db == nil {
		return nil, fmt.Errorf("steampipe: no database connection")
	}

	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("steampipe query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("steampipe columns: %w", err)
	}

	now := time.Now().UTC()
	var resources []*ObservedResource

	for rows.Next() {
		// Scan all columns as generic values.
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// Build properties map from column values.
		props := make(map[string]any)
		var name, externalID, provider, region string
		for i, col := range columns {
			val := values[i]
			if val == nil {
				continue
			}
			// Extract well-known columns for resource metadata.
			switch col {
			case "name", "title":
				name = fmt.Sprintf("%v", val)
			case "arn", "id", "resource_id", "self_link":
				externalID = fmt.Sprintf("%v", val)
			case "region", "location", "zone":
				region = fmt.Sprintf("%v", val)
			case "account_id", "project":
				provider = fmt.Sprintf("%v", val)
			}
			// Normalize the value for JSON storage.
			props[col] = normalizeValue(val)
		}

		if name == "" {
			name = externalID
		}

		r := &ObservedResource{
			ID:           fmt.Sprintf("sp_%s_%s", string(resourceType), externalID),
			ProjectID:    projectID,
			ResourceType: resourceType,
			Environment:  environment,
			Name:         name,
			ExternalID:   externalID,
			Provider:     provider,
			Region:       region,
			Properties:   props,
			ObservedAt:   now,
			Source:        "steampipe",
		}
		resources = append(resources, r)
	}

	return resources, rows.Err()
}

// defaultSteampipeQueries returns the default SQL queries for common resource types.
// These target Steampipe's built-in cloud plugin tables.
func defaultSteampipeQueries() map[ResourceType]string {
	return map[ResourceType]string{
		ResourceContainer: `
			SELECT container_id as id, name, image, status, created_at,
			       region, account_id
			FROM aws_ecs_container_instance
			LIMIT 500`,
		ResourceService: `
			SELECT arn as id, service_name as name, status, launch_type,
			       desired_count, running_count, region, account_id
			FROM aws_ecs_service
			LIMIT 500`,
		ResourceDatabase: `
			SELECT db_instance_identifier as id, db_instance_identifier as name,
			       engine, engine_version, db_instance_class, db_instance_status as status,
			       region, account_id
			FROM aws_rds_db_instance
			LIMIT 500`,
		ResourceLoadBalancer: `
			SELECT arn as id, name, type, state_code as status, scheme,
			       region, account_id
			FROM aws_ec2_application_load_balancer
			LIMIT 500`,
		ResourceBucket: `
			SELECT arn as id, name, region, account_id,
			       versioning_enabled, logging_target_bucket
			FROM aws_s3_bucket
			LIMIT 500`,
		ResourceVM: `
			SELECT instance_id as id, title as name, instance_type, instance_state as status,
			       launch_time, region, account_id
			FROM aws_ec2_instance
			LIMIT 500`,
		ResourceFunction: `
			SELECT arn as id, name, runtime, handler, memory_size,
			       timeout, region, account_id
			FROM aws_lambda_function
			LIMIT 500`,
	}
}

// normalizeValue converts SQL scan results to JSON-friendly types.
func normalizeValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []byte:
		// Try to parse as JSON first.
		var jsonVal any
		if err := json.Unmarshal(val, &jsonVal); err == nil {
			return jsonVal
		}
		return string(val)
	case time.Time:
		return val.UTC().Format(time.RFC3339)
	default:
		return val
	}
}
