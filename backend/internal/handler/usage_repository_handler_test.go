package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type repositoryUsageHandlerRepo struct {
	service.UsageLogRepository
	filters    usagestats.UsageLogFilters
	start, end time.Time
	calls      int
}

func (r *repositoryUsageHandlerRepo) GetRepositoryStats(_ context.Context, start, end time.Time, filters usagestats.UsageLogFilters, _, _ int) ([]usagestats.RepositoryStat, error) {
	r.calls++
	r.start, r.end, r.filters = start, end, filters
	return []usagestats.RepositoryStat{{Repository: "github.com/a/b", ActualCost: 0.75}}, nil
}

func TestUserRepositoryStatsScopeDatesAndKeyOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &repositoryUsageHandlerRepo{}
	keys := &dailyUsageAPIKeyRepoStub{keys: map[int64]*service.APIKey{7: {ID: 7, UserID: 99}, 8: {ID: 8, UserID: 42}}}
	h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), service.NewAPIKeyService(keys, nil, nil, nil, nil, nil, nil), nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage/repositories", h.RepositoryStats)
	get := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/repositories?"+query, nil))
		return w
	}
	w := get("start_date=2026-11-01&end_date=2026-11-01&timezone=America%2FToronto&user_id=99&account_id=123&api_key_id=8&model=gpt-5&billing_mode=token")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.EqualValues(t, 42, repo.filters.UserID)
	require.Zero(t, repo.filters.AccountID)
	require.EqualValues(t, 8, repo.filters.APIKeyID)
	require.Equal(t, "gpt-5", repo.filters.Model)
	require.Equal(t, "token", repo.filters.BillingMode)
	require.Equal(t, 25*time.Hour, repo.end.Sub(repo.start), "end date must include the entire DST fallback day")
	require.Equal(t, http.StatusForbidden, get("api_key_id=7").Code)
	require.Equal(t, http.StatusBadRequest, get("start_date=2026-11-08&end_date=2026-11-01").Code)
	require.Equal(t, http.StatusBadRequest, get("start_date=2024-01-01&end_date=2026-11-01").Code)
	require.Equal(t, 1, repo.calls, "rejected report requests must never reach the repository")
}
