//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const legacyModelPortMigrationRoot = "modelport_legacy/v0.1.176.2"

func TestModelPortMigrationPaths(t *testing.T) {
	t.Run("empty database", func(t *testing.T) {
		db := newMigrationPathDatabase(t, "empty")
		require.NoError(t, ApplyMigrations(context.Background(), db))
		firstLedger := migrationLedger(t, db)
		require.NoError(t, ApplyMigrations(context.Background(), db))
		require.Equal(t, firstLedger, migrationLedger(t, db))
		assertCurrentMigrationLedger(t, db)
	})

	t.Run("Sub2API v0.1.183 database", func(t *testing.T) {
		db := newMigrationPathDatabase(t, "upstream")
		upstreamFS := activeMigrationFSBefore(t, 232)
		require.NoError(t, applyMigrationsFS(context.Background(), db, upstreamFS))
		seedMigrationCoreFixture(t, db, "upstream")
		before := readMigrationCoreInvariant(t, db)

		require.NoError(t, ApplyMigrations(context.Background(), db))
		require.Equal(t, before, readMigrationCoreInvariant(t, db))
		firstLedger := migrationLedger(t, db)
		require.NoError(t, ApplyMigrations(context.Background(), db))
		require.Equal(t, firstLedger, migrationLedger(t, db))
		assertCurrentMigrationLedger(t, db)
	})

	t.Run("ModelPort custom-v0.1.176.2 database", func(t *testing.T) {
		db := newMigrationPathDatabase(t, "legacy")
		legacyFS := legacyModelPortReleaseFS(t)
		require.NoError(t, applyMigrationsFS(context.Background(), db, legacyFS))
		seedMigrationCoreFixture(t, db, "legacy")
		seedLegacyModelPortFixture(t, db)
		beforeCore := readMigrationCoreInvariant(t, db)
		beforeCustom := readLegacyModelPortInvariant(t, db)
		assertLegacyMigrationLedger(t, db)

		require.NoError(t, ApplyMigrations(context.Background(), db))
		require.Equal(t, beforeCore, readMigrationCoreInvariant(t, db))
		require.Equal(t, beforeCustom, readLegacyModelPortInvariant(t, db))
		firstLedger := migrationLedger(t, db)
		require.NoError(t, ApplyMigrations(context.Background(), db))
		require.Equal(t, firstLedger, migrationLedger(t, db))
		assertCurrentMigrationLedger(t, db)
		assertLegacyMigrationLedger(t, db)
	})
}

func TestModelPortMigrationPathRejectsLedgerlessNonEmptyRestoreBeforeWrites(t *testing.T) {
	db := newMigrationPathDatabase(t, "ledgerless_nonempty")
	_, err := db.ExecContext(context.Background(), `
		CREATE TABLE users (
			id BIGINT PRIMARY KEY,
			email TEXT NOT NULL
		);
		INSERT INTO users (id, email) VALUES (41, 'ledgerless@example.invalid');
	`)
	require.NoError(t, err)

	err = ApplyMigrations(context.Background(), db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no public.schema_migrations")
	require.Contains(t, err.Error(), "ambiguous ledgerless restore")
	require.Contains(t, err.Error(), "users")

	var userCount int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&userCount))
	require.Equal(t, 1, userCount)
	assertMigrationPathTableAbsent(t, db, "schema_migrations")
	assertMigrationPathTableAbsent(t, db, "atlas_schema_revisions")
}

