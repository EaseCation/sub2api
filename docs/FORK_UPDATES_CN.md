# JunxuanB/sub2api 更新

本分支 0.2.8 基于官方 main `7c700729c23187d31ed320f6b19c790e2f194826`（官方版本文件为 0.2.7），保留 `c41b8e921` 的支付宝证书认证支持。分支版本与官方版本分别发布。API 密钥分组行为没有改变。

面板的主要版本检查、立即更新、版本回退均使用 `JunxuanB/sub2api` 的 GitHub Releases。官方 `Wei-Shaw/sub2api` 的最新稳定发布仅作展示，没有官方更新按钮。官方查询失败不会阻止本分支更新；旧版本缓存会因来源不匹配而失效。

面板的“立即更新”沿用原来的程序包替换机制，不会操作宿主机 Docker。Docker 部署应通过 Compose 更新镜像，使容器重建后仍然保持正确版本。镜像为 `junxuanb/sub2api:0.2.8`，同时提供 `latest` 标签。

## 从支付宝证书版升级

在现有 Compose 文件所在目录执行，文件名按实际情况修改。不要用仓库示例覆盖现有 Compose 或 `.env`，保留所有现有数据卷、数据库密码、JWT_SECRET 和 TOTP_ENCRYPTION_KEY。

先备份配置、数据库及 `/opt/sub2api/data`：

```bash
cp docker-compose.local.yml docker-compose.local.yml.before-0.2.8
cp .env .env.before-0.2.8
chmod 600 .env.before-0.2.8
docker compose -f docker-compose.local.yml exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > sub2api-before-0.2.8.dump
sudo tar -czf sub2api-data-before-0.2.8.tar.gz -C /opt/sub2api data
```

把应用的 `image` 改成：

```yaml
image: junxuanb/sub2api:0.2.8
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
