package routes

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

// Run after composite resolution so only requests actually targeting OpenAI are gated.
type openAIPriorityStatusReader interface {
	GetOpenAIPriorityStatus(context.Context) (service.OpenAIPriorityStatus, error)
}

func openAIPriorityMiddleware(settings openAIPriorityStatusReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		if getGroupPlatform(c) != service.PlatformOpenAI {
			c.Next()
			return
		}
		key, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || key.User == nil {
			c.Next()
			return
		} // Authentication owns missing-user errors.
		if key.User.OpenAIPriority {
			c.Next()
			return
		}
		status, err := settings.GetOpenAIPriorityStatus(c.Request.Context())
		if err == nil && !status.Active {
			c.Next()
			return
		}
		code, message := "openai_priority_required", "OpenAI priority guarantee mode is active. Only priority-enabled users can use OpenAI. Please try again later."
		if err != nil {
			code, message = "openai_priority_unavailable", "OpenAI priority status temporarily unavailable. Please try again later."
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
		c.Header("Retry-After", "5")
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "service_unavailable", "code": code, "message": message}})
	}
}