func TestModelPortMigrationPathWithOnlyLegacyMonitorMarkerPreservesStorageOnlyPlatforms(t *testing.T) {
	db := newMigrationPathDatabase(t, "legacy_monitor_marker")
	legacyFS := legacyModelPortReleaseFS(t)
	require.NoError(t, applyMigrationsFS(context.Background(), db, legacyFS))
	seedMigrationCoreFixture(t, db, "legacy-monitor-marker")

	var userID, groupID int64
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT id FROM users ORDER BY id LIMIT 1").Scan(&userID))
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT id FROM groups ORDER BY id LIMIT 1").Scan(&groupID))
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO user_platform_quotas (user_id, platform)
		VALUES ($1, 'openrouter')
	`, userID)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO composite_model_routes (
			group_id, public_model, target_platform, enabled
		) VALUES ($1, 'legacy-storage-only', 'openrouter', false)
	`, groupID)
	require.NoError(t, err)

	// Simulate a partially applied legacy release: 197 is authoritative for
	// monitor-provider compatibility, while its unrelated 188 ledger row is
	// absent. The runner must still bridge all terminal provider constraints.
	_, err = db.ExecContext(
		context.Background(),
		"DELETE FROM schema_migrations WHERE filename = $1",
		legacyModelPortOpenAICompatibleProvidersMigration,
	)
	require.NoError(t, err)

	require.NoError(t, ApplyMigrations(context.Background(), db))

	var preserved int
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM user_platform_quotas WHERE platform = 'openrouter') +
			(SELECT COUNT(*) FROM composite_model_routes WHERE target_platform = 'openrouter')
	`).Scan(&preserved))
	require.Equal(t, 2, preserved)

	for _, name := range []string{
		upstreamUserPlatformCNProvidersMigration,
		upstreamChannelMonitorQuotaModeMigration,
		upstreamCompositeRoutesCNProvidersMigration,
		modelPortLegacyPlatformConstraintsMigration,
	} {
		var applied bool
		require.NoError(t, db.QueryRowContext(context.Background(),
			"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)", name,
		).Scan(&applied))
		require.True(t, applied, name)
	}
}

func TestModelPortMigrationPathRejectsBadPartialUpstreamChecksumWithoutBridgeSideEffects(t *testing.T) {
	db := newMigrationPathDatabase(t, "legacy_bad_upstream_checksum")
	legacyFS := legacyModelPortReleaseFS(t)
	require.NoError(t, applyMigrationsFS(context.Background(), db, legacyFS))

	beforeConstraints := modelPortPlatformConstraintDefinitions(t, db)
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO schema_migrations (filename, checksum)
		VALUES ($1, 'wrong-checksum')
	`, upstreamUserPlatformCNProvidersMigration)
	require.NoError(t, err)

	err = ApplyMigrations(context.Background(), db)
	require.Error(t, err)
	require.Contains(t, err.Error(), upstreamUserPlatformCNProvidersMigration)
	require.Contains(t, err.Error(), "checksum mismatch")
	require.Equal(t, beforeConstraints, modelPortPlatformConstraintDefinitions(t, db))

	for _, name := range []string{
		upstreamCompositeRoutesCNProvidersMigration,
		modelPortLegacyPlatformConstraintsMigration,
	} {
		var applied bool
		require.NoError(t, db.QueryRowContext(context.Background(), `
			SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)
		`, name).Scan(&applied))
		require.False(t, applied, name)
	}

	var checksum string
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT checksum FROM schema_migrations WHERE filename = $1
	`, upstreamUserPlatformCNProvidersMigration).Scan(&checksum))
	require.Equal(t, "wrong-checksum", checksum)
}

func TestModelPortMigrationPathDefersLegacyBridgeUntilAllOtherMigrationsSucceed(t *testing.T) {
	db := newMigrationPathDatabase(t, "legacy_deferred_bridge_failure")
	legacyFS := legacyModelPortReleaseFS(t)
	require.NoError(t, applyMigrationsFS(context.Background(), db, legacyFS))
	seedMigrationCoreFixture(t, db, "legacy-deferred-bridge")

	beforeConstraints := modelPortPlatformConstraintDefinitions(t, db)
	currentFS := activeMigrationFS(t)
	// Force a migration after the platform bridge point to fail. The bridge and
	// its equivalent ledger rows must still be absent after the failed pass.
	currentFS["235_batch_image_group_snapshot.sql"] = &fstest.MapFile{Data: []byte("SELECT 1/0;")}

	err := applyMigrationsFS(context.Background(), db, currentFS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "235_batch_image_group_snapshot.sql")
	require.Equal(t, beforeConstraints, modelPortPlatformConstraintDefinitions(t, db))

	for _, name := range []string{
		upstreamUserPlatformCNProvidersMigration,
		upstreamChannelMonitorQuotaModeMigration,
		upstreamCompositeRoutesCNProvidersMigration,
		modelPortLegacyPlatformConstraintsMigration,
	} {
		var applied bool
		require.NoError(t, db.QueryRowContext(context.Background(),
			"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)", name,
		).Scan(&applied))
		require.False(t, applied, name)
	}
}

// A restore can contain the terminal 236 bridge row while the structural 226
// migration row is missing.  In that state 226 must still execute normally so
// its columns, setting, foreign key, and index are not silently skipped.
func TestModelPortMigrationPathRunsMissing226AfterApplied236(t *testing.T) {
	db := newMigrationPathDatabase(t, "legacy_236_without_226")
	legacyFS := legacyModelPortReleaseFS(t)
	require.NoError(t, applyMigrationsFS(context.Background(), db, legacyFS))

	bridgeContent, err := fs.ReadFile(dbmigrations.FS, modelPortLegacyPlatformConstraintsMigration)
	require.NoError(t, err)
	bridgeSQL := strings.TrimSpace(string(bridgeContent))
	_, err = db.ExecContext(context.Background(), bridgeSQL)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(),
		"INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)",
		modelPortLegacyPlatformConstraintsMigration,
		migrationContentChecksum(bridgeSQL),
	)
	require.NoError(t, err)

	assertMigrationPathColumnAbsent(t, db, "channel_monitors", "check_mode")
	assertMigrationPathColumnAbsent(t, db, "channel_monitors", "account_id")
	assertMigrationPathColumnAbsent(t, db, "channel_monitor_histories", "quota")
	assertMigrationPathSettingAbsent(t, db, "channel_monitor_show_quota")
	assertMigrationPathLedgerAbsent(t, db, upstreamChannelMonitorQuotaModeMigration)

	require.NoError(t, ApplyMigrations(context.Background(), db))

	assertMigrationPathColumnPresent(t, db, "channel_monitors", "check_mode")
	assertMigrationPathColumnPresent(t, db, "channel_monitors", "account_id")
	assertMigrationPathColumnPresent(t, db, "channel_monitor_histories", "quota")
	assertMigrationPathSettingPresent(t, db, "channel_monitor_show_quota")
	assertMigrationPathLedgerPresent(t, db, upstreamChannelMonitorQuotaModeMigration)
	assertMigrationPathConstraintPresent(t, db, "channel_monitors", "channel_monitors_check_mode_check")
	assertMigrationPathConstraintPresent(t, db, "channel_monitors", "channel_monitors_provider_check")
	assertMigrationPathIndexPresent(t, db, "idx_channel_monitors_account_id")
}

// Once 236 is recorded, its terminal provider constraints are authoritative.
// An altered constraint must fail during the read-only preflight, before any
// bridge DDL, migration-ledger row, Atlas baseline, or 226 index write.
func TestModelPortMigrationPathRejectsAlteredTerminalConstraintBeforeWrites(t *testing.T) {
	db := newMigrationPathDatabase(t, "legacy_altered_terminal_constraint")
	legacyFS := legacyModelPortReleaseFS(t)
	require.NoError(t, applyMigrationsFS(context.Background(), db, legacyFS))

	bridgeContent, err := fs.ReadFile(dbmigrations.FS, modelPortLegacyPlatformConstraintsMigration)
	require.NoError(t, err)
	bridgeSQL := strings.TrimSpace(string(bridgeContent))
	_, err = db.ExecContext(context.Background(), bridgeSQL)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(),
		"INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)",
		modelPortLegacyPlatformConstraintsMigration,
		migrationContentChecksum(bridgeSQL),
	)
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), `
		ALTER TABLE user_platform_quotas
		DROP CONSTRAINT user_platform_quotas_platform_check;
		ALTER TABLE user_platform_quotas
		ADD CONSTRAINT user_platform_quotas_platform_check
		CHECK (platform IN ('openai', 'anthropic'));
	`)
	require.NoError(t, err)

	beforeLedger := migrationLedger(t, db)
	beforeConstraint := migrationPathConstraintDefinition(t, db,
		"user_platform_quotas", "user_platform_quotas_platform_check")
	beforeAtlas := migrationPathAtlasRows(t, db)
	assertMigrationPathIndexAbsent(t, db, "idx_channel_monitors_account_id")
	assertMigrationPathSettingAbsent(t, db, "channel_monitor_show_quota")

	err = ApplyMigrations(context.Background(), db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "legacy ModelPort schema prevalidation")
	require.Contains(t, err.Error(), "user_platform_quotas_platform_check")

	require.Equal(t, beforeLedger, migrationLedger(t, db))
	require.Equal(t, beforeConstraint, migrationPathConstraintDefinition(t, db,
		"user_platform_quotas", "user_platform_quotas_platform_check"))
	require.Equal(t, beforeAtlas, migrationPathAtlasRows(t, db))
	assertMigrationPathIndexAbsent(t, db, "idx_channel_monitors_account_id")
	assertMigrationPathSettingAbsent(t, db, "channel_monitor_show_quota")
	assertMigrationPathLedgerAbsent(t, db, upstreamChannelMonitorQuotaModeMigration)
	assertMigrationPathLedgerAbsent(t, db, upstreamUserPlatformCNProvidersMigration)
	assertMigrationPathLedgerAbsent(t, db, upstreamCompositeRoutesCNProvidersMigration)
}

// The 226 provider guard must repair a partial list that happens to contain
// the old substring sentinel (kimi), rather than treating it as complete.
func TestChannelMonitorQuotaModeMigrationRepairsPartialProviderConstraints(t *testing.T) {
	db := newMigrationPathDatabase(t, "partial_226_provider_constraints")
	before226 := activeMigrationFSBefore(t, 226)
	require.NoError(t, applyMigrationsFS(context.Background(), db, before226))

	_, err := db.ExecContext(context.Background(), `
		ALTER TABLE channel_monitors
		DROP CONSTRAINT channel_monitors_provider_check;
		ALTER TABLE channel_monitors
		ADD CONSTRAINT channel_monitors_provider_check
		CHECK (provider IN ('openai', 'anthropic', 'gemini', 'kimi'));
		ALTER TABLE channel_monitor_request_templates
		DROP CONSTRAINT channel_monitor_request_templates_provider_check;
		ALTER TABLE channel_monitor_request_templates
		ADD CONSTRAINT channel_monitor_request_templates_provider_check
		CHECK (provider IN ('openai', 'anthropic', 'gemini', 'kimi'));
	`)
	require.NoError(t, err)

	quotaModeFS := fstest.MapFS{
		upstreamChannelMonitorQuotaModeMigration: &fstest.MapFile{
			Data: mustReadMigrationPathFile(t, upstreamChannelMonitorQuotaModeMigration),
		},
	}
	require.NoError(t, applyMigrationsFS(context.Background(), db, quotaModeFS))

	for _, target := range []struct {
		table string
		name  string
		col   string
	}{
		{table: "channel_monitors", name: "channel_monitors_provider_check", col: "provider"},
		{table: "channel_monitor_request_templates", name: "channel_monitor_request_templates_provider_check", col: "provider"},
	} {
		definition := migrationPathConstraintDefinition(t, db, target.table, target.name)
		literals, parseErr := parseLegacyModelPortConstraintLiterals(definition, target.col)
		require.NoError(t, parseErr)
		require.True(t, legacyModelPortStringSetEqual(literals, legacyModelPortUpstreamProviderPlatforms), definition)
	}
	assertMigrationPathLedgerPresent(t, db, upstreamChannelMonitorQuotaModeMigration)
}

// An exact provider literal set combined with a different column must not
// satisfy 226's idempotency guard. This models a named-constraint
// drift/restore spoof: the migration must inspect pg_constraint.conkey and
// rebuild the CHECK against provider rather than trusting the literals alone.
func TestChannelMonitorQuotaModeMigrationRepairsWrongColumnProviderConstraints(t *testing.T) {
	db := newMigrationPathDatabase(t, "wrong_column_226_provider_constraints")
	before226 := activeMigrationFSBefore(t, 226)
	require.NoError(t, applyMigrationsFS(context.Background(), db, before226))

	const providerLiterals = "'openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek'"
	_, err := db.ExecContext(context.Background(), `
		ALTER TABLE channel_monitors
		DROP CONSTRAINT channel_monitors_provider_check;
		ALTER TABLE channel_monitors
		ADD CONSTRAINT channel_monitors_provider_check
		CHECK (provider IN (`+providerLiterals+`) AND (id IS NULL OR id IS NOT NULL));
		ALTER TABLE channel_monitor_request_templates
		DROP CONSTRAINT channel_monitor_request_templates_provider_check;
		ALTER TABLE channel_monitor_request_templates
		ADD CONSTRAINT channel_monitor_request_templates_provider_check
		CHECK (provider IN (`+providerLiterals+`) AND (id IS NULL OR id IS NOT NULL));
	`)
	require.NoError(t, err)

	quotaModeFS := fstest.MapFS{
		upstreamChannelMonitorQuotaModeMigration: &fstest.MapFile{
			Data: mustReadMigrationPathFile(t, upstreamChannelMonitorQuotaModeMigration),
		},
	}
	require.NoError(t, applyMigrationsFS(context.Background(), db, quotaModeFS))

	for _, target := range []struct {
		table string
		name  string
		col   string
	}{
		{table: "channel_monitors", name: "channel_monitors_provider_check", col: "provider"},
		{table: "channel_monitor_request_templates", name: "channel_monitor_request_templates_provider_check", col: "provider"},
	} {
		definition := migrationPathConstraintDefinition(t, db, target.table, target.name)
		literals, parseErr := parseLegacyModelPortConstraintLiterals(definition, target.col)
		require.NoError(t, parseErr)
		require.True(t, legacyModelPortStringSetEqual(literals, legacyModelPortUpstreamProviderPlatforms), definition)
	}
}

func mustReadMigrationPathFile(t *testing.T, name string) []byte {
	t.Helper()
	content, err := fs.ReadFile(dbmigrations.FS, name)
	require.NoError(t, err)
	return content
}

func migrationPathConstraintDefinition(t *testing.T, db *sql.DB, table, name string) string {
	t.Helper()
	var definition string
	err := db.QueryRowContext(context.Background(), `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = 'public' AND t.relname = $1 AND c.conname = $2
	`, table, name).Scan(&definition)
	require.NoError(t, err)
	return definition
}

func assertMigrationPathColumnPresent(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)
	`, table, column).Scan(&present))
	require.True(t, present, "%s.%s", table, column)
}

