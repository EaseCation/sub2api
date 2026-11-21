-- Opt-in Claude five-hour window startup; existing connectivity plans are unchanged.
ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS mode VARCHAR(32) NOT NULL DEFAULT 'connectivity'
    CHECK (mode IN ('connectivity', 'claude_window'));

-- One window plan per account, including disabled plans, prevents duplicate pings.
CREATE UNIQUE INDEX IF NOT EXISTS idx_stp_claude_window_account
    ON scheduled_test_plans(account_id) WHERE mode = 'claude_window';
