//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestModelPortLegacyPlatformBridgePreservesRowsAndAcceptsCurrentProviders(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	schema := pq.QuoteIdentifier(fmt.Sprintf("modelport_platform_bridge_%d", time.Now().UnixNano()))
	_, err := tx.ExecContext(ctx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema)
	require.NoError(t, err)

	createLegacyPlatformBridgeTables(t, ctx, tx)

	_, err = tx.ExecContext(ctx, `
	INSERT INTO user_platform_quotas (platform) VALUES ('qwen'), ('openrouter');
	INSERT INTO composite_model_routes (target_platform, enabled) VALUES ('glm', false), ('siliconflow', false);
	INSERT INTO channel_monitors (provider) VALUES ('qwen'), ('mimo');
	INSERT INTO channel_monitor_request_templates (provider) VALUES ('glm'), ('doubao');
	`)
	require.NoError(t, err)

	unknown, err := legacyModelPortUnknownPlatformValues(ctx, tx)
	require.NoError(t, err)
	require.Empty(t, unknown)

	bridge, err := dbmigrations.FS.ReadFile(modelPortLegacyPlatformConstraintsMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(bridge))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(bridge))
	require.NoError(t, err, "final-state bridge must be idempotent")

	for _, statement := range []string{
		"INSERT INTO user_platform_quotas (platform) VALUES ('zhipu')",
		"INSERT INTO composite_model_routes (target_platform, enabled) VALUES ('zhipu', true)",
		"INSERT INTO channel_monitors (provider) VALUES ('antigravity')",
		"INSERT INTO channel_monitor_request_templates (provider) VALUES ('zhipu')",
	} {
		_, err = tx.ExecContext(ctx, statement)
		require.NoError(t, err)
	}

	var preserved int
	require.NoError(t, tx.QueryRowContext(ctx, `
	SELECT
	    (SELECT COUNT(*) FROM user_platform_quotas WHERE platform IN ('qwen', 'openrouter')) +
	    (SELECT COUNT(*) FROM composite_model_routes WHERE target_platform IN ('glm', 'siliconflow')) +
	    (SELECT COUNT(*) FROM channel_monitors WHERE provider IN ('qwen', 'mimo')) +
	    (SELECT COUNT(*) FROM channel_monitor_request_templates WHERE provider IN ('glm', 'doubao'))
	`).Scan(&preserved))
	require.Equal(t, 8, preserved)
}

func TestModelPortLegacyPlatformBridgeRejectsExecutableRemovedProviders(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	schema := pq.QuoteIdentifier(fmt.Sprintf("modelport_platform_block_%d", time.Now().UnixNano()))
	_, err := tx.ExecContext(ctx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema)
	require.NoError(t, err)
	createLegacyPlatformBridgeTables(t, ctx, tx)

	_, err = tx.ExecContext(ctx, `
	INSERT INTO accounts (platform) VALUES ('qwen');
	INSERT INTO groups (platform) VALUES ('glm');
	INSERT INTO composite_model_routes (target_platform, enabled) VALUES ('doubao', true);
	INSERT INTO user_platform_quotas (platform, daily_limit_usd) VALUES ('openrouter', 10);
	`)
	require.NoError(t, err)

	bridge, err := dbmigrations.FS.ReadFile(modelPortLegacyPlatformConstraintsMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "SAVEPOINT before_modelport_platform_bridge")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(bridge))
	require.Error(t, err)
	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr)
	require.Equal(t, "legacy ModelPort provider configuration blocks migration", pqErr.Message)
	for _, blocked := range []string{
		"accounts.platform='qwen'",
		"groups.platform='glm'",
		"composite_model_routes.target_platform='doubao'",
		"user_platform_quotas.platform='openrouter'",
	} {
		require.Contains(t, pqErr.Detail, blocked)
	}
	require.Contains(t, pqErr.Hint, "will not rename, delete, or disable data")

	_, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT before_modelport_platform_bridge")
	require.NoError(t, rollbackErr)
	var accountCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE platform = 'qwen'").Scan(&accountCount))
	require.Equal(t, 1, accountCount, "failed migration must preserve source rows")
}

