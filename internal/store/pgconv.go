package store

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

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
	i := int(v.Int32)
	return &i
}

func textPtr(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func textVal(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func numericPtr(v *float64) (pgtype.Numeric, error) {
	if v == nil {
		return pgtype.Numeric{}, nil
	}
	return numericFromFloat(*v)
}

func numericVal(v pgtype.Numeric) *float64 {
	if !v.Valid {
		return nil
	}
	f, err := v.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return &f.Float64
}

func numericFromFloat(v float64) (pgtype.Numeric, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return pgtype.Numeric{}, fmt.Errorf("non-finite float %v", v)
	}
	var n pgtype.Numeric
	if err := n.Scan(strconv.FormatFloat(v, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}, err
	}
	if !n.Valid {
		return pgtype.Numeric{}, fmt.Errorf("pgtype numeric invalid after scan of %v", v)
	}
	return n, nil
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timestamptzRequired(t time.Time, column string) (pgtype.Timestamptz, error) {
	if t.IsZero() {
		return pgtype.Timestamptz{}, fmt.Errorf("%s: timestamp required", column)
	}
	return timestamptz(t), nil
}

func dayKeyRequired(v, column string) error {
	if v == "" {
		return fmt.Errorf("%s: day_key required", column)
	}
	return nil
}

func dateKey(s string) (pgtype.Date, error) {
	if err := dayKeyRequired(s, "day_key"); err != nil {
		return pgtype.Date{}, err
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("day_key: %w", err)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func dateKeyString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func textRequired(v, column string) error {
	if v == "" {
		return fmt.Errorf("%s: value required", column)
	}
	return nil
}
