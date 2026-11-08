//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const repositoryDiscoveryTestBody = `{"model":"gpt-5","instructions":"client rules","input":[{"role":"user","content":"fix my project"}],"tools":[{"type":"function","name":"exec_command","parameters":{"type":"object"}}],"prompt_cache_key":"keep-me","stream":true}`

func discoveryTestContext(apiKeyID int64, session string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.150.0")
	c.Request.Header.Set("session_id", session)
	c.Set("api_key", &APIKey{ID: apiKeyID, Key: fmt.Sprintf("sk-test-%d", apiKeyID)})
	return c
}

func discoveryTestService() *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.RepositoryAutoDetect = true
	return &OpenAIGatewayService{cfg: cfg}
}

func discoveryTestMarker(s *OpenAIGatewayService, id int64, session string) string {
	s.repositoryDiscovery.mu.Lock()
	defer s.repositoryDiscovery.mu.Unlock()
	return s.repositoryDiscovery.items[repositoryDiscoveryKey{id, session}].marker
}

func discoveryTestOutput(marker, repository string) []byte {
	body, _ := json.Marshal(map[string]any{
		"model": "gpt-5", "instructions": "client rules", "previous_response_id": "resp_unchanged",
		"input": []any{map[string]string{"type": "function_call_output", "call_id": "call_probe", "output": "Process exited with code 0\n" + marker + repository + "\n"}},
	})
	return body
}

func TestRepositoryDiscoveryOncePreservesRequestAndRestoresAcrossInstances(t *testing.T) {
	s := discoveryTestService()
	c := discoveryTestContext(7, "session-a")
	initial := []byte(repositoryDiscoveryTestBody)
	next := s.PrepareRepositoryDiscovery(c, initial)
	require.NotEqual(t, string(initial), string(next))
	require.Equal(t, "client rules", gjson.GetBytes(initial, "instructions").String())
	require.Contains(t, gjson.GetBytes(next, "instructions").String(), "client rules")
	for _, path := range []string{"model", "input", "tools", "prompt_cache_key", "stream"} {
		require.Equal(t, gjson.GetBytes(initial, path).Raw, gjson.GetBytes(next, path).Raw, path)
	}
	marker := discoveryTestMarker(s, 7, "session-a")
	require.Contains(t, gjson.GetBytes(next, "instructions").String(), marker)
	require.NotContains(t, string(next), "sk-test-7")
	require.Equal(t, string(initial), string(s.PrepareRepositoryDiscovery(c, initial)), "pending sessions must not get another probe")
	follow := discoveryTestOutput(marker, "https://user:secret@github.com/Owner/Repo.git?token=secret")
	require.Equal(t, string(follow), string(s.PrepareRepositoryDiscovery(c, follow)))
	require.Equal(t, "github.com/owner/repo", *s.discoveredUsageRepository(7, "session-a"))
	require.Nil(t, s.discoveredUsageRepository(8, "session-a"))
	require.Nil(t, s.discoveredUsageRepository(7, "session-b"))
	other := discoveryTestService()
	require.Equal(t, string(follow), string(other.PrepareRepositoryDiscovery(c, follow)), "a different instance can recover the same scoped tool marker")
	require.Equal(t, "github.com/owner/repo", *other.discoveredUsageRepository(7, "session-a"))
	// Known sessions must not inspect/copy huge histories.
	huge := []byte(strings.Repeat("x", repositoryDiscoveryMaxBody+1))
	require.Equal(t, &huge[0], &s.PrepareRepositoryDiscovery(c, huge)[0])
}

