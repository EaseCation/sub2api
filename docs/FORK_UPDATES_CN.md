# JunxuanB/sub2api 更新

本分支 1.0.0 基于官方 main `7c700729c23187d31ed320f6b19c790e2f194826`（官方版本文件为 0.2.7），保留 `c41b8e921` 的支付宝证书认证支持。本分支从 1.0.0 开始独立编号，后续修复使用 1.0.1、1.0.2，功能更新使用 1.1.0。官方版本仍单独展示；合并官方 main 时保留本分支版本号。之前的 0.2.8 是过渡发布，保留供回退。API 密钥分组行为没有改变。

面板的主要版本检查、立即更新、版本回退均使用 `JunxuanB/sub2api` 的 GitHub Releases。官方 `Wei-Shaw/sub2api` 的最新稳定发布仅作展示，没有官方更新按钮。官方查询失败不会阻止本分支更新；旧版本缓存会因来源不匹配而失效。

面板的“立即更新”沿用原来的程序包替换机制，不会操作宿主机 Docker。Docker 部署应通过 Compose 更新镜像，使容器重建后仍然保持正确版本。镜像为 `junxuanb/sub2api:1.0.0`，同时提供 `latest` 标签。

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

`.github/workflows/release.yml` 只允许从 GitHub → Actions → Release → Run workflow 手动触发。推送代码或标签均不会发布镜像。触发后，构建和上传自动完成。当前仓库已配置发布所需的两个 Actions Secrets：

| Secret | 值 |
| --- | --- |
| `DOCKERHUB_USERNAME` | `junxuanb` |
| `DOCKERHUB_TOKEN` | Docker Hub Personal Access Token |

每次正式发布会生成 GitHub Release、程序包及校验和，并发布 Docker Hub / GHCR 的 amd64、arm64 镜像，例如 `junxuanb/sub2api:1.0.0`，同时更新 `latest`、`1.0`、`1` 标签。后续正式发布的例子：

```bash
# 先把 backend/cmd/server/VERSION 改为 1.0.1 并提交
# 确认发布说明和待发布提交后创建标签
git tag -a v1.0.1 -m "Release 1.0.1" -m "本次更新说明"
git push origin main v1.0.1
```

推送完成后，打开 https://github.com/JunxuanB/sub2api/actions/workflows/release.yml ，点击 Run workflow，工作流分支选 `main`，tag 填写已存在的版本标签（如 `v1.0.1`）。正常发布时不要勾选 `simple_release` 或 `dry_run`：前者仅发布 GHCR 的 amd64 镜像，后者仅构建而不发布。

只推送代码或标签不会更新正式版 `latest`；必须手动运行 Release。服务器中的固定版本标签也不会因镜像发布自动改变；容器自动更新还需要额外接入服务器部署步骤。

## 获取或替换 Docker Hub Token

1. 登录 https://app.docker.com/，点击头像 → Account settings。
2. 打开 Personal access tokens → Generate new token。
3. 名称填写 `github-sub2api`，设置有效期，授予 Read / Write 权限即可。
4. 生成后复制 Token。它只展示一次，不是 Docker Hub 登录密码。
5. 打开 https://github.com/JunxuanB/sub2api/settings/secrets/actions，将 `DOCKERHUB_TOKEN` 更新为新 Token，并确认 `DOCKERHUB_USERNAME` 为 `junxuanb`。

Token 存入 GitHub Actions Secret，不要写入代码或公开配置。当前 Token 已验证可发布镜像；如果需要更换，按上述步骤更新即可。

Docker 官方说明：https://docs.docker.com/security/access-tokens/personal-access-tokens/
