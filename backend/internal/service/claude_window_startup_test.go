//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type windowUsageFetcher struct {
	ClaudeUsageFetcher
	raw   *ClaudeUsageResponse
	err   error
	calls int
}

func (f *windowUsageFetcher) FetchUsageWithOptions(_ context.Context, opts *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error) {
	f.calls++
	if opts.AccessToken != "test-token" {
		return nil, errors.New("wrong token")
	}
	return f.raw, f.err
}

type windowPlanRepo struct {
	ScheduledTestPlanRepository
	mu   sync.Mutex
	plan *ScheduledTestPlan
}

func (r *windowPlanRepo) ClaimDue(_ context.Context, _ int64, now, lease time.Time) (*ScheduledTestPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.plan.Enabled || r.plan.NextRunAt.After(now) {
		return nil, nil
	}
	r.plan.NextRunAt = &lease
	p := *r.plan
	return &p, nil
}

func (r *windowPlanRepo) UpdateAfterRun(_ context.Context, _ int64, last, next time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plan.LastRunAt, r.plan.NextRunAt = &last, &next
	return nil
}

func (r *windowPlanRepo) Create(_ context.Context, p *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	r.plan = p
	return p, nil
}

func (r *windowPlanRepo) ListByAccountID(context.Context, int64) ([]*ScheduledTestPlan, error) {
	if r.plan == nil {
		return nil, nil
	}
	return []*ScheduledTestPlan{r.plan}, nil
}

func (r *windowPlanRepo) Update(_ context.Context, p *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	r.plan = p
	return p, nil
}

type windowResultRepo struct {
	ScheduledTestResultRepository
	results []*ScheduledTestResult
}

func (r *windowResultRepo) Create(_ context.Context, result *ScheduledTestResult) (*ScheduledTestResult, error) {
	r.results = append(r.results, result)
	return result, nil
}

func (r *windowResultRepo) PruneOldResults(context.Context, int64, int) error { return nil }

// Redirect only the test transport; production keeps its fixed Anthropic URL.
type windowHTTPTransport struct {
	HTTPUpstream
	baseURL string
}

func (u *windowHTTPTransport) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	redirect := req.Clone(req.Context())
	target, err := url.Parse(u.baseURL + "/v1/messages?beta=true")
	if err != nil {
		return nil, err
	}
	redirect.URL = target
	return http.DefaultClient.Do(redirect)
}

func windowTestRunner(now time.Time, upstream HTTPUpstream) (*ScheduledTestRunnerService, *Account, *windowUsageFetcher) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "test-token", "expires_at": now.Add(time.Hour).Format(time.RFC3339)}}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{1: account}}
	fetcher := &windowUsageFetcher{raw: &ClaudeUsageResponse{fiveHourAvailable: true, weeklyAvailable: true}}
	fetcher.raw.SevenDay.ResetsAt = now.Add(24 * time.Hour).Format(time.RFC3339)
	fetcher.raw.SevenDay.Utilization = 25
	tests := &AccountTestService{accountRepo: repo, httpUpstream: upstream, claudeTokenProvider: NewClaudeTokenProvider(repo, nil, nil)}
	runner := &ScheduledTestRunnerService{accountTestSvc: tests, accountUsageSvc: &AccountUsageService{usageFetcher: fetcher}}
	return runner, account, fetcher
}

