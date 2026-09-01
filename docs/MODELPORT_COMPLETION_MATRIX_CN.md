# ModelPort 0.1.183.1 完成矩阵（工作区审计）

本矩阵是当前工作区的证据台账，不是生产变更授权或公开发布批准。每一项只在有可追溯命令、输出或运行证据时标记为 `passed`；`blocked` 表示需要外部权限、真实生产状态或当前环境无法提供的前置条件，`pending` 表示仍需补充核对，`not-run` 表示尚未执行。

审计时间：2026-09-01（Asia/Shanghai）

## 基线与范围

| 项目 | 状态 | 证据 | 缺口/边界 |
| --- | --- | --- | --- |
| ModelPort 候选版本 | `passed` | `custom/VERSION`、`backend/cmd/server/VERSION`、`deploy/tests/modelport-release-contract-test.sh` | 当前候选为 `0.1.183.1`。 |
| 锁定上游基线 | `passed` | `backend/cmd/server/UPSTREAM_VERSION`；`v0.1.183` annotated tag object `c21fd3382a1c39fe491a96ac6780bac927327ae4` peeled 到 `e8cb019fabf8b55199436229044cbf9aa7a82564`；`git show -s --format=%P 5330ee2a7d4fb29b3d6462ba6ae7b22132bb08fe` 证明重建提交直接以该 peeled commit 为父；`git merge-base --is-ancestor v0.1.183 HEAD` 通过 | `upstream/main` `7b693ae4295e20329f18ff451b29a38879cb4705` 相对 `e8cb019...` 领先 80 个提交，仅作比较；锁定未经明确批准不得静默替换。 |
| `v0.1.184` 范围排除确认 | `passed` | `git ls-remote --tags --refs https://github.com/Wei-Shaw/sub2api.git 'v*'` 发现 `v0.1.184` 标签对象 `cbfc2922ce353adc39142a1ebf9d6d54ad06ca4d`、源码提交 `e98ef32eb29aecd30d1def615912ec4dc93173f3`；GitHub Release 元数据及 compare `v0.1.183...v0.1.184` 已完成只读核对（170 commits，2026-08-31 发布） | 本次仅对标 `v0.1.183`；`v0.1.184` 不做差异分析、移植、迁移、构建、测试或发布，后续是否对齐由用户另行决定。 |
| 版本纳入清单 | `passed` | 最终 `git status --short`、`git diff --name-status`、`git diff --cached --name-status` 逐项复核；parser 测试、完成矩阵和 `frontend/pnpm-workspace.yaml` 均纳入候选 | 本地构建目录、测试报告和临时文件未纳入；正式发布仍须从最终候选提交构建。 |

## 产品功能