func assertMigrationPathTableAbsent(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	present, err := tableExists(context.Background(), db, table)
	require.NoError(t, err)
	require.False(t, present, table)
}

func assertMigrationPathColumnAbsent(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)
	`, table, column).Scan(&present))
	require.False(t, present, "%s.%s", table, column)
}

func assertMigrationPathSettingPresent(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM settings WHERE key = $1)", key,
	).Scan(&present))
	require.True(t, present, key)
}

func assertMigrationPathSettingAbsent(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM settings WHERE key = $1)", key,
	).Scan(&present))
	require.False(t, present, key)
}

func assertMigrationPathLedgerPresent(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)", name,
	).Scan(&present))
	require.True(t, present, name)
}

func assertMigrationPathLedgerAbsent(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)", name,
	).Scan(&present))
	require.False(t, present, name)
}

func assertMigrationPathIndexPresent(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1)", name,
	).Scan(&present))
	require.True(t, present, name)
}

func assertMigrationPathIndexAbsent(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1)", name,
	).Scan(&present))
	require.False(t, present, name)
}

func assertMigrationPathConstraintPresent(t *testing.T, db *sql.DB, table, name string) {
	t.Helper()
	var present bool
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			WHERE n.nspname = 'public' AND t.relname = $1 AND c.conname = $2
		)
	`, table, name).Scan(&present))
	require.True(t, present, "%s.%s", table, name)
}

