package mcp

import (
	"context"
	"fmt"
)

// --------------------------------------------------------------------------
// Plugin Provider Interfaces
//
// These interfaces define the Go API that pluggable layer implementations must
// satisfy. Each interface maps directly to a PluginContract's required tools.
// Bundled defaults implement these interfaces; external MCP servers implement
// the equivalent tool endpoints defined in the contract's ToolSpec list.
//
// The system checks contract version compatibility at registration time.
// --------------------------------------------------------------------------

// SymbolInfo represents a code symbol returned by code intelligence queries.
type SymbolInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // function, type, variable, constant, method, interface
	File       string `json:"file"`
	Line       int    `json:"line"`
	Language   string `json:"language"`
	Package    string `json:"package"`
	Visibility string `json:"visibility"` // public, private, internal
	Signature  string `json:"signature,omitempty"`
	DocComment string `json:"doc_comment,omitempty"`
}

// CallSite represents a location where a symbol is called from or calls to.
type CallSite struct {
	Symbol     SymbolInfo `json:"symbol"`
	File       string     `json:"file"`
	Line       int        `json:"line"`
	Context    string     `json:"context,omitempty"` // surrounding code snippet
}

// BlastRadiusResult represents the impact analysis of modifying a symbol.
type BlastRadiusResult struct {
	Symbol          SymbolInfo   `json:"symbol"`
	DirectCallers   int          `json:"direct_callers"`
	TransitiveScope int          `json:"transitive_scope"`
	AffectedFiles   []string     `json:"affected_files"`
	AffectedSymbols []SymbolInfo `json:"affected_symbols"`
	RiskLevel       string       `json:"risk_level"` // low, medium, high, critical
}

// IndexStatus represents the current state of the code index.
type IndexStatus struct {
	ProjectID      string `json:"project_id"`
	LastCommitSHA  string `json:"last_commit_sha"`
	LastIndexedAt  string `json:"last_indexed_at"`
	TotalSymbols   int    `json:"total_symbols"`
	TotalFiles     int    `json:"total_files"`
	Languages      []string `json:"languages"`
	Stale          bool   `json:"stale"`
}

// CodeIntelligenceProvider is the Go interface for Layer 3 (Code Knowledge) plugins.
// Implementations provide structural code analysis capabilities.
//
// Required methods: SymbolLookup, Callers, Callees, BlastRadius, Importers, Status.
// Optional methods are in CodeIntelligenceExtended.
type CodeIntelligenceProvider interface {
	// ContractVersion returns the contract version this provider implements.
	ContractVersion() ContractVersion

	// SymbolLookup finds symbols matching the query.
	SymbolLookup(ctx context.Context, projectID, query string, opts SymbolLookupOpts) ([]SymbolInfo, error)

	// Callers returns all callers of the given symbol, up to the specified depth.
	Callers(ctx context.Context, projectID, symbol string, depth, limit int) ([]CallSite, error)

	// Callees returns all functions/methods called by the given symbol.
	Callees(ctx context.Context, projectID, symbol string, depth, limit int) ([]CallSite, error)

	// BlastRadius estimates the impact of modifying a symbol.
	BlastRadius(ctx context.Context, projectID, symbol string, depth int) (*BlastRadiusResult, error)

	// Importers finds all files/modules that import the given package.
	Importers(ctx context.Context, projectID, pkg string, limit int) ([]string, error)

	// Status returns the current index status for a project.
	Status(ctx context.Context, projectID string) (*IndexStatus, error)
}

// SymbolLookupOpts holds optional parameters for symbol lookup.
type SymbolLookupOpts struct {
	Language string
	Kind     string
	Limit    int
}

// CodeIntelligenceExtended provides optional capabilities beyond the required contract.
type CodeIntelligenceExtended interface {
	CodeIntelligenceProvider

	// Reindex triggers incremental re-indexing for specific paths or the entire project.
	Reindex(ctx context.Context, projectID string, paths []string, commitSHA string) error

	// SymbolMetadata returns detailed metadata for a specific symbol.
	SymbolMetadata(ctx context.Context, projectID, symbol string) (*SymbolInfo, error)
}

// --------------------------------------------------------------------------