func TestModelPortLegacyPlatformBridgeAllowsStorageOnlyRemovedProviders(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	schema := pq.QuoteIdentifier(fmt.Sprintf("modelport_platform_storage_%d", time.Now().UnixNano()))
	_, err := tx.ExecContext(ctx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema)
	require.NoError(t, err)
	createLegacyPlatformBridgeTables(t, ctx, tx)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO accounts (platform, status) VALUES ('qwen', 'disabled');
		INSERT INTO groups (platform, status) VALUES ('glm', 'active');
		UPDATE groups SET deleted_at = NOW();
		INSERT INTO composite_model_routes (target_platform, enabled) VALUES ('doubao', false);
		INSERT INTO composite_model_routes (target_platform, enabled, deleted_at) VALUES ('minimax', true, NOW());
		INSERT INTO user_platform_quotas (platform) VALUES ('openrouter');
		INSERT INTO user_platform_quotas (platform, daily_limit_usd, daily_usage_usd) VALUES ('mimo', 0, 0);
		UPDATE user_platform_quotas SET deleted_at = NOW() WHERE platform = 'mimo';
	`)
	require.NoError(t, err)

	blocked, err := legacyModelPortPlatformValues(ctx, tx, legacyModelPortBlockedPlatformValuesQuery)
	require.NoError(t, err)
	require.Empty(t, blocked)

	bridge, err := dbmigrations.FS.ReadFile(modelPortLegacyPlatformConstraintsMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(bridge))
	require.NoError(t, err)

	unknown, err := legacyModelPortUnknownPlatformValues(ctx, tx)
	require.NoError(t, err)
	require.Empty(t, unknown)
}

func TestModelPortLegacyPlatformPreflightRejectsUnknownAccountAndGroupPlatforms(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	schema := pq.QuoteIdentifier(fmt.Sprintf("modelport_platform_unknown_%d", time.Now().UnixNano()))
	_, err := tx.ExecContext(ctx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema)
	require.NoError(t, err)
	createLegacyPlatformBridgeTables(t, ctx, tx)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO accounts (platform) VALUES ('future-account-provider');
		INSERT INTO groups (platform) VALUES ('future-group-provider');
	`)
	require.NoError(t, err)

	unknown, err := legacyModelPortUnknownPlatformValues(ctx, tx)
	require.NoError(t, err)
	require.Equal(t, []string{
		"accounts.platform=future-account-provider",
		"groups.platform=future-group-provider",
	}, unknown)
}

func createLegacyPlatformBridgeTables(t *testing.T, ctx context.Context, tx *sql.Tx) {
	t.Helper()
	_, err := tx.ExecContext(ctx, `
	CREATE TABLE accounts (
	    id BIGSERIAL PRIMARY KEY,
	    platform VARCHAR(32) NOT NULL,
	    status VARCHAR(20) NOT NULL DEFAULT 'active',
	    deleted_at TIMESTAMPTZ
	);
	CREATE TABLE groups (
	    id BIGSERIAL PRIMARY KEY,
	    platform VARCHAR(32) NOT NULL,
	    status VARCHAR(20) NOT NULL DEFAULT 'active',
	    deleted_at TIMESTAMPTZ
	);
	CREATE TABLE user_platform_quotas (
	    id BIGSERIAL PRIMARY KEY,
	    platform VARCHAR(32) NOT NULL,
	    daily_limit_usd DECIMAL(20, 10),
	    weekly_limit_usd DECIMAL(20, 10),
	    monthly_limit_usd DECIMAL(20, 10),
	    daily_usage_usd DECIMAL(20, 10) NOT NULL DEFAULT 0,
	    weekly_usage_usd DECIMAL(20, 10) NOT NULL DEFAULT 0,
	    monthly_usage_usd DECIMAL(20, 10) NOT NULL DEFAULT 0,
	    deleted_at TIMESTAMPTZ,
	    CONSTRAINT user_platform_quotas_platform_check CHECK (
        platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'deepseek',
                     'qwen', 'glm', 'kimi', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo')
    )
);
	CREATE TABLE composite_model_routes (
	    id BIGSERIAL PRIMARY KEY,
	    target_platform VARCHAR(32) NOT NULL,
	    enabled BOOLEAN NOT NULL DEFAULT true,
	    deleted_at TIMESTAMPTZ,
    CONSTRAINT composite_model_routes_target_platform_check CHECK (
        target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'deepseek',
                            'qwen', 'glm', 'kimi', 'doubao', 'siliconflow', 'openrouter', 'minimax', 'mimo')
    )
);
CREATE TABLE channel_monitors (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    CONSTRAINT channel_monitors_provider_check CHECK (
        provider IN ('openai', 'anthropic', 'gemini', 'grok', 'deepseek',
                     'qwen', 'glm', 'kimi', 'doubao', 'minimax', 'mimo')
    )
);
	CREATE TABLE channel_monitor_request_templates (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    CONSTRAINT channel_monitor_request_templates_provider_check CHECK (
        provider IN ('openai', 'anthropic', 'gemini', 'grok', 'deepseek',
                     'qwen', 'glm', 'kimi', 'doubao', 'minimax', 'mimo')
	    )
	);
	`)
	require.NoError(t, err)
}
