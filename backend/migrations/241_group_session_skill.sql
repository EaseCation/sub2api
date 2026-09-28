ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS session_skill_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS session_skill TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN groups.session_skill IS 'Conditional session-start instructions; model adherence is best effort';
