package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WriteSession stores one immutable envelope with its complete raw set. It
// returns true when an identical envelope already exists, which is a legal
// retry and never permission to skip parsing.
func (s *Store) WriteSession(ctx context.Context, tx pgx.Tx, meta SessionMeta,
	catalog []byte, blobs []RawBlob) (bool, error) {
	if err := validateSessionMeta(meta); err != nil {
		return false, err
	}
	canonicalCatalog, err := canonicalJSON(catalog)
	if err != nil {
		return false, invalidRow("sync_sessions", "catalog is not a JSON value")
	}
	normalized, err := normalizeBlobs(blobs)
	if err != nil {
		return false, err
	}

	q := db.New(tx)
	stored, err := q.GetSessionEnvelope(ctx, meta.ID)
	switch {
	case err == nil:
		if err := compareStoredSession(ctx, q, stored, meta, canonicalCatalog, normalized); err != nil {
			return false, err
		}
		return true, nil
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return false, err
	}

	if err := q.InsertSession(ctx, db.InsertSessionParams{
		SessionID:  meta.ID,
		DeviceMac:  meta.DeviceMAC,
		StartedAt:  tsAt(meta.StartedAt),
		EndedAt:    tsAt(meta.EndedAt),
		BatteryPct: int2Ptr(meta.BatteryPct),
		Catalog:    canonicalCatalog,
	}); err != nil {
		return false, err
	}
	if len(normalized) == 0 {
		return false, nil
	}
	params := db.InsertRawBlobsParams{
		SessionID: make([]string, 0, len(normalized)),
		TypeCode:  make([]string, 0, len(normalized)),
		Sha256:    make([][]byte, 0, len(normalized)),
		Payload:   make([][]byte, 0, len(normalized)),
	}
	for _, b := range normalized {
		sum := b.SHA256
		params.SessionID = append(params.SessionID, meta.ID)
		params.TypeCode = append(params.TypeCode, b.TypeCode)
		params.Sha256 = append(params.Sha256, sum[:])
		params.Payload = append(params.Payload, b.Payload)
	}
	return false, q.InsertRawBlobs(ctx, params)
}

func normalizeBlobs(blobs []RawBlob) ([]RawBlob, error) {
	seen := make(map[string]struct{}, len(blobs))
	out := make([]RawBlob, 0, len(blobs))
	for _, b := range blobs {
		if err := validateTypeCode(b.TypeCode); err != nil {
			return nil, err
		}
		if _, dup := seen[b.TypeCode]; dup {
			return nil, invalidRow("raw_blobs", "duplicate type_code %s", b.TypeCode)
		}
		seen[b.TypeCode] = struct{}{}
		if len(b.Payload) == 0 {
			return nil, invalidRow("raw_blobs", "payload required for %s", b.TypeCode)
		}
		sum := sha256.Sum256(b.Payload)
		if b.SHA256 != ([32]byte{}) && b.SHA256 != sum {
			return nil, invalidRow("raw_blobs", "sha256 mismatch for %s", b.TypeCode)
		}
		b.SHA256 = sum
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TypeCode < out[j].TypeCode })
	return out, nil
}

func compareStoredSession(ctx context.Context, q *db.Queries, stored db.SyncSession,
	meta SessionMeta, catalog []byte, blobs []RawBlob) error {
	if stored.DeviceMac != meta.DeviceMAC {
		return conflict("device_mac changed")
	}
	if !stored.StartedAt.Time.Equal(meta.StartedAt) || !stored.EndedAt.Time.Equal(meta.EndedAt) {
		return conflict("session timestamps changed")
	}
	if !equalOptionalInt(stored.BatteryPct, meta.BatteryPct) {
		return conflict("battery_pct changed")
	}
	storedCatalog, err := canonicalJSON(stored.Catalog)
	if err != nil || !bytes.Equal(storedCatalog, catalog) {
		return conflict("catalog changed")
	}

	raw, err := q.ListRawBlobsBySession(ctx, meta.ID)
	if err != nil {
		return err
	}
	if len(raw) != len(blobs) {
		return conflict("type set changed")
	}
	byCode := make(map[string]db.ListRawBlobsBySessionRow, len(raw))
	for _, r := range raw {
		byCode[r.TypeCode] = r
	}
	for _, b := range blobs {
		r, ok := byCode[b.TypeCode]
		if !ok {
			return conflict("type set changed")
		}
		if !bytes.Equal(r.Sha256, b.SHA256[:]) || !bytes.Equal(r.Payload, b.Payload) {
			return conflict("payload bytes changed")
		}
	}
	return nil
}

func equalOptionalInt(stored pgtype.Int2, want *int) bool {
	if !stored.Valid {
		return want == nil
	}
	return want != nil && int(stored.Int16) == *want
}

func conflict(reason string) error {
	return fmt.Errorf("%w: %s", ErrSessionConflict, reason)
}