func TestRepositoryDiscoveryIgnoresProseAndOtherSessionMarkers(t *testing.T) {
	s := discoveryTestService()
	c := discoveryTestContext(7, "a")
	s.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))
	marker := discoveryTestMarker(s, 7, "a")
	fake, _ := json.Marshal(map[string]any{"input": []any{map[string]string{"role": "user", "content": marker + "github.com/fake/repo"}}})
	s.PrepareRepositoryDiscovery(c, fake)
	require.Nil(t, s.discoveredUsageRepository(7, "a"))
	otherContext := discoveryTestContext(8, "a")
	s.PrepareRepositoryDiscovery(otherContext, discoveryTestOutput(marker, "github.com/fake/repo"))
	require.Nil(t, s.discoveredUsageRepository(8, "a"))
	// Terminal unknown is cached and never re-injected, including if tools reappear.
	s.PrepareRepositoryDiscovery(c, discoveryTestOutput(marker, "unknown"))
	require.Equal(t, repositoryDiscoveryTestBody, string(s.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))))
	require.Nil(t, s.discoveredUsageRepository(7, "a"))
}

func TestRepositoryDiscoveryStopsScanningWhenModelDoesNotReport(t *testing.T) {
	s := discoveryTestService()
	c := discoveryTestContext(7, "a")
	s.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))
	for range repositoryDiscoveryMaxChecks + 1 {
		require.Equal(t, repositoryDiscoveryTestBody, string(s.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))))
	}
	s.repositoryDiscovery.mu.Lock()
	entry := s.repositoryDiscovery.items[repositoryDiscoveryKey{7, "a"}]
	require.True(t, entry.done)
	require.Equal(t, repositoryDiscoveryMaxChecks, entry.checks)
	s.repositoryDiscovery.mu.Unlock()
}

func TestRepositoryDiscoveryEligibilityAndWrappedFrames(t *testing.T) {
	for _, name := range []string{"disabled", "other-client", "no-session", "explicit", "oversized", "no-tools", "tools-disabled", "malformed-instructions", "compaction"} {
		t.Run(name, func(t *testing.T) {
			s := discoveryTestService()
			c := discoveryTestContext(7, "session-a")
			body := repositoryDiscoveryTestBody
			switch name {
			case "disabled":
				s.cfg.Gateway.RepositoryAutoDetect = false
			case "other-client":
				c.Request.Header.Set("User-Agent", "curl/8")
			case "no-session":
				c.Request.Header.Del("session_id")
			case "explicit":
				c.Request.Header.Set(UsageRepositoryHeader, "github.com/manual/repo")
			case "oversized":
				body = strings.Repeat("x", repositoryDiscoveryMaxBody+1)
			case "no-tools":
				body = `{"model":"gpt-5","input":"task"}`
			case "tools-disabled":
				body = strings.TrimSuffix(body, "}") + `,"tool_choice":"none"}`
			case "malformed-instructions":
				body = `{"model":"gpt-5","instructions":[],"tools":[{"type":"shell"}]}`
			case "compaction":
				body = `{"model":"gpt-5","stream":true,"input":[{"type":"compaction_trigger"}],"tools":[{"type":"shell"}]}`
			}
			require.Equal(t, body, string(s.PrepareRepositoryDiscovery(c, []byte(body))))
		})
	}
	s := discoveryTestService()
	c := discoveryTestContext(7, "ws-a")
	body := []byte(`{"type":"response.create","response":{"model":"gpt-5","instructions":"nested rules","input":"task","tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}]}}`)
	next := s.PrepareRepositoryDiscovery(c, body)
	require.Contains(t, gjson.GetBytes(next, "response.instructions").String(), "nested rules")
	require.Contains(t, gjson.GetBytes(next, "response.instructions").String(), discoveryTestMarker(s, 7, "ws-a"))
	require.False(t, gjson.GetBytes(next, "instructions").Exists())
}

func TestRepositoryDiscoveryConcurrentRequestsInjectOnlyOnce(t *testing.T) {
	s := discoveryTestService()
	var injected atomic.Int32
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if string(s.PrepareRepositoryDiscovery(discoveryTestContext(7, "same"), []byte(repositoryDiscoveryTestBody))) != repositoryDiscoveryTestBody {
				injected.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, injected.Load())
}

