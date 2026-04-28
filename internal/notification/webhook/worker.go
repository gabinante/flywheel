package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Worker handles webhook delivery jobs. It loads webhook events, signs payloads,
// POSTs to merchant URLs, records delivery attempts, and manages retry logic.
type Worker struct {
	store  Store
	client *http.Client
	logger *slog.Logger
}

// NewWorker creates a new webhook delivery worker.
func NewWorker(store Store, opts ...WorkerOption) *Worker {
	w := &Worker{
		store: store,
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects (max 3)")
				}
				return nil
			},
		},
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// WithHTTPClient sets a custom HTTP client (useful for testing).
func WithHTTPClient(client *http.Client) WorkerOption {
	return func(w *Worker) { w.client = client }
}

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) WorkerOption {
	return func(w *Worker) { w.logger = logger }
}

// DeliveryResult captures the outcome of a delivery attempt.
type DeliveryResult struct {
	Success       bool
	StatusCode    *int
	ResponseBody  string
	DurationMs    int
	Error         string
	PermanentFail bool // true if the failure should not be retried (4xx except 429)
	RetryAfter    time.Duration // non-zero if server returned Retry-After header
}

// HandleDelivery processes a single webhook-delivery job.
// This is the main entry point called by the job queue handler.
func (w *Worker) HandleDelivery(ctx context.Context, webhookEventID string) error {
	w.logger.Info("webhook delivery: processing event", "webhook_event_id", webhookEventID)

	// 1. Load the webhook event.
	event, err := w.store.GetEvent(ctx, webhookEventID)
	if err != nil {
		return fmt.Errorf("load webhook event %s: %w", webhookEventID, err)
	}

	// Skip if already delivered or failed.
	if event.Status != EventStatusPending {
		w.logger.Info("webhook delivery: event not pending, skipping",
			"webhook_event_id", webhookEventID, "status", event.Status)
		return nil
	}

	// 2. Load webhook configuration for the project.
	config, err := w.store.GetWebhookConfig(ctx, event.ProjectID)
	if err != nil {
		return fmt.Errorf("load webhook config for project %s: %w", event.ProjectID, err)
	}

	if config.WebhookURL == "" {
		w.logger.Warn("webhook delivery: no webhook URL configured, skipping",
			"project_id", event.ProjectID, "webhook_event_id", webhookEventID)
		return nil
	}

	// 3. Build the standardized payload.
	payload := StandardPayload{
		ID:      event.ID,
		Type:    event.EventType,
		Created: event.CreatedAt.UTC().Format(time.RFC3339),
		Data:    event.Payload,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	// 4. Sign the payload.
	timestamp := time.Now().Unix()
	signature := SignPayload(config.WebhookSecret, timestamp, payloadBytes)

	// 5. POST to the webhook URL.
	result := w.doPost(ctx, config.WebhookURL, payloadBytes, event, signature, timestamp)

	// 6. Record the delivery attempt.
	attemptNumber := event.AttemptCount + 1
	delivery := &Delivery{
		ID:             uuid.New().String(),
		WebhookEventID: event.ID,
		ProjectID:      event.ProjectID,
		AttemptNumber:  attemptNumber,
		StatusCode:     result.StatusCode,
		ResponseBody:   TruncateResponseBody(result.ResponseBody),
		DurationMs:     result.DurationMs,
		Error:          result.Error,
		Success:        result.Success,
		CreatedAt:      time.Now().UTC(),
	}

	if err := w.store.RecordDelivery(ctx, delivery); err != nil {
		w.logger.Error("webhook delivery: failed to record delivery attempt",
			"webhook_event_id", webhookEventID, "error", err)
	}

	// 7. Handle success/failure.
	if result.Success {
		w.logger.Info("webhook delivery: delivered successfully",
			"webhook_event_id", webhookEventID, "attempt", attemptNumber)
		if err := w.store.MarkEventDelivered(ctx, event.ID); err != nil {
			w.logger.Error("webhook delivery: failed to mark delivered",
				"webhook_event_id", webhookEventID, "error", err)
		}
		return nil
	}

	// Update attempt count.
	if err := w.store.UpdateEventStatus(ctx, event.ID, EventStatusPending, attemptNumber); err != nil {
		w.logger.Error("webhook delivery: failed to update attempt count",
			"webhook_event_id", webhookEventID, "error", err)
	}

	// Check if we've exhausted retries or hit a permanent failure.
	if result.PermanentFail {
		w.logger.Warn("webhook delivery: permanent failure, marking failed",
			"webhook_event_id", webhookEventID, "status_code", result.StatusCode,
			"error", result.Error)
		if err := w.store.MarkEventFailed(ctx, event.ID); err != nil {
			w.logger.Error("webhook delivery: failed to mark event failed",
				"webhook_event_id", webhookEventID, "error", err)
		}
		return nil
	}

	if attemptNumber >= event.MaxAttempts {
		w.logger.Warn("webhook delivery: max attempts reached, marking failed",
			"webhook_event_id", webhookEventID, "attempts", attemptNumber,
			"max_attempts", event.MaxAttempts)
		if err := w.store.MarkEventFailed(ctx, event.ID); err != nil {
			w.logger.Error("webhook delivery: failed to mark event failed",
				"webhook_event_id", webhookEventID, "error", err)
		}
		return nil
	}

	// Calculate retry delay.
	delay := w.getRetryDelay(attemptNumber, result.RetryAfter)
	w.logger.Info("webhook delivery: scheduling retry",
		"webhook_event_id", webhookEventID, "attempt", attemptNumber,
		"next_attempt", attemptNumber+1, "delay", delay)

	// Return a RetryError to signal the caller to re-queue with the given delay.
	return &RetryError{
		EventID: event.ID,
		Delay:   delay,
		Attempt: attemptNumber,
	}
}

// doPost performs the actual HTTP POST to the webhook URL.
func (w *Worker) doPost(ctx context.Context, url string, payload []byte, event *Event, signature string, timestamp int64) DeliveryResult {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return DeliveryResult{
			Error:      fmt.Sprintf("create request: %s", err),
			DurationMs: int(time.Since(start).Milliseconds()),
		}
	}

	// Set standard headers.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GHP-Signature", signature)
	req.Header.Set("X-GHP-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-GHP-Event-Type", event.EventType)
	req.Header.Set("X-GHP-Event-Id", event.ID)
	req.Header.Set("User-Agent", "GoHighPayment-Webhook/1.0")

	resp, err := w.client.Do(req)
	durationMs := int(time.Since(start).Milliseconds())

	if err != nil {
		// Network error — retryable.
		errMsg := fmt.Sprintf("request failed: %s", err)
		if isNetworkError(err) {
			errMsg = fmt.Sprintf("network error: %s", err)
		}
		return DeliveryResult{
			Error:      errMsg,
			DurationMs: durationMs,
		}
	}
	defer resp.Body.Close()

	// Read response body (limited to prevent memory issues).
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBodySize)+1))
	responseBody := string(bodyBytes)

	statusCode := resp.StatusCode

	// Success: any 2xx.
	if statusCode >= 200 && statusCode < 300 {
		return DeliveryResult{
			Success:      true,
			StatusCode:   &statusCode,
			ResponseBody: responseBody,
			DurationMs:   durationMs,
		}
	}

	// 429 Too Many Requests: retry with Retry-After.
	if statusCode == 429 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return DeliveryResult{
			StatusCode:   &statusCode,
			ResponseBody: responseBody,
			DurationMs:   durationMs,
			Error:        "rate limited (429)",
			RetryAfter:   retryAfter,
		}
	}

	// 4xx (except 429): permanent failure — don't retry.
	if statusCode >= 400 && statusCode < 500 {
		return DeliveryResult{
			StatusCode:    &statusCode,
			ResponseBody:  responseBody,
			DurationMs:    durationMs,
			Error:         fmt.Sprintf("client error: HTTP %d", statusCode),
			PermanentFail: true,
		}
	}

	// 5xx: temporary failure — retry.
	return DeliveryResult{
		StatusCode:   &statusCode,
		ResponseBody: responseBody,
		DurationMs:   durationMs,
		Error:        fmt.Sprintf("server error: HTTP %d", statusCode),
	}
}