// CatalogEntity represents an entity in the project catalog.
type CatalogEntity struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	EntityType  string            `json:"entity_type"` // service, datastore, integration, infrastructure, repository, environment
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Source      string            `json:"source"` // declared, observed
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// CatalogEdge represents a typed relationship between two entities.
type CatalogEdge struct {
	ID       string            `json:"id"`
	FromID   string            `json:"from_id"`
	ToID     string            `json:"to_id"`
	EdgeType string            `json:"edge_type"` // depends_on, provides, consumes, deployed_to, backed_by, monitors, owns
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CatalogProvider is the Go interface for Layer 14 (Project Map) plugins.
// Implementations provide entity and relationship management.
//
// Required methods: ListEntities, GetEntity, CreateEntity, UpdateEntity, ListEdges, CreateEdge.
// Optional methods are in CatalogExtended.
type CatalogProvider interface {
	// ContractVersion returns the contract version this provider implements.
	ContractVersion() ContractVersion

	// ListEntities lists entities with optional filtering.
	ListEntities(ctx context.Context, projectID string, entityType, label string, limit int) ([]CatalogEntity, error)

	// GetEntity returns a single entity by ID.
	GetEntity(ctx context.Context, projectID, entityID string) (*CatalogEntity, error)

	// CreateEntity creates a new entity and returns it with generated ID.
	CreateEntity(ctx context.Context, entity CatalogEntity) (*CatalogEntity, error)

	// UpdateEntity updates an existing entity's mutable fields.
	UpdateEntity(ctx context.Context, projectID, entityID string, updates CatalogEntityUpdate) (*CatalogEntity, error)

	// ListEdges lists edges for an entity, optionally filtered by type and direction.
	ListEdges(ctx context.Context, projectID, entityID, edgeType, direction string) ([]CatalogEdge, error)

	// CreateEdge creates a typed relationship between two entities.
	CreateEdge(ctx context.Context, edge CatalogEdge) (*CatalogEdge, error)
}

// CatalogEntityUpdate holds optional fields for entity updates.
type CatalogEntityUpdate struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CatalogExtended provides optional capabilities beyond the required contract.
type CatalogExtended interface {
	CatalogProvider

	// BootstrapScan discovers entities from repository artifacts.
	BootstrapScan(ctx context.Context, projectID, repoPath string) ([]CatalogEntity, error)

	// DeploymentMatrix returns the service x environment x version matrix.
	DeploymentMatrix(ctx context.Context, projectID, serviceID string) ([]DeploymentEntry, error)
}

// DeploymentEntry represents one cell in the deployment matrix.
type DeploymentEntry struct {
	ServiceID   string `json:"service_id"`
	ServiceName string `json:"service_name"`
	Environment string `json:"environment"`
	Version     string `json:"version"`
	DeployedAt  string `json:"deployed_at"`
}

// --------------------------------------------------------------------------

// ObservedResource represents a resource in the state index.
type ObservedResource struct {
	ID           string            `json:"id"`
	ProjectID    string            `json:"project_id"`
	ResourceType string            `json:"resource_type"`
	Environment  string            `json:"environment"`
	Properties   map[string]string `json:"properties"`
	ObservedAt   string            `json:"observed_at"`
	Attribution  *Attribution      `json:"attribution,omitempty"`
}

// Attribution records who/what caused a state change.
type Attribution struct {
	TicketID string `json:"ticket_id,omitempty"`
	Actor    string `json:"actor,omitempty"`
	ChangeID string `json:"change_id,omitempty"`
}

// DriftResult represents a mismatch between declared and observed state.
type DriftResult struct {
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	Field        string `json:"field"`
	DeclaredVal  string `json:"declared_value"`
	ObservedVal  string `json:"observed_value"`
	Severity     string `json:"severity"` // low, medium, high, critical
	DetectedAt   string `json:"detected_at"`
}

// StateIndexProvider is the Go interface for Layer 10 (Observed State Index) plugins.
// Implementations provide queryable ground-truth views of infrastructure state.
//
// Required methods: Query, GetResource, Staleness, Drift.
// Optional methods are in StateIndexExtended.
type StateIndexProvider interface {
	// ContractVersion returns the contract version this provider implements.
	ContractVersion() ContractVersion

	// Query queries the state index for resources matching the given criteria.
	Query(ctx context.Context, projectID, resourceType, environment string, filter map[string]string, limit int) ([]ObservedResource, error)

	// GetResource returns the full observed state for a specific resource.
	GetResource(ctx context.Context, projectID, resourceID string) (*ObservedResource, error)

	// Staleness returns resources where observation is older than threshold.
	Staleness(ctx context.Context, projectID, resourceType, environment string, thresholdSeconds int) ([]ObservedResource, error)

	// Drift detects mismatches between declared and observed state.
	Drift(ctx context.Context, projectID, resourceID, environment string) ([]DriftResult, error)
}

// StateIndexExtended provides optional capabilities beyond the required contract.
type StateIndexExtended interface {
	StateIndexProvider

	// AttributeChange records attribution for an observed state change.
	AttributeChange(ctx context.Context, projectID, resourceID string, attribution Attribution) error

	// Snapshot captures a point-in-time snapshot of resources for plan freshness stamps.
	Snapshot(ctx context.Context, projectID string, resourceIDs []string, environment string) ([]ObservedResource, error)
}

// --------------------------------------------------------------------------

// Signal represents an ingested production signal.
type Signal struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Source      string            `json:"source"`
	SignalType  string            `json:"signal_type"` // alert, metric_breach, error_spike, deploy_event, status_change, custom
	Severity   string            `json:"severity"`    // critical, warning, info
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	EntityIDs   []string          `json:"entity_ids,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   string            `json:"created_at"`
}

// SignalSourceInfo describes a configured signal source and its status.
type SignalSourceInfo struct {
	Name          string `json:"name"`
	Status        string `json:"status"` // connected, disconnected, error
	PollInterval  string `json:"poll_interval,omitempty"`
	LastPollAt    string `json:"last_poll_at,omitempty"`
	SignalCount   int    `json:"signal_count"`
}

// SignalIngestionProvider is the Go interface for Layer 11 (Signal Ingestion) plugins.
// Implementations provide production signal collection and normalization.
//
// Required methods: Ingest, ListRecent, ListSources, Correlate.
// Optional methods are in SignalIngestionExtended.
type SignalIngestionProvider interface {
	// ContractVersion returns the contract version this provider implements.
	ContractVersion() ContractVersion

	// Ingest ingests a signal into the signal store.
	Ingest(ctx context.Context, signal Signal) (*Signal, error)

	// ListRecent lists recent signals with optional filtering.
	ListRecent(ctx context.Context, projectID string, opts SignalListOpts) ([]Signal, error)

	// ListSources lists configured signal sources and their status.
	ListSources(ctx context.Context, projectID string) ([]SignalSourceInfo, error)

	// Correlate finds signals within a time window, optionally scoped to entities.
	Correlate(ctx context.Context, projectID, startTime, endTime string, entityIDs []string) ([]Signal, error)
}

// SignalListOpts holds optional parameters for listing signals.
type SignalListOpts struct {
	Source     string
	Severity   string
	SignalType string
	EntityID   string
	Since      string
	Limit      int
}

// SignalIngestionExtended provides optional capabilities beyond the required contract.
type SignalIngestionExtended interface {
	SignalIngestionProvider

	// RegisterWebhook registers a webhook endpoint for a source to push signals.
	RegisterWebhook(ctx context.Context, projectID, source, webhookURL string) (string, error)
}

// --------------------------------------------------------------------------

// Notification represents a notification to send to an operator.
type Notification struct {
	ID             string               `json:"id"`
	ProjectID      string               `json:"project_id"`
	Category       string               `json:"category"` // urgent_decision, autonomous_action, calibration_review, anomaly_alert, status_update
	Title          string               `json:"title"`
	Body           string               `json:"body"`
	Urgency        string               `json:"urgency"` // immediate, soon, digest
	TicketID       string               `json:"ticket_id,omitempty"`
	Actions        []NotificationAction `json:"actions,omitempty"`
	TimeoutSeconds int                  `json:"timeout_seconds,omitempty"`
	DefaultAction  string               `json:"default_action,omitempty"`
	SentAt         string               `json:"sent_at"`
}

// NotificationAction represents an action an operator can take on a notification.
type NotificationAction struct {
	Label    string `json:"label"`
	ActionID string `json:"action_id"`
}

// NotificationChannel represents a configured push channel.
type NotificationChannel struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	ChannelType string            `json:"channel_type"` // slack, email, sms, webhook
	Name        string            `json:"name"`
	Config      map[string]string `json:"config"`
	Categories  []string          `json:"categories,omitempty"` // which categories route here
}

// NotificationPreferences holds notification routing preferences.
type NotificationPreferences struct {
	ProjectID      string `json:"project_id"`
	QuietStart     string `json:"quiet_start,omitempty"`     // HH:MM
	QuietEnd       string `json:"quiet_end,omitempty"`       // HH:MM
	DigestSchedule string `json:"digest_schedule,omitempty"` // hourly, every_4h, daily, never
	Timezone       string `json:"timezone,omitempty"`
}

// NotificationProvider is the Go interface for Layer 12 (Notifications) plugins.
// Implementations provide policy-driven async push to operators.
//
// Required methods: Send, ListChannels, ConfigureChannel, Preferences.
// Optional methods are in NotificationExtended.
type NotificationProvider interface {
	// ContractVersion returns the contract version this provider implements.
	ContractVersion() ContractVersion

	// Send sends a notification via configured channels based on urgency and category routing.
	Send(ctx context.Context, notification Notification) (*Notification, error)

	// ListChannels lists configured notification channels.
	ListChannels(ctx context.Context, projectID string) ([]NotificationChannel, error)

	// ConfigureChannel creates or updates a notification channel.
	ConfigureChannel(ctx context.Context, channel NotificationChannel) (*NotificationChannel, error)

	// Preferences gets or updates notification preferences.
	Preferences(ctx context.Context, prefs NotificationPreferences) (*NotificationPreferences, error)
}

// NotificationExtended provides optional capabilities beyond the required contract.
type NotificationExtended interface {
	NotificationProvider

	// Dismiss records a notification dismissal for classifier tuning.
	Dismiss(ctx context.Context, projectID, notificationID, reason string) error

	// DismissalStats returns dismissal rate statistics for tuning.
	DismissalStats(ctx context.Context, projectID string, days int) (map[string]any, error)
}

// --------------------------------------------------------------------------
// Layer 4: Findings — Semantic findings store
// --------------------------------------------------------------------------

// Finding represents a semantic finding: a claim with provenance, symbol-level
// references, and ticket references. Findings are the output of agent analysis,
// test results, or review comments. They support semantic retrieval and
// structural navigation and can be invalidated when underlying code changes.
type Finding struct {
	ID                 string   `json:"id"`
	ProjectID          string   `json:"project_id"`
	Claim              string   `json:"claim"`
	FindingType        string   `json:"finding_type"` // code_quality, architecture, security, performance, design_pattern, investigation
	Confidence         float64  `json:"confidence"`   // 0.0–1.0
	CommitSHA          string   `json:"commit_sha"`
	FunctionBodyHash   string   `json:"function_body_hash,omitempty"`
	SourceType         string   `json:"source_type"`          // agent_analysis, test_result, review_comment
	SourceTicketID     string   `json:"source_ticket_id,omitempty"`
	SourceAgentID      string   `json:"source_agent_id,omitempty"`
	SymbolRefs         []string `json:"symbol_refs,omitempty"`
	TicketRefs         []string `json:"ticket_refs,omitempty"`
	FileRefs           []string `json:"file_refs,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	Summary            string   `json:"summary"`
	Valid              bool     `json:"valid"`
	InvalidatedAt      string   `json:"invalidated_at,omitempty"`
	InvalidationReason string   `json:"invalidation_reason,omitempty"`
	CreatedAt          string   `json:"created_at"`
}

