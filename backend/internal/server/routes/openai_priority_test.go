package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type priorityStatusStub struct {
	active bool
	err    error
	calls  int
}

func (s *priorityStatusStub) GetOpenAIPriorityStatus(context.Context) (service.OpenAIPriorityStatus, error) {
	s.calls++
	return service.OpenAIPriorityStatus{Active: s.active}, s.err
}
func TestOpenAIPriorityAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, platform, target string
		priority, active       bool
		err                    error
		want                   int
	}{
		{name: "ordinary blocked", platform: service.PlatformOpenAI, active: true, want: 503},
		{name: "priority admitted", platform: service.PlatformOpenAI, priority: true, active: true, want: 204},
		{name: "inactive", platform: service.PlatformOpenAI, want: 204},
		{name: "grok unaffected", platform: service.PlatformGrok, active: true, want: 204},
		{name: "anthropic unaffected", platform: service.PlatformAnthropic, active: true, want: 204},
		{name: "gemini unaffected", platform: service.PlatformGemini, active: true, want: 204},
		{name: "composite to openai", platform: service.PlatformComposite, target: service.PlatformOpenAI, active: true, want: 503},
		{name: "composite to grok", platform: service.PlatformComposite, target: service.PlatformGrok, active: true, want: 204},
		{name: "status unavailable", platform: service.PlatformOpenAI, err: errors.New("offline"), want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &priorityStatusStub{active: tc.active, err: tc.err}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{User: &service.User{OpenAIPriority: tc.priority}, Group: &service.Group{Platform: tc.platform}})
				if tc.target != "" {
					c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), tc.target))
				}
			}, openAIPriorityMiddleware(stub))
			router.POST("/v1/responses", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("POST", "/v1/responses", nil))
			require.Equal(t, tc.want, w.Code)
			if tc.want == 503 {
				require.Equal(t, "5", w.Header().Get("Retry-After"))
				if tc.err == nil {
					require.Contains(t, w.Body.String(), "openai_priority_required")
				} else {
					require.Contains(t, w.Body.String(), "openai_priority_unavailable")
				}
			}
			if tc.priority || tc.platform != service.PlatformOpenAI && tc.target != service.PlatformOpenAI {
				require.Zero(t, stub.calls)
			}
		})
	}
}
