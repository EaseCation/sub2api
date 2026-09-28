package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GroupSessionSkill runs after authentication and before composite routing.
// Non-text endpoints and WebSocket handshakes are never read or rewritten.
func GroupSessionSkill() gin.HandlerFunc {
	return func(c *gin.Context) {
		skill := service.GroupSessionSkillFromContext(c)
		c.Set(service.GroupSessionSkillContextKey, skill)
		if skill == "" || c.Request == nil || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		protocol := groupSessionSkillProtocol(c.Request.URL.Path)
		if protocol == "" {
			c.Next()
			return
		}
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err == nil {
			body, err = service.InjectGroupSessionSkill(body, protocol, skill)
		}
		if err != nil {
			status := http.StatusBadRequest
			message := "Invalid request for group Session Skill: " + err.Error()
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status, message = http.StatusRequestEntityTooLarge, "Request body is too large"
			}
			groupModelAllowlistErrorWriter(c)(c, status, message)
			c.Abort()
			return
		}
		requestmodel.ResetRequestBody(c.Request, body)
		c.Next()
	}
}

func groupSessionSkillProtocol(path string) string {
	switch {
	case strings.HasSuffix(path, "/messages"), strings.HasSuffix(path, "/messages/count_tokens"):
		return service.ContentModerationProtocolAnthropicMessages
	case strings.HasSuffix(path, "/chat/completions"):
		return service.ContentModerationProtocolOpenAIChat
	case strings.HasSuffix(path, "/responses"):
		return service.ContentModerationProtocolOpenAIResponses
	case strings.HasSuffix(path, ":generateContent"), strings.HasSuffix(path, ":streamGenerateContent"), strings.HasSuffix(path, ":countTokens"):
		return service.ContentModerationProtocolGemini
	default:
		return ""
	}
}
