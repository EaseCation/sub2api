package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIAccountGuardCheckerStub struct {
	count int
	calls atomic.Int32
	err   error
}

func (s *openAIAccountGuardCheckerStub) OpenAIAvailableAccountCount(context.Context) (int, error) {
	s.calls.Add(1)
	return s.count, s.err
}

func TestOpenAIAccountAvailabilityGuardLocksAndCaches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &openAIAccountGuardCheckerStub{count: 1}
	guard := NewOpenAIAccountAvailabilityGuard(checker, config.OpenAIAccountAvailabilityGuardConfig{
		Enabled:              true,
		MinAvailableAccounts: 2,
		CheckIntervalSeconds: 60,
		Message:              "maintenance",
	}).Middleware()

	router := gin.New()
	router.Use(guard)
	router.POST("/v1/messages", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	for i := 0; i < 2; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
		require.Equal(t, http.StatusServiceUnavailable, resp.Code)
		require.Contains(t, resp.Body.String(), "maintenance")
	}
	require.Equal(t, int32(1), checker.calls.Load(), "requests in one cache window share the count query")
}

func TestOpenAIAccountAvailabilityGuardExemptsConfiguredPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &openAIAccountGuardCheckerStub{count: 0}
	guard := NewOpenAIAccountAvailabilityGuard(checker, config.OpenAIAccountAvailabilityGuardConfig{
		Enabled:              true,
		MinAvailableAccounts: 2,
		CheckIntervalSeconds: 60,
		ExemptPaths:          []string{"GET /v1/models"},
	}).Middleware()

	router := gin.New()
	router.Use(guard)
	router.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusOK) })

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	require.Equal(t, http.StatusOK, resp.Code)
	require.Equal(t, int32(0), checker.calls.Load())
}

func TestOpenAIAccountAvailabilityGuardCachesCheckErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &openAIAccountGuardCheckerStub{err: errors.New("database unavailable")}
	guard := NewOpenAIAccountAvailabilityGuard(checker, config.OpenAIAccountAvailabilityGuardConfig{
		Enabled:              true,
		MinAvailableAccounts: 2,
		CheckIntervalSeconds: 60,
	}).Middleware()

	router := gin.New()
	router.Use(guard)
	router.POST("/v1/messages", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	for i := 0; i < 2; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
		require.Equal(t, http.StatusTeapot, resp.Code)
	}
	require.Equal(t, int32(1), checker.calls.Load(), "failed checks are also cached for the interval")
}
