//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type repositoryDiscoveryHTTPUpstream struct {
	service.HTTPUpstream
	bodies [][]byte
}

func (u *repositoryDiscoveryHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, body)
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_repository_test","object":"response","model":"gpt-5.2","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))}, nil
}

func TestHTTPRepositoryDiscoveryReachesUpstreamOnlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.RepositoryAutoDetect = true
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	groupID := int64(42)
	accounts := &openAIWSFailoverHandlerAccountRepoStub{accounts: []service.Account{{
		ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-upstream", "base_url": "https://api.example.test"}, Extra: map[string]any{"openai_passthrough": true},
	}}}
	upstream := &repositoryDiscoveryHTTPUpstream{}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gw := service.NewOpenAIGatewayService(accounts, nil, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpenAIGatewayHandler(gw, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	request := func(session string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.2","stream":false,"instructions":"keep client instructions","input":"task","tools":[{"type":"function","name":"exec_command","parameters":{"type":"object"}}]}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("User-Agent", "Codex Desktop/0.144.4")
		c.Request.Header.Set("X-Codex-Turn-Metadata", fmt.Sprintf(`{"thread_id":%q,"session_id":%q,"turn_id":"turn"}`, session, session))
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 7, Key: "sk-client", GroupID: &groupID, User: &service.User{ID: 8, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 8})
		h.Responses(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	request("session-a")
	request("session-a")
	request("session-b")
	require.Len(t, upstream.bodies, 3)
	for i, body := range upstream.bodies {
		instructions := gjson.GetBytes(body, "instructions").String()
		require.Contains(t, instructions, "keep client instructions")
		if i == 1 {
			require.NotContains(t, instructions, "Sub2API repository attribution")
		} else {
			require.Contains(t, instructions, "Sub2API repository attribution")
		}
		require.Equal(t, "task", gjson.GetBytes(body, "input").String())
	}
}
