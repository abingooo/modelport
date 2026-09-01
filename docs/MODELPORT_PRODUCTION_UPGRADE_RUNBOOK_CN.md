# ModelPort 生产升级审计、备份与恢复 Runbook

本文用于 ModelPort 正式发布前的生产只读审计、备份和隔离恢复验证；发布章节还定义不接触目标服务器的 `first_install` 公开发布证据路径。它不授权部署、迁移、重启、镜像拉取、配置修改或站内更新。

## 只读审计前硬门槛

开始任何生产连接进行只读审计前，必须同时满足：

1. 用户已在当前对话明确授权开始生产只读审计；仅提供连接信息不视为授权，审计授权也不包含备份或任何生产写入。
2. 对话截图中出现过的登录凭据已经轮换，旧凭据不得再使用、复述或保存。
3. 短期 SSH 私钥已由用户配置到本机 SSH Agent 和专用 SSH 别名；私钥、密码和令牌不得出现在命令参数、仓库或报告中。
4. SSH 主机公钥指纹已从可信的独立渠道取得并核对，连接必须启用严格主机校验。

缺少任一项时停止，不尝试密码登录，也不接受新的未知主机指纹。备份目标、加密和一致性条件在只读审计完成后按“备份前暂停点”单独核对和授权；它们不是开始只读审计的前置授权。

## 操作边界

生产审计阶段只允许读取现有状态。SSH、`docker exec`、数据库连接和 Redis 连接仍可能留下访问日志或短暂增加连接计数；这里的“只读”是指不改变持久业务状态。

禁止执行：

- `docker pull`、`docker compose up/down/restart`、容器重建、服务重启或流量切换。
- 应用启动、数据库迁移、DDL、临时表、`EXPLAIN ANALYZE` 或任何业务写入。
- `SAVE`、`BGSAVE`、`BGREWRITEAOF`、`CONFIG SET`、全库 `KEYS`/`SCAN` 或 `redis-cli --rdb`。
- 创建、移动、读取或删除站内 `update-request` 请求文件。
- 调用站内更新、刷新、回退或备份接口。
- 安装软件、修改 systemd、Compose、Nginx、防火墙、用户、权限或目录属主。
- 输出完整环境变量、完整 `docker inspect`、应用日志、数据库行、Redis 键、密钥、令牌或敏感原文。

不得安装或启动 `datamanagementd`。当前仓库内相关服务接口已经废弃，安装脚本会创建用户、写入 `/opt` 和 `/etc` 并启动 systemd，不属于审计操作。应用内备份服务只覆盖 PostgreSQL，还会写备份记录并上传对象存储，也不能替代本次完整生产备份。

## 只读审计

连接示例只使用用户预先配置的别名：

```sh
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes modelport-production
```

连接后先执行 `umask 077`，不要启用 shell 调试、命令回显或终端录制。按以下顺序采集脱敏结果。

### 宿主资源

```sh
id
date -u
uname -a
cat /etc/os-release
uptime
free -h
df -hT
df -i
lsblk -f
findmnt
```

不要先运行全盘 `find` 或无边界 `du`。如需估算目录大小，先确认具体挂载点并限制范围。

### 容器与真实部署拓扑

```sh
docker compose ls
docker ps --no-trunc --format '{{.Names}}\t{{.Image}}\t{{.Status}}'
docker network ls
docker volume ls
```

从应用容器的 Compose labels 读取真实 project、working directory、config files 和 service，再用选择性的 `docker inspect --format` 查询：

- 容器名称、Compose service、运行和健康状态。
- `.Config.Image`、镜像 ID、`RepoDigests`。
- OCI `version`、`revision`、`source` labels。
- 实际 mount 的类型、源、目标和读写属性。

不要输出 `.Config.Env`、完整 labels、完整 mount options 或完整 inspect JSON。存储驱动的 mount options 可能包含凭据。

### 更新与系统服务

只允许使用 `systemctl is-active`、`is-enabled` 和选择字段的 `systemctl show`。确认更新 path/service、监听目录及待处理请求是否存在，但不读取或触碰请求文件。发现已有待处理更新时立即报告并暂停。

