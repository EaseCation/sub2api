//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRepositoryAttributionIntegration(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{Email: "repository-integration@example.com", Balance: 100})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-repository-" + uuid.NewString(), Name: "repo"})
	account := mustCreateAccount(t, client, &service.Account{Name: "repository-integration"})
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	repository := "github.com/owner/repo"
	requests := make(map[string]string)
	for i, path := range []string{"single", "best-effort-single", "batch", "best-effort-batch", "unattributed", "out-of-period"} {
		log := &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: fmt.Sprintf("repository-%d-%s", i, uuid.NewString()), Repository: &repository, Model: "gpt-5", InputTokens: 100, OutputTokens: 50, TotalCost: 2.5, ActualCost: 1.25, CreatedAt: start.Add(time.Hour)}
		requests[path] = log.RequestID
		if path == "unattributed" {
			log.Repository = nil
		}
		if path == "out-of-period" {
			log.CreatedAt = start.AddDate(0, 0, -1)
		}
		prepared := prepareUsageLogInsert(log)
		switch path {
		case "best-effort-single":
			require.NoError(t, execUsageLogInsertNoResult(ctx, tx, prepared))
		case "batch", "best-effort-batch":
			var query string
			var args []any
			if path == "batch" {
				batchKey := usageLogBatchKey(log.RequestID, log.APIKeyID)
				query, args = buildUsageLogBatchInsertQuery([]string{batchKey}, map[string]usageLogInsertPrepared{batchKey: prepared})
			} else {
				query, args = buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
			}
			rows, err := tx.QueryContext(ctx, query, args...)
			require.NoError(t, err, path)
			require.NoError(t, rows.Close())
		default:
			_, err := repo.Create(ctx, log)
			require.NoError(t, err, path)
		}
	}
	rows, err := repo.GetRepositoryStats(ctx, start, start.AddDate(0, 0, 1), usagestats.UsageLogFilters{UserID: user.ID}, 21, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, repository, rows[0].Repository)
	require.EqualValues(t, 4, rows[0].Requests)
	require.EqualValues(t, 600, rows[0].TotalTokens)
	require.Equal(t, 10.0, rows[0].Cost)
	require.Equal(t, 5.0, rows[0].ActualCost)
	otherRows, err := repo.GetRepositoryStats(ctx, start, start.AddDate(0, 0, 1), usagestats.UsageLogFilters{UserID: user.ID + 1_000_000}, 21, 0)
	require.NoError(t, err)
	require.Empty(t, otherRows)
	// Backfill cannot cross API-key scopes or overwrite an explicit repository.
	require.NoError(t, repo.AttributeUsageRepository(ctx, key.ID+1_000_000, []string{requests["unattributed"]}, "github.com/wrong/scope"))
	require.NoError(t, repo.AttributeUsageRepository(ctx, key.ID, []string{requests["single"]}, "github.com/wrong/overwrite"))
	require.NoError(t, repo.AttributeUsageRepository(ctx, key.ID, []string{requests["unattributed"]}, repository))
	require.NoError(t, repo.AttributeUsageRepository(ctx, key.ID, []string{requests["unattributed"]}, repository))
	rows, err = repo.GetRepositoryStats(ctx, start, start.AddDate(0, 0, 1), usagestats.UsageLogFilters{UserID: user.ID}, 21, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, repository, rows[0].Repository)
	require.EqualValues(t, 5, rows[0].Requests)
	require.EqualValues(t, 750, rows[0].TotalTokens)
	require.Equal(t, 12.5, rows[0].Cost)
	require.Equal(t, 6.25, rows[0].ActualCost)
	var balance float64
	require.NoError(t, scanSingleRow(ctx, tx, "SELECT balance FROM users WHERE id = $1", []any{user.ID}, &balance))
	require.Equal(t, 100.0, balance, "attribution and statistics must not mutate billing")
}
