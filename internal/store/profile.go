package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Profile is the single-row user profile — no history, no per-user identity.
type Profile struct {
	Name     *string  `json:"name"`
	Age      *int     `json:"age"`
	HeightCm *float64 `json:"height_cm"`
	WeightKg *float64 `json:"weight_kg"`
}

func (s *Store) GetProfile(ctx context.Context) (Profile, error) {
	var p Profile
	err := s.pool.QueryRow(ctx,
		`SELECT name, age, height_cm, weight_kg FROM profiles WHERE id = 1`).
		Scan(&p.Name, &p.Age, &p.HeightCm, &p.WeightKg)
	if err != nil {
		// No row yet is not an error — an empty profile is a valid state
		// before the user has ever set one.
		if errors.Is(err, pgx.ErrNoRows) {
			return Profile{}, nil
		}
		return Profile{}, err
	}
	return p, nil
}

func (s *Store) UpsertProfile(ctx context.Context, p Profile) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO profiles (id, name, age, height_cm, weight_kg, updated_at)
		VALUES (1, $1, $2, $3, $4, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			age = EXCLUDED.age,
			height_cm = EXCLUDED.height_cm,
			weight_kg = EXCLUDED.weight_kg,
			updated_at = NOW()`,
		p.Name, p.Age, p.HeightCm, p.WeightKg)
	return err
}
