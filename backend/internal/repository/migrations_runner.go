package repository

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
)

// schemaMigrationsTableDDL 定义迁移记录表的 DDL。
// 该表用于跟踪已应用的迁移文件及其校验和。
// - filename: 迁移文件名，作为主键唯一标识每个迁移
// - checksum: 文件内容的 SHA256 哈希值，用于检测迁移文件是否被篡改
// - applied_at: 迁移应用时间戳
const schemaMigrationsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	filename   TEXT PRIMARY KEY,
	checksum   TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

const atlasSchemaRevisionsTableDDL = `
CREATE TABLE IF NOT EXISTS atlas_schema_revisions (
	version TEXT PRIMARY KEY,
	description TEXT NOT NULL,
	type INTEGER NOT NULL,
	applied INTEGER NOT NULL DEFAULT 0,
	total INTEGER NOT NULL DEFAULT 0,
	executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	execution_time BIGINT NOT NULL DEFAULT 0,
	error TEXT NULL,
	error_stmt TEXT NULL,
	hash TEXT NOT NULL DEFAULT '',
	partial_hashes TEXT[] NULL,
	operator_version TEXT NULL
);
`

const publicBaseTablesQuery = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_type = 'BASE TABLE'
ORDER BY table_name
LIMIT 20
`

// migrationsAdvisoryLockID 是用于序列化迁移操作的 PostgreSQL Advisory Lock ID。
// 在多实例部署场景下，该锁确保同一时间只有一个实例执行迁移。
// 任何稳定的 int64 值都可以，只要不与同一数据库中的其他锁冲突即可。
const migrationsAdvisoryLockID int64 = 694208311321144027
const migrationsLockRetryInterval = 500 * time.Millisecond
const nonTransactionalMigrationSuffix = "_notx.sql"
const paymentOrdersOutTradeNoUniqueMigration = "120_enforce_payment_orders_out_trade_no_unique_notx.sql"
const paymentOrdersOutTradeNoUniqueIndex = "paymentorder_out_trade_no_unique"
const schedulerOutboxPendingDedupKeyMigration = "153_scheduler_outbox_pending_dedup_key_index_notx.sql"
const schedulerOutboxPendingDedupKeyIndex = "idx_scheduler_outbox_pending_dedup_key"
const latestAPIKeyIPIndexMigration = "174_add_usage_logs_api_key_latest_ip_index_notx.sql"
const latestAPIKeyIPIndex = "idx_usage_logs_api_key_latest_ip"
const legacyModelPortInstructionAuditIndexesMigration = "207_instruction_audit_v13_event_indexes_notx.sql"
const legacyModelPortOpenAICompatibleProvidersMigration = "188_add_openai_compatible_providers.sql"
const legacyModelPortChannelMonitorProvidersMigration = "197_channel_monitor_domestic_providers.sql"
const legacyModelPortMigrationManifest = "modelport_legacy/v0.1.176.2/manifest.tsv"
const legacyModelPortMigrationManifestHeader = "# filename\traw_sha256\trunner_trimmed_sha256"
const upstreamUserPlatformCNProvidersMigration = "224_user_platform_quotas_add_cn_providers.sql"
const upstreamChannelMonitorQuotaModeMigration = "226_channel_monitor_quota_mode.sql"
const upstreamCompositeRoutesCNProvidersMigration = "227_composite_routes_add_cn_providers.sql"
const modelPortLegacyPlatformConstraintsMigration = "236_modelport_legacy_platform_constraints.sql"

// The legacy bridge needs these relations and columns in order to inspect
// values and apply the terminal provider constraints. Keep this list explicit:
// a partially restored database must fail before any bridge or ledger write.
const legacyModelPortRequiredTablesQuery = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_name IN (
    'accounts', 'groups', 'settings', 'user_platform_quotas',
    'composite_model_routes', 'channel_monitors',
    'channel_monitor_request_templates', 'channel_monitor_histories'
  )
ORDER BY table_name
`

const legacyModelPortRequiredColumnsQuery = `
SELECT table_name, column_name, data_type,
       character_maximum_length, is_nullable
FROM information_schema.columns
WHERE table_schema = 'public'
  AND (
    (table_name = 'accounts' AND column_name IN ('id', 'platform', 'status', 'deleted_at'))
    OR (table_name = 'groups' AND column_name IN ('platform', 'status', 'deleted_at'))
    OR (table_name = 'settings' AND column_name IN ('key', 'value'))
    OR (table_name = 'user_platform_quotas' AND column_name IN (
      'platform', 'deleted_at', 'daily_limit_usd', 'weekly_limit_usd',
      'monthly_limit_usd', 'daily_usage_usd', 'weekly_usage_usd',
      'monthly_usage_usd'
    ))
    OR (table_name = 'composite_model_routes' AND column_name IN (
      'target_platform', 'deleted_at', 'enabled'
    ))
    OR (table_name = 'channel_monitors' AND column_name IN (
      'provider', 'check_mode', 'account_id'
    ))
    OR (table_name = 'channel_monitor_request_templates' AND column_name IN ('provider'))
    OR (table_name = 'channel_monitor_histories' AND column_name IN ('quota'))
  )
ORDER BY table_name, column_name
`

const legacyModelPortNamedConstraintsQuery = `
SELECT t.relname,
       c.conname,
       c.contype,
       c.convalidated,
       pg_get_constraintdef(c.oid),
       COALESCE(
         ARRAY(
           SELECT a.attname::TEXT
           FROM unnest(c.conkey) WITH ORDINALITY AS key(attnum, ordinal)
           JOIN pg_attribute a
             ON a.attrelid = c.conrelid
            AND a.attnum = key.attnum
           WHERE NOT a.attisdropped
           ORDER BY key.ordinal
         ),
         ARRAY[]::TEXT[]
       )
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = 'public'
  AND (
    (t.relname = 'user_platform_quotas' AND c.conname = 'user_platform_quotas_platform_check')
    OR (t.relname = 'composite_model_routes' AND c.conname = 'composite_model_routes_target_platform_check')
    OR (t.relname = 'channel_monitors' AND c.conname IN (
      'channel_monitors_provider_check', 'channel_monitors_check_mode_check'
    ))
    OR (t.relname = 'channel_monitor_request_templates' AND c.conname = 'channel_monitor_request_templates_provider_check')
  )
ORDER BY t.relname, c.conname
`

const legacyModelPortAccountForeignKeyQuery = `
SELECT c.conname, c.confdeltype, rt.relname, ra.attname
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_class rt ON rt.oid = c.confrelid
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(c.conkey)
JOIN pg_attribute ra ON ra.attrelid = c.confrelid AND ra.attnum = ANY(c.confkey)
WHERE n.nspname = 'public'
  AND t.relname = 'channel_monitors'
  AND a.attname = 'account_id'
  AND c.contype = 'f'
ORDER BY c.conname
`

const legacyModelPortAccountIndexQuery = `
SELECT EXISTS (
  SELECT 1
  FROM pg_indexes
  WHERE schemaname = 'public'
    AND tablename = 'channel_monitors'
    AND indexname = 'idx_channel_monitors_account_id'
)
`

const legacyModelPortQuotaSettingQuery = `
SELECT value
FROM settings
WHERE key = 'channel_monitor_show_quota'
`

var legacyModelPortInstructionAuditIndexes = []string{
	"idx_instruction_audit_events_outcome_created",
	"idx_instruction_audit_events_final_reason_created",
	"idx_instruction_audit_events_group_outcome_created",
	"idx_instruction_audit_events_pass_cleanup",
}

type legacyModelPortColumnRequirement struct {
	table      string
	column     string
	dataType   string
	maxLength  int64
	isNullable string
}

type legacyModelPortColumnInfo struct {
	dataType  string
	maxLength sql.NullInt64
	nullable  string
}

type legacyModelPortConstraintInfo struct {
	typeName      string
	validated     bool
	def           string
	targetColumns []string
}

type legacyModelPortForeignKeyInfo struct {
	name         string
	deleteAction string
	targetTable  string
	targetColumn string
}

// Existing legacy schemas differ in a few unrelated column widths, so the
// preflight checks existence for old columns and exact type/nullability for
// columns introduced by 226. These are the fields whose shape is part of the
// 226 runtime contract.
var legacyModelPortColumnRequirements = []legacyModelPortColumnRequirement{
	{table: "accounts", column: "id"},
	{table: "accounts", column: "platform"},
	{table: "accounts", column: "status"},
	{table: "accounts", column: "deleted_at"},
	{table: "groups", column: "platform"},
	{table: "groups", column: "status"},
	{table: "groups", column: "deleted_at"},
	{table: "settings", column: "key"},
	{table: "settings", column: "value"},
	{table: "user_platform_quotas", column: "platform"},
	{table: "user_platform_quotas", column: "deleted_at"},
	{table: "user_platform_quotas", column: "daily_limit_usd"},
	{table: "user_platform_quotas", column: "weekly_limit_usd"},
	{table: "user_platform_quotas", column: "monthly_limit_usd"},
	{table: "user_platform_quotas", column: "daily_usage_usd"},
	{table: "user_platform_quotas", column: "weekly_usage_usd"},
	{table: "user_platform_quotas", column: "monthly_usage_usd"},
	{table: "composite_model_routes", column: "target_platform"},
	{table: "composite_model_routes", column: "deleted_at"},
	{table: "composite_model_routes", column: "enabled"},
	{table: "channel_monitors", column: "provider"},
	{table: "channel_monitors", column: "check_mode", dataType: "character varying", maxLength: 32, isNullable: "NO"},
	{table: "channel_monitors", column: "account_id", dataType: "bigint", isNullable: "YES"},
	{table: "channel_monitor_request_templates", column: "provider"},
	{table: "channel_monitor_histories", column: "quota", dataType: "jsonb", isNullable: "YES"},
}

var (
	legacyModelPortUpstreamProviderPlatforms = stringSet(
		"anthropic", "openai", "gemini", "antigravity", "grok", "kimi", "zhipu", "deepseek",
	)
	legacyModelPortTerminalQuotaRoutePlatforms = stringSet(
		"anthropic", "openai", "gemini", "antigravity", "grok", "kimi", "zhipu", "deepseek",
		"qwen", "glm", "doubao", "siliconflow", "openrouter", "minimax", "mimo",
	)
	legacyModelPortTerminalMonitorPlatforms = stringSet(
		"openai", "anthropic", "gemini", "grok", "antigravity", "kimi", "zhipu", "deepseek",
		"qwen", "glm", "doubao", "minimax", "mimo",
	)
	legacyModelPortCheckModes = stringSet("probe", "quota", "quota_probe")
)

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

const usageLogsUpstreamModelMismatchIndexMigration = "195_add_usage_log_upstream_model_mismatch_index_notx.sql"
const usageLogsUpstreamModelMismatchIndex = "idx_usage_logs_upstream_model_mismatch_created_at"
const usageLogsEffectiveModelIndexesMigration = "226_add_usage_log_effective_model_indexes_notx.sql"
const usageLogsEffectiveRequestedModelIndex = "idx_usage_logs_effective_requested_model_created"
const usageLogsEffectiveUpstreamModelIndex = "idx_usage_logs_effective_upstream_model_created"

type migrationChecksumCompatibilityRule struct {
	fileChecksum       string
	acceptedDBChecksum map[string]struct{}
	acceptedChecksums  map[string]struct{}
}

