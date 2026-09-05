// Package activity provides ephemeral UI invalidations. REST remains the source
// of truth; subscribers resync on every connection instead of replaying history.
package activity

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Topics are deliberately coarse and contain no row data, prompts, or secrets.
const (
	Orchestrator = "orchestrator"
	Reviews      = "reviews"
	ReviewStatus = "review-status"
	PRs          = "prs"
	Sessions     = "sessions"
	Collector    = "collector"
	Runs         = "runs"
	Tickets      = "tickets"
	Projects     = "projects"
	Workflows    = "workflows"
	Settings     = "settings"
	Dispatch     = "dispatch"
)

var topics = map[string]bool{Orchestrator: true, Reviews: true, ReviewStatus: true, PRs: true, Sessions: true, Collector: true, Runs: true, Tickets: true, Projects: true, Workflows: true, Settings: true, Dispatch: true}

type Event struct {
	Topics []string `json:"topics"`
	Resync bool     `json:"resync"`
	Ready  bool     `json:"ready"`
}

type Hub struct {
	mu                    sync.Mutex
	clients               map[chan Event]struct{}
	pending               map[string]bool
	resync, ready, closed bool
}

func New() *Hub { return &Hub{clients: make(map[chan Event]struct{}), pending: make(map[string]bool)} }

// Publish is safe on a harness output path: it only marks a bounded set of topics.
func (h *Hub) Publish(names ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, name := range names {
		if topics[name] {
			h.pending[name] = true
		}
	}
}

// SourceReady forces a snapshot refresh after LISTEN starts/reconnects. While the
// source is unavailable the browser keeps the stream but falls back to polling.
func (h *Hub) SourceReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready, h.resync = ready, true
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan Event, 1)
	if h.closed {
		close(ch)
		return ch, func() {}
	}
	h.clients[ch] = struct{}{}
	ch <- Event{Topics: []string{}, Resync: true, Ready: h.ready}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
	}
}

func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	defer h.close()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.flush()
		}
	}
}

func (h *Hub) flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.pending) == 0 && !h.resync {
		return
	}
	e := Event{Topics: make([]string, 0, len(h.pending)), Resync: h.resync, Ready: h.ready}
	for name := range h.pending {
		e.Topics = append(e.Topics, name)
	}
	sort.Strings(e.Topics)
	clear(h.pending)
	h.resync = false
	for ch := range h.clients {
		next := e
		// A slow subscriber gets the union of missed invalidations, never an
		// unbounded backlog or a dropped topic. Writers never block producers.
		select {
		case prev := <-ch:
			next.Resync = next.Resync || prev.Resync
			merged := make(map[string]bool, len(topics))
			for _, name := range prev.Topics {
				merged[name] = true
			}
			for _, name := range next.Topics {
				merged[name] = true
			}
			next.Topics = make([]string, 0, len(merged))
			for name := range merged {
				next.Topics = append(next.Topics, name)
			}
			sort.Strings(next.Topics)
		default:
		}
		ch <- next
	}
}

func (h *Hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
}