// FindingsQueryOpts holds optional parameters for findings queries.
type FindingsQueryOpts struct {
	FindingType string // Filter by finding type
	SymbolRef   string // Filter by symbol reference
	FileRef     string // Filter by file reference
	ValidOnly   bool   // Return only valid (non-invalidated) findings
	Limit       int    // Max results (default 20)
}

// FindingsProvider is the Go interface for Layer 4 (Findings) plugins.
// Implementations provide semantic findings storage with provenance tracking,
// semantic retrieval, structural navigation, and invalidation on code changes.
//
// Required methods: SaveFinding, QueryFindings, FindingsBySymbol, FindingsByTicket,
// InvalidateFindings, GetFinding.
// Optional methods are in FindingsExtended.
type FindingsProvider interface {
	// ContractVersion returns the contract version this provider implements.
	ContractVersion() ContractVersion

	// SaveFinding persists a finding with provenance and returns it with generated ID.
	SaveFinding(ctx context.Context, finding Finding) (*Finding, error)

	// QueryFindings performs semantic retrieval: natural language query → relevant findings.
	QueryFindings(ctx context.Context, projectID, query string, opts FindingsQueryOpts) ([]Finding, error)

	// FindingsBySymbol returns findings associated with a given symbol reference.
	FindingsBySymbol(ctx context.Context, projectID, symbolRef string, limit int) ([]Finding, error)

	// FindingsByTicket returns findings associated with a given ticket reference.
	FindingsByTicket(ctx context.Context, projectID, ticketRef string, limit int) ([]Finding, error)

	// InvalidateFindings invalidates findings whose provenance references changed files
	// at a given commit SHA. Returns the count of invalidated findings.
	InvalidateFindings(ctx context.Context, projectID, commitSHA string, changedFiles []string) (int, error)

	// GetFinding returns a single finding by ID.
	GetFinding(ctx context.Context, projectID, findingID string) (*Finding, error)
}

