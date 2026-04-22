package catalog

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CatalogStore is the persistence interface used by Service.
type CatalogStore interface {
	// Entity CRUD
	CreateEntity(ctx context.Context, e *Entity) error
	GetEntityByID(ctx context.Context, projectID, id string) (*Entity, error)
	ListEntities(ctx context.Context, projectID string, entityType string, label string, limit int) ([]*Entity, error)
	UpdateEntity(ctx context.Context, e *Entity) error
	DeleteEntity(ctx context.Context, projectID, id string) error

	// Edge CRUD
	CreateEdge(ctx context.Context, e *Edge) error
	GetEdgeByID(ctx context.Context, projectID, id string) (*Edge, error)
	ListEdges(ctx context.Context, projectID, entityID, edgeType, direction string) ([]*Edge, error)
	DeleteEdge(ctx context.Context, projectID, id string) error

	// Deployment matrix
	UpsertDeployment(ctx context.Context, d *DeploymentEntry) error
	ListDeployments(ctx context.Context, projectID, serviceID string) ([]*DeploymentEntry, error)
}

// Service provides catalog operations for the project map.
type Service struct {
	store CatalogStore
}

// NewService returns a new catalog Service.
func NewService(store CatalogStore) *Service {
	return &Service{store: store}
}

// CreateEntity creates a new entity in the catalog.
func (s *Service) CreateEntity(ctx context.Context, projectID, entityType, name, description string, labels, metadata map[string]string, source string) (*Entity, error) {
	if !IsValidEntityType(entityType) {
		return nil, ErrInvalidType
	}
	src := Source(source)
	if src != SourceDeclared && src != SourceObserved {
		src = SourceDeclared
	}
	now := time.Now().UTC()
	e := &Entity{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   projectID,
		Type:        EntityType(entityType),
		Name:        name,
		Description: description,
		Labels:      labels,
		Metadata:    metadata,
		Source:      src,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.CreateEntity(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// GetEntity returns an entity by ID.
func (s *Service) GetEntity(ctx context.Context, projectID, entityID string) (*Entity, error) {
	return s.store.GetEntityByID(ctx, projectID, entityID)
}

// ListEntities lists entities with optional filters.
func (s *Service) ListEntities(ctx context.Context, projectID, entityType, label string, limit int) ([]*Entity, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListEntities(ctx, projectID, entityType, label, limit)
}

// UpdateEntity updates entity metadata, labels, or description.
func (s *Service) UpdateEntity(ctx context.Context, projectID, entityID, name, description string, labels, metadata map[string]string) (*Entity, error) {
	existing, err := s.store.GetEntityByID(ctx, projectID, entityID)
	if err != nil {
		return nil, err
	}
	if name != "" {
		existing.Name = name
	}
	if description != "" {
		existing.Description = description
	}
	if labels != nil {
		if existing.Labels == nil {
			existing.Labels = make(map[string]string)
		}
		for k, v := range labels {
			existing.Labels[k] = v
		}
	}
	if metadata != nil {
		if existing.Metadata == nil {
			existing.Metadata = make(map[string]string)
		}
		for k, v := range metadata {
			existing.Metadata[k] = v
		}
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateEntity(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteEntity removes an entity and its edges.
func (s *Service) DeleteEntity(ctx context.Context, projectID, entityID string) error {
	return s.store.DeleteEntity(ctx, projectID, entityID)
}

// CreateEdge creates a typed relationship between two entities.
func (s *Service) CreateEdge(ctx context.Context, projectID, fromID, toID, edgeType string, metadata map[string]string, source string) (*Edge, error) {
	if !IsValidEdgeType(edgeType) {
		return nil, ErrInvalidEdge
	}
	// Verify both entities exist.
	if _, err := s.store.GetEntityByID(ctx, projectID, fromID); err != nil {
		return nil, err
	}
	if _, err := s.store.GetEntityByID(ctx, projectID, toID); err != nil {
		return nil, err
	}
	src := Source(source)
	if src != SourceDeclared && src != SourceObserved {
		src = SourceDeclared
	}
	e := &Edge{
		ID:        uuid.Must(uuid.NewV7()).String(),
		ProjectID: projectID,
		FromID:    fromID,
		ToID:      toID,
		Type:      EdgeType(edgeType),
		Metadata:  metadata,
		Source:    src,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateEdge(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// ListEdges lists edges for an entity, optionally filtered by type and direction.
func (s *Service) ListEdges(ctx context.Context, projectID, entityID, edgeType, direction string) ([]*Edge, error) {
	if direction == "" {
		direction = "both"
	}
	return s.store.ListEdges(ctx, projectID, entityID, edgeType, direction)
}

// DeleteEdge removes a relationship.
func (s *Service) DeleteEdge(ctx context.Context, projectID, edgeID string) error {
	return s.store.DeleteEdge(ctx, projectID, edgeID)
}

// DeploymentMatrix returns the deployment matrix for a project.
func (s *Service) DeploymentMatrix(ctx context.Context, projectID, serviceID string) ([]*DeploymentEntry, error) {
	return s.store.ListDeployments(ctx, projectID, serviceID)
}

// UpsertDeployment upserts a deployment entry (observed or declared).
func (s *Service) UpsertDeployment(ctx context.Context, d *DeploymentEntry) error {
	return s.store.UpsertDeployment(ctx, d)
}

// ImportScanResult imports entities and edges from a bootstrap scan result.
// Entities that already exist (by name+type+project) are skipped.
func (s *Service) ImportScanResult(ctx context.Context, projectID string, result *ScanResult) (*ScanResult, error) {
	imported := &ScanResult{}
	for _, ent := range result.Entities {
		created, err := s.CreateEntity(ctx, projectID, string(ent.Type), ent.Name, ent.Description, ent.Labels, ent.Metadata, string(ent.Source))
		if err != nil {
			imported.Errors = append(imported.Errors, "entity "+ent.Name+": "+err.Error())
			continue
		}
		imported.Entities = append(imported.Entities, *created)
	}
	// Edges reference names; resolve to IDs after import.
	// For bootstrap, edges are created with the IDs of just-imported entities.
	for _, edge := range result.Edges {
		created, err := s.CreateEdge(ctx, projectID, edge.FromID, edge.ToID, string(edge.Type), edge.Metadata, string(edge.Source))
		if err != nil {
			imported.Errors = append(imported.Errors, "edge "+edge.FromID+"->"+edge.ToID+": "+err.Error())
			continue
		}
		imported.Edges = append(imported.Edges, *created)
	}
	return imported, nil
}
