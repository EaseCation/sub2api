package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// GetRepositoryStats only scans explicitly attributed rows in the requested time
// range. No joins, separate COUNT, or writes to a per-repository hot counter.
func (r *usageLogRepository) GetRepositoryStats(ctx context.Context, start, end time.Time, filters usagestats.UsageLogFilters, limit, offset int) ([]usagestats.RepositoryStat, error) {
	query := `SELECT ul.repository, COUNT(*),
		COALESCE(SUM(ul.input_tokens::bigint + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0),
		COALESCE(SUM(ul.total_cost), 0), COALESCE(SUM(ul.actual_cost), 0)
		FROM usage_logs ul
		WHERE ul.repository IS NOT NULL AND ul.created_at >= $1 AND ul.created_at < $2`
	args := []any{start, end}
	for _, scope := range []struct {
		column string
		id     int64
	}{{"user_id", filters.UserID}, {"api_key_id", filters.APIKeyID}, {"account_id", filters.AccountID}, {"group_id", filters.GroupID}} {
		if scope.id > 0 {
			query += fmt.Sprintf(" AND ul.%s = $%d", scope.column, len(args)+1)
			args = append(args, scope.id)
		}
	}
	if strings.TrimSpace(filters.Model) != "" {
		query += fmt.Sprintf(" AND %s = $%d", resolveModelDimensionExpressionWithAlias(usagestats.ModelSourceRequested, "ul"), len(args)+1)
		args = append(args, filters.Model)
	}
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, filters.RequestType, filters.Stream)
	query, args = appendNativeCompactionV2QueryFilter(query, args, filters.NativeCompactionV2, "ul")
	if filters.BillingType != nil {
		query += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, int16(*filters.BillingType))
	}
	query, args = appendUsageLogBillingModeQueryFilter(query, args, filters.BillingMode, "ul")
	if filters.UpstreamModelMismatch != nil {
		query += " AND " + upstreamModelMismatchCondition("ul.upstream_model_mismatch", *filters.UpstreamModelMismatch)
	}
	query += fmt.Sprintf(" GROUP BY ul.repository ORDER BY SUM(ul.actual_cost) DESC, ul.repository ASC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := make([]usagestats.RepositoryStat, 0)
	for rows.Next() {
		var row usagestats.RepositoryStat
		if err := rows.Scan(&row.Repository, &row.Requests, &row.TotalTokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
