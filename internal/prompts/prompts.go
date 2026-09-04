// Package prompts is the registry of built-in agent prompts (the "base prompt" of each
// default worker: code review, PR feedback, orchestrator, dispatch worker types, …).
// Services register their defaults at init and read the effective text at run time;
// the operator can override any of them from Settings → Prompts, live.
package prompts

import (
	"sort"
	"strings"
	"sync"
)

// Prompt is one editable built-in prompt.
type Prompt struct {
	ID          string
	Name        string
	Description string
	UsedBy      string // human-readable: which workflow uses it
	Order       int    // display order
	Default     string
}

// Effective is a Prompt with the operator's override applied.
type Effective struct {
	Prompt
	Override   string
	Text       string // what agents actually receive
	Customized bool
}

var (
	mu        sync.RWMutex
	registry  = map[string]Prompt{}
	overrides = map[string]string{}
)

// Register adds or replaces a built-in prompt definition.
func Register(p Prompt) {
	mu.Lock()
	defer mu.Unlock()
	registry[p.ID] = p
}

// SetOverrides replaces every override (empty values mean "use the default").
func SetOverrides(o map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	overrides = map[string]string{}
	for k, v := range o {
		if v = strings.TrimSpace(v); v != "" {
			overrides[k] = v
		}
	}
}

// Text returns the effective prompt for id (override, else default, else "").
func Text(id string) string {
	mu.RLock()
	defer mu.RUnlock()
	if v, ok := overrides[id]; ok && v != "" {
		return v
	}
	return registry[id].Default
}

// Get returns one effective prompt.
func Get(id string) (Effective, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[id]
	if !ok {
		return Effective{}, false
	}
	return effective(p), true
}

// All lists every registered prompt in display order.
func All() []Effective {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Effective, 0, len(registry))
	for _, p := range registry {
		out = append(out, effective(p))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func effective(p Prompt) Effective {
	o := overrides[p.ID]
	e := Effective{Prompt: p, Override: o, Text: p.Default}
	if o != "" {
		e.Text, e.Customized = o, true
	}
	return e
}