### 配置清单

仅记录实际 Compose 文件、override、`.env`、应用配置、Nginx 主配置/include、systemd fragment/drop-ins 的：

- 绝对路径。
- 文件类型、属主、权限和大小。
- SHA-256。

不要把内容打印到终端或复制进仓库。原文件只能进入获准的加密备份目标。

### PostgreSQL

使用现有 PostgreSQL 容器内匹配服务端版本的 `psql -X`，依赖容器内部既有认证，不把密码放入 `-W`、URL 或命令参数。每组查询必须使用显式只读事务和短超时：

```sql
BEGIN READ ONLY;
SET LOCAL statement_timeout = '30s';
SELECT version(), current_database(), pg_database_size(current_database());
SELECT filename, checksum, applied_at FROM schema_migrations ORDER BY filename;
COMMIT;
```

随后只查询聚合信息：public 表清单和估算/精确行数、序列、约束、索引、外键，以及用户、余额、订单、订阅、API Key、账号、渠道、用量、抽奖和审核表的计数/财务汇总。不得导出行级数据、API Key、密文或摘要。

### Redis

依赖容器内既有认证方式，不使用会把密码写入参数的 `redis-cli -a`。只允许：

```text
INFO server
INFO persistence
INFO memory
INFO replication
INFO keyspace
CONFIG GET appendonly
CONFIG GET appendfsync
CONFIG GET save
DBSIZE
LASTSAVE
ROLE
```

记录 RDB/AOF 模式、最后成功持久化时间、复制角色和数据规模。不要枚举键。

## 备份前暂停点

完成只读审计后，向用户汇报以下内容并等待其确认备份目标与一致性方案：

- PostgreSQL 版本、数据库大小和预计 dump 时长。
- Redis 持久化模式、`LASTSAVE`、AOF 状态及可达到的 RPO。
- 目标挂载点、文件系统、可用空间和 inode。
- 预计 CPU、I/O、锁和业务影响。
- PostgreSQL、Redis、配置、持久资产及报告的加密方式、密钥保管人和解密演练方法。

在用户明确确认备份前，还必须同时满足：

1. 用户确认一个可新建文件的独立加密备份目标；目标不得位于系统盘、应用目录、PostgreSQL/Redis 数据卷、站内更新监听目录或本仓库。
2. 已记录目标挂载的 source、文件系统类型、UUID/稳定身份、可用空间和 inode；每次创建 `.partial` 或最终文件前都必须重新核对，挂载消失或身份变化时立即停止。
3. PostgreSQL、Redis、配置、持久资产和恢复报告都具备静态加密、加密传输、`0700` 目录/`0600` 文件权限及独立恢复密钥/接收方；恢复密钥不得与应用密钥相同或存放在备份旁边。
4. 已确认数据库规模、Redis 持久化模式、允许的 I/O 影响、跨存储 RPO 和备份一致性方案。

缺少任一项或用户未明确确认时停止，不创建目录或文件，不把备份暂存在系统盘，也不落地临时明文。

`pg_dump` 是逻辑只读操作，但会消耗 CPU/I/O 并持有 `ACCESS SHARE` 锁，因此必须在此暂停点之后执行。

## 备份目标与加密

在任何写入前，用 `findmnt -T`、`mountpoint` 和目标文件系统的稳定标识重新确认批准的挂载仍然存在。备份根目录使用不可预测的 UTC 唯一名称，以 `0700` 创建且已存在时失败；禁止先写本地系统盘再搬运。

备份必须使用预先批准、主机上已经存在的认证加密工具或具有独立密钥的加密存储。加密密钥只能通过受限文件描述符、Agent、硬件密钥或等价的非回显方式提供，不得进入命令参数、环境转储、shell 历史、日志或同目录文件。所有网络传输必须使用经过主机校验的加密通道。

正式验证同时覆盖解密路径：在隔离环境从加密备份流式解密，校验格式并完成实际恢复。只有密文备份及加密报告可以长期保留；不得在未加密目录落地 PostgreSQL dump、Redis RDB/AOF、配置归档或恢复报告。

