# ModelPort Redis 隔离恢复验证

`modelport-isolated-redis-restore.sh` 只验证已经复制到本机的 Redis 持久化文件。它不会连接生产 Redis、不会下载备份、不会拉取镜像，也不会读取或输出任何 Redis key/value。工具只接受 RDB，或具有独立完整捕获来源证明的旧式单文件 AOF（`legacy-aof`）。

## 安全边界

- 输入必须是绝对路径下可读、非空的普通非符号链接文件；输入和报告必须位于仓库目录之外，报告目标必须尚不存在。
- 预期 SHA-256 必须来自独立备份清单，不能由本次验证临时生成后充当信任依据。
- `legacy-aof` 必须同时提供 `--confirm-complete-legacy-aof`。该参数是操作者对完整捕获来源的显式声明，不是工具从文件内容推断出的结论。
- 标准 Redis multipart AOF 的 manifest、`*.base.aof`、`*.base.rdb` 和 `*.incr.aof` 文件名会在 Docker 启动前被拒绝。重命名文件无法证明其完整性，因此仍必须依据目录级清单、停写复制或原子存储快照判断来源。
- 输入文件、报告目录和 Docker data root 必须位于已批准的加密存储中；`--confirm-sensitive-storage` 是操作者对此条件和容量的显式确认。工具不会检测底层加密状态或剩余容量。
- Redis 镜像必须已经预加载并使用 digest 固定引用。脚本所有容器创建命令都使用 `--pull=never`。
- `DOCKER_HOST` 必须未设置。脚本在第一次 daemon 操作前读取当前 Docker context，并且只接受指向现有本地 Unix socket 的 endpoint；TCP、SSH 和不存在的 socket 都会被拒绝。
- 检查器和初始化容器使用 `--network none`；实际 Redis 使用唯一的 `--internal` 网络，不发布端口。
- 所有容器使用只读 rootfs、`--cap-drop ALL` 和 `no-new-privileges`。输入仅被复制到随机临时 volume，源文件始终不修改；`volume-nocopy` 防止镜像自带的 `/data` 内容或所有权被复制进新 volume。
- 报告父目录会解析到物理路径，并通过当前工作目录及 device/inode 身份绑定；创建和发布报告前后都会重新验证，符号链接祖先被重定向不会改变发布位置。
- 敏感稳定副本和内部诊断只写入报告物理父目录内随机命名的隐藏 `0700` 工作目录，不使用 `/tmp`。
- 成功报告只会在随机容器、网络、volume 和本地工作目录全部删除后发布。任何清理失败都会使验证失败，并移除已经发布的成功报告。

生产备份包含会话及其他敏感状态。隔离主机不得连接生产网络，不得把报告提交到 Git、工单或公开日志；验证结束后仍应按备份保留策略处理输入和报告。

## RPO 时间

`--rpo-at` 是由备份操作独立记录的 UTC 持久化边界，不由文件修改时间推断：

- RDB：使用与源文件身份及独立校验和对应的 `LASTSAVE` 观测时间。
- `legacy-aof`：使用一致复制或原子存储快照的完成时间，即完整捕获窗口的结束时间。

报告同时记录源文件 mtime、验证开始/结束时间和验证完成时的 RPO age。mtime 仅用于取证，不能替代 `LASTSAVE`、一致快照证据或 AOF 完整捕获来源。

## 使用

RDB 是首选格式：

```sh
deploy/modelport-isolated-redis-restore.sh \
  --source-kind rdb \
  --input /secure/backups/redis/dump.rdb \
  --expected-sha256 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --expected-redis-major 8 \
  --rpo-at 2026-08-29T00:00:00Z \
  --report /secure/reports/redis-restore.json \
  --confirm-sensitive-storage
```

具有完整捕获来源证明的旧式单文件 AOF：

```sh
deploy/modelport-isolated-redis-restore.sh \
  --source-kind legacy-aof \
  --input /secure/backups/redis/appendonly.aof \
  --expected-sha256 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --expected-redis-major 8 \
  --rpo-at 2026-08-29T00:00:00Z \
  --report /secure/reports/redis-legacy-aof-restore.json \
  --confirm-complete-legacy-aof \
  --confirm-sensitive-storage
```

