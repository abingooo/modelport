# ModelPort 发布约束

本目录中的四段版本文档是 ModelPort 正式发布的唯一 Release Notes 来源。正式发布仅使用 `.github/workflows/custom-image.yml`，旧的 Sub2API `release.yml` 在 `abingooo/modelport` 仓库中由仓库身份条件禁用。

## 发布前提

- 只从受保护的 `production` 分支手动触发。
- `expected_sha` 必须是已经审查通过且与 `production` 头部完全相同的 40 位提交。
- `custom/VERSION`、`backend/cmd/server/VERSION` 和工作流输入必须一致；`backend/cmd/server/UPSTREAM_VERSION` 必须保持本次上游基线。
- 对应 `docs/releases/<version>.md` 必须存在且已经完成审阅。
- `ghcr.io/abingooo/modelport` 必须允许匿名拉取，否则公开发布验证会失败。
- GitHub 仓库必须预先启用 Immutable Releases；工作流只读检查该设置，不会代替仓库所有者修改。该 REST 检查要求 `admin:read`，内置令牌无权时需配置最小权限的 `MODELPORT_RELEASE_ADMIN_TOKEN` Repository Secret。
- GitHub Environment `modelport-production-release` 必须禁止管理员绕过、启用 `prevent_self_review` 并配置至少一名 required reviewer，同时保存 `MODELPORT_PRODUCTION_RESTORE_ATTESTATION_SHA256` 与 `MODELPORT_PRODUCTION_RESTORE_ATTESTATION_BINDING_SHA256`。前者是仓库外已批准、已加密生产恢复证明的 SHA-256；后者是该哈希、本次版本、40 位候选提交、上游提交和 UTC 时间组成的无末尾换行规范 JSON 的 SHA-256。两者都不是生产报告内容、路径、凭据或数据统计。仓库外证明必须明确覆盖 PostgreSQL、Redis、持久资产/外部存储三类证据；独立 reviewer 必须逐项核对工作流输入与证明原文。质量、GHCR 写入和 GitHub Release 写入三个作业都直接使用该 Environment，并在各自写入前重新验证绑定与 24 小时时效，不能用已成功的上游作业绕过重跑审批。
- 手动触发时必须输入获批证明 SHA-256、证明记录的 UTC 时间，并确认工作流只发布公开产物、不会更新生产。证明时间不得早于候选提交、不得位于未来，且触发时不得超过 24 小时。缺少三类真实恢复证据时不得创建或更新上述批准值。
- 正式版本号使用四段数字，Git 标签和镜像版本标签使用 `custom-v<version>`。

## 不可变发布

工作流只发布两个镜像标签：`custom-v<version>` 和 `sha-<40位提交>`。两者必须解析到同一个 `sha256` manifest digest；不发布 `latest`、主版本或主次版本等可变标签。正式构建使用的 Node、Go、Alpine、PostgreSQL 和 Redis 基础镜像也必须固定 OCI index digest。最终 digest、提交、上游提交、基础镜像和平台记录在 Release 资产中，并为该 digest 生成 provenance、SBOM attestation 和 keyless Cosign 签名。

任何正式 Git 标签、GitHub Release 或镜像标签已存在时，工作流必须失败，不覆盖、不删除、不复用。GitHub Release 发布后必须处于 immutable 状态，标签和资产不能再替换。若工作流在镜像发布后、Release 完成前失败，该版本视为不可发布；修复原因后递增第四段版本号重新发布，不能覆盖残留标签。

## 生产边界

发布工作流只写 GitHub Release 和 GHCR，不包含生产服务器凭据，也不调用生产更新、重启、回退、镜像拉取或站内更新接口。公开的 `production-restore-attestation.json` 只记录批准证明的 SHA-256、版本、候选提交、上游提交、UTC 时间、固定范围和“仓库外加密存储”标识；生产报告本身不得进入仓库、Actions artifact 或 Release。公开验证完成后任务必须停止，由用户在 ModelPort 管理站内确认并发起更新。

站内按钮只负责写入一行四段版本请求。宿主更新器从对应 Release 读取 digest，核对镜像标签、revision 和 source 后把 Compose 镜像固定为 digest。安装或替换宿主更新器、systemd 单元以及 Compose 适配文件是独立的运维动作，不随容器发布自动执行，必须另行确认。

生产更新前必须由运维人员确认数据库和配置备份可恢复、实际 Compose 服务名、部署目录、镜像环境变量以及健康检查地址。不得从截图或文档推断这些值并直接操作生产。

发布资产中的 systemd 单元按 `/opt/modelport`、Compose 服务 `sub2api`、宿主健康地址 `127.0.0.1:8080` 和公开设置地址 `127.0.0.1:8080/api/v1/settings/public` 设计；宿主还必须提供 `jq` 和 `cosign`。更新器会同时验证 GitHub Release 已 immutable、Git 标签提交、镜像 digest、OCI labels 和 `production` 发布工作流的 keyless Cosign 身份。systemd 以 mode `0700` 创建 `/var/cache/modelport-update`，并把 `HOME` 与 `TUF_ROOT` 指向其中，使 Cosign/Sigstore 能在 `ProtectHome=true`、`ProtectSystem=strict` 下更新受保护的 TUF 信任缓存。它要求以下文件共同安装：基础 `docker-compose.yml`、`modelport-compose.override.yml`、`.env` 和绑定目录 `data/`。若实际环境不同，必须通过 systemd drop-in 明确设置 `MODELPORT_DEPLOY_DIR`、`MODELPORT_SERVICE_NAME`、`MODELPORT_COMPOSE_FILE`、`MODELPORT_COMPOSE_OVERRIDE_FILE`、`MODELPORT_HEALTH_URL` 或 `MODELPORT_SETTINGS_URL`，并在启用 path 单元前验证合并后的 Compose 配置。修改部署目录时还必须先用空的 `ReadWritePaths=` 重置列表，再同时加入实际部署目录和 `/var/cache/modelport-update`，并同步重置 path 单元的 `PathExists`；不得移除 `CacheDirectory`、`HOME`、`TUF_ROOT`、`ProtectHome` 或 `ProtectSystem`，仅修改环境变量不会改变 systemd 沙箱或监听路径。容器侧 `MODELPORT_UPDATE_MODE` 仍默认为 `manual`；只有用户确认宿主适配完成后，才可在 `.env` 中切换为 `docker`。

发布的 `modelport-compose.override.yml` 只包含 `sub2api` service key。若生产基础 Compose 使用 `modelport` 等其他 service key，仅设置 `MODELPORT_SERVICE_NAME` 不会让该 override 改写目标镜像；运维人员必须另外准备一个使用相同 service key 的 override，并将 `MODELPORT_COMPOSE_OVERRIDE_FILE` 指向它，或者先证明基础 Compose 已直接通过 `MODELPORT_IMAGE` 引用固定 digest。

## 发布后核对

GitHub Release 必须同时包含：

- `manifest-digest.txt`
- `release-revision.txt`
- `release-metadata.json`
- `production-restore-attestation.json`
- `release-assets.sha256`
- `modelport-sbom.spdx.json`
- `modelport-docker-updater`
- `modelport-update.service`
- `modelport-update.path`
- `modelport-compose.override.yml`

公开验证作业必须确认 Release 非草稿、非预发布且不可变，Git 标签指向预期提交，所有资产校验和正确，恢复证明 SHA-256/版本/候选提交/上游提交/UTC/范围与批准绑定完全一致，版本标签与 SHA 标签解析到同一 digest，镜像 OCI 标签匹配，并且 Cosign 签名可由本工作流的 `production` 分支身份验证。