## PostgreSQL 备份

使用生产 PostgreSQL 容器内的同版本 `pg_dump`，生成 custom-format、无 owner/ACL 的单进程备份，并直接写入批准目标的认证加密流。先写唯一的密文 `.partial` 文件；只有管道中每个命令都成功、密文非空、解密流的 `pg_restore --list` 验证成功且挂载身份再次匹配后，才原子改为最终名称。执行环境必须启用等价于 `pipefail` 的失败传播。

必要参数：

```text
pg_dump --format=custom --no-owner --no-acl
```

不要把数据库密码放进命令参数。不要覆盖已有文件。不要 live-tar PostgreSQL 数据卷，也不要以文件复制代替一致的逻辑 dump。

最终记录：

- UTC 开始/结束时间。
- PostgreSQL server/client 版本。
- 数据库名的脱敏标识与逻辑大小。
- dump 文件名、字节数和 SHA-256。
- `pg_restore --list` 验证结果。
- 加密格式、非敏感密钥标识、恢复密钥保管人和一次解密验证结果。
- 期间出现的超时、锁或性能限制。

## Redis 备份

复制正在变化的 AOF 目录不能提供一致快照。`SAVE`、`BGSAVE`、`redis-cli --rdb` 和存储层快照都会改变状态或产生明显 I/O，不在默认授权范围内。

未取得额外授权时，只能把已经完成的 `dump.rdb` 复制到认证加密流，并同时记录：

- `LASTSAVE`。
- 源文件时间、大小和 SHA-256。
- 复制完成后的目标大小和 SHA-256。
- 明确的 RPO：只覆盖该次 `LASTSAVE` 之前的数据。

如果生产只使用 AOF，或用户要求接近零 RPO，必须暂停并由用户在 `BGSAVE`、存储层原子快照或维护窗口中选择一种方案。无法取得当前一致 Redis 备份时，报告必须标记为未满足，不能用 live copy 冒充成功。

解密后的 Redis 输入只能短暂存在于批准的加密隔离存储中，并使用 `deploy/modelport-isolated-redis-restore.sh` 做 `redis-check-rdb` 或非破坏性 `redis-check-aof`、实际隔离加载、`PING`、`DBSIZE` 和 persistence/keyspace 聚合验证。不得输出键名或值。PostgreSQL 与 Redis 无法形成同一时点时，报告必须分别记录两个 RPO 及业务可接受性，不能宣称强一致快照。

## 配置与运行清单

按审计阶段确定的白名单归档实际配置、Compose/Nginx、systemd units/drop-ins 和必要证书引用。保留 owner、mode、ACL 和 xattr；归档必须在离开生产主机前进入已批准的加密目标。

另行保存脱敏清单：

- 容器、service、网络、卷和 mount 映射。
- 镜像引用、镜像 ID、RepoDigest 和 OCI labels。
- 配置文件路径、权限、大小和 SHA-256。
- 应用/上游版本、迁移 ledger 和备份时间。

清单不得包含环境变量值、密码、私钥、令牌、API Key、Cookie、数据库行或 Redis 键。

## 持久资产与外部存储

逐项盘点 `/app/data` 及真实 Compose mounts，至少覆盖插件包/运行时、公开资源覆盖、上传文件、图片存储、更新请求目录、缓存和临时文件。对每一项标记为：

- 权威数据：必须进入加密备份并实际恢复抽查。
- 可从数据库或发布产物重建：记录重建来源、版本和校验和。
- 临时/运行态数据：明确排除理由和更新前清理规则。

`update-request`、`.processing` 和 `.failed` 文件属于更新控制状态，不得作为普通资产复制或创建；发现待处理状态时暂停。启用 S3/对象存储或外部 Secret 管理时，必须使用对应服务的版本化、快照或导出机制，记录 bucket/namespace 的脱敏身份、RPO、对象数量/总量聚合和恢复抽查结果，不得导出访问密钥。

没有逐项分类、备份和恢复证据时，只能称为“PostgreSQL 备份”或“Redis 备份”，不得称为“完整生产备份”。

