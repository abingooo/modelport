# ModelPort 指令审核架构

本文说明 ModelPort `0.1.183.1` 指令审核的执行边界、数据模型和运行保障。管理员操作见
[`INSTRUCTION_AUDIT_ADMIN_GUIDE_CN.md`](./INSTRUCTION_AUDIT_ADMIN_GUIDE_CN.md)，需求与实现证据见
[`0.1.183.1.md`](./releases/0.1.183.1.md)。

## 安全边界

- Instruction Audit V2 只处理 OpenAI Responses HTTP、SSE 和 WebSocket 请求；`/responses/compact` 明确排除。
- Instruction Audit V2 的审核范围由 API Key 完成认证时得到的下游分组和检测到的客户端共同决定。模型、上游账号、平台和最终路由渠道不参与范围判定。
- 未命中任何启用绑定的请求属于“不适用并放行”，不会生成 `group_not_allowed` 或 `client_not_allowed` 事件。
- 指令审核的放行只结束本模块的判断，请求仍须经过鉴权、内容审核、限流、计费和上游策略。
- User-Agent 只用于客户端分类，可以被调用方伪造；只有服务端可信上下文可以产生 `modelport_internal`。
- 普通日志、通知邮件和错误响应不得包含请求原文、Bearer Token、API Key 或完整请求体。

## 与上游 Prompt Audit 的关系

- 最新上游 Prompt Audit 是 Prompt Audit 的唯一运行时配置源。Instruction Audit V2 不提供 per-client supplement 开关，也不通过 V2 配置改变 Prompt Audit 的运行范围。
- Prompt Audit 覆盖所有适用的提示输入网关路由（至少包括 Chat、Responses、Messages、Embeddings、搜索、图片、视频、音频）以及现有 HTTP、SSE 和 WebSocket 接入链路；无提示路由必须由路由清单显式分类并写明理由。Instruction Audit V2 只审核 Responses 中适用的 `instructions` 与 `input[1]` 指令字段。
- 同一 HTTP 请求或 WebSocket 轮次中，Prompt Audit 与 Instruction Audit V2 各最多执行一次。两条审核链独立得出结论，任一链放行都不能跳过或覆盖另一条。

## Instruction Audit V2 统一处理流程

```text
下游认证
  -> 固化下游分组和客户端类型
  -> 判断是否命中审核绑定
  -> 有界读取和严格解析请求体
  -> 提取 instructions 与 input[1]
  -> SHA-256、规则集、分组和客户端联合匹配
  -> 确定性放行，或形成初始失败原因
  -> 可选 AI 二审
  -> 执行最终原因策略
  -> 在响应前持久化必要事件
  -> 异步通知、归档和统计维护
  -> 继续其余网关链路或返回通用 403
```

HTTP 和 WebSocket 调用同一个解析、范围解析和判定核心。WebSocket 的每个 `response.create` 都独立审核，不能复用上一轮结果。

## 解析与资源限制

默认运行值：

| 参数 | 默认值 | 可配置范围 |
| --- | ---: | ---: |
| 请求体上限 | 64 MiB | 1-128 MiB |
| 解析超时 | 500 ms | 50-5000 ms |
| 进程内请求体工作集预算 | 256 MiB | 至少为请求体上限的 3 倍，最大 2 GiB |
| 单个审核字段 | 1 MiB | 固定安全上限 |

解析器执行 UTF-8、重复键、嵌套深度、值数量、容器大小和截止时间检查，只保留审核所需字段，不将完整 JSON 对象树长期留在内存。压缩请求按解压后的字节数执行上限，HTTP 和 WebSocket 共享同一个进程内工作集预算。HTTP 读取按最多三个同时存在的缓冲区预留，WebSocket 帧读取按两个缓冲区预留；严格解析也从同一预算取得短期租约。因此数据库、后端校验和前端均强制预算至少为请求体上限的 3 倍，默认 256 MiB 相当于默认 64 MiB 上限的 4 倍。生产环境不应在没有满尺寸并发压测的情况下把预算降到最小值。

最终可用请求体上限是 CDN、Nginx、全局 HTTP 限制、网关或 WebSocket 帧限制、指令审核和上游限制中的最小值。边缘上限应高于指令审核上限，使刚超过审核上限的请求能够到达应用并形成 `request_too_large` 事件，同时仍保留独立的边缘硬限制。

`64 MiB` 是默认审核读取硬边界，不是原因策略可以绕过的上传承诺。应用已经读取到超限边界时会先持久化审核结果，但 HTTP 随后仍返回 `413`，WebSocket 仍以“消息过大”关闭；若 CDN 或 Nginx 更早拒绝，请求不会进入 ModelPort，也不会生成指令审核事件。因此 `request_too_large` 的 `allow_and_record` 只改变指令审核事件的处置结果，不能覆盖外层或网关的硬限制，也不能转发一个未被完整读取的请求。

