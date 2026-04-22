package pillar

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store is the persistence interface for pillar data.
type Store interface {
	// Entry CRUD
	CreateEntry(ctx context.Context, e *Entry) error
	GetEntry(ctx context.Context, id string) (*Entry, error)
	ListEntries(ctx context.Context, projectID, entityID, pillarType string, limit int) ([]*Entry, error)
	UpdateEntry(ctx context.Context, e *Entry) error
	DeleteEntry(ctx context.Context, id string) error

	// Claim CRUD
	CreateClaim(ctx context.Context, c *Claim) error
	GetClaim(ctx context.Context, id string) (*Claim, error)
	ListClaims(ctx context.Context, pillarEntryID string) ([]*Claim, error)
	UpdateClaim(ctx context.Context, c *Claim) error
	DeleteClaim(ctx context.Context, id string) error

	// Evaluations
	CreateEvaluation(ctx context.Context, ev *Evaluation) error
	ListEvaluations(ctx context.Context, pillarEntryID string, limit int) ([]*Evaluation, error)

	// Review scheduling
	ListDueForReview(ctx context.Context, projectID string, before time.Time, limit int) ([]*Entry, error)
}

// Service provides pillar layer operations.
type Service struct {
	store Store
}

// NewService returns a new pillar Service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// ────────────────────────────────────────────────────────────────────────────
// Entry operations
// ────────────────────────────────────────────────────────────────────────────