## 校验和与不可覆盖规则

所有备份位于唯一的 UTC 时间戳目录中，目录创建必须在已存在时失败。每次写入前重新验证挂载身份；对最终密文文件生成 SHA-256 manifest，再立即执行一次校验。`.partial` 文件不进入 manifest；失败时保留最小化诊断并明确标记失败，不得把部分文件改名为成功备份。

不删除、覆盖或原地更新任何历史备份。记录目标位置、加密方式、访问控制、保留期和恢复责任人，但不要把真实敏感路径或密钥提交到仓库。manifest、恢复报告和财务/规模聚合均属于生产敏感衍生数据，必须随备份加密、限制访问并按保留期销毁；禁止提交 GitHub、Release、CI artifact、工单或公开日志。

## 隔离恢复与迁移验证

恢复只能在本地或专用非生产主机执行，禁止在生产创建测试容器、数据库、卷或网络。完整生产 dump 天然包含真实 API Key、OAuth、账号配置和密文；隔离环境必须位于批准的加密存储上，禁止外网和生产网络，并在验证后按保留策略处置。不得把恢复出的凭据注入应用、打印、导出或用于请求。

使用与生产 PostgreSQL major/version 对齐且以 digest 固定的预加载镜像。先验证备份 SHA-256 和 `pg_restore --list`，再恢复到随机 `modelport_restore_*` 临时数据库。恢复后运行当前源码内嵌迁移，并生成结构化报告。

报告必须比较迁移前后：

- `schema_migrations` filename/checksum ledger。
- public 表计数。
- 用户余额、订单和其他财务字段汇总。
- 序列状态。
- 约束、索引和外键定义。
- 用户、订阅、API Key、账号、渠道、用量、抽奖和审核记录数。
- 指令审核密文字节数及与稳定主键身份绑定的聚合 SHA-256；对迁移涉及且未被声明生命周期操作的既有记录，字节和摘要必须保持不变；到期清理、用户明确新建/替换/清除等旧业务路径的预期变化须单独记录。报告不得包含密文、摘要明细或原文。

迁移后再次运行迁移验证幂等性。生产快照只允许运行迁移命令和离线不变量查询；不得启动完整 ModelPort，不得运行调度、刷新令牌或执行 API 冒烟。核心 API 冒烟必须使用合成数据库、虚构密钥和无副作用上游。任何恢复错误、checksum 冲突、未声明的计数/财务变化、序列异常、约束/索引/外键缺失、迁移涉及且未被声明生命周期操作的密文字节变化或不明平台值都属于发布阻断；已声明的到期清理及用户明确新建/替换/清除结果必须符合旧业务契约并有记录。

PostgreSQL 工具与说明见 `deploy/modelport-isolated-restore.sh` 和 `deploy/MODELPORT_ISOLATED_RESTORE.md`；Redis 工具与说明见 `deploy/modelport-isolated-redis-restore.sh` 和 `deploy/MODELPORT_ISOLATED_REDIS_RESTORE.md`。生产恢复报告还必须包含持久资产/外部存储清单与抽查。没有三者的真实报告时，只能声明工具或合成演练通过，不能声明完整生产恢复门槛通过。

## 发布与人工更新

公开 GitHub/GHCR 发布必须通过 `production_deployment_mode` 采用一条且仅一条证据路径：省略输入时安全回落到默认的 `existing_upgrade` 并仍要求完整恢复证明，首次部署则必须显式选择 `first_install`。未知模式、两组输入混用或所选模式证据不完整时都必须 fail-closed。模式只决定公开发布所需证据，不授权连接、审计、修改或部署任何特定服务器，也不证明任何无关服务的状态。

### 发布证据路径及共用约束

只要本次部署会接触任何既有 ModelPort 生产数据集，或 ModelPort 所有的 PostgreSQL、Redis、持久资产、部署状态、更新状态，就必须选择 `existing_upgrade`。本 Runbook 前述只读审计、真实备份与隔离恢复章节，以及下列涉及生产快照、恢复证明和恢复 Secret 的现有门槛，仅适用于 `existing_upgrade` 且保持不变；下列 create-only 写入、Environment 保护、VEX 和人工更新边界适用于两种模式：

