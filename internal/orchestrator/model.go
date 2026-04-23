package orchestrator

import "time"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Role      Role      `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Thread struct {
	ProjectID string    `json:"project_id"`
	Messages  []Message `json:"messages"`
	Playbook  Playbook  `json:"playbook"`
}
