# OpenAI 账号低容量保护

网关可以在可调度的 OpenAI 账号数量低于阈值时暂时锁住 API 请求，避免剩余账号在故障或限额期间继续被打穿。计数使用正常调度条件：仅统计 active、可调度、未过期、未处于限额/过载/临时停调窗口且没有错误的 OpenAI 账号；其他平台账号不会计入。

该功能默认关闭。配置放在 `gateway.openai_account_availability_guard`，也可以使用对应的 `GATEWAY_OPENAI_ACCOUNT_AVAILABILITY_GUARD_*` 环境变量：

```yaml
gateway:
  openai_account_availability_guard:
    enabled: false
    min_available_accounts: 2
    check_interval_seconds: 30
    message: "当前服务暂时不可用：可用的 OpenAI 账号数量少于系统要求，请稍后再试。"
    exempt_paths:
      - "GET /v1/models"
      - "GET /models"
      - "GET /v1/usage"
      - "GET /usage"
      - "GET /v1/images/tasks/*"
      - "GET /images/tasks/*"
```

`check_interval_seconds` 是进程内缓存时间，同一时间窗口内只会有一个请求刷新数据库计数，其他请求沿用上一份结果。默认 30 秒，因此账号刚进入或刚离开阈值时允许有短暂延迟；正在进行的请求不会被中途终止。计数查询失败时放行并记录日志，避免数据库瞬时故障扩大为全平台停机。

锁定时返回 HTTP 503，错误码为 `OPENAI_ACCOUNT_POOL_LOCKED`，并使用配置中的 `message` 对所有 API 用户提示。豁免路径只应保留确实需要在锁定期间使用的极少数只读接口；路径支持 `*` 前缀匹配，也可以在路径前指定 HTTP 方法。