// migrationChecksumCompatibilityRules 仅用于兼容历史上误修改过的迁移文件 checksum。
// 规则必须同时匹配「迁移名 + 数据库 checksum + 当前文件 checksum」且两者都落在该迁移的已知版本集合内才会放行，
// 避免放宽全局校验，也允许将误改的历史 migration 回滚为已发布版本而不要求人工修 checksum。
var migrationChecksumCompatibilityRules = map[string]migrationChecksumCompatibilityRule{
	"054_drop_legacy_cache_columns.sql": newMigrationChecksumCompatibilityRule("82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d", "182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4"),
	// 226's provider guard was hardened from a substring sentinel to an exact
	// upstream/terminal literal-set check. Existing databases may legitimately
	// carry the released checksum, so accept that one historical value only.
	"226_channel_monitor_quota_mode.sql": newMigrationChecksumCompatibilityRule(
		"ea9926655a2cf71a23b0f54597f7f57d59fca8d5fb1b5fe45c779acd0a57f784",
		"e4ffdcab2abada4070e6e0b951d944f7b024b8021681bc898fcf568015e28c3c",
		"c36c6c0ec6cc8727bb986e8cdc645990dcf8dad8f56a8c4647422e24e9dff88d",
	),
	"061_add_usage_log_request_type.sql":                      newMigrationChecksumCompatibilityRule("66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c", "08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0", "222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3"),
	"109_auth_identity_compat_backfill.sql":                   newMigrationChecksumCompatibilityRule("0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace", "551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee"),
	"110_pending_auth_and_provider_default_grants.sql":        newMigrationChecksumCompatibilityRule("32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279", "e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925"),
	"112_add_payment_order_provider_key_snapshot.sql":         newMigrationChecksumCompatibilityRule("b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99", "ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e"),
	"115_auth_identity_legacy_external_backfill.sql":          newMigrationChecksumCompatibilityRule("022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f", "4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f"),
	"116_auth_identity_legacy_external_safety_reports.sql":    newMigrationChecksumCompatibilityRule("07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488", "f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877"),
	"118_wechat_dual_mode_and_auth_source_defaults.sql":       newMigrationChecksumCompatibilityRule("b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0", "e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227", "a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb"),
	"119_enforce_payment_orders_out_trade_no_unique.sql":      newMigrationChecksumCompatibilityRule("0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e", "ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34"),
	"120_enforce_payment_orders_out_trade_no_unique_notx.sql": newMigrationChecksumCompatibilityRule("34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074", "e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61", "707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22", "04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a"),
	"123_fix_legacy_auth_source_grant_on_signup_defaults.sql": newMigrationChecksumCompatibilityRule("2ce43c2cd89e9f9e1febd34a407ed9e84d177386c5544b6f02c1f58a21129f57", "6cd33422f215dcd1f486ab6f35c0ea5805d9ca69bb25906d94bc649156657145"),
	"159_batch_image_foundation.sql":                          newMigrationChecksumCompatibilityRule("d902b70982025ec519749faf058aab7631e82c3f48167b9a4ae4db718eb72cce", "82da85b5d98e67a0507647b873a40373e84538e4adafdeed6767c0ac8b6570b2"),
	"161_batch_image_pricing_snapshot.sql":                    newMigrationChecksumCompatibilityRule("4012af3e43636cb6af22e0176d59d1fcc70615c0f310194329461ae462c4fbd6", "96d915c9b7a6941ae99039e0ff3f1a61481eb9bddd933d11c6fadb2274554e87"),
	// These four files were released with both raw-blob constants and correctly
	// trimmed runner checksums. Keep the union narrow and name-scoped so
	// upstream and custom-v0.1.176.2 databases can both upgrade safely.
	"195_channel_monitor_mode.sql": newMigrationChecksumCompatibilityRule(
		"73c39ac374c722253135041466108836845828a6065b499c60e7f27d6b92c21c",
		"f20366e106e3a54c73d4a67df3ba87734427ed859bc4ae42b0708e4cbcbacb56",
		"13f3792f3e3e53ee96e26415c884cf8062c77172824b54fcc9a8c0c2b1f185ec",
		"4c74fe33ef2274cc72e1bb49671e651274532c034b29f5b2982c2a4c88d101a6",
	),
	// 220 originally cleared video prices for all non-grok platforms (including composite);
	// composite is now preserved because it may route to Grok accounts.
	"220_clear_non_grok_video_generation_config.sql": newMigrationChecksumCompatibilityRule(
		"cf4dbfa75ac27d93a30a6a14439fe7dccfc911c043358363d5ec47946aa0e28b",
		"353c8e8e1805f2a6fd61311e03118e7dd8388f264cfd9af9e0cabe2a696388c4",
		"3d08d905a7bca1f56f14b6d2a2a0dcb07480ff52c21393b4e2db1b3a3f83b3d0",
		"85e320b9ec64f2d3fcd8cf705b2b4e76a7b49f7a57140c14bff97f32691c818b",
		"3da48c8fdffe6390325f43d08b8e353e0a365df43d44a78dbbe655d0deb18402",
	),
	"219_group_search_price_per_1k.sql": newMigrationChecksumCompatibilityRule(
		"430c2e3595342fe22c59e9676e9b18ea376f076324b77174a21e6f181f57f4b5",
		"833578274d0eed24d39355298d5659b33e5484c869b331ffd815187c221552d2",
		"e86786ebcc3b14206fd2d321380a4e50e80cdadbfcf4962c639255e6a14008db",
		"df6ffd71b97e30ec2c8fe7b95e15783042dea58c553e32701ee7c42a5619af80",
	),
	"218_group_audio_voice_pricing.sql": newMigrationChecksumCompatibilityRule(
		"a99ade7d0d464c67bf56814570050cc363ffad64eae2cb1e1ed760065f0b3585",
		"343a955e52348ce92c35753e78ca3f8e5a76060c20af71061ca5e04c6ed84085",
		"40ee9f3a2af0e0a5e99dabc878fd0fe98be1011f26bcfcefcac7197f7081f0e7",
		"c2a5e5b4ffd6968ad1c10593289fbc11192cdea19fec3ed9bce3a84eff9a8351",
	),
}

// ApplyMigrations 将嵌入的 SQL 迁移文件应用到指定的数据库。
//
// 该函数可以在每次应用启动时安全调用：
// - 已应用的迁移会被自动跳过（通过校验 filename 判断）
// - 如果迁移文件内容被修改（checksum 不匹配），会返回错误
// - 使用 PostgreSQL Advisory Lock 确保多实例并发安全
//
// 参数：
//   - ctx: 上下文，用于超时控制和取消
//   - db: 数据库连接
//
// 返回：
//   - error: 迁移过程中的任何错误
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	return applyMigrationsFS(ctx, db, migrations.FS)
}

// applyMigrationsFS 是迁移执行的核心实现。
// 它从指定的文件系统读取 SQL 迁移文件并按顺序应用。
//
// 迁移执行流程：
//  1. 获取 PostgreSQL Advisory Lock，防止多实例并发迁移
//  2. 确保 schema_migrations 表存在
//  3. 按文件名排序读取所有 .sql 文件
//  4. 对于每个迁移文件：
//     - 计算文件内容的 SHA256 校验和
//     - 检查该迁移是否已应用（通过 filename 查询）
//     - 如果已应用，验证校验和是否匹配
//     - 如果未应用，在事务中执行迁移并记录
//  5. 释放 Advisory Lock
//
// 参数：
//   - ctx: 上下文
//   - db: 数据库连接
//   - fsys: 包含迁移文件的文件系统（通常是 embed.FS）
func applyMigrationsFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	if db == nil {
		return errors.New("nil sql db")
	}

	// 获取分布式锁，确保多实例部署时只有一个实例执行迁移。
	// 这是 PostgreSQL 特有的 Advisory Lock 机制。
	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migrations lock connection: %w", err)
	}
	defer func() { _ = lockConn.Close() }()
	if err := pgAdvisoryLock(ctx, lockConn); err != nil {
		return err
	}
	defer func() {
		// 无论迁移是否成功，都要释放锁。
		// 独立超时确保原 ctx 取消后仍会尝试释放，但数据库链路异常不会
		// 无限阻塞进程退出。
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pgAdvisoryUnlock(unlockCtx, lockConn)
	}()

	// A legacy restore must be read-only until its marker, catalog, and value
	// preflight has completed. Inspect the ledger table's existence first (a
	// SELECT); creating schema_migrations or aligning the Atlas baseline before
	// this check would leave observable writes behind a failed preflight.
	hasBridgeMigration := hasModelPortBridgeMigration(fsys)
	hasSchemaMigrations := false
	if hasBridgeMigration {
		var err error
		hasSchemaMigrations, err = tableExists(ctx, lockConn, "schema_migrations")
		if err != nil {
			return fmt.Errorf("check schema_migrations: %w", err)
		}
		if !hasSchemaMigrations {
			tables, err := readPublicBaseTables(ctx, lockConn)
			if err != nil {
				return fmt.Errorf("classify database without schema_migrations: %w", err)
			}
			if len(tables) > 0 {
				return fmt.Errorf(
					"database has no public.schema_migrations but public schema is not empty (tables: %s); refusing to treat an ambiguous ledgerless restore as a clean database",
					strings.Join(tables, ", "),
				)
			}
		}
	}
	var legacyPlatformPlan *legacyModelPortPlatformBridgePlan
	if hasSchemaMigrations && hasBridgeMigration {
		legacyPlatformPlan, err = prepareLegacyModelPortMigrations(ctx, lockConn, fsys)
		if err != nil {
			return fmt.Errorf("prepare legacy ModelPort migrations: %w", err)
		}
	}

	// Create the migration ledger only after the legacy read-only preflight. A
	// clean database has no legacy markers and safely takes this normal path.
	if _, err := lockConn.ExecContext(ctx, schemaMigrationsTableDDL); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 自动对齐 Atlas 基线（如果检测到 legacy schema_migrations 且缺失 atlas_schema_revisions）。
	if err := ensureAtlasBaselineAligned(ctx, lockConn, fsys); err != nil {
		return err
	}

	// 获取所有 .sql 迁移文件并按文件名排序。
	// 命名规范：使用零填充数字前缀（如 001_init.sql, 002_add_users.sql）。
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files) // 确保按文件名顺序执行迁移

	for _, name := range files {
		// 读取迁移文件内容
		contentBytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			continue // 跳过空文件
		}

		// 计算文件内容的 SHA256 校验和，用于检测文件是否被修改。
		// 这是一种防篡改机制：如果有人修改了已应用的迁移文件，系统会拒绝启动。
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])

		// Legacy platform migrations are deferred when the final-state bridge is
		// needed.  The read-only planner has already validated their ledger rows
		// and file checksums; applying them here would narrow constraints before
		// the bridge can widen them.
		if legacyPlatformPlan.defers(name) {
			continue
		}

		// 检查该迁移是否已经应用
		var existing string
		rowErr := lockConn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename = $1", name).Scan(&existing)
		if rowErr == nil {
			// 迁移已应用，验证校验和是否匹配。legacy bridge 预检也
			// 复用同一函数，避免两条路径的历史兼容规则漂移。
			if err := validateAppliedMigrationChecksum(name, existing, checksum); err != nil {
				return err
			}
			continue // 迁移已应用且校验和匹配，跳过
		}
		if !errors.Is(rowErr, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", name, rowErr)
		}

		nonTx, err := validateMigrationExecutionMode(name, content)
		if err != nil {
			return fmt.Errorf("validate migration %s: %w", name, err)
		}

		if nonTx {
			if err := prepareNonTransactionalMigration(ctx, lockConn, name); err != nil {
				return fmt.Errorf("prepare migration %s: %w", name, err)
			}

			// *_notx.sql：用于 CREATE/DROP INDEX CONCURRENTLY 场景，必须非事务执行。
			// 逐条语句执行，避免将多条 CONCURRENTLY 语句放入同一个隐式事务块。
			statements := splitSQLStatements(content)
			for i, stmt := range statements {
				trimmed := strings.TrimSpace(stmt)
				if trimmed == "" {
					continue
				}
				if stripSQLLineComment(trimmed) == "" {
					continue
				}
				if _, err := lockConn.ExecContext(ctx, trimmed); err != nil {
					return fmt.Errorf("apply migration %s (non-tx statement %d): %w", name, i+1, err)
				}
			}
			if _, err := lockConn.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
				return fmt.Errorf("record migration %s (non-tx): %w", name, err)
			}
			continue
		}

		// 默认迁移在事务中执行，确保原子性：要么完全成功，要么完全回滚。
		tx, err := lockConn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}

		// 执行迁移 SQL
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		// 记录迁移已完成，保存文件名和校验和
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		// 提交事务
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	if err := applyLegacyModelPortPlatformBridge(ctx, lockConn, fsys, legacyPlatformPlan); err != nil {
		return fmt.Errorf("apply deferred legacy ModelPort platform migrations: %w", err)
	}

	return nil
}

