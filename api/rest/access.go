package rest

import (
	"context"
	"log"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/project"
)

// AgentGetter is used by EnsureOrgAccess. *agent.Store implements it.
type AgentGetter interface {
	GetByID(ctx context.Context, id string) (*agent.Agent, error)
}

// OrgMemberLister is used by EnsureOrgAccess. *org.Service implements it.
type OrgMemberLister interface {
	ListOrgIDsForUser(ctx context.Context, userID string) ([]string, error)
}

// ProjectGetterForAccess is used by EnsureProjectAccess. *project.Service implements it.
type ProjectGetterForAccess interface {
	GetProject(ctx context.Context, projectID string) (*project.Project, error)
}

// CheckOrgAccess returns nil if the caller has org access, or a StructuredError (401/403) otherwise.
// Use from strict server handlers that return (nil, err).
func CheckOrgAccess(ctx context.Context, orgID string, agentStore AgentGetter, orgSvc OrgMemberLister) *apierrors.StructuredError {
	agentID := GetAgentID(ctx)
	if agentID == "" {
		log.Printf("CheckOrgAccess: org=%s denied: no agent in context", orgID)
		return apierrors.New(apierrors.CodeUnauthorized, "authentication required", false)
	}
	a, err := agentStore.GetByID(ctx, agentID)
	if err != nil || a == nil {
		log.Printf("CheckOrgAccess: org=%s agent=%s denied: agent not found (err=%v)", orgID, agentID, err)
		return apierrors.New(apierrors.CodeUnauthorized, "agent not found", false)
	}
	if a.UserID == "" {
		log.Printf("CheckOrgAccess: org=%s agent=%s denied: OAuth required (no user link)", orgID, agentID)
		return apierrors.New(apierrors.CodeUnauthorized, "OAuth required (agent must be linked to a user)", false)
	}
	orgIDs, err := orgSvc.ListOrgIDsForUser(ctx, a.UserID)
	if err != nil {
		log.Printf("CheckOrgAccess: org=%s agent=%s ListOrgIDsForUser error: %v", orgID, agentID, err)
		return apierrors.MapError(err)
	}
	for _, id := range orgIDs {
		if id == orgID {
			return nil
		}
	}
	log.Printf("CheckOrgAccess: org=%s agent=%s user=%s denied: not in org (user orgs=%v)", orgID, agentID, a.UserID, orgIDs)
	return apierrors.New(apierrors.CodeForbidden, "you do not have access to that organization", false)
}

// CheckProjectAccess returns nil if the caller has project access, or a StructuredError (401/403/404) otherwise.
func CheckProjectAccess(ctx context.Context, projectID string, agentStore AgentGetter, orgSvc OrgMemberLister, projectSvc ProjectGetterForAccess) *apierrors.StructuredError {
	proj, err := projectSvc.GetProject(ctx, projectID)
	if err != nil {
		return apierrors.MapError(err)
	}
	if proj == nil {
		return apierrors.New(apierrors.CodeNotFound, "project not found", false)
	}
	return CheckOrgAccess(ctx, proj.OrgID, agentStore, orgSvc)
}

// EnsureOrgAccess requires an authenticated agent with OAuth (user link), verifies the user is a member of the given org,
// and writes 401/403 and returns false if not. Returns true when the caller has access.
func EnsureOrgAccess(ctx context.Context, w http.ResponseWriter, orgID string, agentStore AgentGetter, orgSvc OrgMemberLister) bool {
	if err := CheckOrgAccess(ctx, orgID, agentStore, orgSvc); err != nil {
		WriteStructuredError(w, err)
		return false
	}
	return true
}

// EnsureProjectAccess requires an authenticated agent with OAuth, loads the project to get org_id, and verifies the user
// is a member of that org. Writes 401/403/404 and returns false if not. Returns true when the caller has access.
func EnsureProjectAccess(ctx context.Context, w http.ResponseWriter, projectID string, agentStore AgentGetter, orgSvc OrgMemberLister, projectSvc ProjectGetterForAccess) bool {
	if err := CheckProjectAccess(ctx, projectID, agentStore, orgSvc, projectSvc); err != nil {
		WriteStructuredError(w, err)
		return false
	}
	return true
}
