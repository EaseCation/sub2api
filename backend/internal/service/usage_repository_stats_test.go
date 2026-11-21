package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type repositoryStatsTestRepo struct {
	UsageLogRepository
	load func(context.Context, time.Time, time.Time, usagestats.UsageLogFilters, int, int) ([]usagestats.RepositoryStat, error)
}

func (r *repositoryStatsTestRepo) GetRepositoryStats(ctx context.Context, start, end time.Time, filters usagestats.UsageLogFilters, limit, offset int) ([]usagestats.RepositoryStat, error) {
	return r.load(ctx, start, end, filters, limit, offset)
}

func TestRepositoryStatsCacheScopePaginationAndBounds(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	var calls atomic.Int32
	repo := &repositoryStatsTestRepo{load: func(ctx context.Context, gotStart, gotEnd time.Time, f usagestats.UsageLogFilters, limit, offset int) ([]usagestats.RepositoryStat, error) {
		calls.Add(1)
		require.Equal(t, start, gotStart)
		require.Equal(t, end, gotEnd)
		require.Equal(t, 3, limit)
		require.Zero(t, offset)
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
		return []usagestats.RepositoryStat{{Repository: "first", ActualCost: float64(f.UserID)}, {Repository: "second"}, {Repository: "third"}}, nil
	}}
	s := NewUsageService(repo, nil, nil, nil)
	f := usagestats.UsageLogFilters{UserID: 1}
	page, err := s.GetRepositoryStats(context.Background(), start, end, f, 1, 2)
	require.NoError(t, err)
	require.True(t, page.HasMore)
	require.Len(t, page.Repositories, 2)
	_, err = s.GetRepositoryStats(context.Background(), start, end, f, 1, 2)
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	f.UserID = 2
	other, err := s.GetRepositoryStats(context.Background(), start, end, f, 1, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, other.Repositories[0].ActualCost)
	require.EqualValues(t, 2, calls.Load(), "user-specific cache keys must not leak another user's report")
	for _, span := range []struct{ start, end time.Time }{{start, start}, {end, start}, {start, start.AddDate(1, 0, 1)}, {time.Time{}, end}} {
		_, err := s.GetRepositoryStats(context.Background(), span.start, span.end, f, 1, 20)
		require.True(t, infraerrors.IsBadRequest(err))
	}
	_, err = s.GetRepositoryStats(context.Background(), start, end, f, 1001, 20)
	require.True(t, infraerrors.IsBadRequest(err))
	require.EqualValues(t, 2, calls.Load(), "invalid ranges and pages must never query the DB")
}

func TestRepositoryStatsSingleflightAndAdmissionLimit(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	repo := &repositoryStatsTestRepo{load: func(ctx context.Context, _, _ time.Time, _ usagestats.UsageLogFilters, _, _ int) ([]usagestats.RepositoryStat, error) {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	s := NewUsageService(repo, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for _, id := range []int64{1, 2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.GetRepositoryStats(ctx, start, start.AddDate(0, 0, 1), usagestats.UsageLogFilters{UserID: id}, 1, 20)
		}()
		<-entered
	}
	_, err := s.GetRepositoryStats(ctx, start, start.AddDate(0, 0, 1), usagestats.UsageLogFilters{UserID: 3}, 1, 20)
	require.Equal(t, "REPOSITORY_STATS_BUSY", infraerrors.Reason(err))
	// An identical report shares existing work, even while both DB slots are full.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = s.GetRepositoryStats(ctx, start, start.AddDate(0, 0, 1), usagestats.UsageLogFilters{UserID: 1}, 1, 20)
	}()
	cancel()
	wg.Wait()
	require.EqualValues(t, 2, calls.Load())
}
