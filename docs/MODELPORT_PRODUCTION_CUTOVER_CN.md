# TokensHub 切换 ModelPort 生产运行手册

## 1. 适用范围

本文档用于把 TokensHub 生产站首次切换到 ModelPort。目标版本必须是经过测试服验证的
`linux/amd64` 正式镜像，当前目标系列为 `0.1.168.1`。

本次只保证 ModelPort 核心业务：登录、现有用户和余额、API Key、分组、订阅、兑换码、
账号池、渠道定价、下游 OpenAI/Anthropic/Google 协议、用量记录和计费。TokensHub 的推荐、
抽奖、旧模型广场、首字超时控制页和自定义风控代理等周边能力保留数据但不上线，后续逐项
原生适配。

生产服务器在正式维护窗口前只允许读取检查。任何演练都必须使用本地快照或测试服务器。

## 2. 不可违反的边界

1. 旧 PostgreSQL、旧 Redis、旧应用目录和旧镜像完整保留，不在原卷上首次迁移。
2. 新 PostgreSQL、新 Redis 使用独立目录、Docker volume、容器名和网络。
3. 旧 TokensHub 与新 ModelPort worker 不得同时连接同一个可写数据库。
4. 先进入 Nginx 维护状态并停止所有写入，再制作最终快照。
5. 数据库首次升级只能运行 `/app/sub2api --migrate-only`，不能用普通应用启动代替。
6. Redis 先关闭 AOF 载入 RDB，裁剪后再开启 AOF。直接以 AOF 模式加载 RDB 会得到空库。
7. `JWT_SECRET` 与 `TOTP_ENCRYPTION_KEY` 必须从 TokensHub 原配置逐字节继承。前者关系到现有
   登录令牌和回滚一致性，后者关系到 TOTP、渠道监控、支付、备份及其他已加密配置。
8. 镜像使用固定正式标签和 digest，禁止 `latest`、`develop` 或可移动标签。
9. 流量切换前保持 `TOKEN_REFRESH_ENABLED=false`，避免验收阶段旋转上游 OAuth 凭据。
10. ModelPort 首次以 `TOKEN_REFRESH_ENABLED=true` 启动是凭据层提交点；此后回滚必须评估
    已轮换的上游凭据。接收首个用户写入后，禁止直接回旧快照。
11. 测试站必须使用独立 JWT/TOTP 密钥，不能为了消除告警把生产密钥放到公网测试域名；
    生产密钥的解密验收只能在不对公网开放的快照副本中执行。

## 3. 已验证的迁移事实

基于 2026-07-27 TokensHub 生产只读快照的最终演练结果：

- PostgreSQL 18 原始库有 236 条迁移、107 张表和 88 个序列。
- `--migrate-only` 将迁移数提升到 247，第二次执行无变化。
- 1494 个用户、余额合计 `6181.99823396`、1469 个 API Key、575 条订阅和
  1,077,234 条用量全部保留。
- 既有表双种子数据指纹、既有列、约束、索引、函数和触发器归一化指纹在迁移前后完全一致；
  `settings_id_seq` 因写入一条邀请奖励源码配置正常前移 1，其余 87 个既有序列完全一致。
- 目标只新增 ModelPort 预期字段、空表、邀请奖励源码配置和十一条迁移记录；1928 条历史奖励审核、
  688 条余额发放及 377 条注册 IP 记录的双种子指纹保持不变。
- TokensHub 原审核 API 的 `2026-07-06 06:00 +08:00` 历史发放边界随配置接管；边界前记录
  保持可见、可拒绝，但源码拒绝自动发放，避免迁移后误付历史待处理奖励。
- Redis 保留 9243 个登录会话相关键，删除 3398 个旧调度、计费、OAuth 和并发缓存；
  AOF 开启并重启后指纹不变。
- 同一正式候选镜像连接完整会话副本运行后，9243 个认证键的总数、类型和值指纹保持不变。
  测试站自身约 0.19 MB 的 Redis 数据与 TokensHub 约 3.35 MB 的 RDB 是两套数据，不能把
  两者的键数差异解释为应用删除会话。
