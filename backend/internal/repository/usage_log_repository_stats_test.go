//go:build unit

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestRepositoryIndexMigrationExecution(t *testing.T) {
	const filename = "250_usage_log_repository_index_notx.sql"
	content, err := migrations.FS.ReadFile(filename)
	require.NoError(t, err)
	nonTx, err := validateMigrationExecutionMode(filename, string(content))
	require.NoError(t, err)
	require.True(t, nonTx)
	statements := splitSQLStatements(string(content))
	require.Len(t, statements, 1, "the nontransactional runner must execute one intact statement")
	require.True(t, strings.HasPrefix(stripSQLLineComment(statements[0]), "CREATE INDEX CONCURRENTLY IF NOT EXISTS"))
}

func TestRepositoryStartupAttributionUsesBoundedRequestIDsAndPreservesExplicitValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := newUsageLogRepositoryWithSQL(nil, db)
	mock.ExpectExec(`UPDATE usage_logs SET repository = \$1\s+WHERE api_key_id = \$2 AND repository IS NULL AND request_id IN \(\$3,\$4\)`).
		WithArgs("github.com/a/b", int64(7), "req-a", "req-b").WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, repo.AttributeUsageRepository(context.Background(), 7, []string{"req-a", "req-b"}, "github.com/a/b"))
	require.NoError(t, repo.AttributeUsageRepository(context.Background(), 7, make([]string, 17), "github.com/a/b"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryStatsQueryIsBoundedScopedAndFiltered(t *testing.T) {
	var captured []string
	db, mock := newSQLCapturingMock(t, &captured)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	requestType, billingType, compact := int16(3), int8(0), true
	filters := usagestats.UsageLogFilters{UserID: 7, APIKeyID: 8, AccountID: 9, GroupID: 10, Model: "gpt-5", RequestType: &requestType, BillingType: &billingType, BillingMode: "token", NativeCompactionV2: &compact}
	mock.ExpectQuery("SELECT").WithArgs(start, end, int64(7), int64(8), int64(9), int64(10), "gpt-5", requestType, compact, int16(billingType), "token", 21, 20).
		WillReturnRows(sqlmock.NewRows([]string{"repository", "requests", "tokens", "cost", "actual_cost"}).AddRow("github.com/a/b", 2, 100, 1.5, 0.75))
	rows, err := repo.GetRepositoryStats(context.Background(), start, end, filters, 21, 20)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 0.75, rows[0].ActualCost)
	require.Len(t, captured, 1)
	query := captured[0]
	require.Contains(t, query, "ul.repository IS NOT NULL AND ul.created_at >= $1 AND ul.created_at < $2")
	require.Contains(t, query, "ul.user_id = $3")
	require.Contains(t, query, "GROUP BY ul.repository ORDER BY SUM(ul.actual_cost) DESC, ul.repository ASC LIMIT")
	require.NotContains(t, query, "JOIN")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryAttributionPersistsInBothBatchPathsWithoutChangingCosts(t *testing.T) {
	repository := "github.com/owner/repo"
	log := &service.UsageLog{UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "repository-request", Repository: &repository, ActualCost: 1.25, TotalCost: 2.5, CreatedAt: time.Now()}
	prepared := prepareUsageLogInsert(log)
	require.Equal(t, sql.NullString{String: repository, Valid: true}, prepared.args[len(prepared.args)-1])
	require.Equal(t, 1.25, prepared.args[26])
	require.Equal(t, 2.5, prepared.args[25])
	key := usageLogBatchKey(log.RequestID, log.APIKeyID)
	query, args := buildUsageLogBatchInsertQuery([]string{key}, map[string]usageLogInsertPrepared{key: prepared})
	require.Equal(t, 3, strings.Count(query, "\n\t\t\t\trepository\n")+strings.Count(query, "\n\t\t\trepository\n"))
	require.Equal(t, prepared.args[len(prepared.args)-1], args[len(args)-1])
	query, args = buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
	require.Contains(t, query, "repository")
	require.Equal(t, prepared.args[len(prepared.args)-1], args[len(args)-1])
	log.Repository = nil
	prepared = prepareUsageLogInsert(log)
	require.Equal(t, sql.NullString{}, prepared.args[len(prepared.args)-1])
}
