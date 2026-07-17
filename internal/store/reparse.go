package store

import (
	"context"
	"time"
)

type SessionReplay struct {
	SessionID  string
	DeviceMAC  string
	StartedAt  time.Time
	EndedAt    time.Time
	BatteryPct *int
	Catalog    []byte
	Blobs      map[string][]byte
}

func (s *Store) LatestReplay(ctx context.Context) (*SessionReplay, error) {
	var sid, mac string
	var started, ended time.Time
	var battery *int
	var catalog []byte
	err := s.pool.QueryRow(ctx, `
		SELECT session_id, device_mac, started_at, COALESCE(ended_at, started_at), battery_pct, catalog_json
		FROM sync_sessions ORDER BY ingested_at DESC LIMIT 1`).Scan(&sid, &mac, &started, &ended, &battery, &catalog)
	if err != nil {
		return nil, err
	}
	if len(catalog) == 0 {
		return nil, ErrNoCatalog
	}
	rows, err := s.pool.Query(ctx, `
		SELECT type_code, payload FROM raw_type_blobs WHERE session_id=$1`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	blobs := map[string][]byte{}
	for rows.Next() {
		var code string
		var raw []byte
		if err := rows.Scan(&code, &raw); err != nil {
			return nil, err
		}
		blobs[code] = raw
	}
	return &SessionReplay{
		SessionID: sid, DeviceMAC: mac, StartedAt: started, EndedAt: ended,
		BatteryPct: battery, Catalog: catalog, Blobs: blobs,
	}, rows.Err()
}

var ErrNoCatalog = errNoCatalog{}

type errNoCatalog struct{}

func (errNoCatalog) Error() string { return "no catalog stored for session" }

// ResetDailySteps zeroes out the steps column for all daily_metrics rows whose
// source_session_id matches the given session. Call this before re-running
// RunIngest on an already-ingested session so that the accumulate-on-upsert
// SQL does not double-count the step deltas.
func (s *Store) ResetDailySteps(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE daily_metrics SET steps = 0 WHERE source_session_id = $1`,
		sessionID)
	return err
}
