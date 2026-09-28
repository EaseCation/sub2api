# JunxuanB/sub2api 更新

本分支 1.0.0 基于官方 main `7c700729c23187d31ed320f6b19c790e2f194826`（官方版本文件为 0.2.7），保留 `c41b8e921` 的支付宝证书认证支持。本分支从 1.0.0 开始独立编号，后续修复使用 1.0.1、1.0.2，功能更新使用 1.1.0。官方版本仍单独展示；合并官方 main 时保留本分支版本号。之前的 0.2.8 是过渡发布，保留供回退。API 密钥分组行为没有改变。

面板的主要版本检查、立即更新、版本回退均使用 `JunxuanB/sub2api` 的 GitHub Releases。官方 `Wei-Shaw/sub2api` 的最新稳定发布仅作展示，没有官方更新按钮。官方查询失败不会阻止本分支更新；旧版本缓存会因来源不匹配而失效。

面板的“立即更新”沿用原来的程序包替换机制，不会操作宿主机 Docker。Docker 部署应通过 Compose 更新镜像，使容器重建后仍然保持正确版本。镜像为 `junxuanb/sub2api:1.0.0`，同时提供 `latest` 标签。

## 当前开发分支同步记录

2026-09-28 同步官方 [v0.2.9](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.9)，合并提交为 `4c00df2e0183e2c70b7fa8ba45914205e36aad0c`。同步范围截至该发布标签，不包含标签之后的官方 main 改动。

同步前已完整撤回 OpenAI 可用账号低于阈值时关闭网关的封锁模式，包括后台设置、全局提示及相关配置。继续保留支付宝证书认证、本分支更新与官方版本展示、手动发布流程和镜像仓库配置；API 密钥多分组改动未恢复。

此次仅同步代码，版本文件暂保留 `1.0.0`，不代表重新发布 1.0.0，也不会更新已发布镜像或服务器。后续正式发布必须选择未使用的新版本号。此次上游包含数据库迁移 `239_channel_reasoning_effort_multipliers.sql` 和 `240_affiliate_ledger_operation_id.sql`，部署前应备份数据库。

## 1.2.0 发布范围（2026-09-28）

- 合入官方 v0.2.9，截至上游提交 `4c00df2e0183e2c70b7fa8ba45914205e36aad0c`；本分支合并提交为 `6be44007a4c01f41116a6a10946bef076ff43ee9`。
- 新增分组 Session Skill：支持管理端配置、复制分组和受支持文本请求的规则注入，默认关闭。详见 `docs/GROUP_SESSION_SKILL_CN.md`。
- 新增 OpenAI 优先保障模式：默认关闭，默认阈值 2。可调度实体账号数小于等于阈值时，仅优先保障用户的新 OpenAI 请求放行；其他平台和已经开始的请求不受影响，全站显示提示。详见 `docs/OPENAI_PRIORITY_CN.md`。
- 保留支付宝证书认证、本分支更新与官方版本展示；不恢复 API 密钥多分组或 OpenAI 全员封锁模式。
- GitHub Actions 仅保留手动 Release，发布 GitHub Release 程序包、校验和以及 Docker Hub `junxuanb/sub2api` 的 amd64、arm64 镜像；删除 GHCR、独立 CI、安全扫描、CLA 及其他非产物发布步骤。

此版本在官方迁移 239、240 基础上增加 `241_group_session_skill.sql` 和 `242_user_openai_priority.sql`，应用启动时自动迁移。升级前备份数据库，沿用现有 Compose、数据卷和 `.env`，仅重建 `sub2api` 服务。固定版本镜像使用 `junxuanb/sub2api:1.2.0`。

## 从支付宝证书版升级

在现有 Compose 文件所在目录执行，文件名按实际情况修改。不要用仓库示例覆盖现有 Compose 或 `.env`，保留所有现有数据卷、数据库密码、JWT_SECRET 和 TOTP_ENCRYPTION_KEY。

先备份配置、数据库及 `/opt/sub2api/data`：

