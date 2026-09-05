package agent

import "time"

// Type is the kind of agent (IDE, Claude Teams, CI, etc.).
type Type string

const (
	TypeClaude Type = "claude"
	TypeCustom Type = "custom"
	TypeCI     Type = "ci"
)

// Agent is a client identity that can claim and work tickets.
// May be linked to a local user or use APIKey for headless/CI.
type Agent struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"`
	Name      string    `json:"name"`
	Type      Type      `json:"type"`
	APIKey    string    `json:"-"` // never serialized; empty if the local operator identity
	CreatedAt time.Time `json:"created_at"`
}