// CreateEntry creates a new pillar entry for an entity.
func (s *Service) CreateEntry(ctx context.Context, projectID, entityID, entityType, pillarType, strategy, createdBy string, gaps []Gap, cadence string) (*Entry, error) {
	if !IsValidPillarType(pillarType) {
		return nil, ErrInvalidPillarType
	}
	if cadence == "" {
		// Use default cadence from template.
		if tmpl := TemplateByType(PillarType(pillarType)); tmpl != nil {
			cadence = tmpl.DefaultCadence
		} else {
			cadence = "monthly"
		}
	}
	if !IsValidCadence(cadence) {
		return nil, fmt.Errorf("invalid cadence: %s", cadence)
	}

	now := time.Now().UTC()
	nextReview := nextReviewTime(now, ReviewCadence(cadence))

	e := &Entry{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ProjectID:     projectID,
		EntityID:      entityID,
		EntityType:    entityType,
		PillarType:    PillarType(pillarType),
		Strategy:      strategy,
		Gaps:          gaps,
		ReviewCadence: ReviewCadence(cadence),
		NextReviewAt:  &nextReview,
		Version:       1,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.CreateEntry(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// GetEntry returns a pillar entry by ID.
func (s *Service) GetEntry(ctx context.Context, id string) (*Entry, error) {
	return s.store.GetEntry(ctx, id)
}

// ListEntries lists pillar entries with optional filters.
func (s *Service) ListEntries(ctx context.Context, projectID, entityID, pillarType string, limit int) ([]*Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListEntries(ctx, projectID, entityID, pillarType, limit)
}

// UpdateEntry updates strategy, gaps, and/or review cadence on a pillar entry.
func (s *Service) UpdateEntry(ctx context.Context, id, strategy string, gaps []Gap, cadence string) (*Entry, error) {
	existing, err := s.store.GetEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	if strategy != "" {
		existing.Strategy = strategy
	}
	if gaps != nil {
		existing.Gaps = gaps
	}
	if cadence != "" {
		if !IsValidCadence(cadence) {
			return nil, fmt.Errorf("invalid cadence: %s", cadence)
		}
		existing.ReviewCadence = ReviewCadence(cadence)
	}
	existing.Version++
	existing.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateEntry(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteEntry removes a pillar entry.
func (s *Service) DeleteEntry(ctx context.Context, id string) error {
	return s.store.DeleteEntry(ctx, id)
}

// MarkReviewed records that a pillar entry has been reviewed and computes next review date.
func (s *Service) MarkReviewed(ctx context.Context, id string) (*Entry, error) {
	existing, err := s.store.GetEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	nextReview := nextReviewTime(now, existing.ReviewCadence)
	existing.LastReviewedAt = &now
	existing.NextReviewAt = &nextReview
	existing.Version++
	existing.UpdatedAt = now

	if err := s.store.UpdateEntry(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// ListDueForReview returns entries whose next_review_at is before the given time.
func (s *Service) ListDueForReview(ctx context.Context, projectID string, before time.Time, limit int) ([]*Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListDueForReview(ctx, projectID, before, limit)
}

// ────────────────────────────────────────────────────────────────────────────
// Claim operations
// ────────────────────────────────────────────────────────────────────────────

// CreateClaim adds a structured claim to a pillar entry, referencing a map entity.
func (s *Service) CreateClaim(ctx context.Context, pillarEntryID, statement, entityRefID, entityRefType, evidence string) (*Claim, error) {
	// Verify the pillar entry exists.
	if _, err := s.store.GetEntry(ctx, pillarEntryID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	c := &Claim{
		ID:            uuid.Must(uuid.NewV7()).String(),
		PillarEntryID: pillarEntryID,
		Statement:     statement,
		EntityRefID:   entityRefID,
		EntityRefType: entityRefType,
		Evidence:      evidence,
		Status:        ClaimUnverified,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.CreateClaim(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetClaim returns a claim by ID.
func (s *Service) GetClaim(ctx context.Context, id string) (*Claim, error) {
	return s.store.GetClaim(ctx, id)
}

// ListClaims returns all claims for a pillar entry.
func (s *Service) ListClaims(ctx context.Context, pillarEntryID string) ([]*Claim, error) {
	return s.store.ListClaims(ctx, pillarEntryID)
}

// UpdateClaimStatus updates a claim's verification status and evidence.
func (s *Service) UpdateClaimStatus(ctx context.Context, id string, status, evidence string) (*Claim, error) {
	existing, err := s.store.GetClaim(ctx, id)
	if err != nil {
		return nil, err
	}
	if status != "" {
		if !IsValidClaimStatus(status) {
			return nil, ErrInvalidClaimStatus
		}
		existing.Status = ClaimStatus(status)
	}
	if evidence != "" {
		existing.Evidence = evidence
	}
	now := time.Now().UTC()
	existing.LastEvaluatedAt = &now
	existing.UpdatedAt = now

	if err := s.store.UpdateClaim(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteClaim removes a claim.
func (s *Service) DeleteClaim(ctx context.Context, id string) error {
	return s.store.DeleteClaim(ctx, id)
}

// ────────────────────────────────────────────────────────────────────────────
// Evaluation operations
// ────────────────────────────────────────────────────────────────────────────

// RecordEvaluation records the result of an evaluation check.
func (s *Service) RecordEvaluation(ctx context.Context, pillarEntryID string, checkType EvalCheckType, outcome EvalOutcome, details string) (*Evaluation, error) {
	// Verify the pillar entry exists.
	if _, err := s.store.GetEntry(ctx, pillarEntryID); err != nil {
		return nil, err
	}

	ev := &Evaluation{
		ID:            uuid.Must(uuid.NewV7()).String(),
		PillarEntryID: pillarEntryID,
		CheckType:     checkType,
		Outcome:       outcome,
		Details:       details,
		EvaluatedAt:   time.Now().UTC(),
	}

	if err := s.store.CreateEvaluation(ctx, ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// ListEvaluations returns recent evaluations for a pillar entry.
func (s *Service) ListEvaluations(ctx context.Context, pillarEntryID string, limit int) ([]*Evaluation, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.store.ListEvaluations(ctx, pillarEntryID, limit)
}

// EvaluateEntry runs all three evaluation checks on a pillar entry and its claims.
// Returns the list of evaluation results.
func (s *Service) EvaluateEntry(ctx context.Context, pillarEntryID string) ([]*Evaluation, error) {
	entry, err := s.store.GetEntry(ctx, pillarEntryID)
	if err != nil {
		return nil, err
	}

	claims, err := s.store.ListClaims(ctx, pillarEntryID)
	if err != nil {
		return nil, err
	}

	var results []*Evaluation

	// Check 1: Evidence intact — do all claims still have referenced entities?
	ev1 := evaluateEvidenceIntact(entry, claims)
	ev1.ID = uuid.Must(uuid.NewV7()).String()
	ev1.PillarEntryID = pillarEntryID
	ev1.EvaluatedAt = time.Now().UTC()
	if err := s.store.CreateEvaluation(ctx, ev1); err != nil {
		return nil, err
	}
	results = append(results, ev1)

	// Check 2: Claims match reality — are claims verified and not stale?
	ev2 := evaluateClaimMatchesReality(claims)
	ev2.ID = uuid.Must(uuid.NewV7()).String()
	ev2.PillarEntryID = pillarEntryID
	ev2.EvaluatedAt = time.Now().UTC()
	if err := s.store.CreateEvaluation(ctx, ev2); err != nil {
		return nil, err
	}
	results = append(results, ev2)

	// Check 3: Strategy appropriate — are there high-severity gaps or too many failures?
	ev3 := evaluateStrategyAppropriate(entry, claims)
	ev3.ID = uuid.Must(uuid.NewV7()).String()
	ev3.PillarEntryID = pillarEntryID
	ev3.EvaluatedAt = time.Now().UTC()
	if err := s.store.CreateEvaluation(ctx, ev3); err != nil {
		return nil, err
	}
	results = append(results, ev3)

	return results, nil
}

// GetTemplates returns the default templates for all pillar types.
func (s *Service) GetTemplates() []Template {
	return DefaultTemplates()
}

// GetTemplate returns the default template for a specific pillar type.
func (s *Service) GetTemplate(pillarType string) (*Template, error) {
	if !IsValidPillarType(pillarType) {
		return nil, ErrInvalidPillarType
	}
	t := TemplateByType(PillarType(pillarType))
	return t, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

func nextReviewTime(from time.Time, cadence ReviewCadence) time.Time {
	switch cadence {
	case CadenceWeekly:
		return from.AddDate(0, 0, 7)
	case CadenceBiweekly:
		return from.AddDate(0, 0, 14)
	case CadenceMonthly:
		return from.AddDate(0, 1, 0)
	case CadenceQuarterly:
		return from.AddDate(0, 3, 0)
	default:
		return from.AddDate(0, 1, 0) // default monthly
	}
}
