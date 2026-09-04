package rest

import (
	"context"
	"strings"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/internal/codereview"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

func messageToGen(m codereview.Message) generated.CodeReviewMessage {
	out := generated.CodeReviewMessage{Id: m.ID, ReviewId: m.ReviewID, Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt}
	if m.SessionID != "" {
		v := m.SessionID
		out.SessionId = &v
	}
	return out
}

// ListCodeReviewMessages returns the conversation on a review.
func (s *StrictServer) ListCodeReviewMessages(ctx context.Context, req generated.ListCodeReviewMessagesRequestObject) (generated.ListCodeReviewMessagesResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return nil, err
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	msgs, err := s.CodeReviewSvc.Messages(ctx, req.ReviewID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	var out struct {
		Messages []generated.CodeReviewMessage `json:"messages"`
	}
	out.Messages = []generated.CodeReviewMessage{}
	for _, m := range msgs {
		out.Messages = append(out.Messages, messageToGen(m))
	}
	return generated.ListCodeReviewMessages200JSONResponse(out), nil
}

// AskCodeReview forwards the operator's message to the reviewing agent.
func (s *StrictServer) AskCodeReview(ctx context.Context, req generated.AskCodeReviewRequestObject) (generated.AskCodeReviewResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return nil, err
	}
	if err := s.requireCodeReview(); err != nil {
		return nil, err
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Message) == "" {
		return generated.AskCodeReview400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "message is required", false))), nil
	}
	userMsg, reply, err := s.CodeReviewSvc.Ask(ctx, req.ReviewID, req.Body.Message)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return generated.AskCodeReview404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, err.Error(), false))), nil
		}
		return generated.AskCodeReview400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), true))), nil
	}
	var out struct {
		Messages []generated.CodeReviewMessage `json:"messages"`
	}
	out.Messages = []generated.CodeReviewMessage{}
	if userMsg != nil {
		out.Messages = append(out.Messages, messageToGen(*userMsg))
	}
	if reply != nil {
		out.Messages = append(out.Messages, messageToGen(*reply))
	}
	return generated.AskCodeReview200JSONResponse(out), nil
}

// ContinueSession resumes a tracked harness session with a new message.
func (s *StrictServer) ContinueSession(ctx context.Context, req generated.ContinueSessionRequestObject) (generated.ContinueSessionResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return nil, err
	}
	if s.SessionsSvc == nil {
		return nil, apierrors.New(apierrors.CodeInternal, "sessions are not configured", false)
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Message) == "" {
		return generated.ContinueSession400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, "message is required", false))), nil
	}
	reply, err := s.SessionsSvc.Continue(ctx, req.SessionID, req.Body.Message)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return generated.ContinueSession404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, err.Error(), false))), nil
		}
		return generated.ContinueSession400JSONResponse(seToGen(apierrors.New(apierrors.CodeInvalidInput, err.Error(), true))), nil
	}
	var out struct {
		Reply string `json:"reply"`
	}
	out.Reply = reply
	return generated.ContinueSession200JSONResponse(out), nil
}
