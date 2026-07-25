# ModelPort 开发、发布与服务器更新工作流

本文档记录 ModelPort 二次开发的长期工作流和当前环境快照。长期规则以仓库中的实际
GitHub Actions、版本校验和更新器脚本为准；版本快照只表示本文档更新时的状态。

## 1. 总体链路

```text
Wei-Shaw/sub2api
       |
       | 同步官方更新（人工决定时机）
       v
main -> develop -> production -> custom-v* -> GitHub Release + GHCR AMD64 镜像
          |                            |
          |                            | 管理员在网站中人工点击更新
          v                            v
   dev-v* prerelease          update-request -> systemd -> Docker Compose
                                                   |
                                                   v
                                          健康检查 / 失败回滚
```

核心原则：

- `main` 保持官方代码，不混入 ModelPort 二开提交。
- 日常开发只进入 `develop`，正式产品只从 `production` 发布。
- 开发版和正式版使用不同标签、镜像标签和更新通道。
- GitHub 只构建 `linux/amd64`，不构建 ARM64。
- 发布新版本只让网站发现更新，不自动更新服务器。
- 应用容器不挂载 Docker Socket，真正的容器更新由宿主机 systemd 服务执行。
- ModelPort 更新服务只读取 `abingooo/modelport`，不会把原作者 Release 推送给服务器。

## 2. Git 仓库与分支

远程仓库：

```text
origin    git@github.com:abingooo/modelport.git
upstream  https://github.com/Wei-Shaw/sub2api.git
```

分支职责：

| 分支 | 用途 | 允许的内容 |
| --- | --- | --- |
| `main` | 官方镜像分支 | 只接受 `upstream/main` 的更新 |
| `develop` | ModelPort 集成分支 | 已完成的二开功能、修复和开发版本号 |
| `feature/*`、`fix/*` | 短期工作分支 | 单一功能或修复，完成后合并回 `develop` |
| `production` | 正式产品分支 | 已经过测试、可以直接部署的版本 |

禁止在测试服务器上直接改代码、合并分支或打标签。服务器只消费已发布镜像。

## 3. 版本规范

```text
正式版    0.1.164.5
开发版    0.1.164.6-dev.1
测试版    0.1.164.6-beta.1
候选版    0.1.164.6-rc.1
```

- 前三段对应当前采用的 Sub2API 基线。
- 第四段是 ModelPort 正式迭代号。
- `dev.N` 是同一目标正式版本下的开发构建序号。
- 开发版本记录在 `custom/VERSION`，每次推送 `develop` 前必须保证版本唯一。
- 正式 Git 标签使用 `custom-v<版本>`，例如 `custom-v0.1.164.5`。
- 开发 GitHub prerelease 标签使用 `dev-v<版本>`，例如
  `dev-v0.1.164.6-dev.1`。
- 禁止为二开版本使用 `v*`，避免触发仓库中保留的官方发布流程。

## 4. 日常开发流程

开始工作：

```bash
git switch develop
git pull --ff-only origin develop
git switch -c feature/example
```

完成实现后，先运行与改动范围匹配的测试、类型检查、Lint 和构建。合并前更新
`custom/VERSION`，例如从 `0.1.164.6-dev.1` 递增到
`0.1.164.6-dev.2`。

```bash
git switch develop
git merge --no-ff feature/example
git branch -d feature/example
git push origin develop
```

推送 `develop` 会自动触发 `.github/workflows/development-image.yml`：

1. 校验 `custom/VERSION` 必须符合四段版本加 `-dev.N`。
2. 检查对应开发标签从未发布，禁止覆盖已有版本。
3. 构建并推送唯一运行平台 `linux/amd64`。
4. 创建 GitHub prerelease，记录提交、镜像和摘要。

开发镜像标签：

```text
ghcr.io/abingooo/modelport:<开发版本>
ghcr.io/abingooo/modelport:develop
ghcr.io/abingooo/modelport:dev-sha-<commit>
```

测试和排障应优先使用固定开发版本或 `dev-sha-*`，不要把可移动的 `develop` 标签当作
可复现版本。

## 5. 正式发布流程

正式发布前必须满足：

- 目标功能已经进入 `develop`。
- 针对性测试、前端类型检查和生产构建通过。
- 对应开发镜像已经成功构建。
- 数据库迁移和回滚影响已经确认。
- 正式版本号尚未被标签或 Release 占用。

以发布 `0.1.164.6` 为例：

```bash
git switch production
git pull --ff-only origin production
git merge --no-ff develop -m "release: promote ModelPort 0.1.164.6"
git push origin production
git tag -a custom-v0.1.164.6 -m "ModelPort custom-v0.1.164.6"
git push origin custom-v0.1.164.6
git switch develop
```

`.github/workflows/custom-image.yml` 会执行以下约束：

1. 标签必须指向远端 `production` 的当前提交。
2. 标签必须符合 `custom-vX.Y.Z.N`，不接受开发或官方标签。
3. 只构建并推送 `linux/amd64` 镜像。
4. 创建非 prerelease 的 GitHub Release。
5. Release 自动包含版本说明、提交清单和完整变更对比链接。

正式镜像标签：

```text
ghcr.io/abingooo/modelport:custom-v0.1.164.6
ghcr.io/abingooo/modelport:sha-<commit>
```

发布完成不等于部署完成。工作流不会连接服务器，也不会创建更新请求。

