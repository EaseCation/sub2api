-- Match only attributed traffic. Concurrent build avoids blocking gateway writes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_repository_user_created
    ON usage_logs (user_id, created_at)
    WHERE repository IS NOT NULL;