// FindingsExtended provides optional capabilities beyond the required contract.
type FindingsExtended interface {
	FindingsProvider

	// DeleteFinding permanently removes a finding.
	DeleteFinding(ctx context.Context, projectID, findingID string) error

	// FindingsByFile returns findings whose file references include the given path.
	FindingsByFile(ctx context.Context, projectID, filePath string, limit int) ([]Finding, error)
}

// --------------------------------------------------------------------------
// Plugin Registry
// --------------------------------------------------------------------------

// PluginRegistry manages registered plugin providers and validates contract compatibility.
type PluginRegistry struct {
	codeIntel    CodeIntelligenceProvider
	catalog      CatalogProvider
	stateIndex   StateIndexProvider
	signals      SignalIngestionProvider
	notification NotificationProvider
	findings     FindingsProvider
}

// NewPluginRegistry creates a new empty plugin registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{}
}

// RegisterCodeIntelligence registers a code intelligence provider after version check.
func (r *PluginRegistry) RegisterCodeIntelligence(p CodeIntelligenceProvider) error {
	if !CodeIntelligenceContract.Version.Compatible(p.ContractVersion()) {
		return fmt.Errorf("code intelligence provider version %s incompatible with contract %s",
			p.ContractVersion(), CodeIntelligenceContract.Version)
	}
	r.codeIntel = p
	return nil
}