func migrationPathAtlasRows(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT version, description, type, applied, total, hash
		FROM atlas_schema_revisions
		ORDER BY version
	`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	result := make([]string, 0)
	for rows.Next() {
		var version, description, hash string
		var migrationType, applied, total int
		require.NoError(t, rows.Scan(&version, &description, &migrationType, &applied, &total, &hash))
		result = append(result, fmt.Sprintf("%s\t%s\t%d\t%d\t%d\t%s", version, description, migrationType, applied, total, hash))
	}
	require.NoError(t, rows.Err())
	return result
}

func modelPortPlatformConstraintDefinitions(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT t.relname || '.' || c.conname || '=' || pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = 'public'
		  AND (t.relname, c.conname) IN (
			('user_platform_quotas', 'user_platform_quotas_platform_check'),
			('composite_model_routes', 'composite_model_routes_target_platform_check'),
			('channel_monitors', 'channel_monitors_provider_check'),
			('channel_monitor_request_templates', 'channel_monitor_request_templates_provider_check')
		  )
		ORDER BY t.relname, c.conname
	`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	definitions := make([]string, 0, 4)
	for rows.Next() {
		var definition string
		require.NoError(t, rows.Scan(&definition))
		definitions = append(definitions, definition)
	}
	require.NoError(t, rows.Err())
	require.Len(t, definitions, 4)
	return definitions
}

