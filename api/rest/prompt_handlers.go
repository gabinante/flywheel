package rest

import (
	"context"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/prompts"
)

func promptToGen(e prompts.Effective) generated.PromptDefinition {
	return generated.PromptDefinition{Id: e.ID, Name: e.Name, Description: e.Description, UsedBy: e.UsedBy, DefaultText: e.Default, Override: e.Override, Text: e.Text, Customized: e.Customized}
}

// ListPrompts returns every built-in prompt with its effective text.
func (s *StrictServer) ListPrompts(ctx context.Context, _ generated.ListPromptsRequestObject) (generated.ListPromptsResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.ListPrompts401JSONResponse(seToGen(err)), nil
	}
	var out struct {
		Items []generated.PromptDefinition `json:"items"`
	}
	out.Items = []generated.PromptDefinition{}
	for _, e := range prompts.All() {
		out.Items = append(out.Items, promptToGen(e))
	}
	return generated.ListPrompts200JSONResponse(out), nil
}

// UpdatePrompt overrides (or, with empty text, resets) one built-in prompt.
func (s *StrictServer) UpdatePrompt(ctx context.Context, req generated.UpdatePromptRequestObject) (generated.UpdatePromptResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return nil, err
	}
	if err := s.requireSettings(); err != nil {
		return nil, err
	}
	if _, ok := prompts.Get(req.PromptID); !ok {
		return generated.UpdatePrompt404JSONResponse(seToGen(apierrors.New(apierrors.CodeNotFound, "unknown prompt", false))), nil
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "request body is required", false)
	}
	if _, err := s.SettingsSvc.UpdatePrompt(ctx, req.PromptID, req.Body.Text); err != nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false)
	}
	e, _ := prompts.Get(req.PromptID)
	return generated.UpdatePrompt200JSONResponse(promptToGen(e)), nil
}
