package codereview

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mustUUID() string { return uuid.Must(uuid.NewV7()).String() }

// Store persists review requests, findings, and feedback rounds.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const reqCols = `id, repo, number, url, title, author, base_ref, head_ref, head_sha, origin, recipe, harness, model, reasoning_effort,
	state, attempt, watch, dry_run, verdict, summary, review_url, my_review_state, my_review_id, last_reviewed_head_sha,
	session_id, session_external_id, worktree_path, ticket_id, error, last_checked_at, reviewed_at, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanRequest(r rowScanner) (*Request, error) {
	var q Request
	if err := r.Scan(&q.ID, &q.Repo, &q.Number, &q.URL, &q.Title, &q.Author, &q.BaseRef, &q.HeadRef, &q.HeadSHA, &q.Origin, &q.Recipe,
		&q.Harness, &q.Model, &q.ReasoningEffort, &q.State, &q.Attempt, &q.Watch, &q.DryRun, &q.Verdict, &q.Summary, &q.ReviewURL,
		&q.MyReviewState, &q.MyReviewID, &q.LastReviewedHeadSHA, &q.SessionID, &q.SessionExternalID, &q.WorktreePath, &q.TicketID,
		&q.Error, &q.LastCheckedAt, &q.ReviewedAt, &q.CreatedAt, &q.UpdatedAt); err != nil {
		return nil, err
	}
	return &q, nil
}

// Create inserts a new request.
func (s *Store) Create(ctx context.Context, q *Request) error {
	if q.ID == "" {
		q.ID = mustUUID()
	}
	if q.State == "" {
		q.State = StateQueued
	}
	if q.Attempt == 0 {
		q.Attempt = 1
	}
	if q.Recipe == "" {
		q.Recipe = RecipeInlineP1Gate
	}
	return s.pool.QueryRow(ctx, `INSERT INTO code_review_requests
		(id, repo, number, url, title, author, base_ref, head_ref, head_sha, origin, recipe, harness, model, reasoning_effort, state, attempt, watch, dry_run, ticket_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING created_at, updated_at`,
		q.ID, q.Repo, q.Number, q.URL, q.Title, q.Author, q.BaseRef, q.HeadRef, q.HeadSHA, string(q.Origin), q.Recipe, q.Harness, q.Model,
		q.ReasoningEffort, string(q.State), q.Attempt, q.Watch, q.DryRun, q.TicketID).Scan(&q.CreatedAt, &q.UpdatedAt)
}

// Update writes every mutable column.
func (s *Store) Update(ctx context.Context, q *Request) error {
	_, err := s.pool.Exec(ctx, `UPDATE code_review_requests SET url=$2, title=$3, author=$4, base_ref=$5, head_ref=$6, head_sha=$7, origin=$8,
		harness=$9, model=$10, reasoning_effort=$11, state=$12, attempt=$13, watch=$14, dry_run=$15, verdict=$16, summary=$17, review_url=$18,
		my_review_state=$19, my_review_id=$20, last_reviewed_head_sha=$21, session_id=$22, session_external_id=$23, worktree_path=$24,
		ticket_id=$25, error=$26, last_checked_at=$27, reviewed_at=$28, updated_at=now() WHERE id=$1`,
		q.ID, q.URL, q.Title, q.Author, q.BaseRef, q.HeadRef, q.HeadSHA, string(q.Origin), q.Harness, q.Model, q.ReasoningEffort, string(q.State),
		q.Attempt, q.Watch, q.DryRun, q.Verdict, q.Summary, q.ReviewURL, q.MyReviewState, q.MyReviewID, q.LastReviewedHeadSHA, q.SessionID,
		q.SessionExternalID, q.WorktreePath, q.TicketID, q.Error, q.LastCheckedAt, q.ReviewedAt)
	return err
}

// Get returns a request with its findings for the current attempt, or nil.
func (s *Store) Get(ctx context.Context, id string) (*Request, error) {
	q, err := scanRequest(s.pool.QueryRow(ctx, `SELECT `+reqCols+` FROM code_review_requests WHERE id = $1`, id))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q.Findings, err = s.ListFindings(ctx, q.ID, 0)
	return q, err
}

