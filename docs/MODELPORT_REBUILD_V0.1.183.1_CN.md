# ModelPort 0.1.183.1 重建基线

## 发布目标

ModelPort `0.1.183.1` 基于 Sub2API 稳定标签 `v0.1.183`，对应提交：

```text
e8cb019fabf8b55199436229044cbf9aa7a82564
```

该提交是 annotated tag object `c21fd3382a1c39fe491a96ac6780bac927327ae4` 的 peeled 源码提交。重建提交 `5330ee2a7d4fb29b3d6462ba6ae7b22132bb08fe` 直接以它为父提交；其后的候选收口提交均为 ModelPort 自定义改动。`upstream/main` 的 `7b693ae4295e20329f18ff451b29a38879cb4705` 相对该基线领先 80 个提交，仅作比较，不在本候选历史中。

Sub2API `v0.1.184` 明确不在本版本范围，既有只读比较不属于本版本验收项。

本版本从稳定上游重新建立定制层，不合并旧 ModelPort `production` 分支。保留的产品差异为：

- ModelPort 品牌与动态模型港首页
- 免费分组
- 即时抽奖与定时开奖
- 指令审核 V2
- 下游 API 密钥测试
- ModelPort 版本发现、镜像发布与宿主更新协议
- 旧 ModelPort 生产数据库兼容

Grok、国内平台、Prompt Audit、利润控制、渠道监控、模型广场、网关协议和基础计费继续采用上游实现。

## 数据库兼容边界

- 上游既有迁移文件保持原字节不变。
- 旧 ModelPort 独有迁移归档到非活动目录，只用于生产历史 checksum 校验，不在新数据库重放。
- 新增终态迁移从 `232` 开始，必须同时兼容空数据库、Sub2API `v0.1.183` 数据库和旧 ModelPort 生产数据库。
- 升级迁移不得删除或重写旧生产中的用户、余额、订单、订阅、API Key、账号、渠道、用量、抽奖和审核数据。
- 官方 `224`、`226`、`227` 平台约束升级前必须检查生产历史平台值；Runner 对已执行旧 ModelPort `188`/`197` 的数据库先应用等价超集并记录被超集满足的官方约束迁移，`236_modelport_legacy_platform_constraints.sql` 再建立可重复执行的终态约束。全过程不静默执行 `glm -> zhipu` 或其他数据改名。
- 如果 public schema 已有业务表但缺少 `schema_migrations`，Runner 会在任何 ledger、Atlas 或 bridge 写入前 fail-closed；该状态不能被猜测为空库或纯 Sub2API。226 的 CHECK 兼容预检只接受已验证、仅绑定目标列的单一正向 `IN`/`ANY` 精确集合，并保留已发布与当前 hardened checksum 的窄范围兼容。
- 本次重建、迁移和发布过程中，指令审核现有密文不得批量解密、重加密或隐式轮换生产密钥；独立运维轮换须另行授权。

## 指令审核加密兼容

用户约束：指令审核的加密力度保持当前 ModelPort 现状即可。本节只冻结仓库已确认实现的密码学强度和既有密文兼容面；连接生产后先只读核对实际配置与实现，发现不一致时停止冲突部分并报告。上游适配、代码审计建议或安全扫描结果都不能自行扩大为加密升级范围：

