# 按 Repository 查看费用

此功能只增加用量归属及统计，不改变定价、扣费、余额、订阅或配额行为。

用户和管理员的「使用记录」页均有「按 Repository 查看费用」折叠面板。
展开后，沿用页面上方的日期周期、API Key、模型、分组、请求类型、压缩和计费筛选；
管理员还可按用户及上游账号筛选。表格按实际费用从高到低排序，显示请求数、Token、标准费用及实际费用，分页查看。

统计自动识别或显式上报了仓库标识的请求；识别失败以及历史请求不会凭空归属。
仓库标识是客户端自报数据，仅用于统计，不参与授权或扣费。

## Codex 自动识别（默认启用）

无需修改每个人的 Codex 配置，也无需通过脚本启动。网关为符合条件的 Codex 新会话
自动附加一次只读仓库探测指令，让模型使用客户端原有终端工具读取当前项目的 Git origin，
在本地去除用户名、密码及 URL 查询参数后，通过工具结果返回仓库标识。
网关只读取带会话专属标记的结构化工具结果，不从用户提示词或普通回答中猜测仓库。

- 同时支持 HTTP Responses 和 WebSocket 各种转发模式、后续 response.create 帧。
- 需要 Codex 客户端标识及稳定会话信息；支持旧版会话头，以及桌面端/新版 CLI 的 `X-Codex-Turn-Metadata` 中的 `thread_id` / `session_id`。首次注入不依赖固定终端工具名称，兼容动态工具发现；实际执行仍需要客户端工具并遵守其权限。
  没有 Git origin、工具不可用、权限拒绝或模型未执行指令时，继续用户原任务，保持未归属，不反复探测。
- 自动探测每个会话缓存生命周期内最多注入一次；失败也缓存。每实例最多缓存 8192 个会话，固定保留 24 小时，
  不在请求路径读写数据库或 Redis。缓存满时跳过新会话；探测阶段请求体大于 512 KiB 时跳过解析和指令注入。
  注入后最多检查八次工具结果；模型没有报告则停止解析。已识别或失败的会话不再读取请求体，直接复用结果。
- 会话按 API Key 隔离。专属标记在多实例及重启后保持一致，后续历史仍带该工具结果时可恢复归属；
  没有历史、缓存过期、重启或切换实例时，可能重新尝试。当前不引入分布式锁来保证跨实例严格只执行一次。
- 模型应只执行一次本地 Git 检查，最多等待 2 秒，不联网、不安装工具、不提升权限。
  初次识别可能增加一次工具调用、模型往返和少量 Token，不能承诺完全零延迟或零费用。
- 识别前已经写入的最多 16 条会话启动日志，以已有 request_id + API Key 索引异步补充 repository；
  不扫描历史会话或修改费用。每实例最多两个补充任务、每次超时 2 秒；繁忙或失败时保留未归属，不阻塞模型请求。
- 项目切换应开启新会话；同一会话的多仓库任务无法准确拆分。不改写客户端的 session_id、prompt_cache_key、
  previous_response_id 或工具调用 ID，不改变现有分组 Session Skill。

服务端可通过 `gateway.repository_auto_detect: false` 或
`GATEWAY_REPOSITORY_AUTO_DETECT=false` 关闭自动探测。关闭后，显式请求头上报仍然有效。

## 可选：Codex CLI 显式上报

以下方式优先于自动识别，可省去模型探测开销；不再是使用统计的必需步骤。

在当前使用的自定义 provider 配置中加入以下环境变量请求头映射。
将 `sub2api` 替换为你的 `model_provider` 值，合并进已有 provider 配置，保留原来的 URL、认证及其他设置：

```toml
[model_providers.sub2api.env_http_headers]
"X-Sub2api-Repository" = "SUB2API_REPOSITORY"
```

