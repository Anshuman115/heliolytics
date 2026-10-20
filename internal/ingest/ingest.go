// Package ingest owns one immutable upload's transactional path: parse,
// canonical writes, coverage and the derived rollups.
package ingest

import (
	"context"
	"sort"
	"time"

	"github.com/Anshuman115/heliolytics/internal/parse"
	"github.com/Anshuman115/heliolytics/internal/rollup"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/jackc/pgx/v5"
)

// statusPartial marks a type that accepted a complete prefix only.
const statusPartial = "partial"

// Run stores one immutable upload and every derived aggregate in one locked
// transaction. Structural input errors return before the first write.
func Run(ctx context.Context, st *store.Store, meta store.SessionMeta,
	catalog []byte, blobs map[string][]byte) (store.IngestResult, error) {
	envelope := meta.Normalized()
	var result store.IngestResult
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if err := st.LockCanonical(ctx, tx); err != nil {
			return err
		}
		parsedCatalog, err := parse.ParseCatalog(catalog)
		if err != nil {
			return err
		}
		batch, err := parse.ParseBlobs(parsedCatalog, blobs,
			parse.Window{StartedAt: envelope.StartedAt, EndedAt: envelope.EndedAt})
		if err != nil {
			return err
		}

		raw := make([]store.RawBlob, 0, len(blobs))
		for code, payload := range blobs {
			raw = append(raw, store.RawBlob{TypeCode: code, Payload: payload})
		}
		sort.Slice(raw, func(i, j int) bool { return raw[i].TypeCode < raw[j].TypeCode })
		existing, err := st.WriteSession(ctx, tx, meta, catalog, raw)
		if err != nil {
			return err
		}
		meta = envelope
		mutation, err := applyBatch(ctx, st, tx, meta, batch, existing)
		if err != nil {
			return err
		}
		codes := submittedCodes(batch)
		if err := st.RefreshTypeCoverage(ctx, tx, codes); err != nil {
			return err
		}
		if mutation.Changed {
			in := rollup.RunInput{
				TouchedDays:       mutation.Days,
				MutationIntervals: mutation.Intervals,
				Auto:              &rollup.AutoTrigger{SessionID: meta.ID, ObservedAt: meta.EndedAt},
			}
			if err := rollup.Run(ctx, st, tx, in); err != nil {
				return err
			}
		}

		through, err := store.NewTxView(tx).SessionTypeDataThrough(ctx, meta.ID, codes)
		if err != nil {
			return err
		}
		result = buildResult(meta.ID, batch, through)
		return nil
	})
	if err != nil {
		return store.IngestResult{}, err
	}
	return result, nil
}

// applyBatch writes every decoded row and reports the merged canonical changes.
// existing marks a retried envelope, so candidate writes must match the store.
func applyBatch(ctx context.Context, st *store.Store, tx pgx.Tx,
	meta store.SessionMeta, batch parse.ParsedBatch, existing bool) (store.Mutation, error) {
	rows := convertBatch(batch)
	observed := meta.EndedAt

	var (
		days    []time.Time
		spans   []store.InstantInterval
		changed bool
	)
	collect := func(m store.Mutation, err error) error {
		if err != nil {
			return err
		}
		if !m.Changed {
			return nil
		}
		changed = true
		days = append(days, m.Days...)
		spans = append(spans, m.Intervals...)
		return nil
	}

	writes := []func() (store.Mutation, error){
		func() (store.Mutation, error) {
			return st.WriteHeartRate(ctx, tx, rows.heartRate, meta.ID, observed)
		},
		func() (store.Mutation, error) {
			return st.WriteSteps(ctx, tx, rows.steps, meta.ID, observed)
		},
		func() (store.Mutation, error) {
			return st.WriteTemperature(ctx, tx, rows.temperature, meta.ID, observed)
		},
		func() (store.Mutation, error) {
			return st.WriteHRV(ctx, tx, rows.hrv, meta.ID, observed)
		},
		func() (store.Mutation, error) {
			return st.WriteStress(ctx, tx, rows.stress, meta.ID, observed)
		},
		func() (store.Mutation, error) {
			return st.WriteResp(ctx, tx, rows.resp, meta.ID, observed)
		},
		func() (store.Mutation, error) {
			return st.WriteSpO2(ctx, tx, rows.spo2, meta.ID, observed)
		},
		func() (store.Mutation, error) {
			return st.WriteSleep(ctx, tx, rows.sleep, meta.ID, observed, existing)
		},
		func() (store.Mutation, error) {
			return st.WriteWorkouts(ctx, tx, rows.workouts, meta.ID, observed, existing)
		},
		func() (store.Mutation, error) {
			return st.WritePAI(ctx, tx, rows.pai, meta.ID, observed)
		},
	}
	for _, write := range writes {
		if err := collect(write()); err != nil {
			return store.Mutation{}, err
		}
	}
	return store.Mutation{Days: days, Intervals: spans, Changed: changed}, nil
}

// submittedCodes returns the parsed type codes in a stable order.
func submittedCodes(batch parse.ParsedBatch) []string {
	codes := make([]string, 0, len(batch.Types))
	for code := range batch.Types {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// buildResult renders the C2 response from the parse outcome and the surviving
// canonical watermarks of this session.
func buildResult(sessionID string, batch parse.ParsedBatch,
	through map[string]*time.Time) store.IngestResult {
	types := make(map[string]store.TypeResult, len(batch.Types))
	complete := true
	for code, parsed := range batch.Types {
		warnings := make([]string, 0, len(parsed.Warnings))
		for _, w := range parsed.Warnings {
			warnings = append(warnings, w.String())
		}
		if parsed.Status == statusPartial {
			complete = false
		}
		types[code] = store.TypeResult{
			Status:        parsed.Status,
			Rows:          parsed.Rows,
			Dropped:       parsed.Dropped,
			TrailingBytes: parsed.TrailingBytes,
			Warnings:      warnings,
			DataThrough:   through[code],
		}
	}
	return store.IngestResult{
		SessionID: sessionID,
		Complete:  complete,
		Types:     types,
		Warnings:  []string{},
	}
}
