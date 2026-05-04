package projecttemplate

import (
	"context"
	"fmt"

	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workstream"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) ListWorkstreamTemplates(ctx context.Context, orgID string) ([]WorkstreamTemplate, error) {
	return s.store.ListWorkstreamTemplates(ctx, orgID)
}

func (s *Service) ListProjectTemplates(ctx context.Context, orgID string) ([]ProjectTemplate, error) {
	return s.store.ListProjectTemplates(ctx, orgID)
}

func (s *Service) GetProjectTemplate(ctx context.Context, id string) (*ProjectTemplate, error) {
	return s.store.GetProjectTemplate(ctx, id)
}

func (s *Service) ListProjectTemplatesExpanded(ctx context.Context, orgID string) ([]ProjectTemplateExpanded, error) {
	pts, err := s.store.ListProjectTemplates(ctx, orgID)
	if err != nil {
		return nil, err
	}

	// Collect all referenced workstream template IDs
	idSet := map[string]bool{}
	for _, pt := range pts {
		for _, id := range pt.WorkstreamTemplateIDs {
			idSet[id] = true
		}
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}

	wsts, err := s.store.GetWorkstreamTemplatesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	wstMap := map[string]WorkstreamTemplate{}
	for _, wst := range wsts {
		wstMap[wst.ID] = wst
	}

	result := make([]ProjectTemplateExpanded, 0, len(pts))
	for _, pt := range pts {
		expanded := ProjectTemplateExpanded{ProjectTemplate: pt}
		for _, id := range pt.WorkstreamTemplateIDs {
			if wst, ok := wstMap[id]; ok {
				expanded.WorkstreamTemplates = append(expanded.WorkstreamTemplates, wst)
			}
		}
		result = append(result, expanded)
	}
	return result, nil
}

// SeedProject creates workstreams and tickets from the given workstream template IDs.
func (s *Service) SeedProject(ctx context.Context, projectID string, workstreamTemplateIDs []string, createdBy string, wsSvc *workstream.Service, tSvc *ticket.Service) (*SeedResult, error) {
	wsts, err := s.store.GetWorkstreamTemplatesByIDs(ctx, workstreamTemplateIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch workstream templates: %w", err)
	}

	// Index by ID to preserve requested order
	wstMap := map[string]WorkstreamTemplate{}
	for _, wst := range wsts {
		wstMap[wst.ID] = wst
	}

	result := &SeedResult{}

	for _, id := range workstreamTemplateIDs {
		wst, ok := wstMap[id]
		if !ok {
			continue
		}

		ws, err := wsSvc.CreateWorkStream(ctx, projectID, wst.Name, wst.Slug, wst.Plan)
		if err != nil {
			return nil, fmt.Errorf("create workstream %q: %w", wst.Name, err)
		}
		result.WorkstreamsCreated++
		result.WorkstreamIDs = append(result.WorkstreamIDs, ws.ID)

		for _, tt := range wst.Tickets {
			typ := ticket.TicketType(tt.Type)
			priority := ticket.Priority(tt.Priority)
			objective := ticket.Objective{
				Description:     tt.Description,
				SuccessCriteria: tt.SuccessCriteria,
			}
			t, err := tSvc.CreateTicket(ctx, projectID, tt.Title, typ, priority, createdBy, nil, ws.ID, objective, ticket.TicketContext{}, "")
			if err != nil {
				return nil, fmt.Errorf("create ticket %q in workstream %q: %w", tt.Title, wst.Name, err)
			}
			result.TicketsCreated++
			result.TicketIDs = append(result.TicketIDs, t.ID)
		}
	}

	return result, nil
}
