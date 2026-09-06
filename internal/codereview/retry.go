package codereview

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

var retryDelays = [...]time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

func nonNilIDs(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

// Permanent configuration/permission failures need an operator; other pre-posting
// failures get a bounded retry budget. Never replay an ambiguous GitHub write.
func retryableReviewError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, permanent := range []string{
		"post review:", "inspect github", "unsupported model", "model is not supported", "model_not_found", "unknown harness",
		"unsupported harness", "executable file not found", "no such file or directory", "permission denied",
		"authentication failed", "not logged", "bad credentials", "http 401", "http 403", "http 404",
		"repository not found", "repository origin mismatch", "maximum number of files", "too many files",
	} {
		if strings.Contains(message, permanent) {
			return false
		}
	}
	return true
}

// scheduleRetry atomically creates the next attempt. A stop or newer attempt wins
// over a late worker. The queue also excludes service-owned IDs until cleanup ends.
func (s *Store) scheduleRetry(ctx context.Context, req *Request, reason string, headChanged bool) (bool, error) {
	count, delay := req.RetryCount, 30*time.Second
	if !headChanged {
		if count >= len(retryDelays) {
			return false, nil
		}
		delay = retryDelays[count]
		count++
	}
	// resetAttemptState clears stale findings/session pointers, while retaining
	// publication history and the recovery budget across head changes.
	result, err := s.pool.Exec(ctx, `UPDATE code_review_requests SET `+resetAttemptState+`,
		retry_count=$3, retry_at=$4, error=$5
		WHERE id=$1 AND attempt=$2 AND state IN ('fetching','reviewing','publishing','failed')`,
		req.ID, req.Attempt, count, time.Now().Add(delay), reason)
	return err == nil && result.RowsAffected() == 1, err
}

// Adopt existing pre-publication failures once when upgrading, including the old
// head-change failures. Exhausted retries and ambiguous publications remain visible.
func (s *Store) recoverRetryableFailures(ctx context.Context) error {
	requests, err := s.ListByState(ctx, StateFailed)
	if err != nil {
		return err
	}
	for _, req := range requests {
		headChanged := strings.HasPrefix(req.Error, "PR head changed while preparing") || strings.HasPrefix(req.Error, "PR changed before publication")
		if !headChanged && !retryableReviewError(errors.New(req.Error)) {
			continue
		}
		if _, err := s.scheduleRetry(ctx, req, req.Error, headChanged); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) retry(ctx context.Context, req *Request, reason string, headChanged bool) bool {
	scheduled, err := s.store.scheduleRetry(context.WithoutCancel(ctx), req, reason, headChanged)
	if err != nil {
		slog.Error("codereview: schedule retry failed", "pr", req.Ref(), "error", err)
	}
	if scheduled {
		s.notifyActivity()
		slog.Info("codereview: retry scheduled", "pr", req.Ref(), "reason", reason)
	}
	return scheduled
}
