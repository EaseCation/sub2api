package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const GroupSessionSkillContextKey = "group_session_skill"
const MaxGroupSessionSkillLength = 16000

func normalizeGroupSessionSkill(group *Group) error {
	group.SessionSkill = strings.TrimSpace(group.SessionSkill)
	if !utf8.ValidString(group.SessionSkill) || utf8.RuneCountInString(group.SessionSkill) > MaxGroupSessionSkillLength {
		return infraerrors.New(400, "INVALID_SESSION_SKILL", "Session Skill must be valid text of at most 16000 characters")
	}
	if group.SessionSkillEnabled && group.SessionSkill == "" {
		return infraerrors.New(400, "INVALID_SESSION_SKILL", "Session Skill is required when enabled")
	}
	return nil
}

// Capture the original API-key group's policy before composite routing replaces it.
// The value is immutable for the lifetime of a WebSocket connection.
func GroupSessionSkillFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, exists := c.Get(GroupSessionSkillContextKey); exists {
		skill, _ := value.(string)
		return skill
	}
	if key := getAPIKeyFromContext(c); key != nil && key.Group != nil && key.Group.SessionSkillEnabled {
		return key.Group.SessionSkill
	}
	return ""
}

// Keep this instruction on subsequent requests: stateless clients do not know
// about gateway-added instructions and cannot replay them themselves. Execution
// is conditional, not injection. This is a model instruction, not a tool gate.
func groupSessionSkillPrompt(skill string) string {
	return "[Group session-start Skill]\n" +
		"判断这是否为用户当前 Session（新会话）的首次交互，而不是每一轮新消息。仅在本会话首次交互、尚未执行以下 Skill 时，优先执行 Skill，再处理用户原始需求。" +
		"结合现有对话历史、上下文摘要和已知会话状态判断；请求不含历史并不一定代表新会话，出现工具结果也不代表 Skill 已完成。" +
		"若已完成检查或用户已在本会话确认，不要再次询问，继续原始需求。若此前正在执行检查或等待确认，继续该流程；需要确认时先记住用户原始需求，等待明确确认后再执行，不把其他回复当作确认。" +
		"不要将仅完成工具调用当作用户确认。没有必要的本地工具或上下文时，如实说明，不得编造 Git 检查结果或确认记录。\n\n" +
		skill + "\n[End group session-start Skill]"
}

// InjectGroupSessionSkill preserves client instructions, structured cache blocks,
// conversation/tool history and all unrelated JSON fields. Reapplying is safe.
func InjectGroupSessionSkill(body []byte, protocol, skill string) ([]byte, error) {
	if strings.TrimSpace(skill) == "" {
		return body, nil
	}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, fmt.Errorf("invalid JSON request for Session Skill")
	}
	prompt := groupSessionSkillPrompt(strings.TrimSpace(skill))
	switch protocol {
	case ContentModerationProtocolOpenAIResponses:
		path := "instructions"
		if gjson.GetBytes(body, "response").IsObject() && gjson.GetBytes(body, "type").String() == "response.create" {
			path = "response.instructions"
		}
		v := gjson.GetBytes(body, path)
		if v.Exists() && v.Type != gjson.Null && v.Type != gjson.String {
			return nil, fmt.Errorf("instructions must be a string")
		}
		if strings.Contains(v.String(), prompt) {
			return body, nil
		}
		return sjson.SetBytes(body, path, strings.TrimSpace(v.String()+"\n\n"+prompt))
	case ContentModerationProtocolAnthropicMessages:
		v := gjson.GetBytes(body, "system")
		if !v.Exists() || v.Type == gjson.Null || v.Type == gjson.String {
			if strings.Contains(v.String(), prompt) {
				return body, nil
			}
			return sjson.SetBytes(body, "system", strings.TrimSpace(v.String()+"\n\n"+prompt))
		}
		return appendSessionSkillBlock(body, "system", v, prompt, map[string]string{"type": "text", "text": prompt})
	case ContentModerationProtocolOpenAIChat:
		v := gjson.GetBytes(body, "messages")
		if !v.IsArray() {
			return nil, fmt.Errorf("messages must be an array")
		}
		for _, item := range v.Array() {
			if role := item.Get("role").String(); (role == "system" || role == "developer") && item.Get("content").String() == prompt {
				return body, nil
			}
		}
		block, _ := json.Marshal(map[string]string{"role": "system", "content": prompt})
		var messages []json.RawMessage
		if err := json.Unmarshal([]byte(v.Raw), &messages); err != nil {
			return nil, err
		}
		return sjson.SetBytes(body, "messages", append([]json.RawMessage{block}, messages...))
	case ContentModerationProtocolGemini:
		path := "systemInstruction"
		if !gjson.GetBytes(body, path).Exists() && gjson.GetBytes(body, "system_instruction").Exists() {
			path = "system_instruction"
		}
		v := gjson.GetBytes(body, path)
		if v.Exists() && !v.IsObject() {
			return nil, fmt.Errorf("systemInstruction must be an object")
		}
		return appendSessionSkillBlock(body, path+".parts", v.Get("parts"), prompt, map[string]string{"text": prompt})
	default:
		return body, nil
	}
}

func appendSessionSkillBlock(body []byte, path string, value gjson.Result, prompt string, block map[string]string) ([]byte, error) {
	var blocks []json.RawMessage
	if value.Exists() {
		if !value.IsArray() {
			return nil, fmt.Errorf("%s must be an array", path)
		}
		for _, item := range value.Array() {
			if item.Get("text").String() == prompt {
				return body, nil
			}
		}
		if err := json.Unmarshal([]byte(value.Raw), &blocks); err != nil {
			return nil, err
		}
	}
	encoded, _ := json.Marshal(block)
	return sjson.SetBytes(body, path, append(blocks, encoded))
}

func injectGroupSessionSkillWS(c *gin.Context, body []byte) ([]byte, error) {
	return InjectGroupSessionSkill(body, ContentModerationProtocolOpenAIResponses, GroupSessionSkillFromContext(c))
}