## 判定与最终结果

同一条规则必须同时满足：

```text
摘要命中 AND 下游分组允许 AND 客户端允许
```

按现行字段选择语义，先选择非空且有效的 `instructions`；只有该字段不存在或无效时才选择 `input[1]`。每个请求只对被选字段执行一次判定和摘要匹配，不把两个字段拆成两次独立审核；用户白名单和严格空字段规则形成 `exception_pass`。

最终结果包括：

| 结果 | 含义 |
| --- | --- |
| `blocked` | 最终拒绝请求 |
| `policy_allow` | 确定性审核失败，但原因策略设置为记录后放行 |
| `ai_pass` | AI 二审通过，精确范围临时哈希及事件已原子提交 |
| `hash_pass` | 哈希、分组和客户端联合匹配 |
| `exception_pass` | 用户白名单或严格空字段例外匹配 |

统计恒等式为：

```text
total = blocked + policy_allow + ai_pass + hash_pass + exception_pass
block_rate = blocked / total
```

只有已经进入审核范围的请求计入 `total`。

## 原因策略

每个失败原因保存 `action`、`ai_review_enabled`、`alert_enabled`、临时放行截止时间、配置版本和修改人。`block` 拦截并记录，`allow_and_record` 只忽略本模块的拒绝并完整记录。

- `config_unavailable` 和 `ai_error` 静态强制拦截。
- AI 派生原因不能再次触发 AI，避免递归。
- AI 只支持有可审核文本的 `hash_mismatch` 和部分 `field_invalid`。
- 高风险放行要求 TOTP 和显式二次确认。
- `request_too_large` 的放行必须设置未来 24 小时内的截止时间。
- `request_too_large` 即使形成 `policy_allow`，仍受上述边缘和传输硬限制约束。
- 拦截且开启通知时通知用户和运维；策略放行默认只通知运维。

所有配置修改使用乐观版本，并原子递增全局 `config_version`。实例通过 Redis 失效消息刷新快照；刷新失败时继续使用最近一次有效配置并上报状态。

## AI 二次审核

AI 总开关和各原因的 AI 开关默认关闭。审核服务使用独立 OpenAI 兼容客户端、内部用途标识和受限系统提示词，只发送当前待审核字段，不发送账号、凭据或完整请求体。

AI 输出必须符合严格结构：

```json
{
  "result": "pass | reject | uncertain",
  "approved_source": "instructions | input1 | null",
  "confidence": 0.0,
  "reason": "short reason"
}
```

系统按 `instructions`、`input[1]` 的现行优先级选择一个字段并审核；通过哪个字段，只为该字段创建摘要。AI 通过后，在同一数据库事务内写入事件、AI 来源、加密标准原文、精确 `(group_id, client_type)` 系统规则、规则关联和默认 24 小时临时授权；事务失败则强制形成 `ai_error`，不得先放行。

哈希摘要身份与作用域授权分离。AI 使用 `ai:<group_id>:<client_type>` 唯一键维护专用系统规则集，临时来源、状态和到期时间保存在该规则集与哈希的关联上；命中既有人工或导入哈希时不会修改其全局状态和有效期。普通规则集保存、删除及普通分组绑定接口都会拒绝系统规则集，管理员只能通过精确作用域生命周期接口提升、禁用或永久撤销单个 `(rule_set, hash)` 关联。

Redis 提供单用户 RPM、单用户每日自动加库、全局每日自动加库和并发保护。低于置信度阈值按 `uncertain` 处理。自动哈希可去重；管理员可以把指定精确关联转为永久、禁用或永久撤销，但不会把授权扩展到普通多范围规则集。

## 原文、翻译与访问审计

事件证据使用从固定 `TOTP_ENCRYPTION_KEY` 经 evidence 路径派生的 AES-256-GCM 子密钥；哈希标准原文与 V2 content vault 共同使用原有 hash-raw 派生路径。数据库 BYTEA 保存原始二进制 `nonce || ciphertext || tag`，摘要与密文必须来自同一份 UTF-8 字节，关联数据参与 AEAD 校验。手工创建必须提交原文并保存为 `manual` 来源；只有显式摘要导入可以不带原文并保存为 `import` 来源。旧哈希和摘要导入保持有效并标记为 `raw_content_unavailable`。由指令审核/Prompt Audit 配置保存的凭据（Prompt Audit endpoint token、V2 AI 节点 API Key、AI/翻译 token）仍由现有 SecretEncryptor 使用固定根密钥直用 AES-256-GCM、无 AAD、标准 Base64 文本的 `nonce || ciphertext || tag` 格式；该格式与 evidence/hash-raw 路径相互独立，各调用方的失效节点排除、fail-closed 或跳过行为保持不变。Redis 中由 SecretEncryptor 加密的短期翻译结果沿用该格式，但仅按 TTL/任务生命周期保留，不作长期字节不变承诺。

