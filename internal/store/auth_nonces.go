package store

import (
	"context"
	"errors"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
)

// ConsumeNonce atomically consumes an absent or expired (kid, nonce) pair.
// Exactly one concurrent consumer wins; zero rows means the pair is live.
func (s *Store) ConsumeNonce(ctx context.Context, kid, nonce string, expiresAt time.Time) (bool, error) {
	if kid == "" || nonce == "" {
		return false, invalidRow("auth_nonces", "kid and nonce required")
	}
	if expiresAt.IsZero() {
		return false, invalidRow("auth_nonces", "expires_at required")
	}
	_, err := s.q.ConsumeNonce(ctx, db.ConsumeNonceParams{
		Kid:       kid,
		Nonce:     nonce,
		ExpiresAt: tsAt(normInstant(expiresAt)),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PurgeNonces deletes only rows already expired at the supplied instant.
func (s *Store) PurgeNonces(ctx context.Context, now time.Time) (int64, error) {
	return s.q.PurgeNonces(ctx, tsAt(normInstant(now)))
}
