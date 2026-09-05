// Package runstatus tracks the harness invocations owned by this server process.
// Session recency is deliberately not used as evidence that a process is running.
package runstatus

import (
	"context"
	"github.com/google/uuid"
	"log/slog"
	"sort"
	"sync"
	"time"
)

type Run struct {
	ID                string
	Kind              string
	ProjectID         string
	TicketID          string
	ReviewID          string
	Ref               string
	Title             string
	Worker            string
	Harness           string
	Model             string
	WorkDir           string
	Prompt            string
	State             string
	SessionID         string
	ExternalSessionID string
	StartedAt         time.Time
	FinishedAt        *time.Time
	Progress          Progress
}
type contextKey struct{}
type infoKey struct{}
type Observer func(context.Context, Run) (string, error)
type Registry struct {
	mu       sync.RWMutex
	runs     map[string]Run
	observer Observer
}

var Default = New()

func New() *Registry                       { return &Registry{runs: make(map[string]Run)} }
func (r *Registry) SetObserver(f Observer) { r.mu.Lock(); defer r.mu.Unlock(); r.observer = f }
func WithInfo(ctx context.Context, info Run) context.Context {
	return context.WithValue(ctx, infoKey{}, info)
}
func Info(ctx context.Context) Run { v, _ := ctx.Value(infoKey{}).(Run); return v }
func (r *Registry) Begin(ctx context.Context, run Run) (context.Context, func()) {
	run.ID = uuid.NewString()
	run.StartedAt = time.Now().UTC()
	run.State = "starting"
	run.Progress = Progress{WorkerState: "waiting"}
	r.mu.Lock()
	r.runs[run.ID] = run
	r.mu.Unlock()
	h := &handle{registry: r, id: run.ID}
	return context.WithValue(ctx, contextKey{}, h), func() { h.finish(ctx) }
}
func (r *Registry) Snapshot() []Run {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Run, 0, len(r.runs))
	for _, v := range r.runs {
		v.Progress.Recent = append([]Activity(nil), v.Progress.Recent...)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

type handle struct {
	registry *Registry
	id       string
}

func Running(ctx context.Context) {
	if h, ok := ctx.Value(contextKey{}).(*handle); ok {
		h.registry.mu.Lock()
		if v, exists := h.registry.runs[h.id]; exists {
			v.State = "running"
			h.registry.runs[h.id] = v
		}
		h.registry.mu.Unlock()
	}
}
func Session(ctx context.Context, externalID string) {
	if externalID == "" {
		return
	}
	h, ok := ctx.Value(contextKey{}).(*handle)
	if !ok {
		return
	}
	h.registry.mu.Lock()
	v, exists := h.registry.runs[h.id]
	if !exists || v.ExternalSessionID == externalID {
		h.registry.mu.Unlock()
		return
	}
	v.ExternalSessionID = externalID
	h.registry.runs[h.id] = v
	h.registry.mu.Unlock()
	h.observe(ctx, v)
}
func (h *handle) observe(ctx context.Context, v Run) {
	h.registry.mu.RLock()
	observer := h.registry.observer
	h.registry.mu.RUnlock()
	if observer == nil || v.ExternalSessionID == "" {
		return
	}
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	id, err := observer(cctx, v)
	if err != nil {
		slog.Warn("record live harness session failed", "run", v.ID, "error", err)
		return
	}
	h.registry.mu.Lock()
	if current, ok := h.registry.runs[h.id]; ok {
		current.SessionID = id
		h.registry.runs[h.id] = current
	}
	h.registry.mu.Unlock()
}
func (h *handle) finish(ctx context.Context) {
	h.registry.mu.RLock()
	v, ok := h.registry.runs[h.id]
	h.registry.mu.RUnlock()
	if !ok {
		return
	}
	end := time.Now().UTC()
	v.FinishedAt = &end
	v.Progress.WorkerState = "exited"
	h.observe(ctx, v)
	h.registry.mu.Lock()
	delete(h.registry.runs, h.id)
	h.registry.mu.Unlock()
}