- 使用 TokensHub 原始 `JWT_SECRET` 与 `TOTP_ENCRYPTION_KEY` 在隔离副本启动时无密钥告警；
  8 个渠道监控和 2 个支付实例全部可解密，备份 S3 当前未配置。

这些数字只证明快照演练。生产窗口必须对最终快照重新执行同一套检查，不能直接把旧数字
当作生产最终值。

## 4. 发布物准入

在任意生产操作前记录以下信息：

```bash
export MODELPORT_IMAGE=ghcr.io/abingooo/modelport:custom-v0.1.168.1
docker pull "$MODELPORT_IMAGE"
docker image inspect "$MODELPORT_IMAGE" \
  --format '{{.Id}} {{.Architecture}} {{index .RepoDigests 0}}'
docker run --rm "$MODELPORT_IMAGE" --version
```

必须同时满足：

- 架构为 `amd64`。
- 应用版本为 `0.1.168.1`。
- digest 与 GitHub Release、测试服务器记录一致。
- `custom/migration` 下的 SQL/Lua 文件与发布提交一致。
- GitHub Actions 后端、前端、AMD64 镜像构建全部通过。
- 绿色 Compose 显式继承旧站完整环境配置；至少对 `JWT_SECRET`、`TOTP_ENCRYPTION_KEY` 做
  长度与 SHA-256 对比，禁止把密钥明文写入证据日志。
- Compose 必须把 `TOKEN_REFRESH_ENABLED`、`SERVER_TRUSTED_PROXIES` 和
  `SECURITY_TRUST_FORWARDED_IP_FOR_API_KEY_ACL` 真正传入应用容器；只写入未引用的 `.env`
  不算配置成功。

创建仅 root 可读的证据目录，并从一开始记录所有输出与校验和：

```bash
umask 077
export CUTOVER_DIR="/root/modelport-cutover-$(date +%Y%m%d-%H%M%S)"
install -d -m 0700 "$CUTOVER_DIR"
```

## 5. 容量与运行环境检查

```bash
uname -m
docker version
docker compose version
df -hT
free -h
docker system df
```

准入要求：

- `uname -m` 为 `x86_64`。
- PostgreSQL 目标版本为 18，Redis 目标版本为 8。
- 剩余磁盘至少能同时容纳旧栈、最终数据库 dump、Redis RDB、新 PostgreSQL 数据卷和
  一份额外回滚备份，建议不少于当前数据库实际占用的三倍再加 10 GB。
- 新容器只监听 `127.0.0.1` 或内部 Docker 网络，Nginx 是唯一公网入口。
- 切换前验证域名证书、DNS 和 Nginx 配置，但不提前改变线上 upstream。
- `SERVER_TRUSTED_PROXIES` 只填写宿主 Nginx 进入应用容器时使用的 Docker 网关 `/32`；
  同时设置 `SECURITY_TRUST_FORWARDED_IP_FOR_API_KEY_ACL=false`。切流后确认访问日志中的
  `client_ip` 是真实客户端地址而不是 Docker 网关，否则 146 个已绑定 IP/UA 的 refresh
  token 可能在轮换时失效。
- 前后端同域部署时，CORS 空列表表示主动拒绝跨域请求，可以保留。若另有独立前端域名，
  只加入确切的 HTTPS origin，禁止使用 `*` 搭配凭据。
- TokensHub 快照包含多个自定义上游域名和一个 HTTP 上游。启用 URL allowlist 前必须从
  最终快照提取全部在用 host，并决定该 HTTP 上游是升级到 HTTPS、停用还是显式承担风险；
  不能直接套用默认白名单造成生产渠道中断。

## 6. 周边服务清单与停用策略

快照中发现以下 TokensHub systemd 单元。维护窗口内先记录状态，再停止并禁用；不要删除
文件和数据：

```text
sub2api-affiliate-rewards.service
sub2api-lottery-ui.service
tokenshub-first-token-timeout.service
tokenshub-model-square-admin.service
tokenshub-referral-admin.service
tokenshub-registration-ip-proxy.service
sub2api-risk-proxy.service（若已安装）
sub2api-datamanagementd.service（若已安装）
```

切换前还要执行以下发现命令，不能只依赖上面的快照清单：

