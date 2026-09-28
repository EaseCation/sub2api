package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetOpenAIPrioritySettings(c *gin.Context) {
	settings, err := h.settingService.GetOpenAIPrioritySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *SettingHandler) UpdateOpenAIPrioritySettings(c *gin.Context) {
	var req struct {
		Enabled   *bool `json:"enabled" binding:"required"`
		Threshold *int  `json:"threshold" binding:"required,min=0,max=1000000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "enabled and a non-negative integer threshold are required")
		return
	}
	settings := service.OpenAIPrioritySettings{Enabled: *req.Enabled, Threshold: *req.Threshold}
	if err := h.settingService.SetOpenAIPrioritySettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}