## 6. 测试服务器人工更新

测试站点：

```text
https://test.modelport.link
```

服务器部署目录和关键文件：

```text
/opt/modelport/docker-compose.yml
/opt/modelport/.env
/opt/modelport/data/update-request
/usr/local/sbin/modelport-docker-updater
/etc/systemd/system/modelport-update.path
/etc/systemd/system/modelport-update.service
```

正式镜像中的应用版本形如 `0.1.164.5`，因此更新服务自动进入 `stable` 通道，只接受：

```text
仓库       abingooo/modelport
Release    非 prerelease
标签       custom-vX.Y.Z.N
```

管理员在网站页脚打开版本面板、刷新更新信息并点击更新后：

1. 后端再次校验目标版本和当前更新通道。
2. 后端只向 `/app/data/update-request` 写入一行目标版本，不直接操作 Docker。
3. 共享数据目录把请求暴露为宿主机 `/opt/modelport/data/update-request`。
4. `modelport-update.path` 检测文件并启动 `modelport-update.service`。
5. 更新器把目标版本转换为固定 GHCR 镜像标签并更新 `.env`。
6. 更新器兼容 `docker compose` 和旧版 `docker-compose`，拉取并重建应用容器。
7. 更新器最多等待约 120 秒完成健康检查。
8. 成功后删除请求和临时备份；失败则恢复旧 `.env` 并重建旧容器。

正常更新时间通常为 2 到 5 分钟，主要取决于服务器从 GHCR 下载镜像的速度。容器切换
期间通常有 20 到 60 秒不可用窗口。提交后不要重复点击更新。

## 7. 更新安全边界

- 应用只能提交版本请求，不能执行任意宿主机命令。
- 更新器只允许 `0.1.164.6-dev.1` 或 `0.1.164.6` 这两类严格格式。
- 镜像仓库固定为 `ghcr.io/abingooo/modelport`，请求中不能指定其他仓库。
- 正式通道拒绝 `dev-v*`、原作者 `v*` 和旧缓存中的官方 Release。
- 更新过程中使用文件锁，避免重复更新并发执行。
- `.env` 在更新前备份，健康检查失败自动回滚。
- PostgreSQL、Redis 和应用数据使用持久卷，重建应用容器不会清空现有数据。
- 服务器凭据、管理员密码、API Key 和 `.env` 内容禁止写入 Git、Release 或本文档。

## 8. 检查与排障

服务器只读检查：

```bash
docker ps --filter name=modelport
systemctl is-active modelport-update.path
journalctl -u modelport-update.service -n 50 --no-pager
curl -fsS https://test.modelport.link/health
curl -fsS https://test.modelport.link/api/v1/settings/public
```

检查是否存在未完成请求：

```bash
find /opt/modelport/data -maxdepth 1 -type f \
  \( -name 'update-request' -o -name 'update-request.processing' \
  -o -name 'update-request.failed' \) -print
```

状态含义：

| 文件 | 含义 |
| --- | --- |
| `update-request` | 已提交，等待 systemd 处理 |
| `update-request.processing` | 更新器正在执行 |
| `update-request.failed` | 更新失败，旧镜像已尝试恢复 |

失败时先查看 systemd 日志和当前容器镜像，不要手工删除数据库卷。需要人工回退时，应把
`.env` 中的 `MODELPORT_IMAGE` 固定到已知正式标签，再用 Compose 只重建应用服务。

## 9. 同步原作者更新

官方更新由人工决定合并时机，不会自动进入 ModelPort 产品：

```bash
git fetch upstream --tags
git switch main
git merge --ff-only upstream/main
git push origin main
git switch develop
git merge main
```

合并后必须在 `develop` 解决冲突并执行回归测试。只有确认二开功能完整、数据库迁移安全、
开发镜像验证成功后，才能按正式发布流程进入 `production`。禁止直接把 `upstream/main`
合并到 `production` 或测试服务器。

## 10. 正式发布检查清单

发布前：

- [ ] 工作分支已合并到 `develop`
- [ ] `custom/VERSION` 是唯一开发版本
- [ ] 针对性测试、类型检查、Lint 和构建通过
- [ ] 开发镜像为 `linux/amd64` 且可拉取
- [ ] `production` 只包含准备发布的内容
- [ ] 正式版本标签尚未占用

发布后：

- [ ] GitHub Actions 成功
- [ ] GitHub Release 不是 prerelease
- [ ] Release 包含版本说明和完整变更链接
- [ ] 正式镜像只有 `linux/amd64` 运行清单
- [ ] 镜像摘要已记录
- [ ] 测试服务器仍保持旧版本，等待管理员人工更新
- [ ] 人工更新后健康检查返回 `200`
- [ ] 线上公开版本与目标正式版本一致
- [ ] 更新请求和 `.processing` 文件已清理

## 11. 当前状态快照

快照日期：2026-07-26。

```text
正式分支版本       0.1.164.5
正式标签           custom-v0.1.164.5
下一开发版本       0.1.164.6-dev.2
测试站更新方式     网站管理员人工点击
镜像架构           linux/amd64
官方更新策略       暂缓合并，待基础二开稳定后再同步
```

测试站的实际运行版本可能在管理员点击更新后发生变化，应以公开设置接口和容器镜像为准，
不要把本节快照当作实时状态。
