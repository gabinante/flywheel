package rest

import (
	"context"
	"strings"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/internal/codereview"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

func findingToGen(f codereview.Finding) generated.CodeReviewFinding {
	out := generated.CodeReviewFinding{Id: f.ID, Attempt: f.Attempt, Severity: f.Severity, Path: f.Path, Line: f.Line, Title: f.Title, Body: f.Body, Status: f.Status}
	if f.GitHubCommentID != 0 {
		id := f.GitHubCommentID
		out.GithubCommentId = &id
	}
	return out
}

func codeReviewToGen(r *codereview.Request) generated.CodeReviewRequest {
	out := generated.CodeReviewRequest{
		Id: r.ID, Repo: r.Repo, Number: r.Number, Url: r.URL, Title: r.Title, Author: r.Author, Origin: string(r.Origin), Harness: r.Harness,
		State: string(r.State), Attempt: r.Attempt, Watch: r.Watch, DryRun: r.DryRun, Verdict: r.Verdict, Summary: r.Summary,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Findings: make([]generated.CodeReviewFinding, 0, len(r.Findings)),
		ReviewedAt: r.ReviewedAt, LastCheckedAt: r.LastCheckedAt,
	}
	setOpt := func(dst **string, v string) {
		if v != "" {
			s := v
			*dst = &s
		}
	}
	setOpt(&out.BaseRef, r.BaseRef)
	setOpt(&out.HeadRef, r.HeadRef)
	setOpt(&out.HeadSha, r.HeadSHA)
	setOpt(&out.Recipe, r.Recipe)
	setOpt(&out.Model, r.Model)
	setOpt(&out.ReviewUrl, r.ReviewURL)
	setOpt(&out.MyReviewState, r.MyReviewState)
	setOpt(&out.SessionId, r.SessionID)
	setOpt(&out.Error, r.Error)
	for _, f := range r.Findings {
		out.Findings = append(out.Findings, findingToGen(f))
	}
	return out
}

func feedbackToGen(r *codereview.FeedbackRound) generated.FeedbackRound {
	out := generated.FeedbackRound{Id: r.ID, Repo: r.Repo, Number: r.Number, Url: r.URL, Title: r.Title, Reviewer: r.Reviewer, ReviewState: r.ReviewState,
		ReviewId: r.ReviewID, CommentCount: r.CommentCount, State: r.State, ObservedAt: r.ObservedAt, SubmittedAt: r.SubmittedAt}
	if r.HeadSHA != "" {
		v := r.HeadSHA
		out.HeadSha = &v
	}
	if r.Body != "" {
		v := r.Body
		out.Body = &v
	}
	if r.TicketID != "" {
		v := r.TicketID
		out.TicketId = &v
	}
	if r.SessionID != "" {
		v := r.SessionID
		out.SessionId = &v
	}
	return out
}

func (s *StrictServer) requireCodeReview() *apierrors.StructuredError {
	if s.CodeReviewSvc == nil {
		return apierrors.New(apierrors.CodeInternal, "code review is not configured", false)
	}
	return nil
}

func (s *StrictServer) ListCodeReviews(ctx context.Context, req generated.ListCodeReviewsRequestObject) (generated.ListCodeReviewsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.ListCodeReviews401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	f := codereview.Filter{}
	if req.Params.ProjectId != nil {
		f.ProjectID = *req.Params.ProjectId
		if err := CheckProjectAccess(ctx, f.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
			return nil, err
		}
	}
	if req.Params.State != nil {
		f.State = codereview.State(*req.Params.State)
	}
	if req.Params.Repo != nil {
		f.Repo = *req.Params.Repo
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Offset != nil {
		f.Offset = *req.Params.Offset
	}
	if f.Limit <= 0 {
		f.Limit = 100
	}
	list, total, err := s.CodeReviewSvc.List(ctx, f)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := generated.CodeReviewListResponse{Requests: make([]generated.CodeReviewRequest, 0, len(list)), Total: total, Limit: f.Limit, Offset: f.Offset}
	for _, r := range list {
		out.Requests = append(out.Requests, codeReviewToGen(r))
	}
	return generated.ListCodeReviews200JSONResponse(out), nil
}

func (s *StrictServer) CreateCodeReviews(ctx context.Context, req generated.CreateCodeReviewsRequestObject) (generated.CreateCodeReviewsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.CreateCodeReviews401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Text) == "" {
		return generated.CreateCodeReviews400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "text is required", false))), nil
	}
	opts := codereview.EnqueueOptions{DryRun: req.Body.DryRun, Watch: req.Body.Watch}
	if req.Body.Harness != nil {
		opts.Harness = *req.Body.Harness
	}
	reqs, err := s.CodeReviewSvc.Enqueue(ctx, req.Body.Text, codereview.OriginPaste, opts)
	if err != nil {
		return generated.CreateCodeReviews400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))), nil
	}
	out := generated.CodeReviewListResponse{Requests: make([]generated.CodeReviewRequest, 0, len(reqs)), Total: len(reqs), Limit: len(reqs)}
	for _, r := range reqs {
		out.Requests = append(out.Requests, codeReviewToGen(r))
	}
	return generated.CreateCodeReviews201JSONResponse(out), nil
}

