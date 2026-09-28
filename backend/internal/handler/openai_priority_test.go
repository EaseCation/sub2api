package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIPriorityWebSocketRechecksEveryTurn(t *testing.T) {
	ctx := context.Background()
	active := false
	h := &OpenAIGatewayHandler{openAIPriorityStatus: func(context.Context) (service.OpenAIPriorityStatus, error) {
		return service.OpenAIPriorityStatus{Active: active}, nil
	}}
	account := &service.Account{Platform: service.PlatformOpenAI}
	key := &service.APIKey{User: &service.User{}}
	require.NoError(t, h.checkOpenAIPriorityWSTurn(ctx, account, key))
	active = true
	err := h.checkOpenAIPriorityWSTurn(ctx, account, key)
	require.ErrorIs(t, err, errOpenAIWSPriorityDenied)
	require.False(t, shouldReportOpenAIWSProxyAccountFailure(err), "admission rejection must not degrade upstream account health")
	key.User.OpenAIPriority = true
	require.NoError(t, h.checkOpenAIPriorityWSTurn(ctx, account, key))
	key.User.OpenAIPriority = false
	account.Platform = service.PlatformGrok
	require.NoError(t, h.checkOpenAIPriorityWSTurn(ctx, account, key))
	account.Platform = service.PlatformOpenAI
	active = false
	require.NoError(t, h.checkOpenAIPriorityWSTurn(ctx, account, key))
	h.openAIPriorityStatus = func(context.Context) (service.OpenAIPriorityStatus, error) {
		return service.OpenAIPriorityStatus{}, errors.New("offline")
	}
	require.ErrorIs(t, h.checkOpenAIPriorityWSTurn(ctx, account, key), errOpenAIWSPriorityDenied)
}
