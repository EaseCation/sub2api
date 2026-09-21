package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const openAIAccountGuardSettingKey = "openai_account_availability_guard"
const DefaultOpenAIAccountGuardMessage = "当前服务暂时不可用：可用的 OpenAI 账号数量少于系统要求，请稍后再试。"

// One instance is shared by gateway admission and the public status endpoint.
// Settings are cached for 30 seconds; saves update the local cache immediately.
// Other replicas observe changes on their next settings refresh.
type OpenAIAccountGuard struct {
	settingsMu    sync.Mutex
	settings      config.OpenAIAccountAvailabilityGuardConfig
	settingsUntil time.Time
	repo          SettingRepository

	mu         sync.Mutex
	checker    OpenAIAccountAvailabilityChecker
	checkedAt  time.Time
	count      int
	countErr   error
	refreshing bool
}

type OpenAIAccountAvailabilityChecker interface {
	OpenAIAvailableAccountCount(context.Context) (int, error)
}

type OpenAIAccountGuardStatus struct {
	Locked  bool   `json:"locked"`
	Message string `json:"message"`
}

func NewOpenAIAccountGuard(checker OpenAIAccountAvailabilityChecker, cfg config.OpenAIAccountAvailabilityGuardConfig, repo SettingRepository) *OpenAIAccountGuard {
	if cfg.CheckIntervalSeconds <= 0 {
		cfg.CheckIntervalSeconds = 30
	}
	if strings.TrimSpace(cfg.Message) == "" {
		cfg.Message = DefaultOpenAIAccountGuardMessage
	}
	return &OpenAIAccountGuard{checker: checker, settings: cfg, repo: repo}
}

func (s *SettingService) OpenAIAccountGuard() *OpenAIAccountGuard {
	if s == nil {
		return NewOpenAIAccountGuard(nil, config.OpenAIAccountAvailabilityGuardConfig{
			MinAvailableAccounts: 2,
			CheckIntervalSeconds: 30,
		}, nil)
	}
	s.openAIAccountGuardOnce.Do(func() {
		cfg := config.OpenAIAccountAvailabilityGuardConfig{MinAvailableAccounts: 2, CheckIntervalSeconds: 30}
		if s.cfg != nil {
			cfg = s.cfg.Gateway.OpenAIAccountAvailabilityGuard
		}
		s.openAIAccountGuard = NewOpenAIAccountGuard(nil, cfg, s.settingRepo)
	})
	return s.openAIAccountGuard
}

// SetChecker is called at route registration, before requests are served.
func (g *OpenAIAccountGuard) SetChecker(checker OpenAIAccountAvailabilityChecker) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checker = checker
}

func (g *OpenAIAccountGuard) Settings(ctx context.Context) config.OpenAIAccountAvailabilityGuardConfig {
	g.settingsMu.Lock()
	defer g.settingsMu.Unlock()
	if g.repo != nil && time.Now().After(g.settingsUntil) {
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		raw, err := g.repo.GetValue(dbCtx, openAIAccountGuardSettingKey)
		if err == nil && raw != "" {
			cfg := g.settings
			if json.Unmarshal([]byte(raw), &cfg) == nil && ValidateOpenAIAccountGuardSettings(cfg) == nil {
				g.settings = cfg
			}
		}
		// Retain last-known settings on failure, and cache failures too.
		g.settingsUntil = time.Now().Add(30 * time.Second)
	}
	cfg := g.settings
	cfg.ExemptPaths = append([]string{}, cfg.ExemptPaths...)
	return cfg
}

func ValidateOpenAIAccountGuardSettings(cfg config.OpenAIAccountAvailabilityGuardConfig) error {
	if cfg.MinAvailableAccounts < 1 || cfg.MinAvailableAccounts > 100000 {
		return fmt.Errorf("minimum available accounts must be between 1 and 100000")
	}
	if cfg.CheckIntervalSeconds < 1 || cfg.CheckIntervalSeconds > 3600 {
		return fmt.Errorf("check interval must be between 1 and 3600 seconds")
	}
	if strings.TrimSpace(cfg.Message) == "" || len(cfg.Message) > 4000 {
		return fmt.Errorf("message must be non-empty and at most 4000 bytes")
	}
	if len(cfg.ExemptPaths) > 50 {
		return fmt.Errorf("at most 50 exempt paths are allowed")
	}
	for _, entry := range cfg.ExemptPaths {
		fields := strings.Fields(entry)
		if len(fields) < 1 || len(fields) > 2 {
			return fmt.Errorf("invalid exempt path: %s", entry)
		}
		p := fields[len(fields)-1]
		if !strings.HasPrefix(p, "/") || len(p) > 256 {
			return fmt.Errorf("invalid exempt path: %s", entry)
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("invalid exempt path: %s", entry)
		}
		if len(fields) == 2 {
			switch fields[0] {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
			default:
				return fmt.Errorf("invalid HTTP method: %s", fields[0])
			}
		}
	}
	return nil
}

func (g *OpenAIAccountGuard) SaveSettings(ctx context.Context, cfg config.OpenAIAccountAvailabilityGuardConfig) error {
	if err := ValidateOpenAIAccountGuardSettings(cfg); err != nil {
		return err
	}
	cfg.Message = strings.TrimSpace(cfg.Message)
	cfg.ExemptPaths = append([]string{}, cfg.ExemptPaths...)
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	g.settingsMu.Lock()
	defer g.settingsMu.Unlock()
	if g.repo == nil {
		return errors.New("setting repository is unavailable")
	}
	if err := g.repo.Set(ctx, openAIAccountGuardSettingKey, string(data)); err != nil {
		return err
	}
	g.settings = cfg
	g.settingsUntil = time.Now().Add(30 * time.Second)
	g.mu.Lock()
	g.checkedAt = time.Time{}
	g.countErr = nil
	g.mu.Unlock()
	return nil
}

func (g *OpenAIAccountGuard) Check(ctx context.Context, cfg config.OpenAIAccountAvailabilityGuardConfig) (bool, int) {
	if !cfg.Enabled {
		return false, 0
	}
	g.mu.Lock()
	if g.refreshing || (!g.checkedAt.IsZero() && time.Since(g.checkedAt) < time.Duration(cfg.CheckIntervalSeconds)*time.Second) {
		locked, count := g.countErr == nil && !g.checkedAt.IsZero() && g.count < cfg.MinAvailableAccounts, g.count
		g.mu.Unlock()
		return locked, count
	}
	g.refreshing = true
	checker := g.checker
	g.mu.Unlock()

	// Do not let a cancelled caller or a stalled DB cause repeated checks.
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	count, err := 0, errors.New("account checker is unavailable")
	if checker != nil {
		count, err = checker.OpenAIAvailableAccountCount(dbCtx)
	}
	if err != nil {
		slog.Warn("openai_account_availability_guard_check_failed", "error", err)
	}
	g.mu.Lock()
	g.refreshing = false
	g.checkedAt = time.Now()
	g.count, g.countErr = count, err
	g.mu.Unlock()
	return err == nil && count < cfg.MinAvailableAccounts, count
}

func (g *OpenAIAccountGuard) Status(ctx context.Context) OpenAIAccountGuardStatus {
	cfg := g.Settings(ctx)
	locked, _ := g.Check(ctx, cfg)
	status := OpenAIAccountGuardStatus{Locked: locked}
	if locked {
		status.Message = cfg.Message
	}
	return status
}