只有源码 CI、安全扫描、正式镜像冒烟和生产快照隔离恢复全部通过后，才能创建不可变 Git tag、GitHub Release 和 GHCR 版本/SHA 标签。生产恢复证明必须绑定本次版本、40 位候选提交、上游提交和 UTC 时间，覆盖 PostgreSQL、Redis、持久资产/外部存储三类证据，并保存在批准的仓库外加密目标中；正式发布只接收该证明的 SHA-256 和记录时间，不接收报告内容、真实路径、数据统计、密文或凭据。

GitHub tag/Release 发布使用 create-only REST 请求：先创建指向候选提交的随机 ownership 注解 tag object，再以 `POST` 创建 tag ref、draft Release 和每个资产；同名 tag、Release 或资产存在时必须收到非 `201` 并失败，不能转为更新。所有远端资产按本地 SHA-256 回读验证后才把本次创建的 draft 改为正式 Release。失败回滚只删除 API 返回的本次 Release ID/node ID、资产 ID 和注解 tag object SHA 仍完全匹配的资源；发现外部 Release、未知资产、字段变化或 tag ref 被替换时保留现场并失败，绝不删除无法证明归属的资源。

GHCR/OCI 当前没有已文档化且经本工作流验证的服务端 create-only manifest tag 条件写入。工作流因此先把 `sha-<candidate>` 作为唯一 staging/final 锚点，在写入前一行执行 fail-closed 不存在检查，推送后取得并验证 registry digest；随后再次确认 SHA 标签仍解析到该 digest，在版本标签写入前一行重新检查不存在，并只从 `repository@digest` 创建版本标签，最后核对两个标签都解析到同一 digest。这能防止普通重跑覆盖、缩短竞态窗口并保证版本标签内容来自已验证候选，但不能原子消除外部写入者在“检查不存在”和 registry 写入之间抢占同名标签的竞态。除非 GHCR 提供并启用可核验的不可变标签或条件 PUT，发布记录不得把这一流程描述为完全原子或对任意并发外部写入绝对无覆盖；受保护 Environment、仓库 workflow concurrency 和发布窗口内唯一写入者仍是必要前提。

唯一正式 GitHub/GHCR 写入路径是 `.github/workflows/custom-image.yml` 及其受保护的 `modelport-production-release` Environment。当前仓库只有一个 GitHub 所有者账号，因此采用单账号人工审核：该账号可以配置候选绑定的证明/VEX Secret、触发工作流并在 Environment 中再次显式批准，但不得绕过 Environment，或使用本地个人 token 直接执行 `git push`、`docker push`、创建 tag/Release 或调用 GitHub/GHCR 写 API。用户确认不能替代 Environment 审批，Environment 审批也不能替代用户确认；该模式不应被描述为独立第二人复核。

正式工作流的质量、GHCR 镜像发布和 GitHub Release 三个作业都必须直接通过受保护的 `modelport-production-release` GitHub Environment：禁止管理员绕过保护规则，设置 `prevent_self_review=false`，并且只配置一名 required reviewer；该用户是单账号仓库所有者/security owner，其 GitHub numeric ID 固定为 `206009240`，并必须与获批 VEX owner ID 完全一致。Environment 的 deployment policy 必须且只能允许 `production` 分支。相同账号可以触发和批准，但每个受保护作业仍必须等待 GitHub 的显式人工批准，不能自动放行。Environment 配置仅绑定本仓库且权限限定为 `Administration: read`、`Contents: read and write` 的 7 天 fine-grained PAT `MODELPORT_RELEASE_ADMIN_TOKEN`；工作流必须确认 PAT 属于该 numeric ID，且不得回退到内置 `GITHUB_TOKEN`。Environment 还配置所选路径的两个证明 Secret：`existing_upgrade` 使用 `MODELPORT_PRODUCTION_RESTORE_ATTESTATION_SHA256`、`MODELPORT_PRODUCTION_RESTORE_ATTESTATION_BINDING_SHA256`，`first_install` 使用 `MODELPORT_PRODUCTION_FIRST_INSTALL_ATTESTATION_SHA256`、`MODELPORT_PRODUCTION_FIRST_INSTALL_ATTESTATION_BINDING_SHA256`；两种路径都另配置四个 VEX Secret：`MODELPORT_GO_VEX_DOCUMENT_BASE64`、`MODELPORT_GO_VEX_SHA256`、`MODELPORT_GO_VEX_OWNER_ID`、`MODELPORT_GO_VEX_BINDING_SHA256`。每个作业都重新验证 Environment、所选证明与 VEX；任何重跑都必须重新经过该作业自身的保护和校验，不能复用上游作业结果绕过审批。合成恢复演练、空数据库演练或仅 PostgreSQL/Redis 单项通过都不能生成 `existing_upgrade` 恢复批准证明。