### Prompt Audit `full_prompt` 当前存储事实

Prompt Audit 的全文字段不属于 Instruction Audit V2 的 evidence/hash-raw 密文路径。当前实现明确采用以下行为：

- 迁移 `backend/migrations/182_prompt_audit_full_prompt.sql` 为 `prompt_audit_events` 增加 `full_prompt TEXT NOT NULL DEFAULT ''`；`ExtractPromptSnapshot`/`BuildFullPrompt` 生成未脱敏文本（去除 NUL，以 `65536` 个 rune 为截断阈值，超限追加 `…` 标记），事件写入路径保存 `snapshot.FullPrompt`。
- `prompt_audit_jobs` 仍只保存脱敏元数据；`prompt_event_repository.go` 只在单事件详情查询读取 `full_prompt`，当前管理 API 和前端详情页可以显示，列表接口不返回该列。当前 Prompt Audit 详情路由只继承管理端认证，不应把本节相邻的 Instruction Audit V2 敏感授权/TOTP 规则推定为已有门控；是否增加额外门控仍属独立待决策事项。
- 该明文字段当前没有沿用 evidence/hash-raw 的加密保护；本版本冻结其现状，不新增/删除字段，不自行改变脱敏、留存或访问门控。事件删除沿用现有事件删除流程。
- 历史 OpenSpec/验证材料曾要求完整提示词不得进入 PostgreSQL、管理 API 或前端，这与当前代码和集成测试相冲突。该矛盾属于独立隐私/产品决策，不能在本版本中假定已经解决；后续任何不落库、加密、脱敏、访问控制或留存期修复都必须另行授权并配套迁移、备份及 API 兼容方案。

`0.1.183.1` 的密码学兼容边界以仓库确认实现为准，生产实际配置须在只读审计中核对：证据/hash-raw 路径继续使用 32 字节根密钥、HMAC-SHA256 purpose 派生、原有 purpose/AAD、`nonce || ciphertext || tag` 布局、版本与密码学失败语义；上述 SecretEncryptor 凭据继续使用固定根密钥直用、无 AAD 的标准 Base64 格式。本次重建、迁移和发布过程中不得隐式轮换生产现有密钥、改变或升级/降级算法或参数、增加未经论证的回退密钥、批量解密/重加密或读写转换；任何独立运维轮换或密码学升级须另行授权。必要的结构桥接只能逐字节复制并校验摘要、长度和关联身份，且不得重写仍在有效保留期的记录。只有一次性、可丢弃的非生产空数据库或 CI 可沿用上游临时密钥行为；生产初始化必须在首次启动和任何敏感数据写入前配置固定密钥。TOTP、step-up、敏感访问、原文保留以及协议/路由覆盖属于独立契约，分别按功能和安全要求验收；它们不是改变指令审核加密强度的例外，但不得降低未授权访问防护。

- 列表接口不返回原文。
- 查看、复制、翻译，以及把事件原文加入候选哈希或规则集，必须同时满足：有效真人管理员 JWT 会话、当前有效的“指令敏感内容”授权和 TOTP step-up；Admin API Key 明确拒绝。
- 敏感授权只能由当前授权持有人在 TOTP step-up 后授予或撤销，目标必须是启用 TOTP 的有效管理员，并禁止撤销最后一名有效持有人。
- 迁移 `213` 只在授权表为空时为最早创建的有效管理员建立一次 `migration_bootstrap` 授权；后续授权变更保留授予人、原因、来源和时间。
- AI 精确作用域的提升、禁用、撤销及其他不读取原文的配置操作要求 TOTP step-up，但不以敏感原文授权代替配置权限边界。
- 每次敏感读取或复制记录操作者、授权 ID、认证方式、授权结果、资源、动作、请求 ID、IP、User-Agent、结果和时间。
- Instruction Audit V2 原文到期后删除其证据密文并保留摘要；Prompt Audit 的 `full_prompt` 不属于该密文路径，随现有事件删除流程处理，数据库备份中的历史数据继续遵循备份保留周期。
- 翻译只由管理员主动触发，按 UTF-8 安全边界分段，在后台执行。
- 翻译任务绑定创建任务时的敏感授权；授权撤销后不能继续取得原文或译文。
- 译文使用现有 SecretEncryptor 以固定根密钥直用 AES-256-GCM、无 AAD、标准 Base64(`nonce || ciphertext || tag`) 在 Redis 短期保存，不进入普通日志；缺失/过期、解密或存储失败保持现有 `result_expired`、`result_unavailable` 和敏感访问审计语义。外部翻译默认关闭并先脱敏明显凭据、令牌和邮箱。

