package store

import (
	"context"
	"errors"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
)

// Profile is the nullable singleton profile. No row reads as all null rather
// than invented zeros, and there is no age field.
type Profile struct {
	Name      *string
	BirthDate *time.Time
	Sex       *string
	HeightCm  *float64
	WeightKg  *float64
}

func (s *Store) GetProfile(ctx context.Context) (Profile, error) { return getProfile(ctx, s.q) }

func (v *TxView) GetProfile(ctx context.Context) (Profile, error) { return getProfile(ctx, v.q) }

func getProfile(ctx context.Context, q *db.Queries) (Profile, error) {
	row, err := q.SelectProfile(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Profile{}, nil
		}
		return Profile{}, err
	}
	var p Profile
	p.Name = textVal(row.Name)
	p.Sex = textVal(row.Sex)
	p.HeightCm = numericVal(row.HeightCm)
	p.WeightKg = numericVal(row.WeightKg)
	if row.BirthDate.Valid {
		p.BirthDate = new(dateOf(row.BirthDate))
	}
	return p, nil
}

// PutProfile replaces all five values, including clearing to NULL, inside the
// caller's locked transaction.
func (v *TxView) PutProfile(ctx context.Context, p Profile) error {
	if p.Sex != nil && *p.Sex != "male" && *p.Sex != "female" {
		return invalidRow("profile", "sex must be male or female")
	}
	if p.HeightCm != nil && (*p.HeightCm < 50 || *p.HeightCm > 250) {
		return invalidRow("profile", "height_cm out of range")
	}
	if p.WeightKg != nil && (*p.WeightKg < 20 || *p.WeightKg > 300) {
		return invalidRow("profile", "weight_kg out of range")
	}
	birthDate := p.BirthDate
	if birthDate != nil {
		birthDate = new(dayOf(*birthDate))
	}
	height, err := numericPtr(p.HeightCm)
	if err != nil {
		return invalidRow("profile", "height_cm not representable")
	}
	weight, err := numericPtr(p.WeightKg)
	if err != nil {
		return invalidRow("profile", "weight_kg not representable")
	}
	return v.q.UpsertProfile(ctx, db.UpsertProfileParams{
		Name:      textPtr(p.Name),
		BirthDate: datePtr(birthDate),
		Sex:       textPtr(p.Sex),
		HeightCm:  height,
		WeightKg:  weight,
	})
}
