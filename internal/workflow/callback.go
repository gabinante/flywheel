package workflow

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// CallbackStore is the persistence interface for callback tokens.
type CallbackStore interface {
	StoreCallbackToken(ctx context.Context, token, ticketID, workflowID, phaseID string, expiresAt time.Time) error
	LookupCallbackToken(ctx context.Context, token string) (ticketID, workflowID, phaseID string, err error)
	DeleteCallbackToken(ctx context.Context, token string) error
}

// CallbackHandler manages async callback tokens for external phases.
type CallbackHandler struct {
	secret []byte
	store  CallbackStore
	engine *Engine
}

// NewCallbackHandler creates a new callback handler.
func NewCallbackHandler(secret []byte, store CallbackStore, engine *Engine) *CallbackHandler {
	return &CallbackHandler{secret: secret, store: store, engine: engine}
}

// Store returns the underlying callback store.
func (h *CallbackHandler) Store() CallbackStore {
	return h.store
}

// GenerateToken creates a signed callback token for a ticket's current phase.
func (h *CallbackHandler) GenerateToken(ctx context.Context, ticketID, workflowID, phaseID string, ttl time.Duration) (string, error) {
	data := fmt.Sprintf("%s:%s:%s:%d", ticketID, workflowID, phaseID, time.Now().UnixNano())
	mac := hmac.New(sha256.New, h.secret)
	mac.Write([]byte(data))
	token := hex.EncodeToString(mac.Sum(nil))

	expiresAt := time.Now().Add(ttl)
	if err := h.store.StoreCallbackToken(ctx, token, ticketID, workflowID, phaseID, expiresAt); err != nil {
		return "", fmt.Errorf("store callback token: %w", err)
	}
	return token, nil
}

// HandleCallback processes an incoming callback, advancing the workflow phase.
func (h *CallbackHandler) HandleCallback(ctx context.Context, token string, outcome string, metadata map[string]any) error {
	if outcome == "" {
		outcome = "success"
	}
	if outcome != "success" && outcome != "failed" {
		return errors.New("outcome must be 'success' or 'failed'")
	}

	ticketID, workflowID, phaseID, err := h.store.LookupCallbackToken(ctx, token)
	if err != nil {
		return fmt.Errorf("lookup callback token: %w", err)
	}

	// Delete the token so it can't be replayed
	if err := h.store.DeleteCallbackToken(ctx, token); err != nil {
		return fmt.Errorf("delete callback token: %w", err)
	}

	_, err = h.engine.AdvancePhase(ctx, ticketID, workflowID, phaseID, outcome, metadata, 0)
	if err != nil {
		return fmt.Errorf("advance phase: %w", err)
	}
	return nil
}
