-- Optional client attribution only; no pricing or balance changes. Nullable with
-- no default/backfill keeps the existing hot table migration metadata-only.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS repository VARCHAR(512);
COMMENT ON COLUMN usage_logs.repository IS 'Normalized client-supplied repository host/path; NULL when not supplied';
