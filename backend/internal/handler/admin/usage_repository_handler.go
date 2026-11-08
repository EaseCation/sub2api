package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *UsageHandler) RepositoryStats(c *gin.Context) {
	filters := usagestats.UsageLogFilters{Model: strings.TrimSpace(c.Query("model")), ModelFilterSource: usagestats.ModelSourceRequested, BillingMode: strings.TrimSpace(c.Query("billing_mode"))}
	for name, target := range map[string]*int64{"user_id": &filters.UserID, "api_key_id": &filters.APIKeyID, "account_id": &filters.AccountID, "group_id": &filters.GroupID} {
		if raw := strings.TrimSpace(c.Query(name)); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id < 1 {
				response.BadRequest(c, "Invalid "+name)
				return
			}
			*target = id
		}
	}
	if raw := strings.TrimSpace(c.Query("request_type")); raw != "" {
		requestType, err := service.ParseUsageRequestType(raw)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		value := int16(requestType)
		filters.RequestType = &value
	}
	for name, target := range map[string]**bool{"stream": &filters.Stream, "native_compaction_v2": &filters.NativeCompactionV2, "upstream_model_mismatch": &filters.UpstreamModelMismatch} {
		value, err := parseOptionalBoolDashboardFilter(c, name)
		if err != nil {
			response.BadRequest(c, "Invalid "+name)
			return
		}
		*target = value
	}
	if raw := strings.TrimSpace(c.Query("billing_type")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 8)
		if err != nil || value < 0 || value > 1 {
			response.BadRequest(c, "Invalid billing_type")
			return
		}
		billingType := int8(value)
		filters.BillingType = &billingType
	}
	if filters.BillingMode != "" && !service.BillingMode(filters.BillingMode).IsValidUsageFilter() {
		response.BadRequest(c, "Invalid billing_mode")
		return
	}
	// Strict parsing and calendar-day boundaries, including DST transitions.
	tz := c.Query("timezone")
	now := timezone.NowInUserLocation(tz)
	start := timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -7), tz)
	end := timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), tz)
	if raw := c.Query("start_date"); raw != "" {
		value, err := timezone.ParseInUserLocation("2006-01-02", raw, tz)
		if err != nil {
			response.BadRequest(c, "Invalid start_date")
			return
		}
		start = value
	}
	if raw := c.Query("end_date"); raw != "" {
		value, err := timezone.ParseInUserLocation("2006-01-02", raw, tz)
		if err != nil {
			response.BadRequest(c, "Invalid end_date")
			return
		}
		end = value.AddDate(0, 0, 1)
	}
	page, pageSize := response.ParsePagination(c)
	stats, err := h.usageService.GetRepositoryStats(c.Request.Context(), start, end, filters, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}
