package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const claudeWindowRetryDelay = 10 * time.Minute

func (r *ClaudeUsageResponse) UnmarshalJSON(data []byte) error {
	type plain ClaudeUsageResponse
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var windows map[string]json.RawMessage
	if err := json.Unmarshal(data, &windows); err != nil {
		return err
	}
	available := func(name string) bool {
		var fields map[string]json.RawMessage
		if json.Unmarshal(windows[name], &fields) != nil {
			return false
		}
		util, ok := fields["utilization"]
		return ok && string(bytes.TrimSpace(util)) != "null"
	}
	*r = ClaudeUsageResponse(decoded)
	r.fiveHourAvailable = available("five_hour")
	r.weeklyAvailable = available("seven_day")
	return nil
}

func supportsClaudeWindowStartup(account *Account) bool {
	return account != nil && account.Platform == PlatformAnthropic &&
		account.Type == AccountTypeOAuth && !account.IsSyntheticUITest()
}

// claudeWindowWait requires a known weekly balance. Expired weekly observations
// are refreshed later, rather than treating an old 100% observation as available.
func claudeWindowWait(usage *UsageInfo, model string, now time.Time) (time.Time, string) {
	retry := now.Add(claudeWindowRetryDelay)
	if usage == nil || usage.Error != "" || usage.SevenDay == nil ||
		usage.SevenDay.ResetsAt == nil || !usage.SevenDay.ResetsAt.After(now) {
		return retry, "Weekly usage is unavailable; no message sent"
	}
	weekly := []*UsageProgress{usage.SevenDay}
	if strings.Contains(strings.ToLower(model), "sonnet") && usage.SevenDaySonnet != nil {
		weekly = append(weekly, usage.SevenDaySonnet)
	}
	if strings.Contains(strings.ToLower(model), "fable") && usage.SevenDayFable != nil {
		weekly = append(weekly, usage.SevenDayFable)
	}
	var exhaustedUntil time.Time
	for _, window := range weekly {
		if math.IsNaN(window.Utilization) || math.IsInf(window.Utilization, 0) || window.Utilization < 0 {
			return retry, "Weekly usage is invalid; no message sent"
		}
		if window.Utilization >= 100 {
			if window.ResetsAt == nil || !window.ResetsAt.After(now) {
				return retry, "Weekly usage needs refreshing; no message sent"
			}
			if window.ResetsAt.After(exhaustedUntil) {
				exhaustedUntil = *window.ResetsAt
			}
		}
	}
	if !exhaustedUntil.IsZero() {
		return exhaustedUntil.Add(15 * time.Second), "Weekly quota exhausted; no message sent"
	}
	if usage.FiveHour == nil {
		return retry, "Five-hour usage is unavailable; no message sent"
	}
	if usage.FiveHour.ResetsAt != nil && usage.FiveHour.ResetsAt.After(now) {
		return usage.FiveHour.ResetsAt.Add(15 * time.Second), "Five-hour window is still active; no message sent"
	}
	// A nonzero usage with no reset timestamp is ambiguous, not an idle window.
	if usage.FiveHour.ResetsAt == nil && usage.FiveHour.Utilization != 0 {
		return retry, "Five-hour reset time is unavailable; no message sent"
	}
	return time.Time{}, ""
}

func (s *ScheduledTestRunnerService) runClaudeWindowPlan(ctx context.Context, snapshot *ScheduledTestPlan) {
	claimer, ok := s.planRepo.(ScheduledTestPlanClaimer)
	if !ok {
		return // Fail closed: never send without a distributed claim.
	}
	now := time.Now()
	plan, err := claimer.ClaimDue(ctx, snapshot.ID, now, now.Add(claudeWindowRetryDelay))
	if err != nil || plan == nil {
		return
	}
	// The lease exceeds this entire operation, including a slow upstream.
	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	result, next := s.executeClaudeWindowPlan(runCtx, plan, now)
	cancel()
	if err := s.scheduledSvc.SaveResult(ctx, plan.ID, plan.MaxResults, result); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "Claude window plan=%d save result failed: %v", plan.ID, err)
	}
	if err := s.planRepo.UpdateAfterRun(ctx, plan.ID, now, next); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "Claude window plan=%d save deadline failed: %v", plan.ID, err)
	}
}