```bash
cp docker-compose.local.yml docker-compose.local.yml.before-1.0.0
cp .env .env.before-1.0.0
chmod 600 .env.before-1.0.0
docker compose -f docker-compose.local.yml exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > sub2api-before-1.0.0.dump
sudo tar -czf sub2api-data-before-1.0.0.tar.gz -C /opt/sub2api data
```

把应用的 `image` 改成：

```yaml
image: junxuanb/sub2api:1.0.0
```

然后只更新应用服务，保留现有 PostgreSQL 和 Redis：

```bash
docker compose -f docker-compose.local.yml pull sub2api
docker compose -f docker-compose.local.yml up -d --no-deps sub2api
docker compose -f docker-compose.local.yml ps sub2api
docker compose -f docker-compose.local.yml logs --tail=100 sub2api
```

应用首次启动自动执行官方新增的数据库迁移。确认 `/health` 正常、管理员登录、支付宝证书配置及支付渠道正常，再清理旧镜像。涉及数据库迁移的回退应恢复匹配的备份，不能只假定换回旧镜像即可。

如果在 Portainer 管理 Stack，先备份，再在原 Stack 编辑上述 `image` 一行，重新部署并选择拉取镜像。`docker.junxuanb.com` 是管理面板地址，不是镜像仓库前缀。

## 在 GitHub 手动触发镜像发布

GitHub Actions 仅保留 Release 工作流，用于构建、校验并上传 GitHub Release 程序包及 Docker Hub `junxuanb/sub2api` 镜像；不再发布 GHCR 镜像。测试、lint 和安全检查在本地完成，不运行独立 CI、安全扫描或 CLA 工作流。发布流程不发送 Telegram 通知、不更新 Docker Hub 简介，也不自动回写版本号；正式发布前应在本地更新并提交 `backend/cmd/server/VERSION`。

`.github/workflows/release.yml` 只允许从 GitHub → Actions → Release → Run workflow 手动触发。推送代码或标签均不会发布镜像。触发后，构建和上传自动完成。当前仓库已配置发布所需的两个 Actions Secrets：

| Secret | 值 |
| --- | --- |
| `DOCKERHUB_USERNAME` | `junxuanb` |
| `DOCKERHUB_TOKEN` | Docker Hub Personal Access Token |

每次正式发布会生成 GitHub Release、程序包及校验和，并发布 Docker Hub 的 amd64、arm64 镜像，例如 `junxuanb/sub2api:1.0.0`，同时更新 `latest`、`1.0`、`1` 标签。后续正式发布的例子：

```bash
# 先把 backend/cmd/server/VERSION 改为 1.0.1 并提交
# 确认发布说明和待发布提交后创建标签
git tag -a v1.0.1 -m "Release 1.0.1" -m "本次更新说明"
git push origin main v1.0.1
```

推送完成后，打开 https://github.com/JunxuanB/sub2api/actions/workflows/release.yml ，点击 Run workflow，工作流分支选 `main`，tag 填写已存在的版本标签（如 `v1.0.1`）。正常发布时不要勾选 `dry_run`（该选项仅构建而不发布）。

只推送代码或标签不会更新正式版 `latest`；必须手动运行 Release。服务器中的固定版本标签也不会因镜像发布自动改变；容器自动更新还需要额外接入服务器部署步骤。

## 获取或替换 Docker Hub Token

1. 登录 https://app.docker.com/，点击头像 → Account settings。
2. 打开 Personal access tokens → Generate new token。
3. 名称填写 `github-sub2api`，设置有效期，授予 Read / Write 权限即可。
4. 生成后复制 Token。它只展示一次，不是 Docker Hub 登录密码。
5. 打开 https://github.com/JunxuanB/sub2api/settings/secrets/actions，将 `DOCKERHUB_TOKEN` 更新为新 Token，并确认 `DOCKERHUB_USERNAME` 为 `junxuanb`。

Token 存入 GitHub Actions Secret，不要写入代码或公开配置。当前 Token 已验证可发布镜像；如果需要更换，按上述步骤更新即可。

Docker 官方说明：https://docs.docker.com/security/access-tokens/personal-access-tokens/
