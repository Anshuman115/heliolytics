-- name: GetProfile :one
-- Single-row table: always id=1.
SELECT id, name, age, height_cm, weight_kg, created_at, updated_at
FROM profiles WHERE id = 1;

-- name: UpsertProfile :exec
INSERT INTO profiles (id, name, age, height_cm, weight_kg, updated_at)
VALUES (1, $1, $2, $3, $4, NOW())
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  age = EXCLUDED.age,
  height_cm = EXCLUDED.height_cm,
  weight_kg = EXCLUDED.weight_kg,
  updated_at = NOW();
