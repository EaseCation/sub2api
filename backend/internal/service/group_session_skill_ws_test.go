//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGroupSessionSkillWebSocketFollowUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upstream := newStagedPassthroughConn()
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	account := passthroughLifecycleAccount()
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
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		c.Set(GroupSessionSkillContextKey, "Confirm EaseCation purpose")
		done <- svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "test-token", first, nil)
	}))
	defer server.Close()
	client := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-5.1","instructions":"client rules","input":"task"}`)
	defer client.CloseNow()
	first := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Contains(t, gjson.GetBytes(first, "instructions").String(), "Confirm EaseCation purpose")
	require.Contains(t, gjson.GetBytes(first, "instructions").String(), "client rules")
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_skill_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	err = client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","instructions":"updated rules","previous_response_id":"resp_skill_1","input":"I confirm"}`))
	require.NoError(t, err)
	next := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Contains(t, gjson.GetBytes(next, "instructions").String(), "Confirm EaseCation purpose")
	require.Contains(t, gjson.GetBytes(next, "instructions").String(), "updated rules")
	require.Equal(t, "I confirm", gjson.GetBytes(next, "input").String())
	require.Equal(t, "resp_skill_1", gjson.GetBytes(next, "previous_response_id").String())
	_ = client.CloseNow()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WebSocket did not shut down")
	}
}
