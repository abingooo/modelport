# Sub2API 二次开发工作流

## 本地工具链

每次进入新终端后加载项目工具链：

```bash
cd /home/abin/sub2api
source custom/dev-env.sh
```

首次安装前端依赖：

```bash
pnpm --dir frontend install --frozen-lockfile
```

运行检查：

```bash
make test-backend
make test-frontend
```

## 本地 Docker

先启动 Windows Docker Desktop 并为当前 WSL 发行版开启集成，然后运行：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.dev.yml up --build -d
docker compose --env-file deploy/.env -f deploy/docker-compose.dev.yml ps
```

如果 Docker Hub 下载缓慢，可先通过镜像源构建应用镜像：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.dev.yml build \
  --build-arg NODE_IMAGE=dockerproxy.net/library/node:24-alpine \
  --build-arg GOLANG_IMAGE=dockerproxy.net/library/golang:1.26.5-alpine \
  --build-arg ALPINE_IMAGE=dockerproxy.net/library/alpine:3.21 \
  sub2api
docker compose --env-file deploy/.env -f deploy/docker-compose.dev.yml up -d
```

本地管理员邮箱和密码保存在 `deploy/.env` 的 `ADMIN_EMAIL`、`ADMIN_PASSWORD` 中。

停止本地环境：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.dev.yml down
```

本地数据保存在 `deploy/data`、`deploy/postgres_data` 和
`deploy/redis_data`，这些目录及 `deploy/.env` 均不会提交到 Git。

## 分支与版本规范

- `main`：只同步官方 `Wei-Shaw/sub2api`，不放二开提交。
- `develop`：ModelPort 日常集成分支，始终使用开发版本。
- `feature/*`、`fix/*`：从 `develop` 创建，完成后合并回 `develop` 并删除。
- `production`：只保存验证通过、可以直接部署的正式版本。

版本命名：

```text
正式版    0.1.164.4
开发版    0.1.164.4-dev.4
测试版    0.1.164.4-beta.1
候选版    0.1.164.4-rc.1
```

开发版本记录在 `custom/VERSION`。每次准备新的开发镜像时递增末尾序号，禁止使用
五段纯数字版本。只有正式版创建 `custom-v*` 标签和 GitHub Release。

## 同步官方维护

先更新只跟踪官方的 `main`，再将官方更新合并到 `develop`：

```bash
git fetch upstream --tags
git switch main
git merge --ff-only upstream/main
git push origin main
git switch develop
git merge main
git push origin develop
```

不要在服务器或 `production` 上直接合并官方代码。功能开发示例：

```bash
git switch develop
git switch -c feature/example
# 完成开发和测试
git switch develop
git merge --no-ff feature/example
git branch -d feature/example
git push origin develop
```

## 开发镜像

推送 `develop` 后，GitHub Actions 读取 `custom/VERSION` 并构建多架构镜像，不创建
GitHub Release：

```text
ghcr.io/abingooo/modelport:0.1.164.4-dev.4
ghcr.io/abingooo/modelport:develop
ghcr.io/abingooo/modelport:dev-sha-<commit>
```

开发环境优先使用固定版本或 `dev-sha-*`，不要依赖会移动的 `develop` 标签。

## 正式镜像

功能在 `develop` 验证完成后合并到 `production`，再创建正式标签：

```bash
git switch production
git merge --no-ff develop
git push -u origin production
git tag -a custom-v0.1.164.4 -m "ModelPort custom-v0.1.164.4"
git push origin custom-v0.1.164.4
```

工作流会拒绝不指向当前 `production` 提交的 `custom-v*` 标签。不要使用 `v*`，避免触发 Fork
中保留的官方完整发布流程。为了让 `main` 与官方完全一致，自定义工作流不放入默认
分支，因此发布入口只有标签推送，不使用 Actions 手动触发。Fork 首次运行工作流前，
需要先在 GitHub 仓库的 Actions 页面启用工作流。

标签中的 `custom-v` 前缀用于区分发布流程，镜像内应用版本会显示为
`0.1.164.4`。前三位对应官方版本，第四位是 ModelPort 二开迭代号。镜像构建成功后
工作流会创建同名 GitHub Release，前端“查看发布”会打开该自定义发布页。

GitHub Actions 会构建多架构镜像：

```text
ghcr.io/abingooo/modelport:custom-v0.1.164.4
ghcr.io/abingooo/modelport:sha-<commit>
```

## Docker 服务器

服务器继续使用官方 Compose，并叠加镜像覆盖文件：

```bash
export SUB2API_IMAGE=ghcr.io/abingooo/modelport:custom-v0.1.164.4
docker compose -f deploy/docker-compose.yml -f custom/server-compose.override.yml pull
docker compose -f deploy/docker-compose.yml -f custom/server-compose.override.yml up -d
docker compose -f deploy/docker-compose.yml -f custom/server-compose.override.yml ps
```

GHCR 首次发布的包通常为私有包。可以在 GitHub Package 设置中将 `modelport` 改为
Public；如果保持私有，服务器需先使用具备 `read:packages` 权限的 PAT 登录：

```bash
echo "${GHCR_PAT}" | docker login ghcr.io -u abingooo --password-stdin
```

生产环境只使用固定标签，不使用 `latest`。升级前先备份 PostgreSQL，并保留上一个镜像标签。