Codex 支持 provider 的 `env_http_headers`，见
[官方配置参考](https://learn.chatgpt.com/docs/config-file/config-reference)。
仅配置该映射不会自动读取 Git；需要通过仓库内提供的脚本启动：

```sh
# 在目标项目目录运行，路径指向本仓库中的脚本
sh /path/to/sub2api/tools/codex-repository.sh

# 也可指定工作目录；其余参数原样传给 PATH 中的 codex
sh /path/to/sub2api/tools/codex-repository.sh -C /path/to/project
```

脚本在启动时读取一次项目的 `origin` remote，去掉 HTTP URL 中的凭据、查询串及片段，
通过环境变量传给 Codex。没有 Git 仓库或没有 origin 时不发送标识，并清理继承的旧标识。
也可以手动设置 `SUB2API_REPOSITORY=github.com/owner/repo` 后启动 Codex。
支持终端工具及稳定会话 ID 的 Codex 桌面端也可使用自动识别；其余客户端可显式发送同名请求头。
CLI 启动脚本不会改变已运行的桌面客户端。

一个 Codex 进程使用启动时确定的仓库标识；切换项目或恢复其他项目的会话时应重新在对应目录启动。
多仓库会话若没有逐请求的明确归属，不能准确自动拆分。

HTTPS、SSH 和 `git@host:owner/repo.git` 形式统一保存为 `host/owner/repo`；
GitHub 的 owner/repo 大小写统一，无 `.git` 后缀，worktree 共享仓库归属。
输入最多 2048 字节，规范化标识最多 512 字节；非法值会被忽略，不影响模型请求。
服务端不从提示词猜测仓库、不访问 GitHub、不执行 Git 命令；自动识别通过客户端现有工具完成。

## 性能与统计接口

- 未提供请求头时，保持原 context；自动识别使用有界本地缓存，不增加数据库或 Redis 查询。
- 提供请求头时，只做有界字符串处理并复制短标识至现有异步用量任务；不执行自动探测。
- 仓库字段随现有日志批量 INSERT 写入，不新增逐请求写事务或同步汇总计数。
  自动识别时只为少量已写入的启动日志异步补充归属，不对后续正常请求逐条 UPDATE。
- 新增可空字段，无历史回填；部分索引只覆盖带仓库标识的行，使用 CONCURRENTLY 构建。
  首次构建索引仍会消耗数据库 I/O；有仓库的日志也会增加字段及索引写入开销。
- 折叠面板不发统计请求；展开后支持最多一年的有界周期。查询使用范围条件、5 秒超时、
  同查询合并、30 秒缓存；每个服务实例最多并发两个仓库统计扫描，缓存最多 256 项。
- 报表精度及保留期沿用现有用量日志机制；缓存和异步日志会带来短暂延迟。

本地 Apple M1 Pro 微基准（仅请求头规范化及 context 捕获，不含数据库）：
无标识约 5 ns、0 分配；HTTPS 标识约 0.59 μs、296 字节；SSH 标识约 0.77 μs、360 字节。
可在 `backend/` 执行
`go test -tags unit ./internal/service -run '^$' -bench '^BenchmarkUsageRepositoryCapture$' -benchtime=300ms` 复测。
这不代表端到端压测结果；新增索引的写入成本和大周期报表成本仍取决于实际日志量。

自动识别的已知会话路径，在同机测试样例中约 0.34 μs、64 字节分配；
1 KiB 与 1 MiB 请求体结果接近，命中缓存后不会随对话历史变大而扫描请求体。
可用 `-bench '^BenchmarkRepositoryDiscoveryKnownSession$'` 复测。

接口：`GET /api/v1/usage/repositories`（当前用户）及
`GET /api/v1/admin/usage/repositories`（管理员）。两者均需要对应的面板认证。

参数复用用量筛选：`start_date`、`end_date`（含结束日，`YYYY-MM-DD`）、`timezone`、
`api_key_id`、`group_id`、`model`、`request_type`、`stream`、`native_compaction_v2`、
`billing_type`、`billing_mode`。管理员额外支持 `user_id`、`account_id`、`upstream_model_mismatch`。
用户接口固定当前用户范围，并验证 API Key 归属，忽略外部 user_id/account_id。
`page` 默认为 1（最大 1000），`page_size` 默认为 20（最大 100）；默认最近七天附近的页面日期范围。

```json
{
  "repositories": [
    { "repository": "github.com/owner/repo", "requests": 12, "total_tokens": 32000, "cost": 0.8, "actual_cost": 0.4 }
  ],
  "page": 1,
  "page_size": 20,
  "has_more": false,
  "generated_at": "2026-11-07T12:00:00Z"
}
```

以上为通用成功响应 envelope 内的 data；标准费用为现有日志 total_cost，实际费用为 actual_cost。
结果只包含所选范围内有标识的日志，不代表该用户的全部消费。
范围或分页非法返回 400；并发扫描已满时返回 503，可稍后重试或缩小周期。
