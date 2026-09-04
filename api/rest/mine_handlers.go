package rest

import (
	"context"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/internal/codereview"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

func prCardToGen(c codereview.PRCard) generated.PullRequestCard {
	out := generated.PullRequestCard{
		Repo: c.Repo, Number: c.Number, Title: c.Title, Url: c.URL, Author: c.Author, IsDraft: c.IsDraft, State: c.State,
		HeadRef: c.HeadRef, BaseRef: c.BaseRef, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, MergedAt: c.MergedAt,
		ReviewDecision: c.ReviewDecision, Checks: c.Checks, Mergeable: c.Mergeable, Additions: c.Additions, Deletions: c.Deletions,
		ChangedFiles: c.ChangedFiles, Labels: nonNil(c.Labels), RequestedReviewers: nonNil(c.RequestedReviewers),
		MyReviewState: c.MyReviewState, ReviewRequestedFromMe: c.ReviewRequestedFromMe, LinearRefs: nonNil(c.LinearRefs), Sessions: c.Sessions,
		Reviews: []generated.ReviewerState{},
	}
	for _, r := range c.Reviews {
		out.Reviews = append(out.Reviews, generated.ReviewerState{Login: r.Login, State: r.State, SubmittedAt: r.SubmittedAt})
	}
	if c.Review != nil {
		g := codeReviewToGen(c.Review)
		out.Review = &g
	}
	if c.Feedback != nil {
		out.Feedback = &generated.FeedbackDigest{
			New: c.Feedback.New, Dispatched: c.Feedback.Dispatched, Addressed: c.Feedback.Addressed,
			LastReviewer: c.Feedback.LastReviewer, LastState: c.Feedback.LastState, LastAt: c.Feedback.LastAt, LatestId: c.Feedback.LatestID,
		}
	}
	return out
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func prCardsToGen(cs []codereview.PRCard) []generated.PullRequestCard {
	out := make([]generated.PullRequestCard, 0, len(cs))
	for _, c := range cs {
		out = append(out, prCardToGen(c))
	}
	return out
}

// GetMyPullRequests lists the operator's PRs across repos.
func (s *StrictServer) GetMyPullRequests(ctx context.Context, req generated.GetMyPullRequestsRequestObject) (generated.GetMyPullRequestsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetMyPullRequests401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	force := req.Params.Refresh != nil && *req.Params.Refresh
	res, err := s.CodeReviewSvc.MyPRs(ctx, force)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInternal, "github: "+err.Error(), true)
	}
	return generated.GetMyPullRequests200JSONResponse(generated.MyPullRequests{
		Login: res.Login, FetchedAt: res.FetchedAt, Open: prCardsToGen(res.Open), Merged: prCardsToGen(res.Merged),
	}), nil
}

// GetMyReviews lists PRs waiting on, or reviewed by, the operator.
func (s *StrictServer) GetMyReviews(ctx context.Context, req generated.GetMyReviewsRequestObject) (generated.GetMyReviewsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetMyReviews401JSONResponse(seToGen(err)), nil
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	force := req.Params.Refresh != nil && *req.Params.Refresh
	res, err := s.CodeReviewSvc.MyReviewsOverview(ctx, force)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInternal, "github: "+err.Error(), true)
	}
	return generated.GetMyReviews200JSONResponse(generated.MyReviews{
		Login: res.Login, FetchedAt: res.FetchedAt, Requested: prCardsToGen(res.Requested), Reviewed: prCardsToGen(res.Reviewed),
	}), nil
}