- 继续使用现有 `TOTP_ENCRYPTION_KEY`、32 字节根密钥、HMAC-SHA256 purpose 派生和 AES-256-GCM。
- 指令证据继续使用原有 evidence 派生路径；哈希原文与 V2 content vault 继续共同使用原有 hash-raw 派生路径，数据库字段保持原始 BYTEA `nonce || ciphertext || tag`；V2 vault 既有 `encryption_key_version` 命名继续保留。
- 由指令审核/Prompt Audit 配置保存的凭据（Prompt Audit endpoint token、V2 AI 节点 API Key、AI/翻译 token）继续使用现有根密钥直用 AES-256-GCM、无 AAD、标准 Base64 文本(`nonce || ciphertext || tag`)格式和既有失败语义，不改用 evidence/hash-raw 派生路径；各调用方的失效节点排除、fail-closed 或跳过行为也保持不变。
- Redis 中由 SecretEncryptor 加密的短期翻译结果继续使用同一格式和失败语义；生产升级/迁移不得主动读取、重加密或改写现存值，获批加密隔离副本可按恢复 Runbook 读取验证。TTL 自然过期、任务失效、显式清除和重跑产生的新随机 nonce 密文属于预期，不作长期跨版本字节不变承诺。
- purpose 字符串、AAD 构造、`nonce || ciphertext || tag` 布局、编码、版本标识、可用性判断和密码学失败语义均保持不变。
- 不新增上述指令审核/Prompt Audit 密文使用的未经论证的密钥版本分派、历史别名或弱化性解密回退，不引入第二套根密钥或并行密钥体系；现行实现已有的读取/失败处置保持不变。
- 本次迁移和上游兼容适配不得为兼容目的升级或降级密码学算法、调整参数、隐式轮换生产现有密钥、批量解密或重加密，也不得读时改写、写时转换或清洗版本字段；独立运维轮换须另行授权。若结构桥接确需复制旧密文，只能逐字节复制并校验摘要、长度和关联身份。
- 对迁移涉及且未被声明生命周期操作的、仍在有效保留期的既有记录，迁移和上游兼容适配不得改写密文字节或 SHA-256 摘要；到期清理以及显式业务操作产生的正常新建、替换或清除按旧路径执行，并验证和记录为预期变化。只有一次性、可丢弃的非生产空数据库或 CI 可沿用上游临时密钥行为；生产初始化必须在首次启动和任何敏感数据写入前配置固定密钥。迁移只补齐锁定的 Sub2API `v0.1.183` 基线所需的结构兼容，不重写敏感数据。AES-GCM 对新数据使用随机 nonce 产生不同密文属于现有行为。
- V2 content vault 现有 `encryption_key_version` 字段命名与实际 hash-raw 派生路径之间的历史差异属于兼容性现状，本版本不得自行清理或修正。
- 本节不预先冻结 TOTP 注册/发行者、step-up 门控、敏感访问授权、原文保留、脱敏、日志边界或审核协议/路由覆盖；这些属于独立的功能与安全契约，须分别审查和验收。它们不是改变指令审核加密强度的例外；任何适配都不得降低未授权访问防护，也不得借本节扩大明文采集或暴露范围。
- 上游适配不构成密码学例外：即使只是调用层或接口层适配，也不得改变本节列明的算法、密钥、派生、格式、失败或既有密文语义。发现冲突时停止相关迁移并报告，不得猜测处理。

兼容测试只能使用生成的测试密钥和虚构数据，并通过旧实现生成的固定测试向量验证原始 BYTEA evidence/hash-raw 密文、Base64 服务凭据和 Redis 短期翻译结果的原样读取、格式负测及各调用方失败处置；对 evidence/hash-raw 验证错误密钥、错误 AAD 和篡改拒绝，对无 AAD 的 SecretEncryptor/Redis 格式只验证错误密钥和篡改拒绝，不得为了测试增加 AAD；短期结果同时验证 TTL/任务失效语义。生产密钥、密文、摘要和原文不得进入仓库、日志或发布产物；生产快照只可按恢复 Runbook 在获批加密隔离环境中验证。

## Prompt Audit 明文留存事实

Prompt Audit 的 `full_prompt` 不属于上面的 evidence/hash-raw 密文路径，必须与指令审核加密兼容边界分开记录。当前代码和迁移的事实是：

