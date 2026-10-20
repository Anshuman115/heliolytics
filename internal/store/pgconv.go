package store

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const dateLayout = "2006-01-02"

// parseDay reads an exact YYYY-MM-DD string as an IST calendar date.
func parseDay(s string) (time.Time, error) {
	t, err := time.ParseInLocation(dateLayout, s, IST)
	if err != nil || t.Format(dateLayout) != s {
		return time.Time{}, fmt.Errorf("invalid date %q", s)
	}
	return t, nil
}

// dayRange turns inclusive calendar bounds into a half-open instant range.
func dayRange(from, to string) (time.Time, time.Time, error) {
	fromDay, err := parseDay(from)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	toDay, err := parseDay(to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return fromDay, toDay.AddDate(0, 0, 1), nil
}

func nextDay(t time.Time) time.Time { return t.AddDate(0, 0, 1) }

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func dateOf(d pgtype.Date) time.Time {
	return Day(d.Time.Year(), d.Time.Month(), d.Time.Day())
}

func pgDates(days []time.Time) []pgtype.Date {
	out := make([]pgtype.Date, len(days))
	for i, d := range days {
		out[i] = pgDate(d)
	}
	return out
}

func daysOf(dates []pgtype.Date) []time.Time {
	out := make([]time.Time, 0, len(dates))
	for _, d := range dates {
		if !d.Valid {
			continue
		}
		out = append(out, dateOf(d))
	}
	return out
}

func tsAt(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tsTime(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func int2Ptr(v *int) pgtype.Int2 {
	if v == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: int16(*v), Valid: true}
}

func int2Val(v pgtype.Int2) *int {
	if !v.Valid {
		return nil
	}
	return new(int(v.Int16))
}

func int4Ptr(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

func int4Val(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	return new(int(v.Int32))
}

func float4Val(v pgtype.Float4) *float64 {
	if !v.Valid {
		return nil
	}
	return new(float64(v.Float32))
}

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func textValue(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func textVal(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return new(v.String)
}

// numericFromFloat keeps the exact decimal the caller supplied; non-finite
// floats are rejected rather than stored as NaN/Infinity.
func numericFromFloat(v float64) (pgtype.Numeric, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return pgtype.Numeric{}, fmt.Errorf("non-finite number %v", v)
	}
	return numericFromString(strconv.FormatFloat(v, 'f', -1, 64))
}

func numericFromInt(v int) (pgtype.Numeric, error) {
	return numericFromString(strconv.FormatInt(int64(v), 10))
}

func numericFromString(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, err
	}
	if !n.Valid {
		return pgtype.Numeric{}, fmt.Errorf("numeric invalid after scan of %q", s)
	}
	return n, nil
}

func numericVal(v pgtype.Numeric) *float64 {
	if !v.Valid {
		return nil
	}
	f, err := v.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return new(f.Float64)
}

func numericPtr(v *float64) (pgtype.Numeric, error) {
	if v == nil {
		return pgtype.Numeric{}, nil
	}
	return numericFromFloat(*v)
}

func numericIntPtr(v *int) (pgtype.Numeric, error) {
	if v == nil {
		return pgtype.Numeric{}, nil
	}
	return numericFromInt(*v)
}

// uniqueDays normalizes calendar days and drops repeats, sorted ascending.
func uniqueDays(days []time.Time) []time.Time {
	seen := make(map[int64]struct{}, len(days))
	out := make([]time.Time, 0, len(days))
	for _, day := range days {
		normalized := dayOf(day)
		key := normalized.UnixNano()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// dayBounds reads inclusive calendar bounds for date-keyed reads.
func dayBounds(from, to string) (time.Time, time.Time, error) {
	fromDay, err := parseDay(from)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	toDay, err := parseDay(to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return fromDay, toDay, nil
}

func datePtr(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