func TestClaudeWindowStartupQuotaAndBackoff(t *testing.T) {
	for _, name := range []string{"idle", "expired", "active", "recorded active", "five-hour missing", "invalid reset", "weekly exhausted", "sonnet exhausted", "weekly missing", "weekly stale", "usage failure", "paused", "setup token", "rate limited", "invalid response"} {
		t.Run(name, func(t *testing.T) {
			now := time.Now().Truncate(time.Second)
			reset := now.Add(4 * time.Hour)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls++
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
				var payload map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
				require.Equal(t, float64(1), payload["max_tokens"])
				require.Equal(t, false, payload["stream"])
				require.Equal(t, []any{map[string]any{"role": "user", "content": "."}}, payload["messages"])
				require.NotContains(t, payload, "tools")
				require.NotContains(t, payload, "thinking")
				w.Header().Set("Content-Type", "application/json")
				if name == "rate limited" {
					w.Header().Set("Retry-After", "7200")
					w.WriteHeader(http.StatusTooManyRequests)
					fmt.Fprint(w, `{"type":"error"}`)
					return
				}
				if name == "invalid response" {
					fmt.Fprint(w, `{}`)
					return
				}
				w.Header().Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(reset.Unix(), 10))
				fmt.Fprint(w, `{"type":"message","id":"msg_test","content":[{"type":"text","text":"."}]}`)
			}))
			defer server.Close()
			runner, account, usage := windowTestRunner(now, &windowHTTPTransport{baseURL: server.URL})
			wantStatus, wantCalls := "skipped", 0
			wantNext := now.Add(claudeWindowRetryDelay)
			switch name {
			case "idle", "expired":
				wantStatus, wantCalls, wantNext = "success", 1, reset.Add(15*time.Second)
				if name == "expired" {
					usage.raw.FiveHour.ResetsAt = now.Add(-time.Minute).Format(time.RFC3339)
					usage.raw.FiveHour.Utilization = 100
				}
			case "active":
				usage.raw.FiveHour.ResetsAt = reset.Format(time.RFC3339)
				wantNext = reset.Add(15 * time.Second)
			case "recorded active":
				account.SessionWindowEnd = &reset
				wantNext = reset.Add(15 * time.Second)
			case "five-hour missing":
				usage.raw.fiveHourAvailable = false
			case "invalid reset":
				usage.raw.FiveHour.ResetsAt = "invalid"
			case "weekly exhausted":
				usage.raw.SevenDay.Utilization = 100
				wantNext = now.Add(24*time.Hour + 15*time.Second)
			case "sonnet exhausted":
				usage.raw.SevenDaySonnet.Utilization = 100
				usage.raw.SevenDaySonnet.ResetsAt = reset.Format(time.RFC3339)
				wantNext = reset.Add(15 * time.Second)
			case "weekly missing":
				usage.raw.SevenDay.ResetsAt = ""
			case "weekly stale":
				usage.raw.SevenDay.ResetsAt = now.Add(-time.Minute).Format(time.RFC3339)
			case "usage failure":
				usage.err = errors.New("offline")
				wantStatus = "failed"
			case "paused":
				account.Schedulable = false
			case "setup token":
				account.Type = AccountTypeSetupToken
			case "rate limited":
				wantStatus, wantCalls, wantNext = "failed", 1, now.Add(2*time.Hour+15*time.Second)
			case "invalid response":
				wantStatus, wantCalls = "failed", 1
			}
			result, next := runner.executeClaudeWindowPlan(context.Background(), &ScheduledTestPlan{AccountID: 1, ModelID: "claude-sonnet-4-6"}, now)
			require.Equal(t, wantStatus, result.Status, result.ErrorMessage)
			require.Equal(t, wantCalls, calls)
			require.True(t, wantNext.Equal(next), "expected %v, got %v", wantNext, next)
		})
	}
}

