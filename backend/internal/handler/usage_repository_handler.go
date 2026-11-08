package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// RepositoryStats uses the same date, timezone and usage filters as the other
// user reports. parseUserUsageFilters always scopes UserID to the JWT subject
// and verifies API key ownership, ignoring caller-supplied user/account IDs.
func (h *UsageHandler) RepositoryStats(c *gin.Context) {
	parsed, ok := h.parseUserUsageFilters(c, true)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	stats, err := h.usageService.GetRepositoryStats(c.Request.Context(), parsed.StartTime, parsed.EndTime, parsed.Filters, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}
