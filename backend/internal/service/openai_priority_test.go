//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type priorityAccountStub struct {
	accounts []Account
	err      error
	calls    int
}

func (r *priorityAccountStub) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	if platform != PlatformOpenAI {
		panic("priority must only query OpenAI")
	}
	r.calls++
	return r.accounts, r.err
}
func priorityAccount(id int64) Account {
	return Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
}
func TestOpenAIPriorityDefaultsAndTransitions(t *testing.T) {
	ctx := context.Background()
	s := newSettingServiceForPlatformThresholdTest(nil)
	repo := &priorityAccountStub{accounts: []Account{priorityAccount(1), priorityAccount(2), priorityAccount(3)}}
	s.openAIPriority.accounts = repo
	settings, err := s.GetOpenAIPrioritySettings(ctx)
	require.NoError(t, err)
	require.Equal(t, OpenAIPrioritySettings{Threshold: 2}, settings)
	status, err := s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.Active)
	require.Zero(t, repo.calls, "disabled mode must not scan accounts")
	require.NoError(t, s.SetOpenAIPrioritySettings(ctx, OpenAIPrioritySettings{Enabled: true, Threshold: 2}))
	status, err = s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.Active)
	require.Equal(t, 3, status.AvailableAccounts)
	repo.accounts = repo.accounts[:2]
	s.openAIPriority.expiresAt = time.Time{}
	status, err = s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.True(t, status.Active, "equal threshold activates priority")
	repo.accounts = nil
	s.openAIPriority.expiresAt = time.Time{}
	status, err = s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.True(t, status.Active, "zero accounts also activates priority")
	repo.accounts = []Account{priorityAccount(1), priorityAccount(2), priorityAccount(3)}
	s.openAIPriority.expiresAt = time.Time{}
	status, err = s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.Active, "recovery automatically lifts priority")
	require.NoError(t, s.SetOpenAIPrioritySettings(ctx, OpenAIPrioritySettings{Enabled: true, Threshold: 3}))
	status, err = s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.True(t, status.Active, "config writes invalidate cached status immediately")
	require.NoError(t, s.SetOpenAIPrioritySettings(ctx, OpenAIPrioritySettings{Threshold: 2}))
	status, err = s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.Active, "disabling immediately removes restriction")
	require.Error(t, s.SetOpenAIPrioritySettings(ctx, OpenAIPrioritySettings{Threshold: -1}))
}

func TestOpenAIPriorityExcludesUnavailableAccounts(t *testing.T) {
	ctx := context.Background()
	s := newSettingServiceForPlatformThresholdTest(nil)
	s.SetOpenAIQuotaAutoPauseSettings(OpsOpenAIAccountQuotaAutoPauseSettings{})
	future := time.Now().Add(time.Hour)
	parent := int64(1)
	cases := []struct {
		name   string
		modify func(*Account)
	}{
		{"paused", func(a *Account) { a.Schedulable = false }},
		{"disabled", func(a *Account) { a.Status = StatusDisabled }},
		{"error", func(a *Account) { a.Status = StatusError }},
		{"rate limited", func(a *Account) { a.RateLimitResetAt = &future }},
		{"overloaded", func(a *Account) { a.OverloadUntil = &future }},
		{"temporary pause", func(a *Account) { a.TempUnschedulableUntil = &future }},
		{"expired", func(a *Account) { past := time.Now().Add(-time.Hour); a.ExpiresAt = &past; a.AutoPauseOnExpired = true }},
		{"quota exhausted", func(a *Account) { a.Extra = map[string]any{"quota_limit": 10.0, "quota_used": 10.0} }},
		{"another platform", func(a *Account) { a.Platform = PlatformGrok }},
		{"virtual shadow", func(a *Account) { a.ParentAccountID = &parent }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := priorityAccount(2)
			tc.modify(&account)
			s.openAIPriority.accounts = &priorityAccountStub{accounts: []Account{priorityAccount(1), account, priorityAccount(1)}}
			require.NoError(t, s.SetOpenAIPrioritySettings(ctx, OpenAIPrioritySettings{Enabled: true, Threshold: 1}))
			status, err := s.GetOpenAIPriorityStatus(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, status.AvailableAccounts, "must exclude ineligible accounts and deduplicate IDs")
			require.True(t, status.Active)
		})
	}
}