func (s *ScheduledTestRunnerService) executeClaudeWindowPlan(ctx context.Context, plan *ScheduledTestPlan, now time.Time) (result *ScheduledTestResult, next time.Time) {
	result = &ScheduledTestResult{Status: "skipped", StartedAt: now}
	next = now.Add(claudeWindowRetryDelay)
	defer func() {
		result.FinishedAt = time.Now()
		result.LatencyMs = result.FinishedAt.Sub(now).Milliseconds()
	}()
	account, err := s.accountTestSvc.accountRepo.GetByID(ctx, plan.AccountID)
	if err != nil || !supportsClaudeWindowStartup(account) || !account.IsSchedulable() {
		result.ResponseText = "Account is unavailable or paused; no message sent"
		return
	}
	if account.SessionWindowEnd != nil && account.SessionWindowEnd.After(now) {
		next = account.SessionWindowEnd.Add(15 * time.Second)
		result.ResponseText = "Five-hour window is still active; no message sent"
		return
	}
	if s.accountUsageSvc == nil || s.accountTestSvc.claudeTokenProvider == nil {
		result.ErrorMessage = "Claude window services are unavailable"
		result.Status = "failed"
		return
	}
	// Refresh credentials before querying usage and sending. The refreshed token
	// can come from cache, so copy it onto this local account snapshot as well.
	token, err := s.accountTestSvc.claudeTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		result.Status, result.ErrorMessage = "failed", "Could not obtain Claude access token"
		return
	}
	credentials := make(map[string]any, len(account.Credentials)+1)
	for key, value := range account.Credentials {
		credentials[key] = value
	}
	credentials["access_token"] = token
	account.Credentials = credentials
	// Read the provider directly: cached usage may still describe the old window.
	raw, err := s.accountUsageSvc.fetchOAuthUsageRaw(ctx, account)
	if err != nil || raw == nil {
		result.Status, result.ErrorMessage = "failed", "Could not confirm weekly quota; no message sent"
		return
	}
	usage := s.accountUsageSvc.buildUsageInfo(raw, &now)
	if !raw.fiveHourAvailable || (raw.FiveHour.ResetsAt != "" && usage.FiveHour.ResetsAt == nil) {
		usage.FiveHour = nil
	}
	if !raw.weeklyAvailable {
		usage.SevenDay = nil
	}
	if wait, reason := claudeWindowWait(usage, plan.ModelID, now); reason != "" {
		next, result.ResponseText = wait, reason
		return
	}
	headers, err := s.accountTestSvc.startClaudeWindow(ctx, account, plan.ModelID)
	if s.rateLimitSvc != nil && headers != nil {
		s.rateLimitSvc.UpdateSessionWindow(ctx, account, headers)
	}
	if err != nil {
		result.Status, result.ErrorMessage = "failed", err.Error()
		// Honor provider backoff even if the quota changed after our usage query.
		if headers != nil {
			if reset := parseRetryAfterResetTime(headers, now); reset != nil && reset.After(next) && reset.Before(now.Add(8*24*time.Hour)) {
				next = reset.Add(15 * time.Second)
			}
			if reset, ok := parseAnthropicResetTimestamp(headers.Get("anthropic-ratelimit-unified-reset"), now, 8*24*time.Hour); ok && reset.After(next) {
				next = reset.Add(15 * time.Second)
			}
		}
		return
	}
	next = time.Now().Add(5*time.Hour + 15*time.Second)
	if reset, ok := parseAnthropicResetTimestamp(headers.Get("anthropic-ratelimit-unified-5h-reset"), now, 5*time.Hour+time.Minute); ok && reset.After(now) {
		next = reset.Add(15 * time.Second)
	}
	result.Status, result.ResponseText = "success", "Sent one short message (max_tokens=1)"
	return
}

// startClaudeWindow reuses the Claude Code identity, account proxy and TLS
// transport. There is one POST, no tools/history/thinking, and at most one token.
func (s *AccountTestService) startClaudeWindow(ctx context.Context, account *Account, model string) (http.Header, error) {
	payload, err := createTestPayload(model)
	if err != nil {
		return nil, fmt.Errorf("could not create Claude window request")
	}
	payload["messages"] = []map[string]any{{"role": "user", "content": "."}}
	payload["max_tokens"] = 1
	payload["stream"] = false
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("could not encode Claude window request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, testClaudeAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range claude.DefaultHeaders() {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)
	req.Header.Set("Authorization", "Bearer "+account.GetCredential("access_token"))
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		// Transport errors can contain proxy URLs with credentials.
		return nil, fmt.Errorf("Claude window request failed; retry deferred")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.Header, fmt.Errorf("Claude window request returned HTTP %d", resp.StatusCode)
	}
	var message struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&message); err != nil || message.Type != "message" || message.ID == "" {
		return resp.Header, fmt.Errorf("Claude window request returned an invalid message")
	}
	return resp.Header, nil
}