- `backend/migrations/182_prompt_audit_full_prompt.sql` 为 `prompt_audit_events` 增加 `full_prompt TEXT NOT NULL DEFAULT ''`；`ExtractPromptSnapshot`/`BuildFullPrompt` 生成未脱敏文本（去除 NUL，以 `65536` 个 rune 为截断阈值，超限追加 `…` 标记），`prompt_repository.go` 在事件写入时保存 `snapshot.FullPrompt`。
- 临时 `prompt_audit_jobs` 仍只保存脱敏元数据；`prompt_event_repository.go` 的单事件详情查询读取并返回 `full_prompt`，当前管理 API 和前端详情页可以显示，列表查询不加载该列。当前 Prompt Audit 详情路由只继承管理端认证，不能把 Instruction Audit V2 的敏感授权/TOTP 规则推定为其已有门控；是否增加额外门控仍属独立待决策事项。
- `0.1.183.1` 冻结上述行为，不因“指令审核加密力度保持现状”而增加或删除该字段、引入加密/脱敏、改变留存或访问门控，也不执行明文迁移改写。事件删除沿用现有事件删除路径。
- 历史 OpenSpec/验证材料关于“完整提示词不得进入 PostgreSQL、管理 API 或前端”的要求与当前代码及集成测试冲突；该差异记录为独立隐私/产品决策 `pending`（或 `blocked`），不能把旧要求当作当前实现事实或把当前实现标成已满足。若后续恢复不落库、加密、脱敏、访问控制或保留期策略，必须另行授权并单独设计迁移、备份和 API 兼容方案。

## 已验证的功能契约

- `v0.1.183` 基线内的上游 Prompt Audit 是 Prompt Audit 的唯一运行时配置源。Instruction Audit V2 运行时不再提供、读取或写入 per-client supplement 开关，数据库中的兼容保留字段不参与运行时判定。
- Prompt Audit 覆盖所有适用的提示输入网关路由（至少包括 Chat、Responses、Messages、Embeddings、搜索、图片、视频、音频）以及现有 HTTP、SSE 和 WebSocket 接入链路；无提示路由必须由路由清单显式分类并写明理由。Instruction Audit V2 只在 Responses 的 `instructions` 与 `input[1]` 指令字段适用范围内运行，并保持现行字段优先级（有效非空 `instructions` 优先，否则使用 `input[1]`）。同一 HTTP 请求或 WebSocket 轮次中两条审核链各最多运行一次，任一审核链放行都不得跳过另一条。
- 免费组在已有 usage 记录路径中用户实付/`ActualCost` 为零，余额、订阅金额用量、API Key 金额配额和 user-platform 金额额度不扣减；订阅有效性、用户/分组 RPM、用户并发、鉴权、调度、风控和其他非金额限制仍然执行。沿用现有各模态 usage、`TotalCost`、`account_stats_cost`/账号配额统计记录上游真实成本（含既有账号倍率），不为未落 usage 的路径新增财务台账。仓库当前没有独立的 per-key RPM。
- 即时与定时抽奖继续使用事务化、幂等结算；scheduler 执行错误写入服务日志，不能静默丢失。
- 下游 API 密钥测试继续走真实网关并产生正常用量记录；错误展示必须脱敏上游 URL、账号、渠道和路由信息。

## 发布与生产边界

生产只读审计、完整备份、隔离恢复和人工更新边界见
[`MODELPORT_PRODUCTION_UPGRADE_RUNBOOK_CN.md`](MODELPORT_PRODUCTION_UPGRADE_RUNBOOK_CN.md)。

- 正式标签使用 `custom-v0.1.183.1`，以兼容当前生产站内四段版本发现规则。
- 正式镜像同时提供版本标签与不可变 `sha-<commit>` 标签，并在 Release 中记录 manifest digest、平台、ModelPort 提交和上游提交。
- 首次更新请求继续兼容当前宿主更新器的一行四段版本协议。
- 新宿主更新器必须以 digest 固定候选和回退镜像，但其安装不随容器镜像自动发生，只能由用户另行确认安装。
- Codex 不调用生产更新、回退、重启或强制刷新接口。发布完成后停在等待用户站内更新。

## 验证门槛

- 后端 unit、integration、关键 race 和静态检查通过。
- 前端测试、类型检查、Lint、构建和主要视口视觉检查通过。
- 三类数据库迁移与旧生产备份恢复演练通过。
- 免费计费、抽奖幂等、审核路由、密文兼容、API Key 测试和更新协议有针对性回归测试。
- 正式镜像在生产目标架构完成启动、健康检查和核心 API 冒烟。
- GitHub Release 与 GHCR 校验通过，但不触碰生产更新状态。
