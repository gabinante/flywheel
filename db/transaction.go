package db

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Querier interface {
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type transactionKey struct{}
type transactionState struct {
	tx  pgx.Tx
	err error
}

func Executor(ctx context.Context, pool *pgxpool.Pool) Querier {
	if st, ok := ctx.Value(transactionKey{}).(*transactionState); ok {
		return st.tx
	}
	return pool
}
func InTransaction(ctx context.Context) bool { return ctx.Value(transactionKey{}) != nil }
func Transaction(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	if InTransaction(ctx) {
		return fn(ctx)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	st := &transactionState{tx: tx}
	if err = fn(context.WithValue(ctx, transactionKey{}, st)); err != nil {
		return err
	}
	if st.err != nil {
		return st.err
	}
	return tx.Commit(ctx)
}

// FailTransaction prevents a caller that ignores an outbox error from committing
// its state change without the corresponding event.
func FailTransaction(ctx context.Context, err error) {
	if st, ok := ctx.Value(transactionKey{}).(*transactionState); ok && st.err == nil {
		st.err = err
	}
}