单账号方案保留的是“再次点击批准”的人工确认，不提供第二人职责分离。若唯一 GitHub 会话或该短期 PAT 被攻破，攻击者可能修改 Environment/Secrets 并完成自审；代码无法消除这一账户级风险。因此 PAT 只能保存在该 Environment、发布完成并完成公开验证后立即撤销，后续发布重新生成。

仓库外证明至少包含以下逻辑字段；实际文件及其加密封装不得提交到仓库：

```json
{
  "schema_version": 1,
  "version": "0.1.183.1",
  "candidate_revision": "<40 lowercase hex>",
  "upstream_revision": "e8cb019fabf8b55199436229044cbf9aa7a82564",
  "recorded_at": "<UTC RFC3339 seconds>",
  "postgresql": {"result": "passed", "encrypted_report_sha256": "<64 lowercase hex>"},
  "redis": {"result": "passed", "encrypted_report_sha256": "<64 lowercase hex>"},
  "persistent_assets": {"result": "passed", "encrypted_report_sha256": "<64 lowercase hex>"},
  "production_update_performed": false
}
```

第一个 Environment Secret 和工作流输入使用最终加密证明文件本身的 SHA-256。第二个 Secret 对以下单行规范 JSON 原样计算 SHA-256；字段顺序和无空白编码必须与命令一致：

```sh
binding_json="$(jq -cn \
  --arg sha256 '<encrypted-attestation-sha256>' \
  --arg version '0.1.183.1' \
  --arg candidate_revision '<40 lowercase hex>' \
  --arg upstream_revision 'e8cb019fabf8b55199436229044cbf9aa7a82564' \
  --arg recorded_at '<UTC RFC3339 seconds>' \
  '{schema_version:1,sha256:$sha256,version:$version,
    candidate_revision:$candidate_revision,upstream_revision:$upstream_revision,
    recorded_at:$recorded_at}')"
printf '%s' "$binding_json" | /bin/sh custom/release/sha256-stdin
```

哈希输入不包含末尾换行；不得把 `jq` 的标准输出直接通过管道送入哈希工具，否则会把该换行计入并得到不同结果。仓库内 helper 在 Linux 优先使用 `sha256sum`，在 macOS 回退到 `shasum -a 256`，两者均不可用时明确失败。

唯一 security-owner reviewer 必须先在批准环境解密并核对证明字段、三份报告哈希和候选提交，再核对加密文件哈希及绑定哈希。`recorded_at` 不得早于候选提交，不得位于未来，正式工作流开始时不得超过 24 小时。证明中不得加入真实路径、对象名、数据规模、账户标识、密文、摘要明细或原文。

### `first_install`：首次部署证据

`first_install` 是独立的公开发布证据路径，不是合成恢复通过，也不会把现有恢复结果标成 `passed`。仓库外证明只能声明：本次候选用于首次 ModelPort 部署，不迁移任何既有 ModelPort 生产数据集，也不迁移任何 ModelPort 所有的 PostgreSQL、Redis、持久资产、部署状态或更新状态。它不得绑定或命名某台服务器，不得声称已经审计、连接、修改或部署任何服务器或无关服务。目标端口、目录、Compose service、卷、数据库、Redis、更新监听路径等冲突预检属于公开发布后的实际部署阶段，不能写进或替代本证明。

