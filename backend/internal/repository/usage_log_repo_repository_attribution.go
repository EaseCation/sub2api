package repository

import (
	"context"
	"fmt"
	"strings"
)

// AttributeUsageRepository only fills missing startup attribution. The existing
// unique (request_id, api_key_id) index bounds the update; no session-wide scan,
// pricing changes, balance changes, or overwrites of explicit attribution.
func (r *usageLogRepository) AttributeUsageRepository(ctx context.Context, apiKeyID int64, requestIDs []string, repository string) error {
	if len(requestIDs) == 0 || len(requestIDs) > 16 || apiKeyID <= 0 || repository == "" {
		return nil
	}
	args := []any{repository, apiKeyID}
	placeholders := make([]string, len(requestIDs))
	for i, id := range requestIDs {
		args = append(args, id)
		placeholders[i] = fmt.Sprintf("$%d", len(args))
	}
	_, err := r.sql.ExecContext(ctx, `UPDATE usage_logs SET repository = $1
		WHERE api_key_id = $2 AND repository IS NULL AND request_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}