func newMigrationPathDatabase(t *testing.T, label string) *sql.DB {
	t.Helper()
	require.NotEmpty(t, integrationPostgresDSN)

	name := fmt.Sprintf("modelport_path_%s_%d", label, time.Now().UnixNano())
	quotedName := pq.QuoteIdentifier(name)
	_, err := integrationDB.ExecContext(context.Background(), "CREATE DATABASE "+quotedName)
	require.NoError(t, err)

	parsed, err := url.Parse(integrationPostgresDSN)
	require.NoError(t, err)
	parsed.Path = "/" + name
	parsed.RawPath = ""
	db, err := openSQLWithRetry(context.Background(), parsed.String(), 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		_, dropErr := integrationDB.ExecContext(
			context.Background(),
			"DROP DATABASE IF EXISTS "+quotedName+" WITH (FORCE)",
		)
		require.NoError(t, dropErr)
	})
	return db
}

func activeMigrationFSBefore(t *testing.T, exclusiveUpperBound int) fstest.MapFS {
	t.Helper()
	names, err := fs.Glob(dbmigrations.FS, "*.sql")
	require.NoError(t, err)
	out := make(fstest.MapFS)
	for _, name := range names {
		if migrationNumber(t, name) >= exclusiveUpperBound {
			continue
		}
		content, readErr := fs.ReadFile(dbmigrations.FS, name)
		require.NoError(t, readErr)
		out[name] = &fstest.MapFile{Data: content}
	}
	require.NotEmpty(t, out)
	return out
}