| 项目 | 状态 | 证据 | 缺口/边界 |
| --- | --- | --- | --- |
| ModelPort 品牌体系 | `passed` | `frontend/src/i18n/__tests__/modelPortBrandSurface.spec.ts`、`frontend/src/__tests__/modelPortRepositoryLinks.spec.ts`、HomeView 测试 | 仅代表本地代码与渲染回归通过。 |
| 动态模型港首页 | `passed` | `frontend/e2e/home.visual.spec.ts`、Playwright 本地视觉回归、隔离 smoke 首页 HTTP/静态资源检查 | 最终 clean HEAD 的 `linux/amd64` 镜像仍由正式发布 runner 再次验证。 |
| 免费分组 | `passed` | 免费计费后端回归、`frontend/src/components/common/__tests__/GroupBadge.free.spec.ts`、`backend/migrations/modelport_free_group_bridge_migration_test.go` | 真实生产财务不变量仍需生产快照升级前后核对。 |
| 抽奖系统 | `passed` | `backend/internal/service/lottery_test.go`、前端 Lottery API/View 测试、迁移桥测试 | 真实多实例故障演练尚未连接生产。 |
| Instruction Audit V2 | `passed` | 指令解析/服务/路由测试、`backend/internal/server/routes/instruction_audit_route_coverage_test.go`、前端 V2 API/作用域测试 | 加密行为只按现状兼容门验收，不在本版本升级。 |
| 旧 per-client Prompt Audit supplement 运行时清理 | `passed` | 删除 `backend/internal/securityaudit/instruction_prompt_compat.go` 及其 `ResolvePromptAuditRoute`、eligibility provider、管理 API/UI 开关；`instruction_v2_route.go` 与跨协议/完成后继续执行 Prompt Audit 的回归测试证明当前严格路由边界 | 该删除仅清理旧 supplement 兼容层，不删除独立 Prompt Audit；迁移中的 `prompt_audit_enabled` 列仍按下方历史 schema/storage 兼容边界保留。 |
| Prompt Audit `full_prompt` 实现事实 | `passed` | `backend/migrations/182_prompt_audit_full_prompt.sql`、`backend/internal/securityaudit/prompt_repository.go`、`prompt_event_repository.go`、`prompt_repository_integration_test.go:154-183` | 事件表保存未脱敏 `full_prompt`（去 NUL，以 65536 rune 为截断阈值，超限追加截断标记）；staging job/列表不保存或返回该列，单事件管理详情/API/UI 可读；详情路由当前只继承管理端认证；这不是 evidence/hash-raw 密文。 |
| Prompt Audit 隐私规范一致性 | `pending` | 当前代码与集成测试已证明上述行为；历史 OpenSpec/验证材料仍要求完整提示词不得进入 PostgreSQL、管理 API 或前端 | 本版本冻结现状，不擅自加密、删除、脱敏、改变留存或访问门控；后续隐私/产品决策及迁移、备份、API 兼容方案需单独授权。不能把旧规范当作已通过事实。 |
| 下游 API 密钥测试实现 | `passed` | `frontend/src/components/keys/KeyTestModal.vue` 与 `KeyTestModal.spec.ts` 验证使用所选用户 Key 请求 `/v1/models`，可选择模型、Chat Completions/Responses/Messages 三种协议及流式/非流式模式，并覆盖成功响应、协议终止、超时、取消和错误脱敏；`backend/internal/server/routes/gateway.go`、`api_key_auth_test.go`、`gateway_record_usage_test.go` 与 `openai_gateway_record_usage_test.go` 证明这些是生产网关的正常鉴权、路由和 usage/计费路径 | 没有专用“测试”后端或计费旁路；浏览器发出的请求与普通用户请求具有相同服务端权限、调度和用量语义。 |
| 下游 API 密钥 live 验证 | `blocked` | 本地组件、路由和 usage 回归均已通过 | 尚未提供获准的真实下游实例、专用测试 Key、允许的模型和费用上限；在这些信息到位前不得用第三方凭据发起真实计费请求，也不得把合成上游描述成 live 成功。 |
| ModelPort 更新与发布流程 | `pending` | 发布契约、create-only 发布器、版本发现和人工更新入口测试 | 未执行真实 GitHub/GHCR 写入；站内更新必须由用户人工执行。 |

## 加密兼容边界

| 项目 | 状态 | 证据 | 缺口/边界 |
| --- | --- | --- | --- |
| 指令审核加密实现未改写 | `passed` | `backend/internal/securityaudit/instruction_evidence_cipher.go`、`backend/internal/repository/aes_encryptor.go` 无本轮差异 | 继续使用已发布 ModelPort 行为；不升级/降级算法、参数、密钥或格式。 |
| 历史兼容向量验证 | `passed` | `/bin/sh backend/scripts/verify-modelport-crypto-compat-vectors.sh`（重新执行成功）；输出验证 `custom-v0.1.176.2` / `b6cb4d0c8b47d7561631ab61418e1b6fdeb379bc` | 仅使用生成测试密钥和虚构数据；不包含生产密钥、密文或原文。 |
| 加密强度变更 | `not-run` | 本版本 Goal 明确禁止密码学升级、降级、轮换和批量重加密 | 任何未来改进需另行授权，不能借上游适配或扫描建议实施。 |

