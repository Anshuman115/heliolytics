-- Add sleep hypnogram + nap flag to existing databases.
ALTER TABLE sleep_sessions ADD COLUMN IF NOT EXISTS wake_mins INT NOT NULL DEFAULT 0;
ALTER TABLE sleep_sessions ADD COLUMN IF NOT EXISTS is_nap BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE sleep_sessions ADD COLUMN IF NOT EXISTS stages_json JSONB;
