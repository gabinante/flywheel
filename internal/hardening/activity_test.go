package hardening

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/internal/activity"
)

func TestActivityCommitRollbackAndListenerRecovery(t *testing.T) {
	p := pool(t)
	_, project := fixture(t, p)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := activity.New()
	go hub.Run(ctx)
	go hub.Listen(ctx, p.Config().ConnConfig)
	ch, unsubscribe := hub.Subscribe()
	defer unsubscribe()
	read := func(match func(activity.Event) bool) {
		t.Helper()
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		for {
			select {
			case e := <-ch:
				if match(e) {
					return
				}
			case <-timer.C:
				t.Fatal("activity event not received")
			}
		}
	}
	read(func(e activity.Event) bool { return e.Resync && e.Ready })
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "UPDATE projects SET name='Uncommitted' WHERE id=$1", project); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ch:
		t.Fatalf("uncommitted event: %+v", e)
	case <-time.After(600 * time.Millisecond):
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ch:
		t.Fatalf("rollback event: %+v", e)
	case <-time.After(600 * time.Millisecond):
	}
	if _, err = p.Exec(ctx, "UPDATE projects SET name='Committed' WHERE id=$1", project); err != nil {
		t.Fatal(err)
	}
	read(func(e activity.Event) bool {
		for _, topic := range e.Topics {
			if topic == activity.Projects {
				return true
			}
		}
		return false
	})
	// Repeated reconciliation writes must not create a refresh loop.
	if _, err = p.Exec(ctx, "UPDATE projects SET name=name WHERE id=$1", project); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ch:
		t.Fatalf("no-op event: %+v", e)
	case <-time.After(600 * time.Millisecond):
	}
	if _, err = p.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name='flywheel-activity' AND datname=current_database()"); err != nil {
		t.Fatal(err)
	}
	read(func(e activity.Event) bool { return !e.Ready })
	if _, err = p.Exec(ctx, "UPDATE projects SET name='While disconnected' WHERE id=$1", project); err != nil {
		t.Fatal(err)
	}
	read(func(e activity.Event) bool { return e.Ready && e.Resync })
}