```bash
systemctl list-units --type=service --all > "$CUTOVER_DIR/systemd-services.txt"
systemctl list-timers --all > "$CUTOVER_DIR/systemd-timers.txt"
docker ps --no-trunc > "$CUTOVER_DIR/docker-ps.txt"
nginx -T > "$CUTOVER_DIR/nginx-before.txt" 2>&1
```

`mihomo` 不是数据库 worker；若 ModelPort 上游代理仍依赖它，可以继续运行。独立 OAuth、
模型广场、推荐、抽奖和风控容器必须逐个确认是否连接主 PostgreSQL，连接者一律停止。
新 Nginx 配置不再暴露这些旧页面或脚本注入路由。

迁移后的绿色数据库执行：

```bash
psql -X -v ON_ERROR_STOP=1 "$MODELPORT_DATABASE_URL" \
  < custom/migration/fence-tokenshub-peripherals.sql
```

该脚本只禁用六个 TokensHub referral 自定义触发器，保留 schema、函数和全部历史数据；
ModelPort 认证缓存、订阅销售限制和账号约束等核心触发器保持启用。

## 7. 维护、排空与最终快照

1. 先把 Nginx 切到静态维护响应，阻止 API、登录、支付、兑换和管理写入。
2. 等待现有 HTTP 请求、SSE、WebSocket 和后台计费队列排空。
3. 在旧 Redis 检查 `SCARD billing:upq:dirty`，结果必须为 `0`。
4. 检查待处理支付、批量生图和 prompt audit 作业。存在任务时停止并人工处理，不能忽略。
5. 停止第 6 节的周边服务，再优雅停止 TokensHub 应用容器。
6. 确认 PostgreSQL 中不再有旧应用会话，只保留备份/管理连接。
7. 制作最终 PostgreSQL custom-format dump、Redis RDB、`/opt/sub2api/data`、Compose、`.env`、
   Nginx 和 systemd 配置备份。
8. 对每个文件执行 `sha256sum`，把清单写入证据目录并复制到服务器外的安全位置。

最终 PostgreSQL dump 必须在旧应用和周边 worker 全部停止后生成。Redis RDB 也必须来自同一
停机窗口，避免数据库与登录会话时间点不一致。

## 8. 蓝绿恢复与数据库迁移

1. 创建全新的 PostgreSQL 18 volume 和数据库，不复用旧卷。
2. 把最终 dump 恢复到绿色数据库，`pg_restore` 使用 `--exit-on-error`。
3. 在迁移前生成保留态审计：

```bash
psql -X -v ON_ERROR_STOP=1 "$MODELPORT_DATABASE_URL" \
  < custom/migration/audit-postgres.sql \
  > "$CUTOVER_DIR/postgres-before.tsv"
```

4. 只运行迁移命令；此时不要启动 ModelPort 服务。每次迁移显式给 600 秒超时，避免大库
   使用默认 60 秒导致不必要的中断：

```bash
docker compose run --rm --no-deps \
  -e SETUP_MIGRATION_TIMEOUT_SECONDS=600 modelport --migrate-only \
  > "$CUTOVER_DIR/migrate-first.log" 2>&1
docker compose run --rm --no-deps \
  -e SETUP_MIGRATION_TIMEOUT_SECONDS=600 modelport --migrate-only \
  > "$CUTOVER_DIR/migrate-second.log" 2>&1
```

5. 生成迁移后审计和目标断言：

```bash
psql -X -v ON_ERROR_STOP=1 "$MODELPORT_DATABASE_URL" \
  < custom/migration/audit-postgres.sql \
  > "$CUTOVER_DIR/postgres-after.tsv"
diff -u "$CUTOVER_DIR/postgres-before.tsv" "$CUTOVER_DIR/postgres-after.tsv"

psql -X -v ON_ERROR_STOP=1 "$MODELPORT_DATABASE_URL" \
  < custom/migration/verify-postgres-target.sql \
  > "$CUTOVER_DIR/postgres-target-verification.tsv"
```

准入要求：`diff` 无输出，目标验证显示 `PASS`，迁移数恰好为 247。任何无法解释的表计数、
指纹、序列、财务汇总或约束差异都必须停止切换。

## 9. Redis 会话迁移