func TestRepositoryDiscoveryBoundedCacheAndPendingRows(t *testing.T) {
	s := discoveryTestService()
	c := discoveryTestContext(7, "a")
	s.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))
	session := "a"
	for i := range 100 {
		s.completeRepositoryDiscoveryUsage(&UsageLog{APIKeyID: 7, SessionID: &session, RequestID: fmt.Sprintf("request-%d", i)})
	}
	cache := &s.repositoryDiscovery
	cache.mu.Lock()
	require.Len(t, cache.items[repositoryDiscoveryKey{7, "a"}].pending, repositoryDiscoveryMaxRows)
	now := time.Now()
	for len(cache.items) < repositoryDiscoveryCapacity {
		cache.createLocked(repositoryDiscoveryKey{int64(len(cache.items) + 10), "b"}, "secret", now)
	}
	require.Nil(t, cache.createLocked(repositoryDiscoveryKey{999999, "full"}, "secret", now))
	entry := cache.items[repositoryDiscoveryKey{7, "a"}]
	entry.expiresAt = now.Add(-time.Second)
	require.Nil(t, cache.entryLocked(entry.key, now))
	require.NotNil(t, cache.createLocked(entry.key, "secret", now))
	require.Len(t, cache.items, repositoryDiscoveryCapacity)
	require.Equal(t, repositoryDiscoveryCapacity, cache.order.Len())
	cache.mu.Unlock()
}

type discoveryBackfillCall struct {
	key        int64
	ids        []string
	repository string
}
type discoveryBackfillRepo struct {
	UsageLogRepository
	calls chan discoveryBackfillCall
}

func (r *discoveryBackfillRepo) AttributeUsageRepository(ctx context.Context, key int64, ids []string, repository string) error {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 2*time.Second {
		return fmt.Errorf("missing bounded deadline")
	}
	r.calls <- discoveryBackfillCall{key, ids, repository}
	return nil
}

func TestRepositoryDiscoveryBackfillsBothSidesOfLogWriteRace(t *testing.T) {
	s := discoveryTestService()
	repo := &discoveryBackfillRepo{calls: make(chan discoveryBackfillCall, 2)}
	s.usageLogRepo = repo
	c := discoveryTestContext(7, "a")
	s.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))
	marker := discoveryTestMarker(s, 7, "a")
	session := "a"
	before := &UsageLog{APIKeyID: 7, SessionID: &session, RequestID: "before-discovery", ActualCost: 1.25}
	s.completeRepositoryDiscoveryUsage(before)
	s.PrepareRepositoryDiscovery(c, discoveryTestOutput(marker, "github.com/owner/repo"))
	select {
	case call := <-repo.calls:
		require.Equal(t, discoveryBackfillCall{7, []string{"before-discovery"}, "github.com/owner/repo"}, call)
	case <-time.After(time.Second):
		t.Fatal("startup backfill was not queued")
	}
	s.completeRepositoryDiscoveryUsage(&UsageLog{APIKeyID: 7, SessionID: &session, RequestID: "racing-insert"})
	select {
	case call := <-repo.calls:
		require.Equal(t, []string{"racing-insert"}, call.ids)
	case <-time.After(time.Second):
		t.Fatal("racing insert backfill was not queued")
	}
	require.Equal(t, 1.25, before.ActualCost)
}

func TestRepositoryDiscoveryCommandReadsOriginAndRemovesCredentials(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, remote := range []string{"https://user:secret@github.com/Owner/Repo.git?token=secret#branch", "ssh://user:secret@git.example.com/team/Repo.git", "git@github.com:Owner/Repo.git", ""} {
		t.Run(remote, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "project with spaces")
			out, err := exec.Command("git", "init", dir).CombinedOutput()
			require.NoError(t, err, string(out))
			if remote != "" {
				require.NoError(t, exec.Command("git", "-C", dir, "remote", "add", "origin", remote).Run())
			}
			prompt := repositoryDiscoveryPrompt("TEST_MARKER=")
			start := strings.Index(prompt, "set +x;")
			command, _, _ := strings.Cut(prompt[start:], "\n")
			cmd := exec.Command("sh", "-c", command)
			cmd.Dir = dir
			out, err = cmd.CombinedOutput()
			require.NoError(t, err, string(out))
			require.NotContains(t, string(out), "secret")
			repository, found := repositoryDiscoveryMarker(string(out), "TEST_MARKER=")
			require.True(t, found)
			require.Equal(t, NormalizeUsageRepository(remote), repository)
		})
	}
}