func activeMigrationFS(t *testing.T) fstest.MapFS {
	t.Helper()
	names, err := fs.Glob(dbmigrations.FS, "*.sql")
	require.NoError(t, err)
	out := make(fstest.MapFS, len(names))
	for _, name := range names {
		content, readErr := fs.ReadFile(dbmigrations.FS, name)
		require.NoError(t, readErr)
		out[name] = &fstest.MapFile{Data: content}
	}
	return out
}

func legacyModelPortReleaseFS(t *testing.T) fstest.MapFS {
	t.Helper()
	// custom-v0.1.176.2 contained the upstream migrations through 220 plus
	// ModelPort's archived 187-224 files. Shared files are byte-identical to the
	// current archive; custom files come from the immutable LegacyFS manifest.
	out := activeMigrationFSBefore(t, 221)
	entries, err := fs.ReadDir(dbmigrations.LegacyFS, legacyModelPortMigrationRoot)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		_, duplicate := out[entry.Name()]
		require.Falsef(t, duplicate, "duplicate legacy migration filename %s", entry.Name())
		content, readErr := fs.ReadFile(
			dbmigrations.LegacyFS,
			legacyModelPortMigrationRoot+"/"+entry.Name(),
		)
		require.NoError(t, readErr)
		out[entry.Name()] = &fstest.MapFile{Data: content}
	}
	return out
}

