package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const openAIPrioritySettingKey = "openai_priority_guarantee"
const openAIPriorityStateTTL = 2 * time.Second

// OpenAIPrioritySettings reserves OpenAI capacity for opted-in users when supply is low.
type OpenAIPrioritySettings struct {
	Enabled   bool `json:"enabled"`
	Threshold int  `json:"threshold"`
}

type OpenAIPriorityStatus struct {
	Active            bool `json:"active"`
	AvailableAccounts int  `json:"available_accounts"`
}

type openAIPriorityAccountReader interface {
	ListSchedulableByPlatform(context.Context, string) ([]Account, error)
}

type openAIPriorityRuntime struct {
	mu        sync.Mutex
	accounts  openAIPriorityAccountReader
	status    OpenAIPriorityStatus
	err       error
	expiresAt time.Time
}

func (s *SettingService) GetOpenAIPrioritySettings(ctx context.Context) (OpenAIPrioritySettings, error) {
	result := OpenAIPrioritySettings{Threshold: 2}
	raw, err := s.settingRepo.GetValue(ctx, openAIPrioritySettingKey)
	if errors.Is(err, ErrSettingNotFound) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			return result, err
		}
	}
	if result.Threshold < 0 {
		return result, fmt.Errorf("invalid OpenAI priority threshold")
	}
	return result, nil
}

func (s *SettingService) SetOpenAIPrioritySettings(ctx context.Context, value OpenAIPrioritySettings) error {
	if value.Threshold < 0 {
		return fmt.Errorf("threshold must be non-negative")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	r := &s.openAIPriority
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := s.settingRepo.Set(ctx, openAIPrioritySettingKey, string(raw)); err != nil {
		return err
	}
	r.expiresAt = time.Time{}
	return nil
}

// GetOpenAIPriorityStatus shares a short snapshot between admission and the public
// banner. Never treat a database failure as an empty account pool. Cache errors too
// so an outage cannot produce one database query per incoming request.
func (s *SettingService) GetOpenAIPriorityStatus(ctx context.Context) (OpenAIPriorityStatus, error) {
	if s == nil {
		return OpenAIPriorityStatus{}, nil
	}
	r := &s.openAIPriority
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Now().Before(r.expiresAt) {
		return r.status, r.err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	status, err := s.loadOpenAIPriorityStatus(ctx)
	r.status, r.err, r.expiresAt = status, err, time.Now().Add(openAIPriorityStateTTL)
	return status, err
}

func (s *SettingService) loadOpenAIPriorityStatus(ctx context.Context) (OpenAIPriorityStatus, error) {
	status := OpenAIPriorityStatus{}
	settings, err := s.GetOpenAIPrioritySettings(ctx)
	if err != nil || !settings.Enabled {
		return status, err
	}
	if s.openAIPriority.accounts == nil {
		return status, fmt.Errorf("OpenAI priority account reader unavailable")
	}
	accounts, err := s.openAIPriority.accounts.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return status, err
	}
	ctx = withOpenAIQuotaAutoPauseSettings(ctx, s.GetOpenAIQuotaAutoPauseSettings(ctx))
	thresholds := s.GetAccountSchedulingThresholds(ctx)
	now := time.Now()
	seen := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		a := &accounts[i]
		if a.Platform != PlatformOpenAI || a.IsShadow() || !a.IsSchedulable() {
			continue
		}
		if _, exists := seen[a.ID]; exists {
			continue
		}
		if EvaluateAccountSchedulingThreshold(a, thresholds, now).ShouldPause {
			continue
		}
		if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, a); paused {
			continue
		}
		seen[a.ID] = struct{}{}
		status.AvailableAccounts++
	}
	status.Active = status.AvailableAccounts <= settings.Threshold
	return status, nil
}
