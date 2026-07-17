package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type SessionMeta struct {
	ID          string
	DeviceMAC   string
	StartedAt   time.Time
	EndedAt     *time.Time
	BatteryPct  *int
	CatalogJSON []byte
}

func (s *Store) UpsertSession(ctx context.Context, m SessionMeta) error {
	if err := validateSessionMeta(m); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sync_sessions (session_id, device_mac, started_at, ended_at, battery_pct, catalog_json)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (session_id) DO UPDATE SET
		  device_mac=EXCLUDED.device_mac,
		  ended_at=EXCLUDED.ended_at,
		  battery_pct=EXCLUDED.battery_pct,
		  catalog_json=COALESCE(EXCLUDED.catalog_json, sync_sessions.catalog_json),
		  ingested_at=NOW()`,
		m.ID, m.DeviceMAC, m.StartedAt, m.EndedAt, m.BatteryPct, m.CatalogJSON)
	return err
}

// UpsertSessionTx is UpsertSession run against an already-open transaction,
// for use inside Store.WithTx. One sync_sessions row per sync, never a
// slice, so this is a plain tx.Exec rather than a batched write.
func (s *Store) UpsertSessionTx(ctx context.Context, tx pgx.Tx, m SessionMeta) error {
	if err := validateSessionMeta(m); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO sync_sessions (session_id, device_mac, started_at, ended_at, battery_pct, catalog_json)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (session_id) DO UPDATE SET
		  device_mac=EXCLUDED.device_mac,
		  ended_at=EXCLUDED.ended_at,
		  battery_pct=EXCLUDED.battery_pct,
		  catalog_json=COALESCE(EXCLUDED.catalog_json, sync_sessions.catalog_json),
		  ingested_at=NOW()`,
		m.ID, m.DeviceMAC, m.StartedAt, m.EndedAt, m.BatteryPct, m.CatalogJSON)
	return err
}

func (s *Store) UpsertRaw(ctx context.Context, sessionID, typeCode string, raw []byte) error {
	if err := validateRawBlob(sessionID, typeCode, raw); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO raw_type_blobs (session_id, type_code, byte_len, payload)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (session_id, type_code) DO UPDATE SET
		  byte_len=EXCLUDED.byte_len, payload=EXCLUDED.payload`,
		sessionID, typeCode, len(raw), raw)
	return err
}

// UpsertRawTx is UpsertRaw run against an already-open transaction, for use
// inside Store.WithTx. UpsertRaw writes one raw_type_blobs row per call — the
// caller loops per blob — so this is a plain tx.Exec rather than a batched
// write; there is no per-call slice to chunk.
func (s *Store) UpsertRawTx(ctx context.Context, tx pgx.Tx, sessionID, typeCode string, raw []byte) error {
	if err := validateRawBlob(sessionID, typeCode, raw); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO raw_type_blobs (session_id, type_code, byte_len, payload)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (session_id, type_code) DO UPDATE SET
		  byte_len=EXCLUDED.byte_len, payload=EXCLUDED.payload`,
		sessionID, typeCode, len(raw), raw)
	return err
}