需要保留的键前缀只有：

```text
refresh_token:
user_refresh_tokens:
token_family:
```

正确顺序：

1. 新 Redis 数据卷必须为空，先以 `appendonly no` 启动一次。
2. 放入最终 `dump.rdb` 并启动，确认 `DBSIZE` 与源快照一致。
3. 对源 RDB 记录文件大小、SHA-256，并运行 `audit-auth-redis.lua`，记录三个前缀数量、类型、
   JSON 有效性、绑定会话数、TTL 分桶、TTL 总量和指纹。禁止拿测试站原有 Redis 作为源基线。
4. 运行 `retain-auth-redis.lua`，删除其他缓存。
5. 再次运行审计；会话指纹、类型、TTL 分类必须与裁剪前一致，`persistent`、
   `invalid_types` 和 `invalid_refresh_json` 必须为 0。`refresh_tokens + user_sets +
   family_sets = total`，`bound_refresh_tokens + unbound_refresh_tokens = refresh_tokens`，
   `expiring = total`，五个 TTL 分桶之和也必须等于 `total`。
6. 第二次运行裁剪脚本，删除数必须为 0。
7. 执行 `CONFIG SET appendonly yes`，等待 `aof_last_bgrewrite_status:ok`。
8. 执行 `SAVE` 和 `SHUTDOWN SAVE`，再用正常 `appendonly yes` 配置启动目标 Redis。
9. 重启后再次审计，结果必须与重启前一致。

源快照中已有的 dangling set member 只记录基线，不在切换时擅自清理；只要求裁剪前后数量
不变。Redis 的 TTL 会随停机时间自然减少，这是预期行为；键数不变时，目标 `ttl_sum_ms`
应大致等于源值减去“认证键数 x 经过毫秒数”，不得无解释地增长或归零。

## 10. 首次启动围栏

在启动应用前执行：

```bash
psql -X -v ON_ERROR_STOP=1 "$MODELPORT_DATABASE_URL" \
  < custom/migration/enter-cutover-fence.sql
```

该脚本不禁用账号、不改余额、不改 API Key，只暂时关闭支付、SMTP、通知、监控、定时测试、
备份、内容风控和上游探测，并把原值保存到 `modelport_cutover_guard` schema。若有待处理支付
或后台作业，脚本会失败并回滚。

然后把旧站数据库中的 TokensHub 品牌和周边菜单切换到 ModelPort。`MODELPORT_PUBLIC_URL`
必须是已经通过 DNS、证书和 Nginx 验证的实际生产 HTTPS origin，不含路径；首次切换若继续
沿用旧域名，也必须填写用户当时实际访问的旧 origin，不能提前写入尚未可用的新域名。

```bash
export MODELPORT_PUBLIC_URL=https://modelport.link
psql -X -v ON_ERROR_STOP=1 \
  -v modelport_public_url="$MODELPORT_PUBLIC_URL" \
  "$MODELPORT_DATABASE_URL" \
  < custom/migration/apply-modelport-deployment-settings.sql
```

脚本会把原设置完整保存到 `modelport_cutover_guard.deployment_setting_state`，再设置内置
ModelPort 品牌、公开地址和充值回跳地址，并清空 TokensHub 小铺、生图、旧模型广场、抽奖等
自定义菜单。联系人、文档、支付、订阅和其他业务设置不变。验收输出必须显示 `PASS`。若在
接收用户流量前放弃绿色候选，应先停止绿色应用；仅当期间没有新增设置时，可在绿色库执行
`restore-source-deployment-settings.sql` 精确恢复原设置和 `settings_id_seq`。序列发生额外
变化时脚本会拒绝倒退，常规回滚仍应恢复从未改动的旧 TokensHub 栈。

首次启动 Compose 必须设置：

```text
TOKEN_REFRESH_ENABLED=false
SERVER_TRUSTED_PROXIES=<绿色 Docker 网络网关>/32
SECURITY_TRUST_FORWARDED_IP_FOR_API_KEY_ACL=false
```

无论服务器安装的是 Compose v2 还是 v1，都只选一个实际存在的命令并全程复用：

