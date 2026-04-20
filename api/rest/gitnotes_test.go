package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/gitnotes"
	"github.com/gabinante/flywheel/internal/project"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH, skipping gitnotes REST tests")
	}
}

// makeTempGitRepo creates a temp dir with git init and one commit; returns repo path and HEAD sha.
func makeTempGitRepoForREST(t *testing.T) (repoPath, headSHA string) {
	t.Helper()
	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		dir  string
	}{
		{[]string{"init"}, dir},
		{[]string{"config", "user.email", "test@test"}, dir},
		{[]string{"config", "user.name", "Test"}, dir},
		{[]string{"commit", "--allow-empty", "-m", "first"}, dir},
	} {
		cmd := exec.Command("git", c.args...)
		cmd.Dir = c.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", c.args, err, out)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	headSHA = strings.TrimSpace(string(out))
	return dir, headSHA
}

func gitNotesHandlerRouter(h *GitNotesHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /orgs/{orgID}/projects/{projectID}/git-notes/commits/{commitSha}", h.getCommitNotes)
	mux.HandleFunc("GET /orgs/{orgID}/projects/{projectID}/git-notes/log", h.getLog)
	return mux
}

// requestWithAgent returns a request with optional agent ID in context (for 401 test omit agentID).
func requestWithAgent(method, path string, agentID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if agentID != "" {
		req = req.WithContext(context.WithValue(req.Context(), ContextKeyAgentID, agentID))
	}
	return req
}

func TestGitNotesHandler_getCommitNotes_404_ProjectMissing(t *testing.T) {
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: nil, err: nil},
		OrgSvc:    &mockOrgSvc{},
		AgentStore: &mockAgentStore{},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/commits/abc123?repo_path=/tmp", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestGitNotesHandler_getCommitNotes_404_OrgMismatch(t *testing.T) {
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}},
	}
	r := gitNotesHandlerRouter(h)
	// Request with orgID != project.OrgID (org2 in URL, project has org1)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org2/projects/proj1/git-notes/commits/abc123?repo_path=/tmp", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestGitNotesHandler_getCommitNotes_401_NoAgent(t *testing.T) {
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{},
		AgentStore: &mockAgentStore{},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/commits/abc123?repo_path=/tmp", "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", w.Code)
	}
}

func TestGitNotesHandler_getCommitNotes_403_NoProjectAccess(t *testing.T) {
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"other-org"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/commits/abc123?repo_path=/tmp", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want 403", w.Code)
	}
}

func TestGitNotesHandler_getCommitNotes_501_NoRepoPath(t *testing.T) {
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/commits/abc123", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("got status %d, want 501", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "not_implemented" {
		t.Errorf("got code %v", body["code"])
	}
}

func TestGitNotesHandler_getCommitNotes_501_NotGitRepo(t *testing.T) {
	notRepo := t.TempDir()
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/commits/abc123?repo_path="+notRepo, "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("got status %d, want 501", w.Code)
	}
}

func TestGitNotesHandler_getCommitNotes_200(t *testing.T) {
	requireGit(t)
	repoPath, headSHA := makeTempGitRepoForREST(t)
	ref := gitnotes.RefForType(gitnotes.TypeDecision)
	if err := gitnotes.AddNote(repoPath, ref, headSHA, `{"v":1,"type":"decision","message":"test"}`); err != nil {
		t.Fatalf("add note: %v", err)
	}
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/commits/"+headSHA+"?repo_path="+repoPath+"&type=decision", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d body %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["commit_sha"] != headSHA {
		t.Errorf("commit_sha: got %v", out["commit_sha"])
	}
	notes, _ := out["notes"].(map[string]interface{})
	if notes == nil {
		t.Fatal("notes missing")
	}
	if notes["decision"] != `{"v":1,"type":"decision","message":"test"}` {
		t.Errorf("notes.decision: got %v", notes["decision"])
	}
}

func TestGitNotesHandler_getLog_404_ProjectMissing(t *testing.T) {
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: nil},
		OrgSvc:    &mockOrgSvc{},
		AgentStore: &mockAgentStore{},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/log?repo_path=/tmp", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestGitNotesHandler_getLog_501_NoRepoPath(t *testing.T) {
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/log", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("got status %d, want 501", w.Code)
	}
}

func TestGitNotesHandler_getLog_200(t *testing.T) {
	requireGit(t)
	repoPath, headSHA := makeTempGitRepoForREST(t)
	ref := gitnotes.RefForType(gitnotes.TypeDecision)
	if err := gitnotes.AddNote(repoPath, ref, headSHA, `{"v":1,"type":"decision","message":"log test"}`); err != nil {
		t.Fatalf("add note: %v", err)
	}
	h := &GitNotesHandler{
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}},
	}
	r := gitNotesHandlerRouter(h)
	req := requestWithAgent(http.MethodGet, "http://test/orgs/org1/projects/proj1/git-notes/log?repo_path="+repoPath+"&type=decision&limit=10", "agent1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d body %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	entries, _ := out["entries"].([]interface{})
	if len(entries) < 1 {
		t.Errorf("expected at least one entry, got %d", len(entries))
	}
}
