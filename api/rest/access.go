package rest

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	orgpkg "github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AgentGetter is used by EnsureOrgAccess. *agent.Store implements it.
type AgentGetter interface {
	GetByID(ctx context.Context, id string) (*agent.Agent, error)
}

// OrgMemberLister is used by EnsureOrgAccess. *org.Service implements it.
type OrgMemberLister interface {
	ListOrgIDsForUser(ctx context.Context, userID string) ([]string, error)
}

// OrgSlugResolver resolves org slugs to UUIDs. Nil-safe: if nil, slug resolution is skipped.
type OrgSlugResolver interface {
	GetBySlug(ctx context.Context, slug string) (*orgpkg.Org, error)
}

// ProjectGetterForAccess is used by EnsureProjectAccess. *project.Service implements it.
type ProjectGetterForAccess interface {
	GetProject(ctx context.Context, projectID string) (*project.Project, error)
}

// ProjectSlugResolver resolves project slugs to UUIDs within an org. Nil-safe.
type ProjectSlugResolver interface {
	GetBySlug(ctx context.Context, orgID, slug string) (*project.Project, error)
}

// CheckOrgAccess returns nil if the caller has org access, or a StructuredError (401/403) otherwise.
// Use from strict server handlers that return (nil, err).
func CheckOrgAccess(ctx context.Context, orgID string, agentStore AgentGetter, orgSvc OrgMemberLister) *apierrors.StructuredError {
	agentID := GetAgentID(ctx)
	if agentID == "" {
		slog.Warn("CheckOrgAccess denied: no agent in context", "org", orgID)
		return apierrors.New(apierrors.CodeUnauthorized, "authentication required", false)
	}
	a, err := agentStore.GetByID(ctx, agentID)
	if err != nil || a == nil {
		slog.Warn("CheckOrgAccess denied: agent not found", "org", orgID, "agent", agentID, "error", err)
		return apierrors.New(apierrors.CodeUnauthorized, "agent not found", false)
	}
	if a.UserID == "" {
		slog.Warn("CheckOrgAccess denied: OAuth required", "org", orgID, "agent", agentID)
		return apierrors.New(apierrors.CodeUnauthorized, "OAuth required (agent must be linked to a user)", false)
	}
	orgIDs, err := orgSvc.ListOrgIDsForUser(ctx, a.UserID)
	if err != nil {
		slog.Error("CheckOrgAccess ListOrgIDsForUser failed", "org", orgID, "agent", agentID, "error", err)
		return apierrors.MapError(err)
	}
	for _, id := range orgIDs {
		if id == orgID {
			return nil
		}
	}
	slog.Warn("CheckOrgAccess denied: not in org", "org", orgID, "agent", agentID, "user", a.UserID)
	return apierrors.New(apierrors.CodeForbidden, "you do not have access to that organization", false)
}

// CheckProjectAccess returns nil if the caller has project access, or a StructuredError (401/403/404) otherwise.
// projectID may be a UUID or slug; slug resolution requires orgSlugResolver and projectSlugResolver on the StrictServer.
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

// ResolveOrgID resolves an org slug to UUID using the provided resolver. Returns input if already a UUID or resolver is nil.
func ResolveOrgID(ctx context.Context, orgIDOrSlug string, resolver OrgSlugResolver) string {
	if uuidRE.MatchString(orgIDOrSlug) || resolver == nil {
		return orgIDOrSlug
	}
	o, err := resolver.GetBySlug(ctx, orgIDOrSlug)
	if err != nil || o == nil {
		return orgIDOrSlug
	}
	return o.ID
}

// ResolveProjectID resolves a project slug to UUID using the provided resolver. Returns input if already a UUID or resolver is nil.
func ResolveProjectID(ctx context.Context, orgID, projectIDOrSlug string, resolver ProjectSlugResolver) string {
	if uuidRE.MatchString(projectIDOrSlug) || resolver == nil {
		return projectIDOrSlug
	}
	p, err := resolver.GetBySlug(ctx, orgID, projectIDOrSlug)
	if err != nil || p == nil {
		return projectIDOrSlug
	}
	return p.ID
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
