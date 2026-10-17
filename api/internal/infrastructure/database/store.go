package database

import (
	"context"
	"fmt"

	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey int

const txKey ctxKey = 1

type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		queries: db.New(pool),
	}
}

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(context.WithValue(ctx, txKey, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) q(ctx context.Context) *db.Queries {
	tx, ok := ctx.Value(txKey).(pgx.Tx)
	if ok {
		return s.queries.WithTx(tx)
	}
	return s.queries
}