仓库外证明至少包含以下逻辑字段；实际文件必须在仓库外静态加密并通过加密通道保存，证明内容及其加密封装不得提交到仓库、Actions artifact 或 Release：

```json
{
  "schema_version": 1,
  "evidence_kind": "first_install",
  "version": "0.1.183.1",
  "candidate_revision": "<40 lowercase hex>",
  "upstream_revision": "e8cb019fabf8b55199436229044cbf9aa7a82564",
  "recorded_at": "<UTC RFC3339 seconds>",
  "no_existing_modelport_production_dataset": true,
  "migration": {
    "postgresql": false,
    "redis": false,
    "persistent_assets": false,
    "deployment_state": false,
    "update_state": false
  },
  "existing_restore_proof": "not_applicable",
  "production_update_performed": false
}
```

`existing_restore_proof` 的唯一允许值是 `not_applicable`（`N/A`），不得写成 `passed`。正式 CI 仍必须实际完成干净数据库迁移及第二次运行幂等性验证，并对最终发布镜像完成 smoke；它们作为发布工作流生成的 `ci_evidence`，不能伪装成生产恢复或目标服务器检查。

工作流输入必须使用 `production_first_install_attestation_sha256` 和 `production_first_install_attestation_utc`，并保持两个 `production_restore_attestation_*` 输入为空。受保护 Environment 必须配置 `MODELPORT_PRODUCTION_FIRST_INSTALL_ATTESTATION_SHA256` 和 `MODELPORT_PRODUCTION_FIRST_INSTALL_ATTESTATION_BINDING_SHA256`。前者等于最终仓库外加密证明文件本身的 SHA-256；后者对以下字段顺序固定、无空白、无末尾换行的单行 JSON 原样计算 SHA-256：

```sh
binding_json="$(jq -cn \
  --arg sha256 '<encrypted-attestation-sha256>' \
  --arg version '0.1.183.1' \
  --arg candidate_revision '<40 lowercase hex>' \
  --arg upstream_revision 'e8cb019fabf8b55199436229044cbf9aa7a82564' \
  --arg recorded_at '<UTC RFC3339 seconds>' \
  '{schema_version:1,evidence_kind:"first_install",sha256:$sha256,version:$version,
    candidate_revision:$candidate_revision,upstream_revision:$upstream_revision,
    recorded_at:$recorded_at}')"
printf '%s' "$binding_json" | /bin/sh custom/release/sha256-stdin
```

唯一 security-owner reviewer 必须在批准环境解密证明，确认上述断言、`production_update_performed=false`、加密文件哈希和规范绑定哈希。`recorded_at` 不得早于候选提交，不得位于未来，每个受保护发布作业校验时不得超过 24 小时。Environment 必须禁止管理员绕过、设置 `prevent_self_review=false` 并保留 required reviewer；单账号所有者可以自审，但必须实际执行 GitHub 的人工批准动作，不能让工作流自动代替。

两种模式最终都只公开 `production-deployment-evidence.json`。`existing_upgrade` 的固定范围为 `postgresql`、`redis`、`persistent_assets`，`existing_restore_proof` 为 `approved`，`ci_evidence` 为空；`first_install` 的固定范围为 `no_existing_modelport_production_dataset`、`no_postgresql_migration`、`no_redis_migration`、`no_persistent_assets_migration`、`no_deployment_state_migration`、`no_update_state_migration`，`existing_restore_proof` 为 `not_applicable`，`ci_evidence` 为 `clean_database_migration`、`release_image_smoke`。两者都记录 `production_update_performed=false` 和仓库外加密位置标识，不公开证明正文、路径、统计、密文或凭据。

### 两种模式共用的 VEX 门槛

同一 reviewer 还必须批准最终 OpenVEX 文档。工作流输入必须提供 `go_vex_sha256`、`go_vex_owner_id`、`go_vex_approved_at_utc` 和 `go_vex_expires_at_utc`，并分别与 Environment Secret 及文档内容一致。文档要求如下：

