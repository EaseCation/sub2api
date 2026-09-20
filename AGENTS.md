# JunxuanB/sub2api 维护与发布约定

## 仓库与范围

- 本仓库是 `JunxuanB/sub2api`，官方上游是 `Wei-Shaw/sub2api` 的 `main`。
- 保留本分支的支付宝证书认证功能，以及“本分支可更新、官方版本仅展示”的更新面板。
- API 密钥多分组需求已撤回，不要在后续维护中恢复这项改动，除非用户再次明确提出。
- 合并官方代码时先检查工作区，保护用户未提交修改；保留本分支功能、发布配置及本文件。

## 独立版本号

- 本分支正式版本从 `1.0.0` 开始，与官方 `0.x` 或后续版本独立编号。
- 版本源为 `backend/cmd/server/VERSION`。Git 标签写作 `v1.0.0`，Docker 镜像标签写作 `1.0.0`。
- 普通开发提交不必修改版本号；准备新的正式发布时更新版本文件，并与发布标签一致。
- 修复问题、兼容性修复、配置调整：升级 PATCH，例如 `1.0.0 → 1.0.1`。
- 新增向后兼容的功能：升级 MINOR，例如 `1.0.1 → 1.1.0`。
- 有明确不兼容变更且用户确认迁移方案：升级 MAJOR，例如 `1.1.0 → 2.0.0`。
- 仅合并官方更新时，按本分支实际变化选择 PATCH 或 MINOR；不得直接复制官方版本号覆盖本分支版本。
- 如果用户指定了版本号，优先遵从用户指定；已发布版本和标签不可复用或强制移动，修复已发布版本应发布新版本。
- 发布前检查远端标签和 GitHub Releases，避免与已有版本冲突。保留历史 `0.2.8` 过渡版本。

## 手动发布，不自动部署服务器

- `.github/workflows/release.yml` 只使用 `workflow_dispatch`。不要添加 `push`、标签推送、定时任务等自动发布触发器，除非用户明确改变要求。
- 发布由 GitHub 手动触发；运行后自动构建程序包、创建 GitHub Release 并上传 Docker 镜像。
- 用户只要求修复代码时不发布。用户已要求发布时，完成检查后执行提交、推送、触发和验证，不重复要求确认。
- 发布镜像不会自动更新用户服务器的容器；不要擅自接入 SSH、Portainer Webhook 或容器自动更新服务。

## 发布步骤

1. 明确本次发布范围，更新 `backend/cmd/server/VERSION`，记录变更及官方 main 的合并提交。
2. 按改动运行检查：后端业务变更使用 `go test -tags unit ./...`（在 `backend/`）；前端变更使用 pnpm 9 执行相关 Vitest、类型检查/生产构建；部署配置变更运行 `deploy/tests/` 下对应检查。仅版本号和文档变更无需重跑所有业务测试。
3. 检查 `git diff --check`，提交代码和说明。创建带说明的版本标签，标签必须指向本次发布提交。例如：

   ```bash
   git tag -a v1.0.1 -m "Release 1.0.1" -m "本次更新说明"
   git push origin main v1.0.1
   ```

4. 打开 GitHub → Actions → Release → Run workflow：分支选 `main`；`tag` 填已存在的版本标签；正式完整发布不勾选 `simple_release` 和 `dry_run`。也可由代理在用户授权发布后显式执行：

   ```bash
   gh workflow run release.yml --repo JunxuanB/sub2api --ref main \
     -f tag=v1.0.1 -f simple_release=false -f dry_run=false
   ```

5. 等待工作流成功，检查 GitHub Release 的程序包和 `checksums.txt`，确认 Docker Hub 镜像包含 `linux/amd64` 和 `linux/arm64`。
6. 核对版本标签与 `latest` 的镜像清单摘要相同，并实际运行镜像的 `-version` 确认版本/提交。工作流尚未完成时不得声称已发布。
7. 告诉用户发布链接、镜像标签、验证结果和更新命令。升级沿用用户现有 Compose、数据卷及 `.env`；仅重建 `sub2api`，不重建数据库或清理数据。

## 发布源与凭据

- 可安装版本、回退版本及程序包来自 `JunxuanB/sub2api` 的 GitHub Releases。
- 官方 `Wei-Shaw/sub2api` 版本仅供展示，不提供官方更新按钮，也不得回退到官方源下载。
- Docker Hub：`junxuanb/sub2api`；GHCR：`ghcr.io/junxuanb/sub2api`。完整发布会更新版本标签及 `latest`、主次版本别名。
- GitHub Actions Secrets：`DOCKERHUB_USERNAME=junxuanb`，`DOCKERHUB_TOKEN` 为拥有 Read / Write 权限的 Docker Hub PAT。当前已配置；Token 到期时替换 Secret。
- 不输出 Token，不把凭据写入 Git、发布说明或普通配置文件。
- `simple_release=true` 仅发布 GHCR 的 amd64 镜像，缺少完整程序包，不适用于本分支正常发布；`dry_run=true` 只构建验证，不上传。
- Docker 部署用 Compose 拉取和重建。面板“立即更新”沿用程序包替换机制，不会更新宿主机镜像。

操作说明见 `docs/FORK_UPDATES_CN.md`。