func TestOpenAIPriorityCachesErrorsWithoutReportingEmptyPool(t *testing.T) {
	ctx := context.Background()
	s := newSettingServiceForPlatformThresholdTest(nil)
	repo := &priorityAccountStub{err: errors.New("database offline")}
	s.openAIPriority.accounts = repo
	require.NoError(t, s.SetOpenAIPrioritySettings(ctx, OpenAIPrioritySettings{Enabled: true, Threshold: 2}))
	for i := 0; i < 2; i++ {
		status, err := s.GetOpenAIPriorityStatus(ctx)
		require.Error(t, err)
		require.False(t, status.Active)
	}
	require.Equal(t, 1, repo.calls)
}

func TestAPIKeyAuthSnapshotOpenAIPriority(t *testing.T) {
	for _, priority := range []bool{false, true} {
		s := &APIKeyService{}
		key := &APIKey{ID: 1, UserID: 2, User: &User{ID: 2, OpenAIPriority: priority}}
		raw, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: s.snapshotFromAPIKey(context.Background(), key)})
		require.NoError(t, err)
		var cached APIKeyAuthCacheEntry
		require.NoError(t, json.Unmarshal(raw, &cached))
		result, used, err := s.applyAuthCacheEntry("sk-test", &cached)
		require.NoError(t, err)
		require.True(t, used)
		require.Equal(t, priority, result.User.OpenAIPriority)
	}
}

func TestOpenAIPriorityRespectsQuotaAutoPause(t *testing.T) {
	ctx := context.Background()
	s := newSettingServiceForPlatformThresholdTest(nil)
	s.SetOpenAIQuotaAutoPauseSettings(OpsOpenAIAccountQuotaAutoPauseSettings{})
	account := priorityAccount(1)
	account.Extra = map[string]any{"codex_5h_used_percent": 95.0, "auto_pause_5h_threshold": 0.95}
	s.openAIPriority.accounts = &priorityAccountStub{accounts: []Account{account}}
	require.NoError(t, s.SetOpenAIPrioritySettings(ctx, OpenAIPrioritySettings{Enabled: true, Threshold: 0}))
	status, err := s.GetOpenAIPriorityStatus(ctx)
	require.NoError(t, err)
	require.Zero(t, status.AvailableAccounts)
	require.True(t, status.Active)
}

type priorityUserUpdateStub struct {
	*userRepoStub
	fields UserUpdateFields
}

func (s *priorityUserUpdateStub) Update(_ context.Context, user *User, fields UserUpdateFields) error {
	s.fields = fields
	clone := *user
	s.userRepoStub.user = &clone
	return nil
}
func TestAdminPriorityUpdateInvalidatesExistingKeys(t *testing.T) {
	repo := &priorityUserUpdateStub{userRepoStub: &userRepoStub{user: &User{ID: 42, Email: "u@example.test"}}}
	invalidator := &authCacheInvalidatorStub{}
	s := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{}, authCacheInvalidator: invalidator}
	for _, value := range []bool{true, false} {
		invalidator.userIDs = nil
		updated, err := s.UpdateUser(context.Background(), 42, &UpdateUserInput{OpenAIPriority: &value})
		require.NoError(t, err)
		require.Equal(t, value, updated.OpenAIPriority)
		require.True(t, repo.fields.OpenAIPriority)
		require.Equal(t, []int64{42}, invalidator.userIDs)
	}
	invalidator.userIDs = nil
	_, err := s.UpdateUser(context.Background(), 42, &UpdateUserInput{})
	require.NoError(t, err)
	require.False(t, repo.fields.OpenAIPriority, "omitted field must preserve stored permission")
	require.Empty(t, invalidator.userIDs)
}
