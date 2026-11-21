//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var windowPlanColumns = []string{"id", "account_id", "model_id", "cron_expression", "enabled", "max_results", "auto_recover", "mode", "last_run_at", "next_run_at", "created_at", "updated_at"}

func TestScheduledWindowPlanClaimRechecksDueAndEnabled(t *testing.T) {
	for _, claimed := range []bool{true, false} {
		t.Run(map[bool]string{true: "claimed", false: "stale or disabled"}[claimed], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			now := time.Now()
			lease := now.Add(10 * time.Minute)
			rows := sqlmock.NewRows(windowPlanColumns)
			if claimed {
				rows.AddRow(1, 7, "claude-sonnet-4-6", "* * * * *", true, 50, false, "claude_window", nil, lease, now, now)
			}
			mock.ExpectQuery(`UPDATE scheduled_test_plans SET next_run_at = \$3, updated_at = NOW\(\) WHERE id = \$1 AND enabled = true AND next_run_at <= \$2 RETURNING`).
				WithArgs(int64(1), now, lease).WillReturnRows(rows)
			repo := NewScheduledTestPlanRepository(db).(service.ScheduledTestPlanClaimer)
			plan, err := repo.ClaimDue(context.Background(), 1, now, lease)
			require.NoError(t, err)
			if claimed {
				require.Equal(t, service.ScheduledTestModeClaudeWindow, plan.Mode)
				require.Equal(t, lease, *plan.NextRunAt)
			} else {
				require.Nil(t, plan)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestScheduledWindowPlanEditPreservesDatabaseDeadline(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	deadline := now.Add(5 * time.Hour)
	stale := now.Add(-time.Minute)
	mock.ExpectQuery(`UPDATE scheduled_test_plans SET .*next_run_at = CASE WHEN mode = 'claude_window' THEN next_run_at ELSE \$8 END`).
		WithArgs(int64(1), "claude-sonnet-4-6", "* * * * *", true, 50, false, "claude_window", stale).
		WillReturnRows(sqlmock.NewRows(windowPlanColumns).AddRow(1, 7, "claude-sonnet-4-6", "* * * * *", true, 50, false, "claude_window", now, deadline, now, now))
	plan, err := NewScheduledTestPlanRepository(db).Update(context.Background(), &service.ScheduledTestPlan{
		ID: 1, ModelID: "claude-sonnet-4-6", Mode: service.ScheduledTestModeClaudeWindow, CronExpression: "* * * * *", Enabled: true, MaxResults: 50, NextRunAt: &stale,
	})
	require.NoError(t, err)
	require.Equal(t, deadline, *plan.NextRunAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