- `@context` 固定为 `https://openvex.dev/ns/v0.2.0`，`author` 固定为 `github-user-id:<numeric-owner-id>`，`role` 固定为 `Security Owner`，`timestamp` 等于批准时间。
- 恰好包含 `GO-2026-5158` / `go.opentelemetry.io/otel@v1.43.0`、`GO-2026-5932` / `golang.org/x/crypto@v0.55.0`、`GO-2026-6222` / `golang.org/x/image@v0.43.0` 三条 `not_affected` statement，理由为 `vulnerable_code_not_present`；新增、缺失或版本变化都会失败。
- 每条 statement 只允许一个 product，其 `@id` 必须是 `https://github.com/abingooo/modelport/commit/<40位候选提交>`；该 product 只允许一个 subcomponent，且必须是对应 finding 的精确 PURL。
- `approved_at` 不得早于候选提交或位于未来；`expires_at` 必须晚于批准时间，最长有效期 90 天，并且每个发布作业校验时至少还剩 2 小时，以覆盖最长 90 分钟写入作业及缓冲。

四个 VEX Secret 的含义分别是：完整 OpenVEX 文件的单行 Base64、该文件的 SHA-256、唯一 reviewer 的 GitHub numeric ID、下述规范绑定 JSON 的 SHA-256。绑定计算必须保持字段顺序和无末尾换行：

```sh
binding_json="$(jq -cn \
  --arg document_sha256 '<openvex-file-sha256>' \
  --arg version '0.1.183.1' \
  --arg candidate_revision '<40 lowercase hex>' \
  --arg upstream_revision 'e8cb019fabf8b55199436229044cbf9aa7a82564' \
  --argjson owner_id '<numeric-github-user-id>' \
  --arg approved_at '<UTC RFC3339 seconds>' \
  --arg expires_at '<UTC RFC3339 seconds>' \
  '{schema_version:1,document_sha256:$document_sha256,version:$version,
    candidate_revision:$candidate_revision,upstream_revision:$upstream_revision,
    product_id:("https://github.com/abingooo/modelport/commit/" + $candidate_revision),
    findings:[
      {advisory:"GO-2026-5158",component:"pkg:golang/go.opentelemetry.io%2Fotel@v1.43.0"},
      {advisory:"GO-2026-5932",component:"pkg:golang/golang.org%2Fx%2Fcrypto@v0.55.0"},
      {advisory:"GO-2026-6222",component:"pkg:golang/golang.org%2Fx%2Fimage@v0.43.0"}],
    owner_id:$owner_id,approved_at:$approved_at,expires_at:$expires_at}')"
printf '%s' "$binding_json" | /bin/sh custom/release/sha256-stdin
```

`MODELPORT_GO_VEX_DOCUMENT_BASE64` 只是 Environment 内的受保护传输形式。工作流会把解码后的完整文档以 `modelport-go-vex.openvex.json` 公开到 GitHub Release，并同时公开实时生成的 `modelport-go-module-inventory.json`；security owner 必须确保 `impact_statement`、`action_statement` 和其他字段不含凭据、内部路径、非公开拓扑、生产数据或其他敏感信息。

发布不会触碰生产。公开产物、digest、SBOM、provenance、Cosign 和站内版本发现验证完成后，Codex 必须停止：`existing_upgrade` 等待用户从站内手动更新，`first_install` 等待用户另行授权实际部署和目标冲突预检。

用户点击站内更新或另行开始首次部署前应再次确认：

- `existing_upgrade` 的备份仍足够新且校验和有效；`first_install` 已在此部署阶段独立完成目标冲突预检，且没有把该预检倒填为公开发布证据。
- 当前生产版本、目标版本和镜像 digest 匹配发布记录。
- 维护窗口、监控和回滚责任人就绪。
- PostgreSQL 迁移是前向操作；旧镜像回退不能撤销数据库迁移。需要数据库回滚时必须使用已验证备份并按独立维护流程恢复。

Codex 不点击更新、不创建更新请求、不重启服务、不切换流量，也不替用户执行回滚。