// getRetryDelay returns the delay before the next retry attempt.
// If a Retry-After value is provided (from a 429), it takes precedence.
func (w *Worker) getRetryDelay(attemptNumber int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	if attemptNumber < len(RetryDelays) {
		return RetryDelays[attemptNumber]
	}
	// Fallback: use last defined delay.
	return RetryDelays[len(RetryDelays)-1]
}

// RetryError signals that a delivery should be retried after the given delay.
type RetryError struct {
	EventID string
	Delay   time.Duration
	Attempt int
}

func (e *RetryError) Error() string {
	return fmt.Sprintf("webhook delivery retry: event=%s attempt=%d delay=%s",
		e.EventID, e.Attempt, e.Delay)
}

// IsRetryError checks if an error is a RetryError and returns it.
func IsRetryError(err error) (*RetryError, bool) {
	if re, ok := err.(*RetryError); ok {
		return re, true
	}
	return nil, false
}

// isNetworkError checks if an error is a network-level error.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(net.Error); ok {
		return true
	}
	return false
}

// parseRetryAfter parses the Retry-After HTTP header value.
// Supports both seconds (integer) and HTTP-date formats.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}

	// Try parsing as seconds.
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date.
	if t, err := http.ParseTime(header); err == nil {
		delay := time.Until(t)
		if delay > 0 {
			return delay
		}
	}

	// Fallback: no delay.
	return 0
}
