ALTER TABLE heart_rate_samples
  ADD COLUMN source_type TEXT NOT NULL DEFAULT '0x46'
  CHECK (source_type IN ('0x01', '0x46'));

CREATE INDEX idx_hr_source_time ON heart_rate_samples (source_type, sampled_at DESC);
