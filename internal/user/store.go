package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists local operator identities.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts a user.
func (s *Store) Create(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = uuid.Must(uuid.NewV7()).String()
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, github_id, login, name, email, avatar_url, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.GitHubID, u.Login, u.Name, u.Email, u.AvatarURL, u.CreatedAt)
	return err
}

// GetByGitHubID returns a user by GitHub ID.
func (s *Store) GetByGitHubID(ctx context.Context, githubID int64) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, github_id, login, name, email, avatar_url, created_at FROM users WHERE github_id = $1`, githubID).
		Scan(&u.ID, &u.GitHubID, &u.Login, &u.Name, &u.Email, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByID returns a user by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, github_id, login, name, email, avatar_url, created_at FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.GitHubID, &u.Login, &u.Name, &u.Email, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
