package rest

import (
	"context"
	"strings"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/sessions"
)

func sessionToGen(s *sessions.Session, now time.Time) generated.AgentSession {
	out := generated.AgentSession{
		Id:              s.ID,
		Harness:         string(s.Harness),
		ExternalId:      s.ExternalID,
		Origin:          string(s.Origin),
		Cwd:             s.CWD,
		Repo:            s.Repo,
		Branch:          s.Branch,
		Model:           s.Model,
		ReasoningEffort: s.ReasoningEffort,
		Title:           s.Title,
		FirstPrompt:     s.FirstPrompt,
		TranscriptPath:  s.TranscriptPath,
		TokensIn:        s.TokensIn,
		TokensOut:       s.TokensOut,
		PromptCount:     s.PromptCount,
		ToolCallCount:   s.ToolCallCount,
		StartedAt:       s.StartedAt,
		LastActivityAt:  s.LastActivityAt,
		EndedAt:         s.EndedAt,
		Status:          string(s.Status(now)),
		Links:           make([]generated.SessionLink, 0, len(s.Links)),
	}
	if s.ParentSessionID != "" {
		p := s.ParentSessionID
		out.ParentSessionId = &p
	}
	if len(s.Metadata) > 0 {
		m := s.Metadata
		out.Metadata = &m
	}
	for _, l := range s.Links {
		out.Links = append(out.Links, sessionLinkToGen(l))
	}
	return out
}

func sessionLinkToGen(l sessions.Link) generated.SessionLink {
	return generated.SessionLink{SessionId: l.SessionID, Kind: l.Kind, Ref: l.Ref, Source: l.Source, CreatedAt: l.CreatedAt}
}

func (s *StrictServer) requireSessions() *apierrors.StructuredError {
	if s.SessionsSvc == nil {
		return apierrors.New(apierrors.CodeInternal, "session tracking is not configured", false)
	}
	return nil
}

func (s *StrictServer) ListSessions(ctx context.Context, req generated.ListSessionsRequestObject) (generated.ListSessionsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.ListSessions401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSessions(); err != nil {
		return nil, err
	}
	f := sessions.Filter{}
	p := req.Params
	if p.Harness != nil {
		f.Harness = sessions.Harness(*p.Harness)
	}
	if p.Origin != nil {
		f.Origin = sessions.Origin(*p.Origin)
	}
	if p.Repo != nil {
		f.Repo = strings.TrimSpace(*p.Repo)
	}
	if p.Branch != nil {
		f.Branch = strings.TrimSpace(*p.Branch)
	}
	if p.Status != nil {
		f.Status = sessions.Status(*p.Status)
	}
	if p.Q != nil {
		f.Query = *p.Q
	}
	if p.Since != nil {
		f.Since = p.Since
	}
	if p.Ref != nil {
		f.Ref = strings.TrimSpace(*p.Ref)
	}
	if p.IncludeSubagents != nil {
		f.IncludeSubagents = *p.IncludeSubagents
	}
	if p.Limit != nil {
		f.Limit = *p.Limit
	}
	if p.Offset != nil && *p.Offset > 0 {
		f.Offset = *p.Offset
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	list, total, err := s.SessionsSvc.List(ctx, f)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	now := time.Now()
	out := generated.SessionListResponse{Sessions: make([]generated.AgentSession, 0, len(list)), Total: total, Limit: f.Limit, Offset: f.Offset}
	for _, sess := range list {
		out.Sessions = append(out.Sessions, sessionToGen(sess, now))
	}
	return generated.ListSessions200JSONResponse(out), nil
}

func (s *StrictServer) GetSession(ctx context.Context, req generated.GetSessionRequestObject) (generated.GetSessionResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetSession401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSessions(); err != nil {
		return nil, err
	}
	sess, err := s.SessionsSvc.Get(ctx, req.SessionID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if sess == nil {
		return generated.GetSession404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "session not found", false))), nil
	}
	prompts, err := s.SessionsSvc.Prompts(ctx, sess.ID, 500)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	children, err := s.SessionsSvc.Children(ctx, sess.ID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	now := time.Now()
	detail := generated.SessionDetail{
		Session:  sessionToGen(sess, now),
		Prompts:  make([]generated.SessionPrompt, 0, len(prompts)),
		Children: make([]generated.AgentSession, 0, len(children)),
	}
	for _, p := range prompts {
		detail.Prompts = append(detail.Prompts, generated.SessionPrompt{Seq: p.Seq, Role: p.Role, Text: p.Text, Ts: p.TS})
	}
	for _, c := range children {
		detail.Children = append(detail.Children, sessionToGen(c, now))
	}
	return generated.GetSession200JSONResponse(detail), nil
}

func (s *StrictServer) CreateSessionLink(ctx context.Context, req generated.CreateSessionLinkRequestObject) (generated.CreateSessionLinkResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.CreateSessionLink401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSessions(); err != nil {
		return nil, err
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Ref) == "" {
		return generated.CreateSessionLink400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "kind and ref are required", false))), nil
	}
	sess, err := s.SessionsSvc.Get(ctx, req.SessionID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if sess == nil {
		return generated.CreateSessionLink404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "session not found", false))), nil
	}
	kind := string(req.Body.Kind)
	ref := strings.TrimSpace(req.Body.Ref)
	if err := s.SessionsSvc.Link(ctx, sess.ID, kind, ref, sessions.LinkSourceExplicit); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.CreateSessionLink201JSONResponse(generated.SessionLink{
		SessionId: sess.ID, Kind: kind, Ref: ref, Source: sessions.LinkSourceExplicit, CreatedAt: time.Now(),
	}), nil
}

func (s *StrictServer) GetSessionCollectorStatus(ctx context.Context, req generated.GetSessionCollectorStatusRequestObject) (generated.GetSessionCollectorStatusResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetSessionCollectorStatus401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireSessions(); err != nil {
		return nil, err
	}
	st := s.SessionsSvc.Status(ctx)
	out := generated.SessionCollectorStatus{
		Enabled:        st.Enabled,
		ClaudeDir:      st.ClaudeDir,
		CodexDir:       st.CodexDir,
		LastRunAt:      st.LastRunAt,
		LastDurationMs: st.LastDurationMS,
		SessionsTotal:  st.SessionsTotal,
		ByHarness:      st.ByHarness,
	}
	if st.LastError != "" {
		e := st.LastError
		out.LastError = &e
	}
	return generated.GetSessionCollectorStatus200JSONResponse(out), nil
}
