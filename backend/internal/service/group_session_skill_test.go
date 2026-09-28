//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGroupSessionSkillProtocolPreservation(t *testing.T) {
	const skill = "检查 Git；非 EaseCation 仓库先征求确认。"
	for _, tc := range []struct {
		name, protocol, body, instructionPath, preservedPath string
	}{
		{"responses", ContentModerationProtocolOpenAIResponses, `{"instructions":"client rules","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}],"previous_response_id":"resp_1","model":"gpt-test"}`, "instructions", "input"},
		{"responses string input", ContentModerationProtocolOpenAIResponses, `{"input":"original task"}`, "instructions", "input"},
		{"websocket envelope", ContentModerationProtocolOpenAIResponses, `{"type":"response.create","response":{"instructions":"client rules","input":"task","previous_response_id":"resp_1"}}`, "response.instructions", "response.input"},
		{"anthropic string", ContentModerationProtocolAnthropicMessages, `{"system":"client rules","messages":[{"role":"user","content":"task"}]}`, "system", "messages"},
		{"anthropic blocks", ContentModerationProtocolAnthropicMessages, `{"system":[{"type":"text","text":"client rules","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`, "system.1.text", "messages"},
		{"gemini", ContentModerationProtocolGemini, `{"systemInstruction":{"role":"user","parts":[{"text":"client rules"}]},"contents":[{"role":"user","parts":[{"text":"task"}]}]}`, "systemInstruction.parts.1.text", "contents"},
		{"gemini snake case", ContentModerationProtocolGemini, `{"system_instruction":{"parts":[{"text":"client rules"}]},"contents":[]}`, "system_instruction.parts.1.text", "contents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := InjectGroupSessionSkill([]byte(tc.body), tc.protocol, skill)
			require.NoError(t, err)
			prompt := gjson.GetBytes(out, tc.instructionPath).String()
			require.Contains(t, prompt, skill)
			require.Contains(t, prompt, "不是每一轮新消息")
			require.Contains(t, prompt, "等待明确确认后再执行")
			require.Equal(t, gjson.Get(tc.body, tc.preservedPath).Raw, gjson.GetBytes(out, tc.preservedPath).Raw)
			if strings.Contains(tc.body, "client rules") {
				require.Contains(t, string(out), "client rules")
			}
			if strings.Contains(tc.body, "cache_control") {
				require.Equal(t, "ephemeral", gjson.GetBytes(out, "system.0.cache_control.type").String())
			}
			again, err := InjectGroupSessionSkill(out, tc.protocol, skill)
			require.NoError(t, err)
			require.Equal(t, string(out), string(again), "retries must not duplicate instructions")
		})
	}
}

func TestGroupSessionSkillChatPreservesHistoryAndToolCalls(t *testing.T) {
	body := []byte(`{"messages":[{"role":"developer","content":"existing"},{"role":"user","content":"task"},{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"git","arguments":"{}"}}]},{"role":"tool","tool_call_id":"c1","content":"ok"}],"tools":[{"type":"function","function":{"name":"git"}}]}`)
	out, err := InjectGroupSessionSkill(body, ContentModerationProtocolOpenAIChat, "confirm purpose")
	require.NoError(t, err)
	original, updated := gjson.GetBytes(body, "messages").Array(), gjson.GetBytes(out, "messages").Array()
	require.Len(t, updated, len(original)+1)
	for i := range original {
		require.JSONEq(t, original[i].Raw, updated[i+1].Raw)
	}
	require.Equal(t, gjson.GetBytes(body, "tools").Raw, gjson.GetBytes(out, "tools").Raw)
	again, err := InjectGroupSessionSkill(out, ContentModerationProtocolOpenAIChat, "confirm purpose")
	require.NoError(t, err)
	require.Equal(t, out, again)
}

func TestGroupSessionSkillDisabledAndMalformed(t *testing.T) {
	body := []byte("not json")
	out, err := InjectGroupSessionSkill(body, ContentModerationProtocolOpenAIResponses, "")
	require.NoError(t, err)
	require.Equal(t, body, out)
	for _, body := range []string{`invalid`, `[]`, `{"instructions":42}`} {
		_, err := InjectGroupSessionSkill([]byte(body), ContentModerationProtocolOpenAIResponses, "skill")
		require.Error(t, err)
	}
	require.Error(t, normalizeGroupSessionSkill(&Group{SessionSkillEnabled: true, SessionSkill: "  "}))
	require.Error(t, normalizeGroupSessionSkill(&Group{SessionSkill: strings.Repeat("字", MaxGroupSessionSkillLength+1)}))
	require.NoError(t, normalizeGroupSessionSkill(&Group{SessionSkill: "draft"}))
}

func TestGroupSessionSkillCRUDCacheAndDuplicate(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{groupRepo: repo, authCacheInvalidator: invalidator}
	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name: "skill", Platform: PlatformAnthropic, RateMultiplier: 1,
		SessionSkillEnabled: true, SessionSkill: "  confirm purpose  ",
	})
	require.NoError(t, err)
	require.Equal(t, "confirm purpose", repo.created.SessionSkill)
	require.True(t, repo.created.SessionSkillEnabled)
	repo.getByID = group
	group.ID = 17
	_, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{})
	require.NoError(t, err)
	require.True(t, repo.updated.SessionSkillEnabled, "omitted fields are preserved")
	disabled := false
	_, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{SessionSkillEnabled: &disabled})
	require.NoError(t, err)
	require.False(t, repo.updated.SessionSkillEnabled)
	require.Equal(t, "confirm purpose", repo.updated.SessionSkill, "disable keeps the draft")
	require.Contains(t, invalidator.groupIDs, group.ID)
	group.SessionSkillEnabled = true
	duplicate := cloneGroupForDuplicate(group, "operation")
	require.True(t, duplicate.SessionSkillEnabled)
	require.Equal(t, group.SessionSkill, duplicate.SessionSkill)
	group.Hydrated = true
	key := &APIKey{ID: 1, Key: "test", Status: StatusActive, GroupID: &group.ID, Group: group, User: &User{ID: 1, Status: StatusActive}}
	cacheSvc := &APIKeyService{}
	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: cacheSvc.snapshotFromAPIKey(context.Background(), key)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))
	restored, used, err := cacheSvc.applyAuthCacheEntry(key.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.True(t, restored.Group.SessionSkillEnabled)
	require.Equal(t, group.SessionSkill, restored.Group.SessionSkill)
}
