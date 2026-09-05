package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// RunGrant is a short-lived capability for one harness invocation. It never
// exposes the long-lived operator key to the harness.
type RunGrant struct {
	ParentKey, TicketID, ProjectID, Role string
	PhaseID, PhaseEnteredAt              string
	Expires                              time.Time
}

var runKeys sync.Map

type runContextKey struct{}

func WithRun(ctx context.Context, g RunGrant) context.Context {
	return context.WithValue(ctx, runContextKey{}, g)
}
func RunFromContext(ctx context.Context) (RunGrant, bool) {
	g, ok := ctx.Value(runContextKey{}).(RunGrant)
	return g, ok
}
func IssueRun(g RunGrant) (string, func(), error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := "fwrun_" + hex.EncodeToString(b)
	g.Expires = time.Now().Add(time.Hour)
	runKeys.Store(token, g)
	return token, func() { runKeys.Delete(token) }, nil
}
func LookupRun(token string) (RunGrant, bool) {
	v, ok := runKeys.Load(token)
	if !ok {
		return RunGrant{}, false
	}
	g := v.(RunGrant)
	if time.Now().After(g.Expires) {
		runKeys.Delete(token)
		return RunGrant{}, false
	}
	return g, true
}

type operatorContextKey struct{}

func WithOperator(ctx context.Context) context.Context {
	return context.WithValue(ctx, operatorContextKey{}, true)
}
func IsOperator(ctx context.Context) bool { v, _ := ctx.Value(operatorContextKey{}).(bool); return v }
