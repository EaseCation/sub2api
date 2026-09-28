package handler

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

// Public so every page, including signed-out pages, can display the same alert.
// Account inventory and capacity are deliberately not part of this public API.
func (h *SettingHandler) GetOpenAIPriorityStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	status, err := h.settingService.GetOpenAIPriorityStatus(c.Request.Context())
	if err != nil {
		response.Error(c, 503, "OpenAI priority status temporarily unavailable")
		return
	}
	response.Success(c, gin.H{"active": status.Active})
}

var errOpenAIWSPriorityDenied = errors.New("OpenAI priority admission denied")

// Existing sockets must recheck admission before every response.create. Refresh
// the key through the normal auth cache so administrator revocations also apply.
func (h *OpenAIGatewayHandler) checkOpenAIPriorityWSTurn(ctx context.Context, account *service.Account, key *service.APIKey) error {
	if account == nil || account.Platform != service.PlatformOpenAI || h.openAIPriorityStatus == nil {
		return nil
	}
	status, err := h.openAIPriorityStatus(ctx)
	if err == nil && !status.Active {
		return nil
	}
	if err == nil && h.apiKeyService != nil {
		key, err = h.apiKeyService.GetByKey(ctx, key.Key)
	}
	if err == nil && key != nil && key.User != nil && key.User.OpenAIPriority {
		return nil
	}
	message := "OpenAI priority mode is active; priority-enabled users only"
	if err != nil {
		message = "OpenAI priority status unavailable; please retry later"
	}
	return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, message, errOpenAIWSPriorityDenied)
}
