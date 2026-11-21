//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type discoveryNativeConn struct{ *stagedPassthroughConn }

func (c *discoveryNativeConn) WriteJSON(ctx context.Context, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, body)
}

func TestRepositoryDiscoveryWebSocketProbeAndToolResult(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModePassthrough, OpenAIWSIngressModeCtxPool} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.RepositoryAutoDetect = true
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			upstream := newStagedPassthroughConn()
			svc := newPassthroughLifecycleService(cfg, upstream)
			account := passthroughLifecycleAccount()
			account.Extra["openai_apikey_responses_websockets_v2_mode"] = mode
			if mode == OpenAIWSIngressModeCtxPool {
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(&stagedPassthroughDialer{conn: &discoveryNativeConn{upstream}})
				svc.openaiWSPool = pool
			}
			done := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					done <- err
					return
				}
				defer conn.CloseNow()
				_, first, err := conn.Read(ctx)
				if err != nil {
					done <- err
					return
				}
				c := discoveryTestContext(7, "ws-session")
				c.Set(GroupSessionSkillContextKey, "Keep existing group instructions")
				done <- svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "test-token", first, nil)
			}))
			defer server.Close()
			firstPayload, err := sjson.Set(repositoryDiscoveryTestBody, "type", "response.create")
			require.NoError(t, err)
			client := dialPassthroughLifecycleClientWithPayload(t, server, firstPayload)
			defer client.CloseNow()
			first := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
			instructions := gjson.GetBytes(first, "instructions").String()
			require.Contains(t, instructions, "Sub2API repository attribution")
			require.Contains(t, instructions, "Keep existing group instructions")
			require.Contains(t, instructions, "client rules")
			marker := discoveryTestMarker(svc, 7, "ws-session")
			upstream.Send(`{"type":"response.completed","response":{"id":"resp_unchanged","model":"gpt-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
			_, err = readPassthroughLifecycleFrame(t, client, 3*time.Second)
			require.NoError(t, err)
			follow, err := sjson.SetBytes(discoveryTestOutput(marker, "git@github.com:Owner/Repo.git"), "type", "response.create")
			require.NoError(t, err)
			require.NoError(t, client.Write(ctx, coderws.MessageText, follow))
			next := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
			require.NotContains(t, gjson.GetBytes(next, "instructions").String(), "Sub2API repository attribution")
			require.Contains(t, gjson.GetBytes(next, "instructions").String(), "Keep existing group instructions")
			require.Equal(t, "resp_unchanged", gjson.GetBytes(next, "previous_response_id").String())
			require.Equal(t, "github.com/owner/repo", *svc.discoveredUsageRepository(7, "ws-session"))
			_ = client.CloseNow()
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("websocket did not shut down")
			}
		})
	}
}
