package ticket

import (
	"context"
	"github.com/gabinante/flywheel/internal/project"
	"testing"
)

type reviewWorkflowResolver struct{}

func (reviewWorkflowResolver) ResolveForProject(context.Context, string, string) (string, int, string, error) {
	return "implementation-workflow", 1, "plan", nil
}

func TestReviewLinearImportUsesProjectWorkflow(t *testing.T) {
	svc := NewService(newInMemoryStore(), stubBus{}, &stubProjectGetter{proj: &project.Project{ID: "p", Slug: "project", OrgID: "o"}})
	svc.SetWorkflowResolver(reviewWorkflowResolver{})
	imported, err := svc.ImportExternal(context.Background(), ExternalImport{ProjectID: "p", Title: "Linear work", Provider: "linear", State: StateDraft})
	if err != nil {
		t.Fatal(err)
	}
	if imported.WorkflowID == "" {
		t.Fatal("Linear import bypassed configured project workflow")
	}
}