`--expected-redis-major` 是必填的独立恢复契约，必须来自备份/生产运行时清单。默认镜像与 ModelPort 发布工作流一致。若生产 Redis major 不同，应先预加载对应版本的 digest 固定镜像，并通过 `--redis-image repository@sha256:...` 指定。脚本不会接受 tag-only 引用。

## 验证内容

1. 在任何 Docker daemon 操作前校验本地文件的预期 SHA-256、RDB 格式签名和本地 Unix-socket Docker context。
2. 把输入复制到禁用镜像 copy-up 的随机 volume，并在容器内再次校验 SHA-256。
3. RDB 调用 `redis-check-rdb`。`legacy-aof` 的恢复基文件保持 `0444`，检查器仅处理单独的 `0600` 可写临时副本，调用 `redis-check-aof` 时不使用 `--fix`；检查完成后再次校验只读恢复基文件。
4. 对 RDB，从 `redis-check-rdb` 的 `Selecting DB ID` 输出计算需要的逻辑库数量，取“最大 DB ID + 1”和 16 中的较大值，再据此启动 Redis。
5. 实际启动隔离 Redis，验证 `PING`、`DBSIZE`、`INFO server`、`INFO persistence` 和 `INFO keyspace`，并要求 `INFO server` 返回的实际 major 与 `--expected-redis-major` 完全一致。
6. 再次确认隔离恢复基文件和本地输入都未发生变化。
7. 删除全部 Docker 资源及本地工作目录后，在绑定的物理父目录中以 `0600` 权限原子、不可覆盖地发布 JSON 报告。

报告包含 Redis/CLI 版本、实际 Redis server major、expected major、镜像引用和 ID、输入 digest/大小/mtime、legacy AOF 完整性确认、RPO 时间、配置的逻辑库数量、DB0 数量、各逻辑库的聚合 key 数量、检查器副本/只读基文件证据，以及选择性的持久化状态。Redis 在 AOF 关闭时未提供的 AOF size 字段记录为 `null`。报告不包含源路径、key 名、value、密码或连接信息。

## 限制

- Redis 7/8 的 multipart AOF 目录由 manifest、base 和 incremental 文件共同组成，不能用单文件 SHA-256 表示。本工具对此 fail closed；必须通过停写或原子存储快照取得完整目录，再使用专门的目录级清单与恢复流程。
- `--confirm-complete-legacy-aof` 只能记录操作者声明，不能从任意改名后的字节流证明来源或完整性。没有独立捕获证据时不得使用 `legacy-aof` 模式。
- RDB 的逻辑库数量会动态扩展；`legacy-aof` 使用 Redis 默认的 16 个逻辑库。引用更高 DB ID 的 legacy AOF 会验证失败，需使用经过评审的专用恢复流程。
- 过期 key 会在加载和探测期间按 Redis 正常语义消失，因此报告中的数量是验证时可见数量。
- 本工具不证明 PostgreSQL 与 Redis 处于同一时间点，也不验证应用业务语义。两者的 RPO 和跨存储一致性必须在生产备份报告中单独说明。
- 本工具不启动 ModelPort 应用，避免恢复出的生产凭据触发上游访问、后台任务或业务写入。

## 契约测试

默认契约测试使用 mock Docker，不需要 Docker daemon 或真实 Redis 镜像：

```sh
bash deploy/modelport-isolated-redis-restore-contract-test.sh
```

若本机已经预加载 digest 固定的 Redis 镜像，可额外运行真实镜像 drill：

```sh
MODELPORT_REDIS_DRILL_IMAGE='docker.m.daocloud.io/library/redis@sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576' \
  bash deploy/modelport-isolated-redis-restore-contract-test.sh
```

真实 drill 只生成合成数据：RDB 覆盖 DB0 和 DB20，legacy AOF 覆盖 DB0 和 DB2，并检查报告及标准输出/错误输出没有泄露合成 key/value。它不读取真实审计密文，也不修改任何加密、密钥或重加密行为。
