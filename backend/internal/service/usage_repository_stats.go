package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"golang.org/x/sync/singleflight"
)

const repositoryStatsCacheTTL = 30 * time.Second

type repositoryStatsCacheEntry struct {
	page      *usagestats.RepositoryStatsPage
	expiresAt time.Time
}

// This cache and admission limit are confined to report reads, never the gateway.
// Bound both resident results and concurrent DB scans even with distinct filters.
type repositoryStatsCache struct {
	mu    sync.Mutex
	items map[string]repositoryStatsCacheEntry
	slots chan struct{}
	sf    singleflight.Group
}

func (c *repositoryStatsCache) get(key string) *usagestats.RepositoryStatsPage {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.page
	}
	delete(c.items, key)
	return nil
}

func (s *UsageService) GetRepositoryStats(ctx context.Context, start, end time.Time, filters usagestats.UsageLogFilters, page, pageSize int) (*usagestats.RepositoryStatsPage, error) {
	if start.IsZero() || !end.After(start) || end.After(start.AddDate(1, 0, 0)) {
		return nil, infraerrors.BadRequest("INVALID_REPOSITORY_RANGE", "Choose a date range of at most one year")
	}
	if page < 1 || page > 1000 || pageSize < 1 || pageSize > 100 {
		return nil, infraerrors.BadRequest("INVALID_REPOSITORY_PAGE", "page must be 1-1000 and page_size 1-100")
	}
	repo, ok := s.usageRepo.(interface {
		GetRepositoryStats(context.Context, time.Time, time.Time, usagestats.UsageLogFilters, int, int) ([]usagestats.RepositoryStat, error)
	})
	if !ok {
		return nil, fmt.Errorf("repository usage statistics unavailable")
	}
	// Normalize range pointers before keying to avoid equivalent filters missing
	// the cache. UserID is part of the key, keeping user results isolated.
	filters.StartTime, filters.EndTime = &start, &end
	rawKey, err := json.Marshal(struct {
		Filters  usagestats.UsageLogFilters
		Page     int
		PageSize int
	}{filters, page, pageSize})
	if err != nil {
		return nil, err
	}
	key := string(rawKey)
	cache := &s.repositoryStatsCache
	if cached := cache.get(key); cached != nil {
		return cached, nil
	}
	result := cache.sf.DoChan(key, func() (any, error) {
		if cached := cache.get(key); cached != nil {
			return cached, nil
		}
		cache.mu.Lock()
		if cache.slots == nil {
			cache.slots = make(chan struct{}, 2)
		}
		slots := cache.slots
		cache.mu.Unlock()
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			return nil, infraerrors.New(http.StatusServiceUnavailable, "REPOSITORY_STATS_BUSY", "Repository statistics are busy; retry shortly")
		}
		// Shared work has its own deadline. Canceling one HTTP request does not
		// cancel the result for other waiters on the same report.
		queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		rows, err := repo.GetRepositoryStats(queryCtx, start, end, filters, pageSize+1, (page-1)*pageSize)
		if err != nil {
			return nil, err
		}
		hasMore := len(rows) > pageSize
		if hasMore {
			rows = rows[:pageSize]
		}
		if rows == nil {
			rows = []usagestats.RepositoryStat{}
		}
		out := &usagestats.RepositoryStatsPage{Repositories: rows, Page: page, PageSize: pageSize, HasMore: hasMore, GeneratedAt: time.Now()}
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if cache.items == nil {
			cache.items = make(map[string]repositoryStatsCacheEntry)
		}
		if len(cache.items) >= 256 {
			// Evict the oldest result; bounded cache size avoids unbounded growth
			// from users exploring many different periods and filters.
			oldestKey := ""
			var oldest time.Time
			for k, entry := range cache.items {
				if oldestKey == "" || entry.expiresAt.Before(oldest) {
					oldestKey, oldest = k, entry.expiresAt
				}
			}
			delete(cache.items, oldestKey)
		}
		cache.items[key] = repositoryStatsCacheEntry{page: out, expiresAt: time.Now().Add(repositoryStatsCacheTTL)}
		return out, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		page, ok := value.Val.(*usagestats.RepositoryStatsPage)
		if !ok || page == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "REPOSITORY_STATS_INVALID", "Invalid repository statistics result")
		}
		return page, nil
	}
}
