-- Split readiness: device 0x39 stays in `readiness` (authoritative); our derived
-- score goes in `computed_readiness`. Reads use COALESCE(readiness, computed).
ALTER TABLE daily_metrics ADD COLUMN IF NOT EXISTS computed_readiness INT;
