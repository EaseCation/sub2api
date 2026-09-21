package middleware

import (
	"net/http"
	"path"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type OpenAIAccountAvailabilityGuard struct{ guard *service.OpenAIAccountGuard }

func NewOpenAIAccountAvailabilityGuard(checker service.OpenAIAccountAvailabilityChecker, cfg config.OpenAIAccountAvailabilityGuardConfig) *OpenAIAccountAvailabilityGuard {
	return &OpenAIAccountAvailabilityGuard{guard: service.NewOpenAIAccountGuard(checker, cfg, nil)}
}

func OpenAIAccountGuardMiddleware(guard *service.OpenAIAccountGuard) gin.HandlerFunc {
	return (&OpenAIAccountAvailabilityGuard{guard: guard}).Middleware()
}

func (g *OpenAIAccountAvailabilityGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil || g.guard == nil {
			c.Next()
			return
		}
		cfg := g.guard.Settings(c.Request.Context())
		if !cfg.Enabled || isOpenAIAccountGuardExempt(c, cfg) {
			c.Next()
			return
		}
		locked, count := g.guard.Check(c.Request.Context(), cfg)
		if !locked {
			c.Next()
			return
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"type":               "service_unavailable",
				"code":               "OPENAI_ACCOUNT_POOL_LOCKED",
				"message":            cfg.Message,
				"available_accounts": count,
				"required_accounts":  cfg.MinAvailableAccounts,
			},
		})
	}
}

func isOpenAIAccountGuardExempt(c *gin.Context, cfg config.OpenAIAccountAvailabilityGuardConfig) bool {
	if c == nil || c.Request == nil {
		return false
	}
	requestPath := c.Request.URL.Path
	method := c.Request.Method
	for _, raw := range cfg.ExemptPaths {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		entryMethod := ""
		entryPath := entry
		if fields := strings.Fields(entry); len(fields) == 2 {
			entryMethod, entryPath = strings.ToUpper(fields[0]), fields[1]
		}
		if entryMethod != "" && entryMethod != method {
			continue
		}
		if strings.HasSuffix(entryPath, "*") {
			if strings.HasPrefix(requestPath, strings.TrimSuffix(entryPath, "*")) {
				return true
			}
			continue
		}
		if ok, err := path.Match(entryPath, requestPath); err == nil && ok {
			return true
		}
	}
	return false
}
