# Claude 5 小时窗口预启动

在账号管理中打开某个 **Claude OAuth 账号 → 定时测试 → 添加计划**，启用「Claude 5 小时窗口预启动」，选择平时使用的模型并保存。默认不创建预启动计划；每个账号最多有一个，可以暂停、重新启用或删除。

这个模式的行为是：

- 5 小时窗口仍有效时不发消息，等待上游报告的重置时间。
- 窗口到期后，先主动查询额度。只有能确认 weekly 仍有剩余时，才发送一条 `.`，最多生成 **1 个输出 token**。没有对话历史、工具或思考模式；必要的客户端身份提示与输入仍会消耗少量额度。
- weekly 用完时等待 weekly 重置；Sonnet/Fable 模型还会检查上游已返回的对应模型周额度。
- 无法查询额度、窗口字段缺失或为 null、周重置时间无法确认时跳过，不盲发。仅支持可以主动查询用量的 OAuth 账号；Setup Token、API Key、Bedrock 等账号不提供此选项。
- 账号禁用、暂停调度或处于限流/冷却状态时跳过。预启动不会自动恢复这些账号。
- 发送成功后，按响应头的真实 5 小时重置时间等待；缺少该响应头时保守等待完整 5 小时。截止时间保存在数据库中，重启、编辑或重新启用计划不会把它清空。
- 扫描任务每分钟执行，重置时间后留 15 秒余量，因此发送不会精确到重置那一秒。失败或额度未知通常在 10 分钟后重查；上游要求更长等待时遵从其 `Retry-After` / 重置时间。
- 执行记录显示成功、失败或跳过及原因；多个实例通过数据库抢占任务，避免同一个到期计划重复发送。网络结果不明确时，下一次仍先查询额度，不立即补发。

现有普通定时测试计划继续按自己的 Cron 运行。如已有频繁测试的 Claude 计划，可暂停它，避免同时消耗额度。

根据 [Claude 官方窗口说明](https://support.claude.com/en/articles/11647753-how-do-usage-and-length-limits-work)，5 小时窗口从首条消息开始计时。预启动只是尽早开启下一个窗口，不会增加额度、提前恢复当前窗口或解除 weekly 限制，也不保证全天都有剩余额度。

## 管理接口

复用 `POST /admin/scheduled-test-plans`：新增可选的 `mode`，省略时仍为 `connectivity`。创建窗口计划的示例：

```json
{
  "account_id": 123,
  "model_id": "claude-sonnet-4-6",
  "mode": "claude_window",
  "enabled": true,
  "max_results": 50
}
```

窗口模式由服务端决定下一次执行时间，无需填写 Cron；`auto_recover` 固定关闭。模式在创建时选定，更新接口可修改模型、保留条数和启停状态。重复创建同一账号的窗口计划会被拒绝，应编辑已有计划。

数据库迁移 `251_scheduled_claude_window.sql` 添加模式字段和每账号唯一索引，旧计划默认保留为普通连接测试。

## 本地验证

相关 Go 单测覆盖窗口到期、weekly/模型周额度耗尽、缺失额度、暂停账号、失败退避、请求长度、任务抢占及保存截止时间。

以下模拟只使用合成凭据，不访问 Anthropic，也不消耗真实额度：

```bash
# 在仓库根目录启动，另一个终端运行测试；结束后 Ctrl-C 停止。
postman simulation run postman/simulations/claude-window/claude-window.sim.yaml \
  --port 4519 --no-history --output ndjson

cd backend
CLAUDE_WINDOW_SIM_URL=http://localhost:4519/claude-window \
  go test -tags unit ./internal/service -run '^TestClaudeWindowStartupSimulation$' -count=1 -v
```

模拟验证请求格式、成功响应、429 和异常响应；尚未用真实 OAuth 账号验证最小请求的上游接受情况和窗口变化。
