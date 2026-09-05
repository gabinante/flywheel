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
	secret     []byte
	store      CallbackStore
	engine     *Engine
	onComplete func(context.Context, string) error
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
	if tx, ok := h.store.(interface {
		Transaction(context.Context, func(context.Context) error) error
	}); ok {
		return tx.Transaction(ctx, func(ctx context.Context) error { return h.handleCallback(ctx, token, outcome, metadata) })
	}
	return h.handleCallback(ctx, token, outcome, metadata)
}
func (h *CallbackHandler) handleCallback(ctx context.Context, token, outcome string, metadata map[string]any) error {
	version := 0
	if st, ok := h.store.(interface {
		Attempt(context.Context, string) (int, string, error)
	}); ok {
		v, entered, err := st.Attempt(ctx, token)
		if err != nil {
			return err
		}
		version = v
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["phase_entered_at"] = entered
	}

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

	next, err := h.engine.AdvancePhase(ctx, ticketID, workflowID, phaseID, outcome, metadata, version)
	if err != nil {
		return fmt.Errorf("advance phase: %w", err)
	}
	if next == nil && h.onComplete != nil {
		if err := h.onComplete(ctx, ticketID); err != nil {
			return err
		}
	}
	return h.store.DeleteCallbackToken(ctx, token)
}

func (h *CallbackHandler) SetCompletion(fn func(context.Context, string) error) { h.onComplete = fn }
