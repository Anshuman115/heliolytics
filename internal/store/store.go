package store

import (
	"context"

	"github.com/heliolytics/api/internal/store/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func New(ctx context.Context, dbURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, q: db.New(pool)}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
