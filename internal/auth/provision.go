package auth

import (
	"context"
	"errors"
	"time"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/user"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Provisioner creates or finds the user and agent for an operator identity.
type Provisioner struct {
	UserStore  *user.Store
	AgentStore *agent.Store
}

// Provision finds the user by identity ID, or creates user + agent. Returns user and agent.
func (p *Provisioner) Provision(ctx context.Context, gh *Identity) (*user.User, *agent.Agent, error) {
	u, err := p.UserStore.GetByGitHubID(ctx, gh.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	now := time.Now().UTC()
	if errors.Is(err, pgx.ErrNoRows) {
		u = &user.User{
			ID: uuid.Must(uuid.NewV7()).String(), GitHubID: gh.ID,
			Login: gh.Login, Name: gh.Name, Email: gh.Email, AvatarURL: gh.AvatarURL,
			CreatedAt: now,
		}
		if err := p.UserStore.Create(ctx, u); err != nil {
			return nil, nil, err
		}
	} else {
		a, err := p.AgentStore.GetByUserID(ctx, u.ID)
		if err == nil {
			return u, a, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, err
		}
		// A previous attempt may have created the user but not its linked agent.
	}
	name := gh.Login
	if gh.Name != "" {
		name = gh.Name
	}
	a := &agent.Agent{
		ID:        uuid.Must(uuid.NewV7()).String(),
		UserID:    u.ID,
		Name:      name,
		Type:      agent.TypeCustom,
		APIKey:    "", // local operator identity; harnesses use API keys
		CreatedAt: now,
	}
	if err := p.AgentStore.Create(ctx, a); err != nil {
		return nil, nil, err
	}
	return u, a, nil
}
