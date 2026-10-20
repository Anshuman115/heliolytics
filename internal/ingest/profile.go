package ingest

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/rollup"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/jackc/pgx/v5"
)

// UpdateProfile replaces the singleton profile. A birth date or sex change
// recomputes every historical load and readiness row in the same transaction.
func UpdateProfile(ctx context.Context, st *store.Store, profile *store.Profile) error {
	return st.WithTx(ctx, func(tx pgx.Tx) error {
		if err := st.LockCanonical(ctx, tx); err != nil {
			return err
		}
		view := store.NewTxView(tx)
		before, err := view.GetProfile(ctx)
		if err != nil {
			return err
		}
		if err := view.PutProfile(ctx, *profile); err != nil {
			return err
		}
		if !formulaChanged(before, *profile) {
			// Canonical support cannot change here, so the affected set is empty.
			_, err := view.NormalizeDailyDomainTx(ctx, nil)
			return err
		}

		first, last, err := view.CanonicalDayBounds(ctx)
		if err != nil {
			return err
		}
		if first == nil || last == nil {
			return nil
		}
		now := time.Now().In(store.IST)
		end := store.Day(now.Year(), now.Month(), now.Day())
		if last.After(end) {
			end = *last
		}
		days := make([]time.Time, 0, 64)
		for day := *first; !day.After(end); day = day.AddDate(0, 0, 1) {
			days = append(days, day)
		}
		return rollup.Run(ctx, st, tx, rollup.RunInput{TouchedDays: days})
	})
}

// formulaChanged reports whether a load input changed.
func formulaChanged(before, after store.Profile) bool {
	return !sameDate(before.BirthDate, after.BirthDate) || !sameText(before.Sex, after.Sex)
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
