package store

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IST is the single calendar zone for day keys. Asia/Kolkata is a fixed
// +05:30 zone with no daylight saving, so no tzdata is needed at runtime.
var IST = time.FixedZone("Asia/Kolkata", 19800)

// Day returns IST midnight for a civil date.
func Day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, IST)
}

// dayOf normalizes an instant to its IST calendar date.
func dayOf(t time.Time) time.Time {
	local := t.In(IST)
	return Day(local.Year(), local.Month(), local.Day())
}

// Store is the single SQL boundary for the API, ingest and rebuild paths.
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

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