func TestClaudeWindowStartupClaimAndPersistedDeadline(t *testing.T) {
	now := time.Now()
	upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(200, `{"type":"message","id":"msg_test"}`)}}
	runner, _, _ := windowTestRunner(now, upstream)
	plan := &ScheduledTestPlan{ID: 1, AccountID: 1, Mode: ScheduledTestModeClaudeWindow, ModelID: "claude-sonnet-4-6", Enabled: true, MaxResults: 50, NextRunAt: &now}
	planRepo, resultRepo := &windowPlanRepo{plan: plan}, &windowResultRepo{}
	runner.planRepo = planRepo
	runner.scheduledSvc = NewScheduledTestService(planRepo, resultRepo, runner.accountTestSvc.accountRepo)
	snapshot := *plan
	runner.runOnePlan(context.Background(), &snapshot)
	runner.runOnePlan(context.Background(), &snapshot) // stale ListDue snapshot / another instance
	require.Len(t, upstream.requests, 1)
	require.Len(t, resultRepo.results, 1)
	require.True(t, planRepo.plan.NextRunAt.After(now.Add(5*time.Hour)))
	// Changing settings must not reopen an already started window.
	deadline := *planRepo.plan.NextRunAt
	_, err := runner.scheduledSvc.UpdatePlan(context.Background(), planRepo.plan)
	require.NoError(t, err)
	require.Equal(t, deadline, *planRepo.plan.NextRunAt)
	planRepo.plan.Enabled = false
	runner.runOnePlan(context.Background(), &snapshot)
	require.Len(t, upstream.requests, 1)
}

func TestClaudeWindowStartupModeValidation(t *testing.T) {
	runner, account, _ := windowTestRunner(time.Now(), nil)
	repo := &windowPlanRepo{}
	svc := NewScheduledTestService(repo, nil, runner.accountTestSvc.accountRepo)
	plan := &ScheduledTestPlan{AccountID: 1, Mode: ScheduledTestModeClaudeWindow, ModelID: "claude-sonnet-4-6", AutoRecover: true}
	created, err := svc.CreatePlan(context.Background(), plan)
	require.NoError(t, err)
	require.Equal(t, "* * * * *", created.CronExpression)
	require.False(t, created.AutoRecover)
	_, err = svc.CreatePlan(context.Background(), plan)
	require.ErrorContains(t, err, "already has")
	account.Type = AccountTypeSetupToken
	_, err = svc.CreatePlan(context.Background(), plan)
	require.ErrorContains(t, err, "OAuth")
	_, err = svc.CreatePlan(context.Background(), &ScheduledTestPlan{Mode: "unknown"})
	require.ErrorContains(t, err, "invalid scheduled test mode")
	_, err = svc.CreatePlan(context.Background(), &ScheduledTestPlan{CronExpression: "not cron"})
	require.ErrorContains(t, err, "invalid cron")
}

func TestClaudeWindowStartupUsagePresence(t *testing.T) {
	for _, tc := range []struct {
		body             string
		fiveHour, weekly bool
	}{
		{`{"five_hour":{"utilization":0},"seven_day":{"utilization":25}}`, true, true},
		{`{"five_hour":null,"seven_day":{"utilization":25}}`, false, true},
		{`{"five_hour":{"utilization":0},"seven_day":null}`, true, false},
		{`{"five_hour":{},"seven_day":{"utilization":null}}`, false, false},
		{`{}`, false, false},
	} {
		var raw ClaudeUsageResponse
		require.NoError(t, json.Unmarshal([]byte(tc.body), &raw))
		require.Equal(t, tc.fiveHour, raw.fiveHourAvailable)
		require.Equal(t, tc.weekly, raw.weeklyAvailable)
	}
}

// Optional real HTTP check against the committed Postman stand-in. No real
// credentials or provider calls; see docs/CLAUDE_WINDOW_STARTUP_CN.md.
func TestClaudeWindowStartupSimulation(t *testing.T) {
	base := os.Getenv("CLAUDE_WINDOW_SIM_URL")
	if base == "" {
		t.Skip("set CLAUDE_WINDOW_SIM_URL to run the local Postman simulation")
	}
	svc := &AccountTestService{httpUpstream: &windowHTTPTransport{baseURL: base}}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "simulation-token"}}
	for _, tc := range []struct {
		model   string
		failure bool
	}{{"claude-sonnet-4-6", false}, {"test-rate-limit", true}, {"test-malformed", true}} {
		t.Run(tc.model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			headers, err := svc.startClaudeWindow(ctx, account, tc.model)
			if tc.failure {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, headers.Get("anthropic-ratelimit-unified-5h-reset"))
			}
		})
	}
}