func TestRepositoryDiscoveryReadsStructuredAndOrchestratedOutputs(t *testing.T) {
	for _, output := range []any{
		"TEST=github.com/a/b\n",
		[]any{map[string]string{"type": "text", "text": "TEST=github.com/a/b\n"}},
		[]any{map[string]string{"stdout": "TEST=github.com/a/b\n"}},
		"Script completed\nOutput:\n{\"output\":\"TEST=github.com/a/b\\n\"}",
	} {
		body, _ := json.Marshal([]any{map[string]any{"type": "function_call_output", "output": output}})
		repository, found := repositoryDiscoveryToolResult(gjson.ParseBytes(body), "TEST=")
		require.True(t, found, string(body))
		require.Equal(t, "github.com/a/b", repository)
	}
}

func TestRepositoryDiscoveryAsyncUsageKeepsBillingAndExplicitAttribution(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, nil, nil)
	in := &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "baseline", Model: "gpt-5.1", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 50}, Duration: time.Second},
		APIKey: &APIKey{ID: 7, Group: &Group{RateMultiplier: 1}}, User: &User{ID: 42}, Account: &Account{ID: 10, Type: AccountTypeAPIKey}, SessionID: "a",
	}
	require.NoError(t, svc.RecordUsage(context.Background(), in))
	expectedCost, expectedBalance := usageRepo.lastLog.ActualCost, billingRepo.lastCmd.BalanceCost
	require.Greater(t, expectedCost, 0.0)
	svc.cfg.Gateway.RepositoryAutoDetect = true
	c := discoveryTestContext(7, "a")
	svc.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))
	svc.PrepareRepositoryDiscovery(c, discoveryTestOutput(discoveryTestMarker(svc, 7, "a"), "github.com/owner/repo"))
	in.Result.RequestID = "auto"
	// Queued billing uses a detached context, with no gin context or body.
	require.NoError(t, svc.RecordUsage(context.Background(), in))
	require.Equal(t, "github.com/owner/repo", *usageRepo.lastLog.Repository)
	require.Equal(t, expectedCost, usageRepo.lastLog.ActualCost)
	require.Equal(t, expectedBalance, billingRepo.lastCmd.BalanceCost)
	in.Result.RequestID = "manual"
	require.NoError(t, svc.RecordUsage(WithUsageRepository(context.Background(), "github.com/manual/repo"), in))
	require.Equal(t, "github.com/manual/repo", *usageRepo.lastLog.Repository)
	require.Equal(t, expectedCost, usageRepo.lastLog.ActualCost)
	require.Equal(t, expectedBalance, billingRepo.lastCmd.BalanceCost)
}

func BenchmarkRepositoryDiscoveryKnownSession(b *testing.B) {
	s := discoveryTestService()
	c := discoveryTestContext(7, "a")
	s.PrepareRepositoryDiscovery(c, []byte(repositoryDiscoveryTestBody))
	s.PrepareRepositoryDiscovery(c, discoveryTestOutput(discoveryTestMarker(s, 7, "a"), "github.com/owner/repo"))
	for name, size := range map[string]int{"small": 1000, "large": 1 << 20} {
		b.Run(name, func(b *testing.B) {
			body := []byte(strings.Repeat("x", size))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = s.PrepareRepositoryDiscovery(c, body)
			}
		})
	}
}