## 迁移、备份与恢复

| 项目 | 状态 | 证据 | 缺口/边界 |
| --- | --- | --- | --- |
| 空库迁移与幂等 | `passed` | `go test ./cmd/modelport-restore-migrate -count=1`、迁移路径/约束/桥测试 | 结果是隔离环境证据。 |
| Sub2API 数据库路径 | `passed` | 锁定 `v0.1.183` 数据库的本地/隔离迁移 drill 与 ledger/checksum 测试 | 只覆盖锁定候选；`v0.1.184` 明确不在当前 Goal 范围，也不构成本次发布阻断。 |
| 旧 ModelPort bridge 路径 | `passed` | legacy migration path tests、PostgreSQL 隔离恢复 drill、checksum/平台约束验证 | 没有真实生产快照时不得声称生产恢复通过。 |
| 非空无 ledger 数据库 | `passed` | `modelport_migration_paths_integration_test.go` 验证缺少 `schema_migrations` 但存在 public 业务表时，在任何 ledger/Atlas 写入前 fail-closed，并保留原数据 | 该状态无法证明是空库或纯 Sub2API；必须先人工分类和恢复 ledger，不能猜测迁移。 |
| 226 约束与 checksum 兼容 | `passed` | `migrations_runner_manifest_parser_test.go`、`channel_monitor_quota_mode_migration_test.go` 和隔离 PostgreSQL 迁移测试；当前 raw SHA-256 `ebbe62cedfd602a67f6a3e08a705e5982f314a668ee681e1eebc63ca1c639733`、Runner trimmed SHA-256 `ea9926655a2cf71a23b0f54597f7f57d59fca8d5fb1b5fe45c779acd0a57f784` | parser 要求已验证 CHECK 只绑定目标列，且表达式是单一正向 `IN`/`ANY` 精确集合；这是必要的非密码学迁移适配，不改变指令审核加密。 |
| `existing_upgrade` 生产只读审计 | `blocked` | 当前没有用户提供的专用 SSH 别名、轮换后 Agent 和独立主机指纹 | 选择既有升级路径时需用户明确授权后才能连接；只读审计不得修改生产。`first_install` 公开发布路径不要求也不授权该审计。 |
| `existing_upgrade` 生产备份 | `blocked` | Runbook 与备份工具/契约测试已存在 | 选择既有升级路径时仍缺审计后的规模、空间、目标挂载、RPO/一致性、独立加密安排和逐项备份确认。 |
| `existing_upgrade` 真实生产隔离恢复 | `blocked` | `deploy/modelport-isolated-restore.sh`、Redis restore 工具及合成/空库 drill | 尚无真实生产快照和恢复证明；不得以合成结果替代。仅对明确批准的 `first_install` 路径为 `N/A`，绝不标记为 `passed`。 |
| `first_install` 首次部署无存量数据证明 | `blocked` | `not-run`；当前没有受保护的仓库外证明、哈希或候选绑定 | 证明只能断言本次发布不迁移既有 ModelPort 生产数据集或 ModelPort 所有的 PostgreSQL、Redis、持久资产、部署状态、更新状态；须仓库外加密、记录不早于候选且不超过 24 小时、绑定版本/候选/上游，并由唯一 security-owner reviewer 在 Environment 中显式批准，记录现有恢复为 `N/A`、`production_update_performed=false`。当前单账号模式允许所有者自审，但不能绕过 required reviewer；证明不代表服务器或无关服务已审计/修改，冲突预检属于以后实际部署阶段。 |

## 历史兼容字段

