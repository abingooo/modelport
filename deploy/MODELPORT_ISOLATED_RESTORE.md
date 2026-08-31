# ModelPort PostgreSQL 隔离恢复验证

`modelport-isolated-restore.sh` 用于把已经复制到本机的 PostgreSQL 备份恢复到一次性 Docker 环境，并用当前 checkout 内嵌的迁移验证兼容性。它不会连接生产数据库、不会下载备份、不会拉取镜像，也不会启动 ModelPort 应用。

## 安全边界

- PostgreSQL 镜像必须已经预加载，并使用 `repository@sha256:...` 固定。所有 Docker 创建命令都使用 `--pull=never`。
- `DOCKER_HOST` 必须未设置。脚本在第一次 daemon 操作前读取当前 Docker context，并且只接受指向现有本地 Unix socket 的 endpoint；TCP、SSH 和不存在的 socket 都会被拒绝。
- PostgreSQL 容器只加入唯一的 `--internal` 网络，不发布主机端口；空间检查容器使用 `--network none`。
- PostgreSQL rootfs 只读，进程以 UID/GID `70:70` 运行，并使用 `--cap-drop ALL`、`no-new-privileges`、PID、内存和 CPU 限制。
- 数据只写入随机命名的临时 volume。源备份不挂载进容器，而是通过经过身份校验的只读文件描述符流式传入。
- 当前 checkout 的 `modelport-restore-migrate` 以 `CGO_ENABLED=0` 为镜像的 Linux 架构静态构建。生产模式还强制 `GOTOOLCHAIN=local`、`GOPROXY=off`、`GOSUMDB=off`、`GOWORK=off` 和 `-mod=readonly`，不会下载工具链或模块，也不会修改依赖文件。脚本记录主机侧 SHA-256，通过 stdin 复制到 UID 70 独占的容器私有 `/tmp` tmpfs，再在容器内校验相同 SHA-256 后执行。该 tmpfs 显式允许执行，但保留 `nosuid,nodev`，并随一次性容器删除。
- 迁移命令只接受 loopback 主机，以及共享同一 32 位随机十六进制后缀的 `modelport_restore_user_*` 临时用户和 `modelport_restore_*` 临时库。脚本没有 DSN、主机、端口、库名或凭据覆盖参数。
- 成功报告只在容器、网络和 volume 全部删除后发布。清理失败即整次验证失败；失败路径也会尝试精确名称清理并发布失败报告。
- 敏感稳定副本和内部诊断只写入报告物理父目录内随机命名的隐藏 `0700` 工作目录，不使用 `/tmp`；成功或失败报告发布前都会清理该目录。

生产备份可能包含用户资料、密钥密文、账务和审计记录。隔离 Docker 主机、输入目录和报告目录必须位于批准的加密存储上，且隔离主机不得连接生产网络。不要把输入、内部诊断目录或报告提交到 Git、公开日志或工单。

## 支持的来源与格式

`--source-kind` 必须是以下之一：

- `empty`：不读取备份，从空数据库验证当前迁移链；禁止 `--dump` 和 `--expected-sha256`。
- `upstream`：验证受支持的上游 ModelPort 基线指纹。
- `legacy-modelport`：验证 `v0.1.176.2` 归档迁移清单和账本后再升级。
- `production`：接受受支持的上游、legacy 或 current 指纹，并启用最严格的 revision、clean checkout、存储和路径门槛。

非空来源支持以下内容格式，格式由内容探测而不是文件名决定：

- plain SQL
- gzip 压缩的 plain SQL
- PostgreSQL custom-format archive
- gzip 压缩的 custom-format archive

plain SQL 使用 `psql --single-transaction`；custom archive 先通过 `pg_restore --list`，再使用 `pg_restore --single-transaction`。恢复过程不授予临时角色 superuser、createdb 或 createrole 权限。

## 前置条件

运行主机需要 Docker、Go、`jq`、OpenSSL、gzip 和标准 Unix 工具。还需要：

1. 预加载与生产 PostgreSQL major/架构相符的 digest 固定镜像。
2. 从独立备份清单取得预期 SHA-256；不要用本次运行临时计算的值充当生产信任依据。
3. 为生产验证固定 40 位 checkout revision，并保持工作树完全干净。
4. 确认 Docker data root、备份和报告目录均位于批准的加密存储，且有完整恢复所需容量。
5. 清除 `DOCKER_HOST`，并确认当前 Docker context 指向隔离主机上的现有本地 Unix socket。
6. 生产验证前预装所需 Go toolchain，并预热当前 revision 的模块缓存；生产构建完全离线且使用只读模块模式。

报告路径必须是绝对路径、父目录必须已经存在，而且目标文件必须不存在。工具不会覆盖既有报告。

## 使用

先做不含生产数据的空库演练：

```sh
/bin/bash deploy/modelport-isolated-restore.sh \
  --source-kind empty \
  --postgres-image 'postgres:18-alpine@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2' \
  --report /secure/reports/modelport-empty-restore.json
```

