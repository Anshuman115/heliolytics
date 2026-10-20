package store

import (
	"context"
	"errors"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RefreshTypeCoverage recomputes the exact canonical maximum for each accepted
// measured type and deletes watermarks with no canonical rows left. Codes
// outside the measured set, including raw-only 0x26, never contribute.
func (s *Store) RefreshTypeCoverage(ctx context.Context, tx pgx.Tx, typeCodes []string) error {
	q := db.New(tx)
	for _, code := range typeCodes {
		if !isMeasuredType(code) {
			continue
		}
		at, err := maxCanonicalAt(ctx, q, code, pgtype.Text{})
		if err != nil {
			return err
		}
		if err := writeCoverage(ctx, q, code, at); err != nil {
			return err
		}
	}
	return nil
}

// UpsertCoverage assigns one exact watermark, or deletes it when nil.
func (s *Store) UpsertCoverage(ctx context.Context, tx pgx.Tx, typeCode string, dataThrough *time.Time) error {
	if !isMeasuredType(typeCode) {
		return invalidRow("coverage", "type_code %q is not a measured type", typeCode)
	}
	return writeCoverage(ctx, db.New(tx), typeCode, dataThrough)
}

func writeCoverage(ctx context.Context, q *db.Queries, typeCode string, dataThrough *time.Time) error {
	if dataThrough == nil {
		return q.DeleteCoverage(ctx, typeCode)
	}
	return q.UpsertCoverage(ctx, db.UpsertCoverageParams{
		TypeCode:    typeCode,
		DataThrough: tsAt(*dataThrough),
	})
}

// maxCanonicalAt maps one type code to its canonical watermark. A non-empty
// session restricts the read to that session's surviving canonical rows.
func maxCanonicalAt(ctx context.Context, q *db.Queries, typeCode string, sessionID pgtype.Text) (*time.Time, error) {
	var (
		at  pgtype.Timestamptz
		err error
	)
	switch typeCode {
	case "0x01":
		steps, stepErr := scalarTime(q.MaxStepSampleAt(ctx, sessionID))
		if stepErr != nil {
			return nil, stepErr
		}
		heartRate, hrErr := scalarTime(q.MaxHeartRateSampleAt(ctx, db.MaxHeartRateSampleAtParams{
			Source:    "0x01",
			SessionID: sessionID,
		}))
		if hrErr != nil {
			return nil, hrErr
		}
		return latestOf(timePtr(steps), timePtr(heartRate)), nil
	case "0x46":
		at, err = q.MaxHeartRateSampleAt(ctx, db.MaxHeartRateSampleAtParams{Source: "0x46", SessionID: sessionID})
	case "0x05":
		at, err = q.MaxWorkoutEndAt(ctx, db.MaxWorkoutEndAtParams{SessionID: sessionID})
	case "0x06":
		at, err = q.MaxWorkoutEndAt(ctx, db.MaxWorkoutEndAtParams{
			HasDetail: pgtype.Bool{Bool: true, Valid: true},
			SessionID: sessionID,
		})
	case "0x0D":
		at, err = q.MaxPaiSampledAt(ctx, sessionID)
	case "0x13":
		at, err = q.MaxStressSampleAt(ctx, sessionID)
	case "0x25":
		at, err = q.MaxSpo2SampleAt(ctx, sessionID)
	case "0x2E":
		at, err = q.MaxTemperatureSampleAt(ctx, sessionID)
	case "0x38":
		at, err = q.MaxRespSampleAt(ctx, sessionID)
	case "0x48":
		at, err = q.MaxSleepEndAt(ctx, sessionID)
	case "0x49":
		at, err = q.MaxHrvSampleAt(ctx, sessionID)
	default:
		return nil, invalidRow("coverage", "type_code %q has no canonical mapping", typeCode)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return tsTime(at), nil
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return new(t)
}

func scalarTime(at pgtype.Timestamptz, err error) (time.Time, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return at.Time, nil
}

func latestOf(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.After(*a):
		return b
	default:
		return a
	}
}

// GetCoverage reads only the small coverage and session tables.
func (s *Store) GetCoverage(ctx context.Context) (DataCoverage, error) {
	rows, err := s.q.ListCoverage(ctx)
	if err != nil {
		return DataCoverage{}, err
	}
	types := make(map[string]*time.Time, len(MeasuredTypeCodes))
	for _, code := range MeasuredTypeCodes {
		types[code] = nil
	}
	for _, r := range rows {
		if _, ok := types[r.TypeCode]; !ok {
			continue
		}
		types[r.TypeCode] = tsTime(r.DataThrough)
	}

	lastIngest, err := s.q.LastSessionReceivedAt(ctx)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DataCoverage{}, err
	}

	out := DataCoverage{Types: types}
	if err == nil {
		out.LastIngestAt = tsTime(lastIngest)
	}
	for _, at := range types {
		if at == nil {
			continue
		}
		out.HasData = true
		if out.DataThrough == nil || at.After(*out.DataThrough) {
			out.DataThrough = at
		}
	}
	return out, nil
}