// GetByRepoNumber returns the request for a PR, or nil.
func (s *Store) GetByRepoNumber(ctx context.Context, repo string, number int) (*Request, error) {
	q, err := scanRequest(s.pool.QueryRow(ctx, `SELECT `+reqCols+` FROM code_review_requests WHERE repo = $1 AND number = $2`, repo, number))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return q, err
}

// List returns requests ordered by recency plus the total count.
func (s *Store) List(ctx context.Context, f Filter) ([]*Request, int, error) {
	var where []string
	var args []any
	if f.State != "" {
		args = append(args, string(f.State))
		where = append(where, fmt.Sprintf("state = $%d", len(args)))
	}
	if f.Repo != "" {
		args = append(args, f.Repo)
		where = append(where, fmt.Sprintf("repo = $%d", len(args)))
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM code_review_requests`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit, f.Offset)
	rows, err := s.pool.Query(ctx, `SELECT `+reqCols+` FROM code_review_requests`+clause+
		fmt.Sprintf(` ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Request
	for rows.Next() {
		q, err := scanRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, q)
	}
	return out, total, rows.Err()
}

// ListByState returns all requests in a state.
func (s *Store) ListByState(ctx context.Context, st State) ([]*Request, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+reqCols+` FROM code_review_requests WHERE state = $1 ORDER BY updated_at`, string(st))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Request
	for rows.Next() {
		q, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// ClaimQueued atomically moves up to n queued requests to fetching and returns them.
func (s *Store) ClaimQueued(ctx context.Context, n int) ([]*Request, error) {
	rows, err := s.pool.Query(ctx, `WITH picked AS (
			SELECT id FROM code_review_requests WHERE state = 'queued' ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		UPDATE code_review_requests r SET state = 'fetching', updated_at = now() FROM picked WHERE r.id = picked.id
		RETURNING `+prefixCols("r.", reqCols), n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Request
	for rows.Next() {
		q, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// Requeue schedules another review attempt.
func (s *Store) Requeue(ctx context.Context, id string, origin Origin) error {
	_, err := s.pool.Exec(ctx, `UPDATE code_review_requests SET state = 'queued', origin = $2, attempt = attempt + 1, error = '', verdict = '',
		summary = '', review_url = '', updated_at = now() WHERE id = $1`, id, string(origin))
	return err
}

// CountsByState returns request counts per state.
func (s *Store) CountsByState(ctx context.Context) (map[State]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT state, count(*) FROM code_review_requests GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[State]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[State(st)] = n
	}
	return out, rows.Err()
}

// ReplaceFindings stores the findings for an attempt (deleting any previous rows for that attempt).
func (s *Store) ReplaceFindings(ctx context.Context, requestID string, attempt int, findings []Finding) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM code_review_findings WHERE request_id = $1 AND attempt = $2`, requestID, attempt); err != nil {
		return err
	}
	for i := range findings {
		f := &findings[i]
		if f.ID == "" {
			f.ID = mustUUID()
		}
		if f.Side == "" {
			f.Side = "RIGHT"
		}
		if f.Status == "" {
			f.Status = "pending"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO code_review_findings (id, request_id, attempt, severity, path, line, side, title, body, github_comment_id, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, f.ID, requestID, attempt, f.Severity, f.Path, f.Line, f.Side, f.Title, f.Body, f.GitHubCommentID, f.Status); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// UpdateFindingStatus sets status and the posted comment id.
func (s *Store) UpdateFindingStatus(ctx context.Context, id, status string, commentID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE code_review_findings SET status = $2, github_comment_id = $3 WHERE id = $1`, id, status, commentID)
	return err
}

// ListFindings returns findings for a request; attempt 0 = latest attempt.
func (s *Store) ListFindings(ctx context.Context, requestID string, attempt int) ([]Finding, error) {
	q := `SELECT id, request_id, attempt, severity, path, line, side, title, body, github_comment_id, status, created_at FROM code_review_findings
		WHERE request_id = $1 AND attempt = COALESCE(NULLIF($2, 0), (SELECT max(attempt) FROM code_review_findings WHERE request_id = $1))
		ORDER BY CASE severity WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 WHEN 'P2' THEN 2 ELSE 3 END, path, line`
	rows, err := s.pool.Query(ctx, q, requestID, attempt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Finding{}
	for rows.Next() {
		var f Finding
		if err := rows.Scan(&f.ID, &f.RequestID, &f.Attempt, &f.Severity, &f.Path, &f.Line, &f.Side, &f.Title, &f.Body, &f.GitHubCommentID, &f.Status, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CountPostedReviews counts requests that have posted a review.
func (s *Store) CountPostedReviews(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM code_review_requests WHERE review_url <> ''`).Scan(&n)
	return n, err
}

// ---- feedback rounds ----------------------------------------------------

// InsertFeedbackRound stores a landed review; returns false if it was already known.
func (s *Store) InsertFeedbackRound(ctx context.Context, r *FeedbackRound) (bool, error) {
	if r.ID == "" {
		r.ID = mustUUID()
	}
	if r.State == "" {
		r.State = "new"
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO pr_feedback_rounds (id, repo, number, url, title, head_sha, reviewer, review_state, review_id, comment_count, body, state, submitted_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (repo, number, review_id) DO NOTHING`,
		r.ID, r.Repo, r.Number, r.URL, r.Title, r.HeadSHA, r.Reviewer, r.ReviewState, r.ReviewID, r.CommentCount, r.Body, r.State, r.SubmittedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// MaxFeedbackReviewID returns the highest review id seen for a PR.
func (s *Store) MaxFeedbackReviewID(ctx context.Context, repo string, number int) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(max(review_id), 0) FROM pr_feedback_rounds WHERE repo = $1 AND number = $2`, repo, number).Scan(&id)
	return id, err
}

// ListFeedbackRounds returns rounds, newest first; state "" = all.
func (s *Store) ListFeedbackRounds(ctx context.Context, state string, limit int) ([]*FeedbackRound, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id, repo, number, url, title, head_sha, reviewer, review_state, review_id, comment_count, body, state, ticket_id, session_id, observed_at, submitted_at
		FROM pr_feedback_rounds WHERE ($1 = '' OR state = $1) ORDER BY observed_at DESC LIMIT $2`, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FeedbackRound
	for rows.Next() {
		var r FeedbackRound
		if err := rows.Scan(&r.ID, &r.Repo, &r.Number, &r.URL, &r.Title, &r.HeadSHA, &r.Reviewer, &r.ReviewState, &r.ReviewID, &r.CommentCount, &r.Body, &r.State, &r.TicketID, &r.SessionID, &r.ObservedAt, &r.SubmittedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// GetFeedbackRound returns one round, or nil.
func (s *Store) GetFeedbackRound(ctx context.Context, id string) (*FeedbackRound, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, repo, number, url, title, head_sha, reviewer, review_state, review_id, comment_count, body, state, ticket_id, session_id, observed_at, submitted_at
		FROM pr_feedback_rounds WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var r FeedbackRound
	if err := rows.Scan(&r.ID, &r.Repo, &r.Number, &r.URL, &r.Title, &r.HeadSHA, &r.Reviewer, &r.ReviewState, &r.ReviewID, &r.CommentCount, &r.Body, &r.State, &r.TicketID, &r.SessionID, &r.ObservedAt, &r.SubmittedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

// UpdateFeedbackRound sets state and linkage.
func (s *Store) UpdateFeedbackRound(ctx context.Context, id, state, ticketID, sessionID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE pr_feedback_rounds SET state = $2, ticket_id = COALESCE(NULLIF($3,''), ticket_id), session_id = COALESCE(NULLIF($4,''), session_id) WHERE id = $1`, id, state, ticketID, sessionID)
	return err
}

// CountFeedbackByState returns counts per round state.
func (s *Store) CountFeedbackByState(ctx context.Context) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT state, count(*) FROM pr_feedback_rounds GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

var _ = time.Now

// prefixCols qualifies a comma-separated column list with a table alias.
func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = prefix + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}
