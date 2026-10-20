-- name: SelectProfile :one
SELECT singleton, name, birth_date, sex, height_cm, weight_kg
FROM profile
WHERE singleton;

-- name: UpsertProfile :exec
INSERT INTO profile (singleton, name, birth_date, sex, height_cm, weight_kg, updated_at)
VALUES (true, sqlc.arg(name), sqlc.arg(birth_date), sqlc.arg(sex),
        sqlc.arg(height_cm), sqlc.arg(weight_kg), clock_timestamp())
ON CONFLICT (singleton) DO UPDATE SET
  name = EXCLUDED.name,
  birth_date = EXCLUDED.birth_date,
  sex = EXCLUDED.sex,
  height_cm = EXCLUDED.height_cm,
  weight_kg = EXCLUDED.weight_kg,
  updated_at = clock_timestamp();