## 持久化、统计与保留

`blocked`、`policy_allow` 和 `ai_pass` 在返回响应前完成必要持久化。`hash_pass` 和 `exception_pass` 先写完整事件，超过正常放行保留期后分批归档到小时聚合表。

需要通知的普通审核结果使用“事件 + 通知意图”同一 PostgreSQL 事务提交；AI 结果还把 AI 审核记录、自动哈希、精确作用域、来源记录、事件和通知意图放在同一事务中。任一必要写入失败都会回滚该事务，不会留下没有对应事件的通知意图或已经放行但未提交的 AI 临时授权。SMTP 发送由事务提交后的 outbox 工作者异步完成，发送失败不会改变已经确定的请求结果。

通知状态区分：`pending`（等待发送）、`processing`（已认领）、`retry`（等待重试）、`sent`（已发送）、`failed`（重试耗尽）、`suppressed`（去重或限流抑制）、`no_recipient`（没有有效收件人）和 `enqueue_failed`（收件人或通知意图准备失败但失败事实已持久化）。`enqueue_failed` 不等同于 SMTP 发送失败。

统计接口同时查询完整事件和小时聚合，支持时间、分组、用户、模型、客户端、最终结果和最终原因筛选。每个聚合分片最多对应 4096 个事件时间戳，按事件编号确定分片，既限制单行数组增长，又保留任意时间边界的精确筛选能力。聚合记录默认保留 365 天，后台任务分批清理并累计“已过期聚合事件”数量。事件持久化失败和统计丢失先进入原子待提交计数，后台批量写入单行 PostgreSQL 累计器；写入失败会恢复本批增量并重试，已提交值在进程重启后仍存在，不以纯进程内计数冒充可靠统计。

运行状态公开以下运维指标：

- 完整事件和聚合事件数量。
- 已清理聚合事件和统计丢失数量。
- 非 AI 审核 p95/p99 与 AI p95/p99。
- 翻译待处理、处理中、失败、活跃工作器和累计工作器失败。
- 事件持久化失败和通知队列积压。

目标为未启用 AI 时审核延迟 `p95 < 15 ms`、`p99 < 40 ms`。AI 延迟单独统计。

## 数据库迁移

下列旧 ModelPort 迁移建立了本指南涉及的运行保障；`0.1.183.1` 通过兼容桥保留这些数据和行为，不重放旧迁移，也不重写既有密文：

| 迁移 | 内容 |
| --- | --- |
| `204` | 最终结果、原因策略、运行配置及事件元数据 |
| `205` | 哈希原文、来源、AI 审核、敏感访问和翻译任务 |
| `206` | 小时结果聚合及归档状态 |
| `207` | 大表并发索引 |
| `208` | 翻译重试、认领、进度和结果元数据 |
| `209` | 聚合保留期、过期数量和清理时间 |
| `210` | 小时聚合时间戳数组的有界分片键 |
| `211` | 将开发候选及历史遗留聚合重分片，并强制每片最多 4096 条时间戳 |
| `212` | 将 AI 精确范围授权的来源与有效期下沉到规则关联，支持安全复用既有哈希身份 |
| `213` | 新增敏感内容授权生命周期，将授权 ID、认证方式和授权结果关联到访问日志与翻译任务，并在空表升级时引导一名初始持有人 |
| `214` | 扩展通知 outbox 状态，持久化区分通知意图准备失败的 `enqueue_failed` |
| `215` | 将旧工作集预算提升到至少请求体上限的 3 倍，并在数据库约束中固定该下限 |
| `216` | 持久化事件写入失败和统计丢失累计计数，并将旧 `expired` 原文状态统一收敛为 `raw_content_unavailable` |
| `217` | 建立 Instruction Audit V2 的配置、客户端作用域、哈希、风险项、事件和 AI 节点 |
| `219` | 建立 V2 的全局可信/风险判定、内容保险库和多节点复核流程 |
| `220` | 为 V2 哈希、风险项和复核任务补充来源用户快照 |
| `234` | 在当前上游迁移序列中直接声明最终兼容结构，并保留旧 ModelPort 数据与密文字节 |

当前迁移不会重建用户、API Key、余额、订阅、用量、分组或既有指令审核表。旧事件保持 `blocked` 的历史含义，旧规则和绑定保持原有生效范围。归档的 `204-220` 只用于历史 checksum 与来源审计，不得在新数据库手工重放；当前 Runner 和 `234` 兼容桥必须分别在空数据库、Sub2API `v0.1.183` 数据库和旧 ModelPort 数据库副本上验证完整保留数据且第二次执行无变化，不能只依赖迁移文本审查。
