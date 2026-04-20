package rest

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/gitnotes"
)

// GitNotesHandler handles REST endpoints for Flywheel git notes (read-only).
// Tenancy: all endpoints require project access. Repo path is passed as query param repo_path;
// if missing or not accessible, returns 501.
type GitNotesHandler struct {
	ProjectSvc ProjectGetterForAccess
	OrgSvc     OrgMemberLister
	AgentStore AgentGetter
}

func (h *GitNotesHandler) getCommitNotes(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	projectID := PathParam(r, "projectID")
	commitSha := PathParam(r, "commitSha")
	repoPath := r.URL.Query().Get("repo_path")
	noteType := r.URL.Query().Get("type")

	proj, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if proj == nil || proj.OrgID != orgID {
		WriteStructuredError(w, apierrors.New(apierrors.CodeNotFound, "project not found", false))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	if !repoPathAccessible(repoPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "repo_path query param required and must point to an accessible git repo (server does not have the repo)",
			"code":  "not_implemented",
		})
		return
	}

	out := map[string]any{"commit_sha": commitSha, "notes": make(map[string]string)}
	if noteType != "" {
		ref := gitnotes.RefForType(noteType)
		if ref == "" {
			WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false))
			return
		}
		body, err := gitnotes.ShowNote(repoPath, ref, commitSha)
		if err != nil {
			WriteStructuredError(w, apierrors.MapError(err))
			return
		}
		out["notes"].(map[string]string)[filepath.Base(ref)] = body
	} else {
		for _, ref := range gitnotes.AllRefs() {
			body, _ := gitnotes.ShowNote(repoPath, ref, commitSha)
			if body != "" {
				out["notes"].(map[string]string)[filepath.Base(ref)] = body
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (h *GitNotesHandler) getLog(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	projectID := PathParam(r, "projectID")
	repoPath := r.URL.Query().Get("repo_path")
	noteType := r.URL.Query().Get("type")
	if noteType == "" {
		noteType = gitnotes.TypeDecision
	}
	limit := 20
	if n := r.URL.Query().Get("limit"); n != "" {
		if i, err := strconv.Atoi(n); err == nil && i > 0 {
			limit = i
		}
	}

	proj, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if proj == nil || proj.OrgID != orgID {
		WriteStructuredError(w, apierrors.New(apierrors.CodeNotFound, "project not found", false))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	if !repoPathAccessible(repoPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "repo_path query param required and must point to an accessible git repo (server does not have the repo)",
			"code":  "not_implemented",
		})
		return
	}

	ref := gitnotes.RefForType(noteType)
	if ref == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false))
		return
	}
	entries, err := gitnotes.Log(repoPath, ref, limit)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	list := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		list = append(list, map[string]any{"commit_sha": e.CommitSHA, "ref": e.Ref, "body": e.Body})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"entries": list})
}

func repoPathAccessible(repoPath string) bool {
	if repoPath == "" {
		return false
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(abs, ".git"))
	return err == nil
}
