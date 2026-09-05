package activity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// Listen uses one dedicated connection, independent of the query pool and of the
// durable workflow bus. Postgres sends NOTIFY only after the writer commits.
func (h *Hub) Listen(ctx context.Context, cfg *pgx.ConnConfig) {
	delay := time.Second
	for ctx.Err() == nil {
		err := h.listen(ctx, cfg, func() { delay = time.Second })
		h.SourceReady(false)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("activity listener reconnecting", "error", err, "delay", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func (h *Hub) listen(ctx context.Context, cfg *pgx.ConnConfig, connected func()) error {
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	config := cfg.Copy()
	config.RuntimeParams["application_name"] = "flywheel-activity"
	conn, err := pgx.ConnectConfig(connectCtx, config)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(connectCtx, "LISTEN flywheel_activity"); err != nil {
		return err
	}
	connected()
	h.SourceReady(true)
	for {
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		n, err := conn.WaitForNotification(waitCtx)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Ping(pingCtx)
			cancel()
			if err == nil {
				continue
			}
		}
		if err != nil {
			return err
		}
		h.Publish(n.Payload)
	}
}
