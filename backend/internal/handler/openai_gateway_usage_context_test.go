package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// Request metadata must be copied when queued, before the request is released.
type releasedUsageRequestContext struct {
	context.Context
	released bool
}

func (c *releasedUsageRequestContext) Value(key any) any {
	if c.released {
		panic("queued billing task accessed a released request context")
	}
	return c.Context.Value(key)
}

func TestQueuedUsageTaskSnapshotsMetadataBeforeRequestRelease(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(
		context.WithValue(context.Background(), ctxkey.ClientRequestID, " client-123 "),
		ctxkey.RequestID, " request-456 ",
	))
	defer cancel()
	request := &releasedUsageRequestContext{Context: parent}
	task, abandon := wrapUsageRecordTaskContext(request, func(ctx context.Context) {
		require.Equal(t, "client-123", ctx.Value(ctxkey.ClientRequestID))
		require.Equal(t, "request-456", ctx.Value(ctxkey.RequestID))
		require.NoError(t, ctx.Err(), "request cancellation must not cancel billing")
		_, ok := ctx.Deadline()
		require.True(t, ok, "billing must keep the worker timeout")
	})
	defer abandon()
	request.released = true
	cancel()
	worker, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NotPanics(t, func() { task(worker) })
}

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
}