| 项目 | 状态 | 证据 | 缺口/边界 |
| --- | --- | --- | --- |
| `prompt_audit_enabled` 历史 schema/storage 兼容 | `passed` | 旧 ModelPort `224_prompt_audit_instruction_patch.sql` 与当前 `234_modelport_instruction_audit_bridge.sql` 仍声明该列及必要约束；当前运行时代码、管理 API/UI 不读取、不写入也不据此判定 Prompt Audit | 该列仅用于历史 schema、迁移 checksum 和存量结构兼容；不得把它解释为重新启用 per-client supplement，也不得在本版本删除、改名或将其值迁移为新的运行时配置。 |

## 测试与发布门槛

| 项目 | 状态 | 证据 | 缺口/边界 |
| --- | --- | --- | --- |
| 后端测试、vet、lint | `passed` | `GOTOOLCHAIN=local go test ./...`；锁定 Colima socket 的 integration suite；`GOTOOLCHAIN=local go test -race ./internal/service ./internal/repository ./internal/securityaudit -run 'Lottery|Instruction|Free|APIKey' -count=1`；`GOTOOLCHAIN=local go vet ./...`；`golangci-lint run --timeout=10m` 均成功 | 后续改动若触及后端需重跑受影响套件。 |
| 前端测试、类型检查、生产构建 | `passed` | ESLint；`./node_modules/.bin/vitest run`：268 files/1882 tests；`./node_modules/.bin/vue-tsc --noEmit`；`./node_modules/.bin/vite build`；pnpm 9 frozen-lockfile 离线安装和 pnpm 11 workspace 配置读取均成功 | 构建保留既有动态导入与 chunk-size 警告；`frontend/pnpm-workspace.yaml` 已纳入候选。 |
| Playwright 视觉门 | `passed` | 桌面/移动端、深浅色、减少动画和 Canvas/资源检查；此前 5 项通过、3 项按设计跳过 | 跳过项必须在发布证据中保留理由。 |
| 迁移/恢复契约 | `passed` | PostgreSQL/Redis 合成与空库 restore contract tests、`actionlint`、ShellCheck | 不等同于 `existing_upgrade` 的真实生产恢复证明；空库迁移/幂等性是 `first_install` 的 CI release evidence 之一，不会把恢复状态改成 `passed`。 |
| 安全扫描 | `pending` | Gitleaks 差异/历史扫描、Trivy secret/config/image、高危/严重配置扫描、pnpm production audit 与例外校验已通过；非最终本地诊断已证明固定 `govulncheck v1.7.0` 对 `CGO_ENABLED=0`、`linux/amd64`、`embed` 发布入口源码和同构未剥离诊断二进制报告 Symbol 0、Package 0、实际可达漏洞 0 | 已新增发布门，要求从正式 backend builder 派生诊断二进制，归一化路径后比对完整 Go 版本、模块和构建设置，核对全部运行时分配 ELF 节的地址和大小，并逐字节比较除 build-id notes 外所有有文件内容的运行时分配节；须在最终 clean HEAD 重新执行后改回 `passed`。正式 stripped 二进制会因无符号而退化为模块级保守结果，不能写成二进制扫描为 0。 |
| Go 模块级漏洞可达性 | `passed` | `CVE-2026-46603` / `GO-2026-6222` 只影响未进入发布依赖闭包的 `x/image/vp8l`，候选闭包中的 `x/image` 仅有 `draw` 与 `math/f64`，头像入口只注册 GIF/JPEG/PNG；`GO-2026-5932` 只影响未进入闭包的 `x/crypto/openpgp*`；`GO-2026-5158`（OpenTelemetry）未进入发布入口调用链。精确源码扫描及符号保留诊断二进制均为 0 | 正式镜像使用 `-s -w`；govulncheck 无法提取符号时会按 `go.mod` 精度保守报告上述三项并退出 3，该结果不是实际符号可达性证据。`x/image` 项有 `v0.45.0` 修复，`openpgp` 项无修复版；本次不为消除工具退化结果扩大 `v0.1.183` 的依赖差异。 |
| 安全 owner 可达性例外/VEX | `blocked` | 技术可达性证据已完成；`.github/audit-exceptions.yml` 当前为空 | 公开发布前仍需真实安全 owner 记录范围、理由、缓解措施、有效期和批准；占位 owner 或空例外列表不能视为批准。 |
| 正式候选镜像构建 | `pending` | 旧候选提交 `2b005c81fab60395e71b5128195055926b5502f0` 的 `linux/amd64` 镜像曾通过架构、非 root、健康、版本和 PostgreSQL/Redis smoke，但该镜像早于当前 Dockerfile 诊断 target 与发布门修改 | 必须从最终冻结提交重新构建并重新完成镜像、govulncheck、Trivy、SBOM 与 smoke，旧候选证据不能转移。 |
| 公开发布验证 | `not-run` | `modelport-production-release` 已配置唯一 `@abingooo` reviewer、允许单账号人工自审、禁止管理员绕过且只允许 `production`；create-only publisher、Release/GHCR 契约与 deployment evidence 校验脚本待最终修改纳入 Git 后重跑 | 仍缺最终候选绑定的获批 OpenVEX 和 `first_install` 受保护无存量迁移证明；这些门槛和最终 CI 未满足前不得写入 Tag、Release 或 GHCR 正式槽位。 |
| `linux/amd64` 发布镜像 smoke | `blocked` | 已被后续修改取代的候选提交 `2b005c81fab60395e71b5128195055926b5502f0` 曾在本地 ARM64 Colima 通过 QEMU 隔离 smoke：容器 healthy、`/health`/设置/API 契约和非 root UID 已核对；既有视觉门覆盖首页 | 最终冻结提交尚未在 GitHub 原生 amd64 runner 重跑；这是 `first_install` 必需的 CI release evidence，不代表任何目标服务器已验证，也不能替代以后用户手动操作后的实例健康检查。 |