func migrationNumber(t *testing.T, name string) int {
	t.Helper()
	prefixEnd := 0
	for prefixEnd < len(name) && name[prefixEnd] >= '0' && name[prefixEnd] <= '9' {
		prefixEnd++
	}
	require.Greater(t, prefixEnd, 0, "migration filename must start with a numeric prefix: %s", name)
	number, err := strconv.Atoi(name[:prefixEnd])
	require.NoError(t, err)
	return number
}

type migrationCoreInvariant struct {
	Users           int64
	Balance         string
	Groups          int64
	Accounts        int64
	APIKeys         int64
	UsageLogs       int64
	UsageTotalCost  string
	UsageActualCost string
	UserSequence    string
	GroupSequence   string
	AccountSequence string
	APIKeySequence  string
	UsageSequence   string
}

func seedMigrationCoreFixture(t *testing.T, db *sql.DB, label string) {
	t.Helper()
	ctx := context.Background()
	var userID, groupID, accountID, apiKeyID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, role, status, balance, concurrency)
		VALUES ($1, 'fixture-hash', 'user', 'active', 123.45678901, 7)
		RETURNING id
	`, "migration-"+label+"@example.invalid").Scan(&userID))
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO groups (name, status, subscription_type)
		VALUES ($1, 'active', 'standard')
		RETURNING id
	`, "migration-"+label).Scan(&groupID))
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO accounts (name, platform, type, status, extra)
		VALUES ($1, 'openai', 'oauth', 'active', '{}'::jsonb)
		RETURNING id
	`, "migration-"+label).Scan(&accountID))
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO api_keys (user_id, key, name, group_id, status)
		VALUES ($1, $2, 'migration fixture', $3, 'active')
		RETURNING id
	`, userID, "sk-migration-"+label, groupID).Scan(&apiKeyID))
	_, err := db.ExecContext(ctx, `
		INSERT INTO usage_logs (
			user_id, api_key_id, account_id, model, input_tokens, output_tokens,
			total_cost, actual_cost, created_at
		) VALUES ($1, $2, $3, 'gpt-5.6', 11, 13, 0.1234567890, 0.1200000000, NOW())
	`, userID, apiKeyID, accountID)
	require.NoError(t, err)
}

func readMigrationCoreInvariant(t *testing.T, db *sql.DB) migrationCoreInvariant {
	t.Helper()
	var got migrationCoreInvariant
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COALESCE(SUM(balance), 0)::text FROM users),
			(SELECT COUNT(*) FROM groups),
			(SELECT COUNT(*) FROM accounts),
			(SELECT COUNT(*) FROM api_keys),
			(SELECT COUNT(*) FROM usage_logs),
			(SELECT COALESCE(SUM(total_cost), 0)::text FROM usage_logs),
			(SELECT COALESCE(SUM(actual_cost), 0)::text FROM usage_logs),
			(SELECT last_value::text || ':' || is_called::text FROM users_id_seq),
			(SELECT last_value::text || ':' || is_called::text FROM groups_id_seq),
			(SELECT last_value::text || ':' || is_called::text FROM accounts_id_seq),
			(SELECT last_value::text || ':' || is_called::text FROM api_keys_id_seq),
			(SELECT last_value::text || ':' || is_called::text FROM usage_logs_id_seq)
	`).Scan(
		&got.Users,
		&got.Balance,
		&got.Groups,
		&got.Accounts,
		&got.APIKeys,
		&got.UsageLogs,
		&got.UsageTotalCost,
		&got.UsageActualCost,
		&got.UserSequence,
		&got.GroupSequence,
		&got.AccountSequence,
		&got.APIKeySequence,
		&got.UsageSequence,
	))
	return got
}

type legacyModelPortInvariant struct {
	FreeGroups       int64
	LotteryCampaigns int64
	LotteryEntries   int64
	VaultRows        int64
	VaultDigest      string
	VaultCiphertext  string
	VaultSequence    string
}

func seedLegacyModelPortFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	var userID, groupID, campaignID int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&userID))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM groups ORDER BY id LIMIT 1`).Scan(&groupID))
	_, err := db.ExecContext(ctx, `UPDATE groups SET is_free = TRUE WHERE id = $1`, groupID)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO lottery_campaigns (
			name, mode, status, starts_at, ends_at, per_user_limit, created_by, updated_by
		) VALUES (
			'migration legacy campaign', 'instant', 'active', NOW() - INTERVAL '1 hour',
			NOW() + INTERVAL '1 hour', 1, $1, $1
		) RETURNING id
	`, userID).Scan(&campaignID))
	_, err = db.ExecContext(ctx, `
		INSERT INTO lottery_entries (campaign_id, user_id, idempotency_key)
		VALUES ($1, $2, 'migration-legacy-entry')
	`, campaignID, userID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO instruction_audit_v2_content_vault (
			sha256, raw_ciphertext, content_bytes, stored_bytes, observed_field,
			encryption_key_version
		) VALUES (
			repeat('a', 64), decode('00112233445566778899aabbccddeeff', 'hex'),
			16, 16, 'instructions', 'instruction-evidence-v1'
		)
	`)
	require.NoError(t, err)
}