```bash
if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
else
  COMPOSE=(docker-compose)
fi
"${COMPOSE[@]}" config >/dev/null
```

应用只绑定回环地址，不接公网流量。完成以下只读或可回滚验证：

- `/health` 正常，版本为 ModelPort `0.1.168.1`；公开设置中的 `site_name` 为 `ModelPort`，
  `api_base_url` 与实际生产 origin 一致，`custom_menu_items` 为空。
- 管理员和普通用户登录、refresh-token 刷新正常。
- 用户数、余额、API Key、订阅、兑换码、账号、分组和价格读取正常。
- 管理员账号凭据、监控 API Key、支付配置和备份配置可解密。
- 旧 TokensHub 周边路由不可访问，六个 referral 触发器保持 disabled。
- Redis 只有目标实例的调度缓存，登录会话指纹仍匹配。

使用专门的迁移验收 Key 发起低成本真实请求，至少覆盖 OpenAI Chat 流式、Codex Responses
含 `prompt_cache_key`、Anthropic Messages 和 Google 协议。逐条核对首字节、用量日志、实际
成本、用户扣费和免费分组语义。

私网启动仍会运行订阅/账号到期、聚合和运维日志等本地 worker，已经到期的状态可能按当前
时间正常收敛，运维表也会新增记录。因此启动后的全库指纹不应再与离线迁移指纹硬比较；
必须单独核对用户数、余额、Key、订阅总数、用量、支付订单等核心不变量，并记录每一项合法
时效变化。需要最干净回滚时，应在普通应用首次启动前完成决定。

## 11. 恢复核心能力与切流

私网验收通过后，先恢复支付和基础通知的原设置：

```bash
psql -X -v ON_ERROR_STOP=1 "$MODELPORT_DATABASE_URL" \
  < custom/migration/restore-cutover-core.sql
```

渠道监控、定时测试、备份、内容风控、运维邮件、Ollama 用量和上游账单探测继续保持关闭；
它们的原值保存在 `modelport_cutover_guard`，后续逐项适配和验证后恢复。

提交切换：

1. 把 Compose 的 `TOKEN_REFRESH_ENABLED` 改为 `true` 并重建 ModelPort 应用容器。
2. 健康检查通过后，把 Nginx upstream 原子切换到 ModelPort 回环端口。
3. `nginx -t` 通过后 reload，不停止 Nginx 主进程。
4. 立即验证登录、API Key、一个付费请求和一个免费分组请求。
5. 观察 HTTP 5xx、上游 401/429、计费队列、Redis、PostgreSQL 连接数和磁盘增长。

切流后保留维护页配置，以便快速阻断新写入，但不要自动切回旧数据库。

## 12. 回滚边界

### ModelPort 普通启动前

停止并删除绿色容器即可恢复旧 TokensHub。旧数据库、Redis 和应用从未被修改，这是最干净
的回滚点。

### ModelPort 启动后、Token refresh 开启前

绿色数据库可能发生过到期状态或缓存维护，但旧栈仍可恢复。先停止绿色应用，再恢复旧
Nginx upstream 和旧服务。不要把绿色数据库覆盖回旧卷。

### Token refresh 开启后、用户流量进入前

上游 OAuth 凭据可能已经轮换。回滚前必须比较绿色数据库中的账号凭据，并把已轮换值安全
同步回旧数据库；无法确认时优先修复绿色栈，不直接启动旧 worker。

### 接收首个用户写入后

禁止直接恢复旧快照，否则会丢失新注册、扣费、兑换、支付、配置和凭据更新。应重新进入
维护状态，备份绿色数据库，选择向前修复；确需回旧版本时，必须先设计并验证绿色到旧库的
增量回迁。

## 13. 验收窗口与清理

- 至少保留旧栈、最终 dump、Redis RDB、环境配置和镜像 7 天，建议 14 天。
- 每个备份至少保留服务器内和服务器外两份，并验证 SHA-256。
- 验收期内不删除 `modelport_cutover_guard`、`lottery` 或 `referral` schema。
- 周边功能恢复时一次只启用一个，先测试服务器验证，再记录变更和回滚方式。
- 验收完成后再决定是否归档旧 TokensHub 容器和停用配置，绝不直接删除用户历史数据。