## 当前发布结论

当前状态为 `blocked-before-public-release`：本地实现、测试、迁移工具和加密兼容门已有证据，但以下事项仍不能由本地代码或意图文件替代：

1. 尚未选定并满足一条完整生产证据路径：`existing_upgrade` 仍缺只读审计授权、连接条件、备份安排和真实隔离恢复证明；替代的 `first_install` 路径也没有仓库外加密、24 小时内、候选绑定且由 Environment required reviewer 显式审批的无存量迁移证明。两者不能拼接，现有恢复在 `first_install` 下只能是 `N/A`。
2. 获准的下游 live 测试实例、专用 Key、模型和费用上限。
3. 针对模块级不可达漏洞的真实安全 owner 可达性例外/VEX 记录。
4. 正式 runner 生成的 registry digest、SBOM/provenance/signature、干净数据库迁移/幂等性、发布镜像 smoke 和公开发布验证。

已满足但正式运行仍会重新校验的仓库外门槛：受保护发布 Environment 已配置唯一 `@abingooo` reviewer（numeric ID `206009240`），允许该所有者人工自审、禁止管理员绕过，且 deployment policy 仅允许 `production`。用户在对话中的发布确认仍不能替代所选部署证据或 GitHub Environment 的显式审批。

注：已有 `v0.1.184` 只读比较仅作范围排除记录；本次不继续评估或对齐，且不阻止 `v0.1.183` 的开发、验证和发布。

在上述门槛满足前，不执行 GitHub/GHCR 正式写入、生产迁移、拉取镜像、重启、流量切换或 ModelPort 站内更新。`first_install` 的目标冲突预检属于公开发布后的独立部署阶段，不是本矩阵的发布证据，也未被授权执行。即使公开发布完成，`existing_upgrade` 的执行终态仍为“等待用户从 ModelPort 站内手动更新”，`first_install` 的执行终态为“等待用户另行授权实际部署和目标冲突预检”。
