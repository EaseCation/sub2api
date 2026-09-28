# 分组 Session Skill

在管理端「分组管理」创建或编辑分组，打开「Session 发起后执行 Skill」，填写 Skill 内容并保存。关闭开关会保留文本草稿；复制分组也会复制此配置。

例如：

> 检查当前工作目录的 Git 状态和远程仓库。如果不属于 EaseCation/ 下的仓库，在其他操作之前要求用户确认本次 AI 调用用于 EaseCation 服务开发。记住用户原始需求，等待明确确认后继续。

网关会自动附加条件：仅在当前 Session 的首次交互、尚未执行 Skill 时优先执行；正在检查或等待确认时继续该流程；已完成检查或已确认时继续原始任务，不重复询问。不需要在文本中再写一遍该条件。

这是模型指令，不是客户端工具执行门禁。模型需要结合对话历史和上下文判断会话状态；缺失历史、上下文压缩或客户端未保留确认记录时，可能重复询问或漏判。网关不会自行访问用户电脑、检查 Git 或替用户确认，实际检查需要客户端提供相应工具。

为了适配无状态客户端，规则会随每次受支持的文本请求发送，但要求仅在会话开始执行。规则文本会增加输入 token；首次收到工具结果并不意味着 Skill 已完成，也不会据此将用户标记为已确认。

支持 Messages（及 count_tokens）、Chat Completions、Responses HTTP 和 Responses WebSocket，以及 Gemini generateContent / streamGenerateContent / countTokens。合成分组使用 API Key 原始绑定分组的规则。独立图片、视频、音频接口、Realtime 和 Responses compact 请求不注入。

配置更新会使分组鉴权缓存失效；已经建立的 WebSocket 使用连接建立时的配置，重连后生效。部署新代码时会自动执行数据库迁移 `241_group_session_skill.sql`；默认关闭，不影响已有分组。
