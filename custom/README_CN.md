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

## 同步官方维护

`main` 只跟踪官方，`production` 保存二次开发：

```bash
git fetch upstream --tags
git switch main
git merge --ff-only upstream/main
git switch production
git merge main
```

测试通过后同步自己的 Fork，不要在服务器上直接合并官方代码：

```bash
git push origin main production
```

## 构建自定义镜像

当前 Fork 为 `abingooo/modelport`，已配置为 `origin`。首次推送二开分支并发布固定版本标签：

```bash
git push -u origin production
git tag -a custom-v0.1.164.2 -m "ModelPort custom-v0.1.164.2"
git push origin custom-v0.1.164.2
```

只使用指向 `production` 提交的 `custom-v*` 标签，不要使用 `v*`，避免触发 Fork
中保留的官方完整发布流程。为了让 `main` 与官方完全一致，自定义工作流不放入默认
分支，因此发布入口只有标签推送，不使用 Actions 手动触发。Fork 首次运行工作流前，
需要先在 GitHub 仓库的 Actions 页面启用工作流。

标签中的 `custom-v` 前缀用于区分发布流程，镜像内应用版本会显示为
`0.1.164.2`。前三位对应官方版本，第四位是 ModelPort 二开迭代号。

GitHub Actions 会构建多架构镜像：

```text
ghcr.io/abingooo/modelport:custom-v0.1.164.2
ghcr.io/abingooo/modelport:sha-<commit>
```

## Docker 服务器

服务器继续使用官方 Compose，并叠加镜像覆盖文件：

```bash
export SUB2API_IMAGE=ghcr.io/abingooo/modelport:custom-v0.1.164.2
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