上游或 legacy 备份验证应提供独立 checksum：

```sh
/bin/bash deploy/modelport-isolated-restore.sh \
  --source-kind legacy-modelport \
  --dump /secure/backups/modelport/postgres.dump \
  --expected-sha256 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --postgres-image 'postgres:18-alpine@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2' \
  --report /secure/reports/modelport-legacy-restore.json
```

生产快照必须同时提供 checksum、revision、预期 PostgreSQL major 和敏感存储确认：

```sh
/bin/bash deploy/modelport-isolated-restore.sh \
  --source-kind production \
  --dump /secure/backups/modelport/postgres.dump \
  --expected-sha256 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --expected-revision 0123456789abcdef0123456789abcdef01234567 \
  --expected-postgres-major 18 \
  --postgres-image 'postgres:18-alpine@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2' \
  --confirm-production-storage \
  --report /secure/reports/modelport-production-restore.json
```

生产输入和报告的物理路径都必须位于仓库外。`--confirm-production-storage` 是操作者对批准的加密存储和容量作出的显式确认，不会自动探测加密状态。

## 验证内容

1. 在 Docker daemon 操作前验证参数、物理路径、文件身份、SHA-256、格式、checkout revision、工作树状态和本地 Unix-socket Docker context。
2. 创建随机内部网络、数据 volume、非特权 PostgreSQL 角色和 `modelport_restore_*` 临时库。
3. 流式恢复备份，采集迁移前结构与聚合证据，并识别 empty、upstream、legacy 或 current lineage。
4. 读取运行中服务的 PostgreSQL major；生产模式要求它与 `--expected-postgres-major` 完全一致，再静态构建、复制并校验当前 checkout 的迁移命令，通过容器内 `127.0.0.1:5432` 执行第一次迁移。
5. 验证迁移账本、232 至 236 bridge contract、表计数、设置迁移、财务聚合、密文字节聚合、序列位置和 catalog 对象。
6. 再次执行同一迁移命令，要求第二次运行后的 catalog 与数据证据保持稳定。
7. 再次验证 checkout 和源文件未变化，删除全部 Docker 资源，再以 `0600` 权限原子发布不可覆盖的 JSON 报告。

报告包含镜像引用/ID、实际 PostgreSQL server major、生产验证要求的 expected major、PostgreSQL 与工具版本、checkout commit/tree、迁移二进制 SHA-256、输入 digest/大小/格式、lineage、迁移次数和聚合不变量。报告不包含源路径、数据库凭据、DSN、设置值、设置值 hash、密文原文或逐行业务数据。

## 失败处理

所有运行时失败都会返回非零状态，并在报告准备完成后尽量发布 `result: "failed"` 和稳定的 `failure_code`。发布失败报告前会先清理本次随机名称和 label 对应的容器、网络及 volume。若第一次清理失败，退出处理会重试仍由本次运行拥有的精确资源；最终仍失败时必须人工隔离 Docker 主机并按报告/标准错误中的资源类型调查，不能把该次运行视为通过。

脚本不会覆盖报告，因此重试必须使用新的报告路径。不要在未确认残留资源状态前删除失败证据。

## 限制

- empty drill 只证明当前迁移链能在一次性 PostgreSQL 上从空库执行两次，不证明任何生产备份可恢复。
- 合成或上游 fixture 通过不等于生产恢复通过；生产门槛需要真实生产备份生成的独立 checksum 和本工具报告。
- 聚合不变量用于发现迁移破坏，不替代对关键业务记录和外部对象存储的抽查。
- 本工具不解密凭据或密文字段，也不验证加密密钥可用性。
- 本工具不验证 Redis、对象存储或 PostgreSQL 与其他存储的同一时间点一致性。
- 本工具不启动应用，因此不会验证登录、API 请求、后台任务或上游供应商连接。

## 契约与空库演练

mock contract 不需要 Docker daemon 或真实 PostgreSQL 镜像：

```sh
/bin/bash -n deploy/modelport-isolated-restore.sh
/bin/bash -n deploy/tests/modelport-isolated-restore-test.sh
shellcheck -x -S warning deploy/modelport-isolated-restore.sh
shellcheck -x -S warning deploy/tests/modelport-isolated-restore-test.sh
/bin/bash deploy/tests/modelport-isolated-restore-test.sh
(cd backend && go test ./cmd/modelport-restore-migrate)
```

真实 empty drill 需要 Docker daemon 和已经预加载的固定镜像，但不读取生产数据。运行后应检查报告 `result == "passed"`、`runtime.migration_runs == 2`、`runtime.isolation.host_port_published == false`、全部 invariant 为 true，并确认不存在本次 label 对应的容器、网络或 volume。

也可以用契约测试的真实分支完成相同检查；它只生成空数据库，不读取生产备份：

```sh
MODELPORT_POSTGRES_DRILL_IMAGE='postgres:18-alpine@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2' \
  /bin/bash deploy/tests/modelport-isolated-restore-test.sh
```
