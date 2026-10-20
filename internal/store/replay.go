package store

import (
	"context"
	"fmt"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// ReplayCursor is the keyset position in (ended_at, session_id) order.
type ReplayCursor struct {
	EndedAt   time.Time
	SessionID string
}

// SessionReplay is one complete immutable source session.
type SessionReplay struct {
	Meta    SessionMeta
	Catalog []byte
	Blobs   []RawBlob
}

// DatabaseIdentity is the stable cluster plus database identity used to refuse
// a rebuild into the source database.
type DatabaseIdentity struct {
	SystemIdentifier string
	DatabaseOID      uint32
	Database         string
}

// v4Relations is the exact application relation inventory schema v4 creates.
var v4Relations = []string{
	"sync_sessions", "raw_blobs",
	"heart_rate_samples", "step_samples", "temperature_samples", "hrv_samples",
	"stress_samples", "resp_samples", "spo2_samples",
	"sleep_sessions", "activities", "sleep_observations", "workout_observations",
	"pai_scores", "daily_metrics", "coverage", "profile", "auth_nonces",
	"canonical_day_keys",
}

// databaseIdentitySQL reads the owner-only helper. It stays a single constant
// outside sqlc because sqlc cannot resolve a RETURNS TABLE function's columns.
const databaseIdentitySQL = `SELECT system_identifier, database_oid, database_name
FROM public.rebuild_database_identity()`

// Identity reads the owner-only stable identity helper and fails closed when
// the caller lacks the owner privilege.
func (s *Store) Identity(ctx context.Context) (DatabaseIdentity, error) {
	var (
		systemIdentifier string
		databaseOID      pgtype.Uint32
		databaseName     string
	)
	if err := s.pool.QueryRow(ctx, databaseIdentitySQL).
		Scan(&systemIdentifier, &databaseOID, &databaseName); err != nil {
		return DatabaseIdentity{}, err
	}
	if !databaseOID.Valid {
		return DatabaseIdentity{}, fmt.Errorf("database identity: database oid missing")
	}
	if systemIdentifier == "" || databaseName == "" {
		return DatabaseIdentity{}, fmt.Errorf("database identity: incomplete")
	}
	return DatabaseIdentity{
		SystemIdentifier: systemIdentifier,
		DatabaseOID:      databaseOID.Uint32,
		Database:         databaseName,
	}, nil
}

// ReadReplaySessions pages source sessions in (ended_at, session_id) order and
// returns each one's complete raw set.
func (v *TxView) ReadReplaySessions(ctx context.Context, after *ReplayCursor, limit int) ([]SessionReplay, error) {
	if limit <= 0 {
		return nil, invalidRow("sync_sessions", "replay page limit must be positive")
	}
	params := db.ReadReplaySessionIDsParams{PageLimit: int32(limit)}
	if after != nil {
		params.AfterEndedAt = tsAt(after.EndedAt)
		params.AfterSessionID = textValue(after.SessionID)
	}
	page, err := v.q.ReadReplaySessionIDs(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(page) == 0 {
		return []SessionReplay{}, nil
	}

	ids := make([]string, 0, len(page))
	for _, r := range page {
		ids = append(ids, r.SessionID)
	}
	sessions, err := v.q.ListSessionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	blobs, err := v.q.ListRawBlobsForSessions(ctx, ids)
	if err != nil {
		return nil, err
	}
	blobsBySession := make(map[string][]RawBlob, len(sessions))
	for _, b := range blobs {
		var sum [32]byte
		copy(sum[:], b.Sha256)
		blobsBySession[b.SessionID] = append(blobsBySession[b.SessionID], RawBlob{
			TypeCode: b.TypeCode,
			SHA256:   sum,
			Payload:  b.Payload,
		})
	}
	metaByID := make(map[string]db.ListSessionsByIDsRow, len(sessions))
	for _, s := range sessions {
		metaByID[s.SessionID] = s
	}

	out := make([]SessionReplay, 0, len(page))
	for _, r := range page {
		session, ok := metaByID[r.SessionID]
		if !ok {
			return nil, fmt.Errorf("replay: session %s missing from page", r.SessionID)
		}
		out = append(out, SessionReplay{
			Meta: SessionMeta{
				ID:         session.SessionID,
				DeviceMAC:  session.DeviceMac,
				StartedAt:  session.StartedAt.Time,
				EndedAt:    session.EndedAt.Time,
				BatteryPct: int2Val(session.BatteryPct),
			},
			Catalog: session.Catalog,
			Blobs:   blobsBySession[r.SessionID],
		})
	}
	return out, nil
}

// IsFreshTarget verifies the actual v4 relation shape and that every
// application table is empty before a rebuild mutates the target.
func (v *TxView) IsFreshTarget(ctx context.Context) (bool, error) {
	relations, err := v.q.CountRequiredRelations(ctx, v4Relations)
	if err != nil {
		return false, err
	}
	if relations != int64(len(v4Relations)) {
		return false, nil
	}
	rows, err := v.q.AnyApplicationRows(ctx)
	if err != nil {
		return false, err
	}
	for _, present := range rows {
		if present {
			return false, nil
		}
	}
	return true, nil
}
