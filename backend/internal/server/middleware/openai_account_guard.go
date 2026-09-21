package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const defaultOpenAIAccountGuardMessage = "当前服务暂时不可用：可用的 OpenAI 账号数量少于系统要求，请稍后再试。"

// OpenAIAccountAvailabilityGuard blocks gateway traffic while the configured
// pool has fewer usable OpenAI accounts. The cached count keeps the request
// path bounded while still reacting quickly to account recovery.
type OpenAIAccountAvailabilityGuard struct {
	checker openAIAccountAvailabilityChecker
	cfg     config.OpenAIAccountAvailabilityGuardConfig

	mu         sync.Mutex
	checkedAt  time.Time
	count      int
	locked     bool
	refreshing bool
}

type openAIAccountAvailabilityChecker interface {
	OpenAIAvailableAccountCount(context.Context) (int, error)
}

func NewOpenAIAccountAvailabilityGuard(checker openAIAccountAvailabilityChecker, cfg config.OpenAIAccountAvailabilityGuardConfig) *OpenAIAccountAvailabilityGuard {
	if cfg.CheckIntervalSeconds <= 0 {
		cfg.CheckIntervalSeconds = 30
	}
	if cfg.MinAvailableAccounts < 0 {
		cfg.MinAvailableAccounts = 0
	}
	if strings.TrimSpace(cfg.Message) == "" {
		cfg.Message = defaultOpenAIAccountGuardMessage
	}
	return &OpenAIAccountAvailabilityGuard{checker: checker, cfg: cfg}
}

func (g *OpenAIAccountAvailabilityGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil || !g.cfg.Enabled || g.isExempt(c) {
			c.Next()
			return
		}

		locked, count, err := g.isLocked(c.Request.Context())
		if err != nil {
			// A transient count query failure must not turn into a platform-wide
			// outage. Normal scheduler selection still provides its own safeguards.
			slog.Warn("openai_account_availability_guard_check_failed", "error", err)
			c.Next()
			return
		}
		if !locked {
			c.Next()
			return
		}

		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"type":               "service_unavailable",
				"code":               "OPENAI_ACCOUNT_POOL_LOCKED",
				"message":            g.cfg.Message,
				"available_accounts": count,
				"required_accounts":  g.cfg.MinAvailableAccounts,
			},
		})
	}
}

func (g *OpenAIAccountAvailabilityGuard) isLocked(ctx context.Context) (bool, int, error) {
	now := time.Now()
	g.mu.Lock()
	if !g.checkedAt.IsZero() && now.Sub(g.checkedAt) < time.Duration(g.cfg.CheckIntervalSeconds)*time.Second {
		locked, count := g.locked, g.count
		g.mu.Unlock()
		return locked, count, nil
	}
	if g.refreshing {
		locked, count := g.locked, g.count
		g.mu.Unlock()
		return locked, count, nil
	}
	g.refreshing = true
	g.mu.Unlock()

	if g.checker == nil {
		g.mu.Lock()
		g.refreshing = false
		g.checkedAt = now
		g.locked = false
		g.count = 0
		g.mu.Unlock()
		return false, 0, context.Canceled
	}
	count, err := g.checker.OpenAIAvailableAccountCount(ctx)
	if err != nil {
		g.mu.Lock()
		g.refreshing = false
		// Cache a fail-open result for the same interval so a database outage
		// cannot turn into one count query per request.
		g.checkedAt = now
		g.locked = false
		g.count = 0
		g.mu.Unlock()
		return false, 0, err
	}

	g.mu.Lock()
	g.refreshing = false
	g.checkedAt = now
	g.count = count
	g.locked = count < g.cfg.MinAvailableAccounts
	locked := g.locked
	g.mu.Unlock()
	return locked, count, nil
}

func (g *OpenAIAccountAvailabilityGuard) isExempt(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	requestPath := c.Request.URL.Path
	method := c.Request.Method
	for _, raw := range g.cfg.ExemptPaths {
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