func (s *StrictServer) GetCodeReviewStatus(ctx context.Context, req generated.GetCodeReviewStatusRequestObject) (generated.GetCodeReviewStatusResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetCodeReviewStatus401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	st := s.CodeReviewSvc.Status(ctx)
	out := generated.CodeReviewStatus{Enabled: st.Enabled, Harness: st.Harness, Publish: st.Publish, WatchRequested: st.WatchRequested, WatchAuthored: st.WatchAuthored,
		LastQueueRunAt: st.LastQueueRunAt, LastPollAt: st.LastPollAt, Active: st.Active, Queued: st.Queued, Watching: st.Watching, NewFeedback: st.NewFeedback,
		ReviewsPosted: st.ReviewsPosted, MaxConcurrent: st.MaxConcurrent}
	if st.Login != "" {
		v := st.Login
		out.Login = &v
	}
	if st.LastError != "" {
		v := st.LastError
		out.LastError = &v
	}
	if st.RepoRoot != "" {
		v := st.RepoRoot
		out.RepoRoot = &v
	}
	return generated.GetCodeReviewStatus200JSONResponse(out), nil
}

func (s *StrictServer) ListFeedbackRounds(ctx context.Context, req generated.ListFeedbackRoundsRequestObject) (generated.ListFeedbackRoundsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.ListFeedbackRounds401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	projectID := ""
	if req.Params.ProjectId != nil {
		projectID = *req.Params.ProjectId
		if err := CheckProjectAccess(ctx, projectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
			return nil, err
		}
	}
	state := ""
	if req.Params.State != nil {
		state = *req.Params.State
	}
	limit := 100
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	rounds, err := s.CodeReviewSvc.FeedbackRounds(ctx, state, limit, projectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	out := generated.FeedbackRoundListResponse{Rounds: make([]generated.FeedbackRound, 0, len(rounds))}
	for _, r := range rounds {
		out.Rounds = append(out.Rounds, feedbackToGen(r))
	}
	return generated.ListFeedbackRounds200JSONResponse(out), nil
}

func (s *StrictServer) SetFeedbackRoundState(ctx context.Context, req generated.SetFeedbackRoundStateRequestObject) (generated.SetFeedbackRoundStateResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.SetFeedbackRoundState401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "state is required", false)
	}
	r, err := s.CodeReviewSvc.SetFeedbackState(ctx, req.RoundID, string(req.Body.State), "", "")
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if r == nil {
		return generated.SetFeedbackRoundState404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "feedback round not found", false))), nil
	}
	return generated.SetFeedbackRoundState200JSONResponse(feedbackToGen(r)), nil
}

func (s *StrictServer) GetCodeReview(ctx context.Context, req generated.GetCodeReviewRequestObject) (generated.GetCodeReviewResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetCodeReview401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	r, err := s.CodeReviewSvc.Get(ctx, req.ReviewID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if r == nil {
		return generated.GetCodeReview404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "review request not found", false))), nil
	}
	return generated.GetCodeReview200JSONResponse(codeReviewToGen(r)), nil
}

func (s *StrictServer) RerunCodeReview(ctx context.Context, req generated.RerunCodeReviewRequestObject) (generated.RerunCodeReviewResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.RerunCodeReview401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	r, err := s.CodeReviewSvc.Rerun(ctx, req.ReviewID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if r == nil {
		return generated.RerunCodeReview404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "review request not found", false))), nil
	}
	return generated.RerunCodeReview200JSONResponse(codeReviewToGen(r)), nil
}

func (s *StrictServer) CloseCodeReview(ctx context.Context, req generated.CloseCodeReviewRequestObject) (generated.CloseCodeReviewResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.CloseCodeReview401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	r, err := s.CodeReviewSvc.Close(ctx, req.ReviewID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if r == nil {
		return generated.CloseCodeReview404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "review request not found", false))), nil
	}
	return generated.CloseCodeReview200JSONResponse(codeReviewToGen(r)), nil
}

func (s *StrictServer) AddressFeedbackRound(ctx context.Context, req generated.AddressFeedbackRoundRequestObject) (generated.AddressFeedbackRoundResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.AddressFeedbackRound401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	r, err := s.CodeReviewSvc.AddressFeedback(ctx, req.RoundID)
	if err != nil {
		if r != nil {
			return generated.AddressFeedbackRound409JSONResponse(seToGen(apierrors.New(apierrors.CodeConflict, err.Error(), false))), nil
		}
		return nil, apierrors.MapError(err)
	}
	if r == nil {
		return generated.AddressFeedbackRound404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "feedback round not found", false))), nil
	}
	return generated.AddressFeedbackRound202JSONResponse(feedbackToGen(r)), nil
}