// RegisterCatalog registers a catalog provider after version check.
func (r *PluginRegistry) RegisterCatalog(p CatalogProvider) error {
	if !CatalogContract.Version.Compatible(p.ContractVersion()) {
		return fmt.Errorf("catalog provider version %s incompatible with contract %s",
			p.ContractVersion(), CatalogContract.Version)
	}
	r.catalog = p
	return nil
}

// RegisterStateIndex registers a state index provider after version check.
func (r *PluginRegistry) RegisterStateIndex(p StateIndexProvider) error {
	if !StateIndexContract.Version.Compatible(p.ContractVersion()) {
		return fmt.Errorf("state index provider version %s incompatible with contract %s",
			p.ContractVersion(), StateIndexContract.Version)
	}
	r.stateIndex = p
	return nil
}

// RegisterSignalIngestion registers a signal ingestion provider after version check.
func (r *PluginRegistry) RegisterSignalIngestion(p SignalIngestionProvider) error {
	if !SignalIngestionContract.Version.Compatible(p.ContractVersion()) {
		return fmt.Errorf("signal ingestion provider version %s incompatible with contract %s",
			p.ContractVersion(), SignalIngestionContract.Version)
	}
	r.signals = p
	return nil
}

// RegisterNotification registers a notification provider after version check.
func (r *PluginRegistry) RegisterNotification(p NotificationProvider) error {
	if !NotificationContract.Version.Compatible(p.ContractVersion()) {
		return fmt.Errorf("notification provider version %s incompatible with contract %s",
			p.ContractVersion(), NotificationContract.Version)
	}
	r.notification = p
	return nil
}

// CodeIntelligence returns the registered code intelligence provider, or nil.
func (r *PluginRegistry) CodeIntelligence() CodeIntelligenceProvider { return r.codeIntel }

// Catalog returns the registered catalog provider, or nil.
func (r *PluginRegistry) Catalog() CatalogProvider { return r.catalog }

// StateIndex returns the registered state index provider, or nil.
func (r *PluginRegistry) StateIndex() StateIndexProvider { return r.stateIndex }

// SignalIngestion returns the registered signal ingestion provider, or nil.
func (r *PluginRegistry) SignalIngestion() SignalIngestionProvider { return r.signals }

// Notification returns the registered notification provider, or nil.
func (r *PluginRegistry) Notification() NotificationProvider { return r.notification }

// RegisterFindings registers a findings provider after version check.
func (r *PluginRegistry) RegisterFindings(p FindingsProvider) error {
	if !FindingsContract.Version.Compatible(p.ContractVersion()) {
		return fmt.Errorf("findings provider version %s incompatible with contract %s",
			p.ContractVersion(), FindingsContract.Version)
	}
	r.findings = p
	return nil
}

// Findings returns the registered findings provider, or nil.
func (r *PluginRegistry) Findings() FindingsProvider { return r.findings }