type migrationConnection interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// legacyModelPortPlatformBridgePlan describes the legacy-only migration work
// that must be committed as one unit.  The planner performs only read-only
// checks; the runner applies the plan after all unrelated migrations have
// succeeded, so a later migration failure cannot leave the bridge committed.
type legacyModelPortPlatformBridgePlan struct {
	deferredMigrations  map[string]struct{}
	equivalentChecksums map[string]string
	bridgeContent       string
	bridgeChecksum      string
	appliedMigrations   map[string]bool
	validationOnly      bool
}

func (p *legacyModelPortPlatformBridgePlan) defers(name string) bool {
	if p == nil {
		return false
	}
	_, ok := p.deferredMigrations[name]
	return ok
}

type migrationQueryConnection interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func prepareNonTransactionalMigration(ctx context.Context, db migrationConnection, name string) error {
	switch name {
	case paymentOrdersOutTradeNoUniqueMigration:
		return preparePaymentOrdersOutTradeNoUniqueMigration(ctx, db)
	case schedulerOutboxPendingDedupKeyMigration:
		return dropInvalidIndexIfPresent(ctx, db, schedulerOutboxPendingDedupKeyIndex)
	case latestAPIKeyIPIndexMigration:
		return dropInvalidIndexIfPresent(ctx, db, latestAPIKeyIPIndex)
	case usageLogsUpstreamModelMismatchIndexMigration:
		return dropInvalidIndexIfPresent(ctx, db, usageLogsUpstreamModelMismatchIndex)
	case usageLogsEffectiveModelIndexesMigration:
		for _, indexName := range []string{usageLogsEffectiveRequestedModelIndex, usageLogsEffectiveUpstreamModelIndex} {
			if err := dropInvalidIndexIfPresent(ctx, db, indexName); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func hasModelPortBridgeMigration(fsys fs.FS) bool {
	_, err := fs.Stat(fsys, "232_modelport_free_group_bridge.sql")
	return err == nil
}

func prepareLegacyModelPortMigrations(
	ctx context.Context,
	db migrationConnection,
	fsys fs.FS,
) (*legacyModelPortPlatformBridgePlan, error) {
	// Platform preflight must run before any ModelPort-specific cleanup can
	// mutate the database. A bad upstream ledger must remain a read-only failure.
	plan, err := planLegacyModelPortPlatformConstraints(ctx, db, fsys)
	if err != nil {
		return nil, err
	}
	if plan != nil {
		if err := prevalidateLegacyModelPortPlatformSchema(ctx, db, plan); err != nil {
			return nil, err
		}
	}
	if err := prepareLegacyModelPortInstructionAuditIndexes(ctx, db); err != nil {
		return nil, err
	}
	return plan, nil
}

// prevalidateLegacyModelPortPlatformSchema checks the schema that the legacy
// bridge will read or leave in place.  It intentionally performs only SELECTs:
// a malformed restore must fail before any bridge DDL, migration-ledger write,
// or legacy index cleanup can occur.  Objects belonging to a pending migration
// may be absent (the migration can create them), but an object that is already
// present with the wrong shape is treated as drift and fails closed.
func prevalidateLegacyModelPortPlatformSchema(
	ctx context.Context,
	db migrationQueryConnection,
	plan *legacyModelPortPlatformBridgePlan,
) error {
	if plan == nil {
		return nil
	}

	applied := plan.appliedMigrations
	if applied == nil {
		applied = make(map[string]bool)
	}

	tables, err := readLegacyModelPortTables(ctx, db)
	if err != nil {
		return fmt.Errorf("legacy ModelPort schema prevalidation: read required tables: %w", err)
	}
	for _, table := range []string{
		"accounts",
		"groups",
		"settings",
		"user_platform_quotas",
		"composite_model_routes",
		"channel_monitors",
		"channel_monitor_request_templates",
		"channel_monitor_histories",
	} {
		if _, ok := tables[table]; !ok {
			return fmt.Errorf("legacy ModelPort schema prevalidation: required table public.%s is missing", table)
		}
	}

	columns, err := readLegacyModelPortColumns(ctx, db)
	if err != nil {
		return fmt.Errorf("legacy ModelPort schema prevalidation: read required columns: %w", err)
	}
	for _, requirement := range legacyModelPortColumnRequirements {
		key := legacyModelPortSchemaKey(requirement.table, requirement.column)
		info, ok := columns[key]
		if !ok {
			// 226 is the only migration that creates these three columns.  A
			// missing column is therefore valid while 226 is pending; once its
			// ledger is present, absence is schema drift.
			if requirement.table == "channel_monitors" &&
				(requirement.column == "check_mode" || requirement.column == "account_id") &&
				!applied[upstreamChannelMonitorQuotaModeMigration] {
				continue
			}
			if requirement.table == "channel_monitor_histories" &&
				requirement.column == "quota" &&
				!applied[upstreamChannelMonitorQuotaModeMigration] {
				continue
			}
			return fmt.Errorf("legacy ModelPort schema prevalidation: required column public.%s.%s is missing", requirement.table, requirement.column)
		}

		if requirement.dataType == "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(info.dataType), requirement.dataType) {
			return fmt.Errorf(
				"legacy ModelPort schema prevalidation: column public.%s.%s has type %q, want %q",
				requirement.table, requirement.column, info.dataType, requirement.dataType,
			)
		}
		if requirement.maxLength > 0 {
			if !info.maxLength.Valid || info.maxLength.Int64 != requirement.maxLength {
				return fmt.Errorf(
					"legacy ModelPort schema prevalidation: column public.%s.%s has character length %s, want %d",
					requirement.table,
					requirement.column,
					formatNullableInt64(info.maxLength),
					requirement.maxLength,
				)
			}
		}
		if requirement.isNullable != "" && !strings.EqualFold(strings.TrimSpace(info.nullable), requirement.isNullable) {
			return fmt.Errorf(
				"legacy ModelPort schema prevalidation: column public.%s.%s is_nullable=%q, want %q",
				requirement.table, requirement.column, info.nullable, requirement.isNullable,
			)
		}
	}

	constraints, err := readLegacyModelPortConstraints(ctx, db)
	if err != nil {
		return fmt.Errorf("legacy ModelPort schema prevalidation: read named constraints: %w", err)
	}

	// Provider CHECK constraints have different terminal sets: quotas/routes
	// retain the five storage-only gateways, while channel-monitor providers
	// retain only the five providers with monitor adapters.
	providerRequirements := []struct {
		table       string
		name        string
		target      string
		owner       string
		terminal    map[string]struct{}
		pendingSets []map[string]struct{}
	}{
		{
			table:    "user_platform_quotas",
			name:     "user_platform_quotas_platform_check",
			target:   "platform",
			owner:    upstreamUserPlatformCNProvidersMigration,
			terminal: legacyModelPortTerminalQuotaRoutePlatforms,
			pendingSets: []map[string]struct{}{
				stringSet("anthropic", "openai", "gemini", "antigravity", "grok"),
				legacyModelPortUpstreamProviderPlatforms,
				// custom-v0.1.176.2's 188 file predates zhipu but already
				// contains the other removed storage-only providers.
				stringSet("anthropic", "openai", "gemini", "antigravity", "grok", "deepseek", "qwen", "glm", "kimi", "doubao", "siliconflow", "openrouter", "minimax", "mimo"),
				legacyModelPortTerminalQuotaRoutePlatforms,
			},
		},
		{
			table:    "composite_model_routes",
			name:     "composite_model_routes_target_platform_check",
			target:   "target_platform",
			owner:    upstreamCompositeRoutesCNProvidersMigration,
			terminal: legacyModelPortTerminalQuotaRoutePlatforms,
			pendingSets: []map[string]struct{}{
				stringSet("anthropic", "openai", "gemini", "antigravity", "grok"),
				legacyModelPortUpstreamProviderPlatforms,
				stringSet("anthropic", "openai", "gemini", "antigravity", "grok", "deepseek", "qwen", "glm", "kimi", "doubao", "siliconflow", "openrouter", "minimax", "mimo"),
				legacyModelPortTerminalQuotaRoutePlatforms,
			},
		},
		{
			table:    "channel_monitors",
			name:     "channel_monitors_provider_check",
			target:   "provider",
			owner:    upstreamChannelMonitorQuotaModeMigration,
			terminal: legacyModelPortTerminalMonitorPlatforms,
			pendingSets: []map[string]struct{}{
				stringSet("openai", "anthropic", "gemini"),
				stringSet("openai", "anthropic", "gemini", "grok"),
				stringSet("openai", "anthropic", "gemini", "grok", "deepseek", "qwen", "glm", "kimi", "doubao", "minimax", "mimo"),
				legacyModelPortUpstreamProviderPlatforms,
				legacyModelPortTerminalMonitorPlatforms,
			},
		},
		{
			table:    "channel_monitor_request_templates",
			name:     "channel_monitor_request_templates_provider_check",
			target:   "provider",
			owner:    upstreamChannelMonitorQuotaModeMigration,
			terminal: legacyModelPortTerminalMonitorPlatforms,
			pendingSets: []map[string]struct{}{
				stringSet("openai", "anthropic", "gemini"),
				stringSet("openai", "anthropic", "gemini", "grok"),
				stringSet("openai", "anthropic", "gemini", "grok", "deepseek", "qwen", "glm", "kimi", "doubao", "minimax", "mimo"),
				legacyModelPortUpstreamProviderPlatforms,
				legacyModelPortTerminalMonitorPlatforms,
			},
		},
	}

	for _, requirement := range providerRequirements {
		key := legacyModelPortSchemaKey(requirement.table, requirement.name)
		info, ok := constraints[key]
		ownerApplied := applied[requirement.owner]
		bridgeApplied := applied[modelPortLegacyPlatformConstraintsMigration]
		if !ok {
			if ownerApplied || bridgeApplied {
				return fmt.Errorf(
					"legacy ModelPort schema prevalidation: applied migration %s requires constraint public.%s.%s",
					constraintOwnerName(ownerApplied, bridgeApplied, requirement.owner), requirement.table, requirement.name,
				)
			}
			// A pending owner migration (or a pending bridge) can create the
			// missing CHECK, so absence alone is not drift.
			continue
		}

		allowed := requirement.pendingSets
		if bridgeApplied {
			allowed = []map[string]struct{}{requirement.terminal}
		} else if ownerApplied {
			allowed = []map[string]struct{}{legacyModelPortUpstreamProviderPlatforms, requirement.terminal}
		}
		if err := validateLegacyModelPortCheckConstraint(info, requirement.target, allowed); err != nil {
			return fmt.Errorf("legacy ModelPort schema prevalidation: public.%s.%s: %w", requirement.table, requirement.name, err)
		}
	}

	// check_mode is introduced by 226 alongside account_id/quota.  If it is
	// already present, any named constraint must be exact even while 226 is
	// pending; ADD CONSTRAINT in 226 cannot repair an altered existing one.
	checkModeKey := legacyModelPortSchemaKey("channel_monitors", "channel_monitors_check_mode_check")
	if info, ok := constraints[checkModeKey]; ok {
		if err := validateLegacyModelPortCheckConstraint(info, "check_mode", []map[string]struct{}{legacyModelPortCheckModes}); err != nil {
			return fmt.Errorf("legacy ModelPort schema prevalidation: public.channel_monitors.channel_monitors_check_mode_check: %w", err)
		}
	} else if applied[upstreamChannelMonitorQuotaModeMigration] {
		return fmt.Errorf("legacy ModelPort schema prevalidation: applied migration %s requires constraint public.channel_monitors.channel_monitors_check_mode_check", upstreamChannelMonitorQuotaModeMigration)
	}

	// 226's account relation is part of its runtime contract.  Validate an
	// existing relation even when the ledger is pending (a partial restore must
	// not be mistaken for a clean starting point); absence is creatable only
	// while account_id itself is absent.
	foreignKeys, err := readLegacyModelPortAccountForeignKeys(ctx, db)
	if err != nil {
		return fmt.Errorf("legacy ModelPort schema prevalidation: read channel_monitors.account_id foreign key: %w", err)
	}
	accountIDPresent := hasLegacyModelPortColumn(columns, "channel_monitors", "account_id")
	if len(foreignKeys) > 1 {
		return fmt.Errorf("legacy ModelPort schema prevalidation: channel_monitors.account_id has %d foreign keys; want exactly one to accounts(id) ON DELETE SET NULL", len(foreignKeys))
	}
	if len(foreignKeys) == 1 {
		fk := foreignKeys[0]
		if fk.targetTable != "accounts" || fk.targetColumn != "id" || fk.deleteAction != "n" {
			return fmt.Errorf("legacy ModelPort schema prevalidation: foreign key %s on channel_monitors.account_id targets %s(%s) with delete action %s; want accounts(id) ON DELETE SET NULL", fk.name, fk.targetTable, fk.targetColumn, fk.deleteAction)
		}
	} else if applied[upstreamChannelMonitorQuotaModeMigration] || accountIDPresent {
		return fmt.Errorf("legacy ModelPort schema prevalidation: channel_monitors.account_id is missing its accounts(id) ON DELETE SET NULL foreign key")
	}

	accountIndex, err := readLegacyModelPortAccountIndex(ctx, db)
	if err != nil {
		return fmt.Errorf("legacy ModelPort schema prevalidation: read channel_monitors.account_id index: %w", err)
	}
	if applied[upstreamChannelMonitorQuotaModeMigration] && !accountIndex {
		return fmt.Errorf("legacy ModelPort schema prevalidation: applied migration %s requires index public.idx_channel_monitors_account_id", upstreamChannelMonitorQuotaModeMigration)
	}

	settingPresent, err := readLegacyModelPortQuotaSetting(ctx, db)
	if err != nil {
		return fmt.Errorf("legacy ModelPort schema prevalidation: read settings.channel_monitor_show_quota: %w", err)
	}
	if applied[upstreamChannelMonitorQuotaModeMigration] && !settingPresent {
		return fmt.Errorf("legacy ModelPort schema prevalidation: applied migration %s requires settings.channel_monitor_show_quota", upstreamChannelMonitorQuotaModeMigration)
	}

	return nil
}

func legacyModelPortSchemaKey(parent, name string) string {
	return parent + "." + name
}

func formatNullableInt64(value sql.NullInt64) string {
	if !value.Valid {
		return "NULL"
	}
	return fmt.Sprintf("%d", value.Int64)
}

func hasLegacyModelPortColumn(columns map[string]legacyModelPortColumnInfo, table, column string) bool {
	_, ok := columns[legacyModelPortSchemaKey(table, column)]
	return ok
}

func constraintOwnerName(ownerApplied, bridgeApplied bool, owner string) string {
	if bridgeApplied {
		return modelPortLegacyPlatformConstraintsMigration
	}
	if ownerApplied {
		return owner
	}
	return owner
}

func readLegacyModelPortTables(ctx context.Context, db migrationQueryConnection) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, legacyModelPortRequiredTablesQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	tables := make(map[string]struct{})
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables[table] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func readLegacyModelPortColumns(ctx context.Context, db migrationQueryConnection) (map[string]legacyModelPortColumnInfo, error) {
	rows, err := db.QueryContext(ctx, legacyModelPortRequiredColumnsQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns := make(map[string]legacyModelPortColumnInfo)
	for rows.Next() {
		var table, column, dataType, nullable string
		var maxLength sql.NullInt64
		if err := rows.Scan(&table, &column, &dataType, &maxLength, &nullable); err != nil {
			return nil, err
		}
		columns[legacyModelPortSchemaKey(table, column)] = legacyModelPortColumnInfo{
			dataType:  dataType,
			maxLength: maxLength,
			nullable:  nullable,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func readLegacyModelPortConstraints(ctx context.Context, db migrationQueryConnection) (map[string]legacyModelPortConstraintInfo, error) {
	rows, err := db.QueryContext(ctx, legacyModelPortNamedConstraintsQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	constraints := make(map[string]legacyModelPortConstraintInfo)
	for rows.Next() {
		var table, name, typeName, def string
		var validated bool
		var targetColumns pq.StringArray
		if err := rows.Scan(&table, &name, &typeName, &validated, &def, &targetColumns); err != nil {
			return nil, err
		}
		key := legacyModelPortSchemaKey(table, name)
		if _, duplicate := constraints[key]; duplicate {
			return nil, fmt.Errorf("duplicate catalog row for constraint public.%s.%s", table, name)
		}
		constraints[key] = legacyModelPortConstraintInfo{
			typeName:      typeName,
			validated:     validated,
			def:           def,
			targetColumns: append([]string(nil), targetColumns...),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return constraints, nil
}

func readLegacyModelPortAccountForeignKeys(ctx context.Context, db migrationQueryConnection) ([]legacyModelPortForeignKeyInfo, error) {
	rows, err := db.QueryContext(ctx, legacyModelPortAccountForeignKeyQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	foreignKeys := make([]legacyModelPortForeignKeyInfo, 0, 1)
	for rows.Next() {
		var fk legacyModelPortForeignKeyInfo
		if err := rows.Scan(&fk.name, &fk.deleteAction, &fk.targetTable, &fk.targetColumn); err != nil {
			return nil, err
		}
		foreignKeys = append(foreignKeys, fk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return foreignKeys, nil
}

func readLegacyModelPortAccountIndex(ctx context.Context, db migrationQueryConnection) (bool, error) {
	var present bool
	if err := db.QueryRowContext(ctx, legacyModelPortAccountIndexQuery).Scan(&present); err != nil {
		return false, err
	}
	return present, nil
}

func readLegacyModelPortQuotaSetting(ctx context.Context, db migrationQueryConnection) (bool, error) {
	var value string
	err := db.QueryRowContext(ctx, legacyModelPortQuotaSettingQuery).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func validateLegacyModelPortCheckConstraint(
	info legacyModelPortConstraintInfo,
	target string,
	allowed []map[string]struct{},
) error {
	if !strings.EqualFold(strings.TrimSpace(info.typeName), "c") {
		return fmt.Errorf("constraint type is %q, want CHECK", info.typeName)
	}
	if !info.validated {
		return errors.New("CHECK constraint is NOT VALID")
	}
	if len(info.targetColumns) != 1 || info.targetColumns[0] != target {
		return fmt.Errorf(
			"CHECK constraint columns are %v, want exactly [%s]",
			info.targetColumns,
			target,
		)
	}
	actual, err := parseLegacyModelPortConstraintLiterals(info.def, target)
	if err != nil {
		return err
	}
	for _, expected := range allowed {
		if legacyModelPortStringSetEqual(actual, expected) {
			return nil
		}
	}
	return fmt.Errorf("CHECK literal set %s is not one of the accepted exact sets", formatLegacyModelPortStringSet(actual))
}

// parseLegacyModelPortConstraintLiterals extracts SQL string literals from a
// pg_get_constraintdef result and verifies that the expected column participates
// in an IN/ANY check.  PostgreSQL renders CHECKs as either IN lists or = ANY
// (ARRAY[...]); both forms are accepted, while arbitrary expressions and
// partial/extra literal sets are rejected by the exact-set comparison caller.
func parseLegacyModelPortConstraintLiterals(def, target string) (map[string]struct{}, error) {
	masked, err := maskLegacyModelPortSQLNonCode(def)
	if err != nil {
		return nil, err
	}
	if !containsSQLIdentifierWithMask(def, masked, target) {
		return nil, fmt.Errorf("CHECK does not reference column %s", target)
	}
	if !containsSQLIdentifierToken(masked, "in") && !containsSQLIdentifierToken(masked, "any") {
		return nil, errors.New("CHECK is not an IN/ANY membership constraint")
	}
	hasMembership, err := legacyModelPortConstraintTargetHasMembership(def, target)
	if err != nil {
		return nil, err
	}
	if !hasMembership {
		return nil, fmt.Errorf("CHECK target column %s is not part of an IN/ANY membership", target)
	}
	if err := validateLegacyModelPortMembershipShape(def, target); err != nil {
		return nil, err
	}

	literals, err := scanLegacyModelPortSQLStringLiterals(def)
	if err != nil {
		return nil, err
	}
	if len(literals) == 0 {
		return nil, errors.New("CHECK contains no SQL string literals")
	}
	set := make(map[string]struct{}, len(literals))
	for _, literal := range literals {
		if _, duplicate := set[literal]; duplicate {
			return nil, fmt.Errorf("CHECK contains duplicate literal %q", literal)
		}
		set[literal] = struct{}{}
	}
	return set, nil
}

// validateLegacyModelPortMembershipShape rejects a CHECK that merely embeds
// the expected membership inside a different boolean expression. Catalog
// column binding catches additional column references; this shape check also
// catches column-free changes such as NOT, OR TRUE, or comparison with FALSE.
// The accepted token vocabulary is intentionally limited to the forms emitted
// by pg_get_constraintdef for varchar/text IN and = ANY (ARRAY[...]) checks.
func validateLegacyModelPortMembershipShape(def, target string) error {
	tokens, err := scanLegacyModelPortSQLTokens(def)
	if err != nil {
		return err
	}
	if len(tokens) < 4 || !legacyModelPortSQLTokenIsKeyword(tokens[0], "check") ||
		tokens[1].value != "(" || tokens[len(tokens)-1].value != ")" {
		return errors.New("CHECK is not a simple positive IN/ANY membership constraint")
	}

	allowedIdentifiers := map[string]struct{}{
		"check": {}, "in": {}, "any": {}, "array": {},
		"text": {}, "character": {}, "varying": {}, "varchar": {},
		"bpchar": {}, "pg_catalog": {},
	}
	allowedPunctuation := map[string]struct{}{
		"(": {}, ")": {}, "[": {}, "]": {}, ",": {}, ".": {},
		"::": {}, "=": {},
	}

	targetCount := 0
	inCount := 0
	anyCount := 0
	arrayCount := 0
	equalsCount := 0
	parenDepth := 0
	bracketDepth := 0
	for _, token := range tokens {
		if token.kind == legacyModelPortSQLIdentifierToken {
			if legacyModelPortSQLTokenMatchesTarget(token, target) {
				targetCount++
				continue
			}
			if token.quoted {
				return fmt.Errorf("CHECK contains unexpected quoted identifier %q", token.value)
			}
			keyword := strings.ToLower(token.value)
			if _, ok := allowedIdentifiers[keyword]; !ok {
				return fmt.Errorf("CHECK contains unsupported identifier or operator %q", token.value)
			}
			switch keyword {
			case "in":
				inCount++
			case "any":
				anyCount++
			case "array":
				arrayCount++
			}
			continue
		}

		if _, ok := allowedPunctuation[token.value]; !ok {
			return fmt.Errorf("CHECK contains unsupported token %q", token.value)
		}
		switch token.value {
		case "(":
			parenDepth++
		case ")":
			parenDepth--
			if parenDepth < 0 {
				return errors.New("CHECK contains unbalanced parentheses")
			}
		case "[":
			bracketDepth++
		case "]":
			bracketDepth--
			if bracketDepth < 0 {
				return errors.New("CHECK contains unbalanced array brackets")
			}
		case "=":
			equalsCount++
		}
	}
	if parenDepth != 0 || bracketDepth != 0 {
		return errors.New("CHECK contains unbalanced membership syntax")
	}
	if targetCount != 1 {
		return fmt.Errorf("CHECK references target column %s %d times, want exactly once", target, targetCount)
	}
	if inCount == 1 && anyCount == 0 && arrayCount == 0 && equalsCount == 0 {
		return nil
	}
	if inCount == 0 && anyCount == 1 && arrayCount == 1 && equalsCount == 1 {
		return nil
	}
	return errors.New("CHECK is not a single positive IN/ANY membership constraint")
}

func legacyModelPortSQLTokenIsKeyword(token legacyModelPortSQLToken, keyword string) bool {
	return token.kind == legacyModelPortSQLIdentifierToken && !token.quoted &&
		strings.EqualFold(token.value, keyword)
}

func containsSQLIdentifier(expression, identifier string) bool {
	masked, err := maskLegacyModelPortSQLNonCode(expression)
	if err != nil {
		return false
	}
	return containsSQLIdentifierWithMask(expression, masked, identifier)
}

func containsSQLIdentifierWithMask(expression, masked, identifier string) bool {
	if identifier == "" {
		return false
	}
	if containsSQLIdentifierToken(masked, identifier) {
		return true
	}
	return containsLegacyModelPortQuotedIdentifier(expression, identifier)
}

func containsSQLIdentifierToken(expression, identifier string) bool {
	expression = strings.ToLower(expression)
	identifier = strings.ToLower(identifier)
	if identifier == "" {
		return false
	}
	for start := 0; ; {
		idx := strings.Index(expression[start:], identifier)
		if idx < 0 {
			return false
		}
		idx += start
		beforeOK := idx == 0 || !isSQLIdentifierPart(expression[idx-1])
		after := idx + len(identifier)
		afterOK := after >= len(expression) || !isSQLIdentifierPart(expression[after])
		if beforeOK && afterOK {
			return true
		}
		start = after
		if start >= len(expression) {
			return false
		}
	}
}

func isSQLIdentifierPart(value byte) bool {
	return (value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= '0' && value <= '9') || value == '_'
}

func scanLegacyModelPortSQLStringLiterals(expression string) ([]string, error) {
	literals := make([]string, 0)
	for i := 0; i < len(expression); {
		switch {
		case expression[i] == '-' && i+1 < len(expression) && expression[i+1] == '-':
			i = skipLegacyModelPortSQLLineComment(expression, i)
		case expression[i] == '/' && i+1 < len(expression) && expression[i+1] == '*':
			var err error
			i, err = skipLegacyModelPortSQLBlockComment(expression, i)
			if err != nil {
				return nil, err
			}
		case expression[i] == '"':
			var err error
			i, err = skipLegacyModelPortSQLDoubleQuotedIdentifier(expression, i)
			if err != nil {
				return nil, err
			}
		case expression[i] == '$':
			if end, ok, err := skipLegacyModelPortSQLDollarQuoted(expression, i); ok {
				if err != nil {
					return nil, err
				}
				i = end
			} else {
				i++
			}
		case expression[i] == '\'':
			literal, end, err := parseLegacyModelPortSQLStringLiteral(expression, i)
			if err != nil {
				return nil, err
			}
			literals = append(literals, literal)
			i = end
		default:
			i++
		}
	}
	return literals, nil
}

type legacyModelPortSQLToken struct {
	kind   byte
	value  string
	quoted bool
}

const legacyModelPortSQLIdentifierToken byte = 'i'

// legacyModelPortConstraintTargetHasMembership binds the target column to the
// membership operator itself. A global "column exists" plus "IN exists"
// check is insufficient: CHECK (platform = 1 AND other IN (...)) must not be
// accepted as the platform provider constraint.
func legacyModelPortConstraintTargetHasMembership(expression, target string) (bool, error) {
	tokens, err := scanLegacyModelPortSQLTokens(expression)
	if err != nil {
		return false, err
	}
	for index, token := range tokens {
		if token.kind != legacyModelPortSQLIdentifierToken || !legacyModelPortSQLTokenMatchesTarget(token, target) {
			continue
		}
		if legacyModelPortSQLTargetMembershipAt(tokens, index) {
			return true, nil
		}
	}
	return false, nil
}

func legacyModelPortSQLTokenMatchesTarget(token legacyModelPortSQLToken, target string) bool {
	if token.quoted {
		// PostgreSQL preserves case inside quoted identifiers.
		return token.value == target
	}
	return strings.EqualFold(token.value, target)
}

func legacyModelPortSQLTargetMembershipAt(tokens []legacyModelPortSQLToken, targetIndex int) bool {
	// A target wrapped in parentheses is valid, but a function call such as
	// coalesce(other, platform) IN (...) is not a direct provider-column check.
	opening := 0
	for index := targetIndex - 1; index >= 0 && tokens[index].value == "("; index-- {
		opening++
	}
	if opening > 0 {
		beforeOpening := targetIndex - opening - 1
		if beforeOpening >= 0 && tokens[beforeOpening].kind == legacyModelPortSQLIdentifierToken &&
			!isLegacyModelPortSQLControlKeyword(tokens[beforeOpening].value) {
			return false
		}
	}
	if targetIndex > 0 && (tokens[targetIndex-1].value == "," || tokens[targetIndex-1].value == ".") {
		// A comma denotes a function/array argument; a dot here means this is
		// the second half of a qualified name and is handled only when the
		// target token itself is the column after the qualifier.
		if tokens[targetIndex-1].value == "," {
			return false
		}
	}

	index := targetIndex + 1
	closing := 0
	for index < len(tokens) {
		if tokens[index].value == ")" {
			closing++
			if closing > opening {
				return false
			}
			index++
			continue
		}
		if tokens[index].value == "::" {
			next, ok := consumeLegacyModelPortSQLCast(tokens, index)
			if !ok {
				return false
			}
			index = next
			continue
		}
		break
	}
	if index >= len(tokens) {
		return false
	}
	if tokens[index].kind == legacyModelPortSQLIdentifierToken && !tokens[index].quoted && strings.EqualFold(tokens[index].value, "in") {
		return true
	}
	return tokens[index].value == "=" && index+1 < len(tokens) &&
		tokens[index+1].kind == legacyModelPortSQLIdentifierToken &&
		!tokens[index+1].quoted && strings.EqualFold(tokens[index+1].value, "any")
}

func isLegacyModelPortSQLControlKeyword(value string) bool {
	switch strings.ToLower(value) {
	case "check", "not", "and", "or", "is", "null", "true", "false",
		"case", "when", "then", "else", "end", "between", "like", "similar",
		"escape", "collate", "at", "local", "isnull", "notnull":
		return true
	default:
		return false
	}
}

func consumeLegacyModelPortSQLCast(tokens []legacyModelPortSQLToken, start int) (int, bool) {
	if start >= len(tokens) || tokens[start].value != "::" {
		return start, false
	}
	index := start + 1
	if index >= len(tokens) || tokens[index].kind != legacyModelPortSQLIdentifierToken {
		return start, false
	}
	index++
	// Schema-qualified and array type casts are both emitted by PostgreSQL.
	for index+1 < len(tokens) && tokens[index].value == "." && tokens[index+1].kind == legacyModelPortSQLIdentifierToken {
		index += 2
	}
	for index+1 < len(tokens) && tokens[index].value == "[" && tokens[index+1].value == "]" {
		index += 2
	}
	// Type modifiers such as varchar(32) are optional; consume a balanced
	// modifier only when it immediately follows the type name.
	if index < len(tokens) && tokens[index].value == "(" {
		depth := 1
		index++
		for index < len(tokens) && depth > 0 {
			switch tokens[index].value {
			case "(":
				depth++
			case ")":
				depth--
			}
			index++
		}
		if depth != 0 {
			return start, false
		}
	}
	return index, true
}

func scanLegacyModelPortSQLTokens(expression string) ([]legacyModelPortSQLToken, error) {
	tokens := make([]legacyModelPortSQLToken, 0, 16)
	for index := 0; index < len(expression); {
		if expression[index] == ' ' || expression[index] == '\t' || expression[index] == '\n' || expression[index] == '\r' {
			index++
			continue
		}
		if expression[index] == '-' && index+1 < len(expression) && expression[index+1] == '-' {
			index = skipLegacyModelPortSQLLineComment(expression, index)
			continue
		}
		if expression[index] == '/' && index+1 < len(expression) && expression[index+1] == '*' {
			var err error
			index, err = skipLegacyModelPortSQLBlockComment(expression, index)
			if err != nil {
				return nil, err
			}
			continue
		}
		if expression[index] == '\'' {
			_, next, err := parseLegacyModelPortSQLStringLiteral(expression, index)
			if err != nil {
				return nil, err
			}
			index = next
			continue
		}
		if expression[index] == '"' {
			value, next, err := parseLegacyModelPortSQLDoubleQuotedIdentifier(expression, index)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, legacyModelPortSQLToken{kind: legacyModelPortSQLIdentifierToken, value: value, quoted: true})
			index = next
			continue
		}
		if expression[index] == '$' {
			if next, ok, err := skipLegacyModelPortSQLDollarQuoted(expression, index); ok {
				if err != nil {
					return nil, err
				}
				index = next
				continue
			}
		}
		if isLegacyModelPortSQLIdentifierStart(expression[index]) {
			start := index
			index++
			for index < len(expression) && isSQLIdentifierPart(expression[index]) {
				index++
			}
			tokens = append(tokens, legacyModelPortSQLToken{kind: legacyModelPortSQLIdentifierToken, value: expression[start:index]})
			continue
		}
		if expression[index] == ':' && index+1 < len(expression) && expression[index+1] == ':' {
			tokens = append(tokens, legacyModelPortSQLToken{value: "::"})
			index += 2
			continue
		}
		tokens = append(tokens, legacyModelPortSQLToken{value: string(expression[index])})
		index++
	}
	return tokens, nil
}

func isLegacyModelPortSQLIdentifierStart(value byte) bool {
	return value == '_' || (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

// maskLegacyModelPortSQLNonCode replaces comments, quoted values, quoted
// identifiers, and dollar-quoted strings with spaces. Keeping newlines intact
// preserves token boundaries while ensuring comments/literals cannot spoof a
// target column or the IN/ANY membership keyword.
func maskLegacyModelPortSQLNonCode(expression string) (string, error) {
	masked := []byte(expression)
	mask := func(start, end int) {
		for i := start; i < end && i < len(masked); i++ {
			if masked[i] != '\n' && masked[i] != '\r' {
				masked[i] = ' '
			}
		}
	}

	for i := 0; i < len(expression); {
		switch {
		case expression[i] == '-' && i+1 < len(expression) && expression[i+1] == '-':
			end := skipLegacyModelPortSQLLineComment(expression, i)
			mask(i, end)
			i = end
		case expression[i] == '/' && i+1 < len(expression) && expression[i+1] == '*':
			end, err := skipLegacyModelPortSQLBlockComment(expression, i)
			if err != nil {
				return "", err
			}
			mask(i, end)
			i = end
		case expression[i] == '\'':
			_, end, err := parseLegacyModelPortSQLStringLiteral(expression, i)
			if err != nil {
				return "", err
			}
			mask(i, end)
			i = end
		case expression[i] == '"':
			end, err := skipLegacyModelPortSQLDoubleQuotedIdentifier(expression, i)
			if err != nil {
				return "", err
			}
			mask(i, end)
			i = end
		case expression[i] == '$':
			end, ok, err := skipLegacyModelPortSQLDollarQuoted(expression, i)
			if !ok {
				i++
				continue
			}
			if err != nil {
				return "", err
			}
			mask(i, end)
			i = end
		default:
			i++
		}
	}
	return string(masked), nil
}

func containsLegacyModelPortQuotedIdentifier(expression, identifier string) bool {
	for i := 0; i < len(expression); {
		switch {
		case expression[i] == '-' && i+1 < len(expression) && expression[i+1] == '-':
			i = skipLegacyModelPortSQLLineComment(expression, i)
		case expression[i] == '/' && i+1 < len(expression) && expression[i+1] == '*':
			end, err := skipLegacyModelPortSQLBlockComment(expression, i)
			if err != nil {
				return false
			}
			i = end
		case expression[i] == '\'':
			_, end, err := parseLegacyModelPortSQLStringLiteral(expression, i)
			if err != nil {
				return false
			}
			i = end
		case expression[i] == '$':
			if end, ok, err := skipLegacyModelPortSQLDollarQuoted(expression, i); ok {
				if err != nil {
					return false
				}
				i = end
			} else {
				i++
			}
		case expression[i] == '"':
			value, end, err := parseLegacyModelPortSQLDoubleQuotedIdentifier(expression, i)
			if err != nil {
				return false
			}
			if value == identifier {
				return true
			}
			i = end
		default:
			i++
		}
	}
	return false
}

func skipLegacyModelPortSQLLineComment(expression string, start int) int {
	for i := start + 2; i < len(expression); i++ {
		if expression[i] == '\n' {
			return i
		}
	}
	return len(expression)
}

func skipLegacyModelPortSQLBlockComment(expression string, start int) (int, error) {
	depth := 1
	for i := start + 2; i < len(expression); {
		if i+1 < len(expression) && expression[i] == '/' && expression[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if i+1 < len(expression) && expression[i] == '*' && expression[i+1] == '/' {
			depth--
			i += 2
			if depth == 0 {
				return i, nil
			}
			continue
		}
		i++
	}
	return 0, errors.New("CHECK contains an unterminated SQL block comment")
}

func parseLegacyModelPortSQLStringLiteral(expression string, start int) (string, int, error) {
	var builder strings.Builder
	escapeBackslash := start > 0 && (expression[start-1] == 'e' || expression[start-1] == 'E') &&
		(start == 1 || !isSQLIdentifierPart(expression[start-2]))
	for i := start + 1; i < len(expression); {
		switch expression[i] {
		case '\'':
			if i+1 < len(expression) && expression[i+1] == '\'' {
				_ = builder.WriteByte('\'')
				i += 2
				continue
			}
			return builder.String(), i + 1, nil
		case '\\':
			if !escapeBackslash {
				_ = builder.WriteByte('\\')
				i++
				continue
			}
			if i+1 >= len(expression) {
				return "", 0, errors.New("CHECK contains an unterminated SQL string literal")
			}
			escaped := expression[i+1]
			switch escaped {
			case 'b':
				_ = builder.WriteByte('\b')
			case 'f':
				_ = builder.WriteByte('\f')
			case 'n':
				_ = builder.WriteByte('\n')
			case 'r':
				_ = builder.WriteByte('\r')
			case 't':
				_ = builder.WriteByte('\t')
			case 'v':
				_ = builder.WriteByte('\v')
			default:
				_ = builder.WriteByte(escaped)
			}
			i += 2
		default:
			_ = builder.WriteByte(expression[i])
			i++
		}
	}
	return "", 0, errors.New("CHECK contains an unterminated SQL string literal")
}

func skipLegacyModelPortSQLDoubleQuotedIdentifier(expression string, start int) (int, error) {
	for i := start + 1; i < len(expression); i++ {
		if expression[i] != '"' {
			continue
		}
		if i+1 < len(expression) && expression[i+1] == '"' {
			i++
			continue
		}
		return i + 1, nil
	}
	return 0, errors.New("CHECK contains an unterminated SQL quoted identifier")
}

func parseLegacyModelPortSQLDoubleQuotedIdentifier(expression string, start int) (string, int, error) {
	var builder strings.Builder
	for i := start + 1; i < len(expression); i++ {
		if expression[i] != '"' {
			_ = builder.WriteByte(expression[i])
			continue
		}
		if i+1 < len(expression) && expression[i+1] == '"' {
			_ = builder.WriteByte('"')
			i++
			continue
		}
		return builder.String(), i + 1, nil
	}
	return "", 0, errors.New("CHECK contains an unterminated SQL quoted identifier")
}

func legacyModelPortSQLDollarQuoteDelimiter(expression string, start int) (string, bool) {
	if expression[start] != '$' {
		return "", false
	}
	i := start + 1
	if i < len(expression) && expression[i] == '$' {
		return "$$", true
	}
	if i >= len(expression) || !isSQLIdentifierStart(expression[i]) {
		return "", false
	}
	i++
	for i < len(expression) && (isSQLIdentifierPart(expression[i])) {
		i++
	}
	if i >= len(expression) || expression[i] != '$' {
		return "", false
	}
	return expression[start : i+1], true
}

func isSQLIdentifierStart(value byte) bool {
	return value == '_' || (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

func skipLegacyModelPortSQLDollarQuoted(expression string, start int) (int, bool, error) {
	delimiter, ok := legacyModelPortSQLDollarQuoteDelimiter(expression, start)
	if !ok {
		return start, false, nil
	}
	contentStart := start + len(delimiter)
	end := strings.Index(expression[contentStart:], delimiter)
	if end < 0 {
		return 0, true, errors.New("CHECK contains an unterminated SQL dollar-quoted string")
	}
	return contentStart + end + len(delimiter), true, nil
}

func legacyModelPortStringSetEqual(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if _, ok := right[value]; !ok {
			return false
		}
	}
	return true
}

func formatLegacyModelPortStringSet(values map[string]struct{}) string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return "{" + strings.Join(quoted, ", ") + "}"
}

func prepareLegacyModelPortInstructionAuditIndexes(ctx context.Context, db migrationConnection) error {
	applied, err := migrationWasApplied(ctx, db, legacyModelPortInstructionAuditIndexesMigration)
	if err != nil {
		return err
	}
	if !applied {
		return nil
	}
	for _, indexName := range legacyModelPortInstructionAuditIndexes {
		if err := dropInvalidIndexIfPresent(ctx, db, indexName); err != nil {
			return err
		}
	}
	return nil
}

// legacyModelPortBlockedPlatformValuesQuery reports only known-removed values
// that are still executable configuration. Disabled/deleted accounts and
// groups, disabled/deleted composite routes, and quota rows with no configured
// limit or usage are storage-only and therefore do not block the bridge.
const legacyModelPortBlockedPlatformValuesQuery = `
SELECT source, value
FROM (
	SELECT 'accounts.platform' AS source, platform::text AS value
	FROM accounts
	WHERE status = 'active'
	  AND deleted_at IS NULL
	  AND platform IN (
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'groups.platform', platform::text
	FROM groups
	WHERE status = 'active'
	  AND deleted_at IS NULL
	  AND platform IN (
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'composite_model_routes.target_platform', target_platform::text
	FROM composite_model_routes
	WHERE deleted_at IS NULL
	  AND enabled
	  AND target_platform IN (
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'user_platform_quotas.platform' AS source, platform::text AS value
	FROM user_platform_quotas
	WHERE deleted_at IS NULL
	  AND platform IN (
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	  AND (
		daily_limit_usd IS NOT NULL
		OR weekly_limit_usd IS NOT NULL
		OR monthly_limit_usd IS NOT NULL
		OR daily_usage_usd <> 0
		OR weekly_usage_usd <> 0
		OR monthly_usage_usd <> 0
	  )
) AS blocked_platforms
GROUP BY source, value
ORDER BY source, value
LIMIT 20
`

// legacyModelPortUnknownPlatformValuesQuery validates values against the
// final storage CHECKs. Removed providers are intentionally included in the
// quota/route/monitor sets above; only values outside those terminal sets are
// unknown. Account/group rows use the broad runtime list plus archived names so
// disabled legacy rows remain preservable while arbitrary values fail closed.
const legacyModelPortUnknownPlatformValuesQuery = `
SELECT source, value
FROM (
	SELECT 'accounts.platform' AS source, platform::text AS value
	FROM accounts
	WHERE platform IS NOT NULL
	  AND platform NOT IN (
		'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
		'kimi', 'zhipu', 'deepseek', 'composite', 'kiro',
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'groups.platform', platform::text
	FROM groups
	WHERE platform IS NOT NULL
	  AND platform NOT IN (
		'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
		'kimi', 'zhipu', 'deepseek', 'composite', 'kiro',
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'user_platform_quotas.platform' AS source, platform::text AS value
	FROM user_platform_quotas
	WHERE platform IS NOT NULL
	  AND platform NOT IN (
		'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
		'kimi', 'zhipu', 'deepseek',
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'composite_model_routes.target_platform', target_platform::text
	FROM composite_model_routes
	WHERE target_platform IS NOT NULL
	  AND target_platform NOT IN (
		'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
		'kimi', 'zhipu', 'deepseek',
		'qwen', 'glm', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'channel_monitors.provider', provider::text
	FROM channel_monitors
	WHERE provider IS NOT NULL
	  AND provider NOT IN (
		'openai', 'anthropic', 'gemini', 'grok', 'antigravity',
		'kimi', 'zhipu', 'deepseek',
		'qwen', 'glm', 'doubao', 'minimax', 'mimo'
	  )
	UNION ALL
	SELECT 'channel_monitor_request_templates.provider', provider::text
	FROM channel_monitor_request_templates
	WHERE provider IS NOT NULL
	  AND provider NOT IN (
		'openai', 'anthropic', 'gemini', 'grok', 'antigravity',
		'kimi', 'zhipu', 'deepseek',
		'qwen', 'glm', 'doubao', 'minimax', 'mimo'
	  )
) AS unknown_platforms
GROUP BY source, value
ORDER BY source, value
LIMIT 20
`

// prepareLegacyModelPortPlatformConstraints is retained as a compatibility
// helper for callers/tests that explicitly request the bridge. The normal
// migration runner uses planLegacyModelPortPlatformConstraints and defers the
// write until the end of the migration pass.
func prepareLegacyModelPortPlatformConstraints(ctx context.Context, db migrationConnection, fsys fs.FS) error {
	// Keep the historical direct helper permissive for callers that use it as a
	// constraint-only compatibility probe. The real runner goes through
	// prepareLegacyModelPortMigrations, which enforces the terminal 236 bridge.
	plan, err := planLegacyModelPortPlatformConstraintsCompat(ctx, db, fsys)
	if err != nil {
		return err
	}
	return applyLegacyModelPortPlatformBridgeImmediate(ctx, db, plan)
}

// planLegacyModelPortPlatformConstraints performs all legacy detection and
// validation without changing the database. The returned plan is committed by
// the main runner only after unrelated migrations have completed successfully.
func planLegacyModelPortPlatformConstraints(
	ctx context.Context,
	db migrationQueryConnection,
	fsys fs.FS,
) (*legacyModelPortPlatformBridgePlan, error) {
	return planLegacyModelPortPlatformConstraintsWithOptions(ctx, db, fsys, true)
}

func planLegacyModelPortPlatformConstraintsCompat(
	ctx context.Context,
	db migrationQueryConnection,
	fsys fs.FS,
) (*legacyModelPortPlatformBridgePlan, error) {
	return planLegacyModelPortPlatformConstraintsWithOptions(ctx, db, fsys, false)
}

func planLegacyModelPortPlatformConstraintsWithOptions(
	ctx context.Context,
	db migrationQueryConnection,
	fsys fs.FS,
	strictTerminalBridge bool,
) (*legacyModelPortPlatformBridgePlan, error) {
	legacyQuotaPlatforms, err := legacyModelPortMigrationMatchesArchive(ctx, db, legacyModelPortOpenAICompatibleProvidersMigration)
	if err != nil {
		return nil, err
	}
	legacyMonitorPlatforms, err := legacyModelPortMigrationMatchesArchive(ctx, db, legacyModelPortChannelMonitorProvidersMigration)
	if err != nil {
		return nil, err
	}
	if !legacyQuotaPlatforms && !legacyMonitorPlatforms {
		return nil, nil
	}

	applied, err := prevalidateLegacyModelPortPlatformMigrationChecksums(ctx, db, fsys)
	if err != nil {
		return nil, err
	}

	// 236 owns the terminal constraints for all four provider-bearing tables.
	// Even a database with only the legacy 197 marker can carry storage-only
	// values in quotas/routes, so let the bridge satisfy constraint-only 224/227
	// before they can narrow those rows. Migration 226 must still run for its
	// structural changes.
	needUserQuota := !applied[upstreamUserPlatformCNProvidersMigration]
	needCompositeRoutes := !applied[upstreamCompositeRoutesCNProvidersMigration]
	// The structural part of 226 is applicable to every legacy bridge path,
	// even when only the 188 marker is present.  If 236 is already applied we
	// deliberately leave 226 in the normal migration loop; that exercises its
	// real DDL and records its ledger instead of silently treating it as an
	// equivalent constraint-only migration.
	needChannelMonitor := !applied[upstreamChannelMonitorQuotaModeMigration]
	needBridge := !applied[modelPortLegacyPlatformConstraintsMigration]
	if !needUserQuota && !needChannelMonitor && !needCompositeRoutes && !needBridge {
		// Keep a non-nil validation-only plan so the catalog state is checked on
		// every startup, without opening an empty bridge transaction.
		return &legacyModelPortPlatformBridgePlan{
			deferredMigrations: make(map[string]struct{}),
			appliedMigrations:  applied,
			validationOnly:     true,
		}, nil
	}
	if !strictTerminalBridge && !needUserQuota && !needChannelMonitor && !needCompositeRoutes {
		// The direct compatibility helper predates migration 236 and retains its
		// historical no-op behavior when all three upstream inputs are complete.
		return &legacyModelPortPlatformBridgePlan{
			deferredMigrations: make(map[string]struct{}),
			appliedMigrations:  applied,
			validationOnly:     true,
		}, nil
	}

	blocked, err := legacyModelPortPlatformValues(ctx, db, legacyModelPortBlockedPlatformValuesQuery)
	if err != nil {
		return nil, fmt.Errorf("precheck known-removed legacy ModelPort platform values: %w", err)
	}
	if len(blocked) > 0 {
		return nil, fmt.Errorf(
			"known-removed legacy ModelPort provider values block migration; preserve and explicitly reconfigure each active row before retrying: %s",
			strings.Join(blocked, ", "),
		)
	}

	unknown, err := legacyModelPortUnknownPlatformValues(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("precheck legacy ModelPort platform values: %w", err)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf(
			"unknown legacy ModelPort platform values block migration; preserve and classify unknown values before retrying: %s",
			strings.Join(unknown, ", "),
		)
	}

	bridgeContent, err := migrationFileContent(fsys, modelPortLegacyPlatformConstraintsMigration)
	if err != nil {
		return nil, err
	}
	bridgeChecksum := migrationContentChecksum(bridgeContent)
	checksums := make(map[string]string, 2)
	for name, needed := range map[string]bool{
		upstreamUserPlatformCNProvidersMigration:    needUserQuota,
		upstreamCompositeRoutesCNProvidersMigration: needCompositeRoutes,
	} {
		if !needed {
			continue
		}
		checksum, err := migrationFileChecksum(fsys, name)
		if err != nil {
			return nil, err
		}
		checksums[name] = checksum
	}

	plan := &legacyModelPortPlatformBridgePlan{
		deferredMigrations:  make(map[string]struct{}),
		equivalentChecksums: checksums,
		bridgeChecksum:      bridgeChecksum,
		appliedMigrations:   applied,
	}
	if needBridge {
		plan.bridgeContent = bridgeContent
		plan.deferredMigrations[modelPortLegacyPlatformConstraintsMigration] = struct{}{}
		// 226 must observe the widened provider constraints. Deferring it along
		// with the bridge keeps its structural changes in the same transaction.
		if needChannelMonitor {
			plan.deferredMigrations[upstreamChannelMonitorQuotaModeMigration] = struct{}{}
		}
	}
	if needUserQuota {
		plan.deferredMigrations[upstreamUserPlatformCNProvidersMigration] = struct{}{}
	}
	if needCompositeRoutes {
		plan.deferredMigrations[upstreamCompositeRoutesCNProvidersMigration] = struct{}{}
	}
	return plan, nil
}

// applyLegacyModelPortPlatformBridgeImmediate preserves the direct helper's
// historical behavior for focused callers. The normal runner uses the
// deferred variant below so the structural 226 migration is included in the
// same transaction as the bridge.
func applyLegacyModelPortPlatformBridgeImmediate(
	ctx context.Context,
	db migrationConnection,
	plan *legacyModelPortPlatformBridgePlan,
) error {
	if plan == nil || plan.validationOnly {
		return nil
	}
	return applyLegacyModelPortPlatformBridgeTx(ctx, db, plan, "", "", false, false)
}

func applyLegacyModelPortPlatformBridge(
	ctx context.Context,
	db migrationConnection,
	fsys fs.FS,
	plan *legacyModelPortPlatformBridgePlan,
) error {
	if plan == nil || plan.validationOnly {
		return nil
	}

	var channelMonitorContent, channelMonitorChecksum string
	if plan.defers(upstreamChannelMonitorQuotaModeMigration) {
		var err error
		channelMonitorContent, err = migrationFileContent(fsys, upstreamChannelMonitorQuotaModeMigration)
		if err != nil {
			return err
		}
		if _, err := validateMigrationExecutionMode(upstreamChannelMonitorQuotaModeMigration, channelMonitorContent); err != nil {
			return fmt.Errorf("validate migration %s: %w", upstreamChannelMonitorQuotaModeMigration, err)
		}
		channelMonitorChecksum = migrationContentChecksum(channelMonitorContent)
	}
	return applyLegacyModelPortPlatformBridgeTx(
		ctx,
		db,
		plan,
		channelMonitorContent,
		channelMonitorChecksum,
		true,
		true,
	)
}

func applyLegacyModelPortPlatformBridgeTx(
	ctx context.Context,
	db migrationConnection,
	plan *legacyModelPortPlatformBridgePlan,
	channelMonitorContent string,
	channelMonitorChecksum string,
	includeDeferredStructural bool,
	recordBridgeLedger bool,
) error {
	if plan == nil {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin legacy ModelPort platform bridge: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if plan.bridgeContent != "" {
		// Apply the final-state bridge first so 226 observes the widened
		// provider constraints instead of narrowing preserved legacy values.
		if _, err := tx.ExecContext(ctx, plan.bridgeContent); err != nil {
			return fmt.Errorf("apply legacy ModelPort platform constraint bridge: %w", err)
		}
	}
	if includeDeferredStructural && channelMonitorContent != "" {
		if _, err := tx.ExecContext(ctx, channelMonitorContent); err != nil {
			return fmt.Errorf("apply deferred migration %s: %w", upstreamChannelMonitorQuotaModeMigration, err)
		}
	}

	// 224/227 only change provider CHECK constraints. The bridge is a strict
	// semantic superset, so recording their exact file checksums is sufficient
	// and avoids replaying SQL that would narrow the constraint again.
	names := make([]string, 0, len(plan.equivalentChecksums))
	for name := range plan.equivalentChecksums {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)",
			name, plan.equivalentChecksums[name],
		); err != nil {
			return fmt.Errorf("record equivalent legacy migration %s: %w", name, err)
		}
	}
	if includeDeferredStructural && channelMonitorContent != "" {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)",
			upstreamChannelMonitorQuotaModeMigration, channelMonitorChecksum,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", upstreamChannelMonitorQuotaModeMigration, err)
		}
	}
	if recordBridgeLedger && plan.bridgeContent != "" {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)",
			modelPortLegacyPlatformConstraintsMigration, plan.bridgeChecksum,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", modelPortLegacyPlatformConstraintsMigration, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy ModelPort platform bridge: %w", err)
	}
	committed = true
	return nil
}

func prevalidateLegacyModelPortPlatformMigrationChecksums(
	ctx context.Context,
	db migrationQueryConnection,
	fsys fs.FS,
) (map[string]bool, error) {
	applied := make(map[string]bool, 4)
	for _, name := range []string{
		upstreamUserPlatformCNProvidersMigration,
		upstreamChannelMonitorQuotaModeMigration,
		upstreamCompositeRoutesCNProvidersMigration,
		modelPortLegacyPlatformConstraintsMigration,
	} {
		wasApplied, err := migrationWasAppliedWithValidChecksum(ctx, db, fsys, name)
		if err != nil {
			return nil, fmt.Errorf("prevalidate legacy ModelPort migration %s: %w", name, err)
		}
		applied[name] = wasApplied
	}
	return applied, nil
}

func migrationWasAppliedWithValidChecksum(
	ctx context.Context,
	db migrationQueryConnection,
	fsys fs.FS,
	name string,
) (bool, error) {
	var existing string
	err := db.QueryRowContext(ctx,
		"SELECT checksum FROM schema_migrations WHERE filename = $1",
		name,
	).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read migration ledger checksum: %w", err)
	}

	checksum, err := migrationFileChecksum(fsys, name)
	if err != nil {
		return false, err
	}
	if err := validateAppliedMigrationChecksum(name, existing, checksum); err != nil {
		return false, err
	}
	return true, nil
}

func migrationWasApplied(ctx context.Context, db migrationQueryConnection, name string) (bool, error) {
	var applied bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations WHERE filename = $1
		)
	`, name).Scan(&applied); err != nil {
		return false, fmt.Errorf("check legacy migration %s: %w", name, err)
	}
	return applied, nil
}

// legacyModelPortMigrationMatchesArchive treats a historical filename as
// ModelPort evidence only when its ledger checksum exactly matches the
// immutable custom-v0.1.176.2 manifest. A same-name row with any other
// checksum is ambiguous and must stop before the bridge mutates constraints or
// records an upstream migration as semantically satisfied.
func legacyModelPortMigrationMatchesArchive(ctx context.Context, db migrationQueryConnection, name string) (bool, error) {
	expected, err := legacyModelPortArchivedMigrationChecksum(name)
	if err != nil {
		return false, err
	}

	var existing string
	err = db.QueryRowContext(ctx,
		"SELECT checksum FROM schema_migrations WHERE filename = $1",
		name,
	).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read legacy ModelPort migration %s ledger checksum: %w", name, err)
	}
	if existing != expected {
		return false, fmt.Errorf(
			"legacy ModelPort migration %s checksum mismatch (db=%s archive=%s); refusing platform bridge because database identity is ambiguous",
			name, existing, expected,
		)
	}
	return true, nil
}

type legacyModelPortArchivedMigrationDigest struct {
	raw     string
	trimmed string
}

// legacyModelPortArchivedMigrationChecksum retains the historical API used by
// the migration planner, but now obtains its value only after the complete
// manifest and archive have been authenticated.
func legacyModelPortArchivedMigrationChecksum(name string) (string, error) {
	digests, err := legacyModelPortArchivedMigrationDigests()
	if err != nil {
		return "", err
	}
	digest, ok := digests[name]
	if !ok {
		return "", fmt.Errorf("legacy ModelPort migration %s is missing from manifest", name)
	}
	return digest.trimmed, nil
}

func legacyModelPortArchivedMigrationDigests() (map[string]legacyModelPortArchivedMigrationDigest, error) {
	manifest, err := fs.ReadFile(migrations.LegacyFS, legacyModelPortMigrationManifest)
	if err != nil {
		return nil, fmt.Errorf("read legacy ModelPort migration manifest: %w", err)
	}
	entries, err := parseLegacyModelPortMigrationManifest(manifest)
	if err != nil {
		return nil, err
	}

	archiveRoot := "modelport_legacy/v0.1.176.2"
	archiveEntries, err := fs.ReadDir(migrations.LegacyFS, archiveRoot)
	if err != nil {
		return nil, fmt.Errorf("read legacy ModelPort migration archive: %w", err)
	}
	seen := make(map[string]struct{}, len(entries))
	for _, archiveEntry := range archiveEntries {
		if archiveEntry.IsDir() || !strings.HasSuffix(archiveEntry.Name(), ".sql") {
			continue
		}
		name := archiveEntry.Name()
		expected, ok := entries[name]
		if !ok {
			return nil, fmt.Errorf("legacy ModelPort migration archive contains unmanifested file %s", name)
		}
		content, err := fs.ReadFile(migrations.LegacyFS, archiveRoot+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("read archived legacy ModelPort migration %s: %w", name, err)
		}
		rawSum := sha256.Sum256(content)
		trimmedSum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		actualRaw := hex.EncodeToString(rawSum[:])
		actualTrimmed := hex.EncodeToString(trimmedSum[:])
		if actualRaw != expected.raw {
			return nil, fmt.Errorf("legacy ModelPort migration %s raw checksum mismatch (manifest=%s archive=%s)", name, expected.raw, actualRaw)
		}
		if actualTrimmed != expected.trimmed {
			return nil, fmt.Errorf("legacy ModelPort migration %s runner checksum mismatch (manifest=%s archive=%s)", name, expected.trimmed, actualTrimmed)
		}
		seen[name] = struct{}{}
	}
	for name := range entries {
		if _, ok := seen[name]; !ok {
			return nil, fmt.Errorf("legacy ModelPort migration %s is missing from archive", name)
		}
	}
	return entries, nil
}

func parseLegacyModelPortMigrationManifest(manifest []byte) (map[string]legacyModelPortArchivedMigrationDigest, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(manifest)))
	lineNumber := 0
	entries := make(map[string]legacyModelPortArchivedMigrationDigest)
	headerSeen := false
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !headerSeen {
			headerSeen = true
			if line != legacyModelPortMigrationManifestHeader {
				return nil, fmt.Errorf("unexpected legacy ModelPort migration manifest header %q", line)
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("legacy ModelPort migration manifest line %d has %d fields, want 3", lineNumber, len(fields))
		}
		name := fields[0]
		if name == "" || strings.TrimSpace(name) != name || !strings.HasSuffix(name, ".sql") || strings.ContainsAny(name, "/\\") {
			return nil, fmt.Errorf("legacy ModelPort migration manifest line %d has invalid migration name %q", lineNumber, name)
		}
		raw, err := validateLegacyModelPortManifestSHA(fields[1], "raw", name)
		if err != nil {
			return nil, fmt.Errorf("legacy ModelPort migration manifest line %d: %w", lineNumber, err)
		}
		trimmed, err := validateLegacyModelPortManifestSHA(fields[2], "runner", name)
		if err != nil {
			return nil, fmt.Errorf("legacy ModelPort migration manifest line %d: %w", lineNumber, err)
		}
		if _, duplicate := entries[name]; duplicate {
			return nil, fmt.Errorf("legacy ModelPort migration manifest contains duplicate %s", name)
		}
		entries[name] = legacyModelPortArchivedMigrationDigest{raw: raw, trimmed: trimmed}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan legacy ModelPort migration manifest: %w", err)
	}
	if !headerSeen {
		return nil, errors.New("legacy ModelPort migration manifest is empty")
	}
	if len(entries) == 0 {
		return nil, errors.New("legacy ModelPort migration manifest contains no migration rows")
	}
	return entries, nil
}

func validateLegacyModelPortManifestSHA(value, label, name string) (string, error) {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return "", fmt.Errorf("legacy ModelPort migration manifest has invalid %s checksum for %s", label, name)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("legacy ModelPort migration manifest has invalid %s checksum for %s", label, name)
	}
	return value, nil
}

func legacyModelPortUnknownPlatformValues(ctx context.Context, db migrationQueryConnection) ([]string, error) {
	return legacyModelPortPlatformValues(ctx, db, legacyModelPortUnknownPlatformValuesQuery)
}

func legacyModelPortPlatformValues(ctx context.Context, db migrationQueryConnection, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	unknown := make([]string, 0)
	for rows.Next() {
		var source, value string
		if err := rows.Scan(&source, &value); err != nil {
			return nil, err
		}
		unknown = append(unknown, source+"="+value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return unknown, nil
}

func migrationFileContent(fsys fs.FS, name string) (string, error) {
	contentBytes, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("read migration %s: %w", name, err)
	}
	content := strings.TrimSpace(string(contentBytes))
	if content == "" {
		return "", fmt.Errorf("migration %s is empty", name)
	}
	return content, nil
}

func migrationFileChecksum(fsys fs.FS, name string) (string, error) {
	content, err := migrationFileContent(fsys, name)
	if err != nil {
		return "", err
	}
	return migrationContentChecksum(content), nil
}

func migrationContentChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func preparePaymentOrdersOutTradeNoUniqueMigration(ctx context.Context, db migrationConnection) error {
	duplicates, err := findDuplicatePaymentOrderOutTradeNos(ctx, db)
	if err != nil {
		return fmt.Errorf("precheck duplicate out_trade_no: %w", err)
	}
	if len(duplicates) > 0 {
		return fmt.Errorf(
			"duplicate out_trade_no values block %s; remediate duplicates before retrying: %s",
			paymentOrdersOutTradeNoUniqueMigration,
			strings.Join(duplicates, ", "),
		)
	}

	return dropInvalidIndexIfPresent(ctx, db, paymentOrdersOutTradeNoUniqueIndex)
}

func dropInvalidIndexIfPresent(ctx context.Context, db migrationConnection, indexName string) error {
	invalid, err := indexIsInvalid(ctx, db, indexName)
	if err != nil {
		return fmt.Errorf("check invalid index %s: %w", indexName, err)
	}
	if !invalid {
		return nil
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s", indexName)); err != nil {
		return fmt.Errorf("drop invalid index %s: %w", indexName, err)
	}
	return nil
}

func findDuplicatePaymentOrderOutTradeNos(ctx context.Context, db migrationConnection) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT out_trade_no, COUNT(*) AS duplicate_count
		FROM payment_orders
		WHERE out_trade_no <> ''
		GROUP BY out_trade_no
		HAVING COUNT(*) > 1
		ORDER BY duplicate_count DESC, out_trade_no
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	duplicates := make([]string, 0, 5)
	for rows.Next() {
		var outTradeNo string
		var duplicateCount int
		if err := rows.Scan(&outTradeNo, &duplicateCount); err != nil {
			return nil, err
		}
		duplicates = append(duplicates, fmt.Sprintf("%s (count=%d)", outTradeNo, duplicateCount))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return duplicates, nil
}

func indexIsInvalid(ctx context.Context, db migrationConnection, indexName string) (bool, error) {
	var invalid bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_class idx
			JOIN pg_namespace ns ON ns.oid = idx.relnamespace
			JOIN pg_index i ON i.indexrelid = idx.oid
			WHERE ns.nspname = 'public'
			  AND idx.relname = $1
			  AND NOT i.indisvalid
		)
	`, indexName).Scan(&invalid)
	return invalid, err
}

func ensureAtlasBaselineAligned(ctx context.Context, db migrationConnection, fsys fs.FS) error {
	hasLegacy, err := tableExists(ctx, db, "schema_migrations")
	if err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if !hasLegacy {
		return nil
	}

	hasAtlas, err := tableExists(ctx, db, "atlas_schema_revisions")
	if err != nil {
		return fmt.Errorf("check atlas_schema_revisions: %w", err)
	}
	if !hasAtlas {
		if _, err := db.ExecContext(ctx, atlasSchemaRevisionsTableDDL); err != nil {
			return fmt.Errorf("create atlas_schema_revisions: %w", err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM atlas_schema_revisions").Scan(&count); err != nil {
		return fmt.Errorf("count atlas_schema_revisions: %w", err)
	}
	if count > 0 {
		return nil
	}

	version, description, hash, err := latestMigrationBaseline(fsys)
	if err != nil {
		return fmt.Errorf("atlas baseline version: %w", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO atlas_schema_revisions (version, description, type, applied, total, executed_at, execution_time, hash)
		VALUES ($1, $2, $3, 0, 0, NOW(), 0, $4)
	`, version, description, 1, hash); err != nil {
		return fmt.Errorf("insert atlas baseline: %w", err)
	}
	return nil
}

func tableExists(ctx context.Context, db migrationConnection, tableName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, tableName).Scan(&exists)
	return exists, err
}

func readPublicBaseTables(ctx context.Context, db migrationQueryConnection) ([]string, error) {
	rows, err := db.QueryContext(ctx, publicBaseTablesQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	tables := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func latestMigrationBaseline(fsys fs.FS) (string, string, string, error) {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return "", "", "", err
	}
	if len(files) == 0 {
		return "baseline", "baseline", "", nil
	}
	sort.Strings(files)
	name := files[len(files)-1]
	contentBytes, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", "", "", err
	}
	content := strings.TrimSpace(string(contentBytes))
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	version := strings.TrimSuffix(name, ".sql")
	return version, version, hash, nil
}

func checksumSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func newMigrationChecksumCompatibilityRule(fileChecksum string, acceptedDBChecksums ...string) migrationChecksumCompatibilityRule {
	return migrationChecksumCompatibilityRule{
		fileChecksum:       fileChecksum,
		acceptedDBChecksum: checksumSet(acceptedDBChecksums...),
		acceptedChecksums:  checksumSet(append([]string{fileChecksum}, acceptedDBChecksums...)...),
	}
}

func isMigrationChecksumCompatible(name, dbChecksum, fileChecksum string) bool {
	rule, ok := migrationChecksumCompatibilityRules[name]
	if !ok {
		return false
	}
	_, dbOK := rule.acceptedChecksums[dbChecksum]
	if !dbOK {
		return false
	}
	_, fileOK := rule.acceptedChecksums[fileChecksum]
	return fileOK
}

func validateAppliedMigrationChecksum(name, dbChecksum, fileChecksum string) error {
	if dbChecksum == fileChecksum || isMigrationChecksumCompatible(name, dbChecksum, fileChecksum) {
		return nil
	}
	return fmt.Errorf(
		"migration %s checksum mismatch (db=%s file=%s)\n"+
			"This means the migration file was modified after being applied to the database.\n"+
			"Solutions:\n"+
			"  1. Revert to original: git log --oneline -- migrations/%s && git checkout <commit> -- migrations/%s\n"+
			"  2. For new changes, create a new migration file instead of modifying existing ones\n"+
			"Note: Modifying applied migrations breaks the immutability principle and can cause inconsistencies across environments",
		name, dbChecksum, fileChecksum, name, name,
	)
}

func validateMigrationExecutionMode(name, content string) (bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	upperContent := strings.ToUpper(content)
	nonTx := strings.HasSuffix(normalizedName, nonTransactionalMigrationSuffix)

	if !nonTx {
		if strings.Contains(upperContent, "CONCURRENTLY") {
			return false, errors.New("CONCURRENTLY statements must be placed in *_notx.sql migrations")
		}
		return false, nil
	}

	if strings.Contains(upperContent, "BEGIN") || strings.Contains(upperContent, "COMMIT") || strings.Contains(upperContent, "ROLLBACK") {
		return false, errors.New("*_notx.sql must not contain transaction control statements (BEGIN/COMMIT/ROLLBACK)")
	}

	statements := splitSQLStatements(content)
	for _, stmt := range statements {
		normalizedStmt := strings.ToUpper(stripSQLLineComment(strings.TrimSpace(stmt)))
		if normalizedStmt == "" {
			continue
		}

		if strings.Contains(normalizedStmt, "CONCURRENTLY") {
			isCreateIndex := strings.Contains(normalizedStmt, "CREATE") && strings.Contains(normalizedStmt, "INDEX")
			isDropIndex := strings.Contains(normalizedStmt, "DROP") && strings.Contains(normalizedStmt, "INDEX")
			if !isCreateIndex && !isDropIndex {
				return false, errors.New("*_notx.sql currently only supports CREATE/DROP INDEX CONCURRENTLY statements")
			}
			if isCreateIndex && !strings.Contains(normalizedStmt, "IF NOT EXISTS") {
				return false, errors.New("CREATE INDEX CONCURRENTLY in *_notx.sql must include IF NOT EXISTS for idempotency")
			}
			if isDropIndex && !strings.Contains(normalizedStmt, "IF EXISTS") {
				return false, errors.New("DROP INDEX CONCURRENTLY in *_notx.sql must include IF EXISTS for idempotency")
			}
			continue
		}

		return false, errors.New("*_notx.sql must not mix non-CONCURRENTLY SQL statements")
	}

	return true, nil
}

func splitSQLStatements(content string) []string {
	parts := strings.Split(content, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func stripSQLLineComment(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// pgAdvisoryLock 获取 PostgreSQL Advisory Lock。
// Advisory Lock 是一种轻量级的锁机制，不与任何特定的数据库对象关联。
// 它非常适合用于应用层面的分布式锁场景，如迁移序列化。
type advisoryLockConnection interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func pgAdvisoryLock(ctx context.Context, db advisoryLockConnection) error {
	ticker := time.NewTicker(migrationsLockRetryInterval)
	defer ticker.Stop()

	for {
		var locked bool
		if err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrationsAdvisoryLockID).Scan(&locked); err != nil {
			return fmt.Errorf("acquire migrations lock: %w", err)
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire migrations lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// pgAdvisoryUnlock 释放 PostgreSQL Advisory Lock。
// 必须在获取锁后确保释放，否则会阻塞其他实例的迁移操作。
func pgAdvisoryUnlock(ctx context.Context, db advisoryLockConnection) error {
	_, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationsAdvisoryLockID)
	if err != nil {
		return fmt.Errorf("release migrations lock: %w", err)
	}
	return nil
}
