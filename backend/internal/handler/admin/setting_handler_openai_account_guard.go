package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetOpenAIAccountGuardSettings(c *gin.Context) {
	response.Success(c, h.settingService.OpenAIAccountGuard().Settings(c.Request.Context()))
}

func (h *SettingHandler) UpdateOpenAIAccountGuardSettings(c *gin.Context) {
	var cfg config.OpenAIAccountAvailabilityGuardConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid settings")
		return
	}
	if err := service.ValidateOpenAIAccountGuardSettings(cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	guard := h.settingService.OpenAIAccountGuard()
	if err := guard.SaveSettings(c.Request.Context(), cfg); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, guard.Settings(c.Request.Context()))
}