func readLegacyModelPortInvariant(t *testing.T, db *sql.DB) legacyModelPortInvariant {
	t.Helper()
	var got legacyModelPortInvariant
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM groups WHERE is_free),
			(SELECT COUNT(*) FROM lottery_campaigns),
			(SELECT COUNT(*) FROM lottery_entries),
			(SELECT COUNT(*) FROM instruction_audit_v2_content_vault),
			(SELECT sha256 FROM instruction_audit_v2_content_vault ORDER BY id LIMIT 1),
			(SELECT encode(raw_ciphertext, 'hex') FROM instruction_audit_v2_content_vault ORDER BY id LIMIT 1),
			(SELECT last_value::text || ':' || is_called::text FROM instruction_audit_v2_content_vault_id_seq)
	`).Scan(
		&got.FreeGroups,
		&got.LotteryCampaigns,
		&got.LotteryEntries,
		&got.VaultRows,
		&got.VaultDigest,
		&got.VaultCiphertext,
		&got.VaultSequence,
	))
	return got
}

func migrationLedger(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT filename, checksum FROM schema_migrations ORDER BY filename
	`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	ledger := make([]string, 0)
	for rows.Next() {
		var filename, checksum string
		require.NoError(t, rows.Scan(&filename, &checksum))
		ledger = append(ledger, filename+"\t"+checksum)
	}
	require.NoError(t, rows.Err())
	return ledger
}

func assertCurrentMigrationLedger(t *testing.T, db *sql.DB) {
	t.Helper()
	names, err := fs.Glob(dbmigrations.FS, "*.sql")
	require.NoError(t, err)
	sort.Strings(names)
	for _, name := range names {
		content, readErr := fs.ReadFile(dbmigrations.FS, name)
		require.NoError(t, readErr)
		expected := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		var actual string
		require.NoError(t, db.QueryRowContext(
			context.Background(),
			"SELECT checksum FROM schema_migrations WHERE filename = $1",
			name,
		).Scan(&actual))
		require.Equal(t, hex.EncodeToString(expected[:]), actual, name)
	}
}

func assertLegacyMigrationLedger(t *testing.T, db *sql.DB) {
	t.Helper()
	entries, err := fs.ReadDir(dbmigrations.LegacyFS, legacyModelPortMigrationRoot)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		expected, checksumErr := legacyModelPortArchivedMigrationChecksum(entry.Name())
		require.NoError(t, checksumErr)
		var actual string
		require.NoError(t, db.QueryRowContext(
			context.Background(),
			"SELECT checksum FROM schema_migrations WHERE filename = $1",
			entry.Name(),
		).Scan(&actual))
		require.Equal(t, expected, actual, entry.Name())
	}
}
