//go:build integration

package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration188ExtendsOpenAICompatiblePlatformConstraints(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx, `
CREATE SCHEMA migration_188_test;
SET LOCAL search_path TO migration_188_test;

CREATE TABLE user_platform_quotas (
    platform text NOT NULL,
    marker text NOT NULL,
    CONSTRAINT user_platform_quotas_platform_check
        CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok'))
);

CREATE TABLE composite_model_routes (
    target_platform text NOT NULL,
    marker text NOT NULL,
    CONSTRAINT composite_model_routes_target_platform_check
        CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok'))
);
`)
	require.NoError(t, err)

	migration187, err := dbmigrations.FS.ReadFile("187_add_deepseek_platform.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration187))
	require.NoError(t, err)

	pre188Platforms := []string{"anthropic", "openai", "gemini", "antigravity", "grok", "deepseek"}
	for _, platform := range pre188Platforms {
		_, err = tx.ExecContext(ctx, `INSERT INTO user_platform_quotas (platform, marker) VALUES ($1, $2)`, platform, "before-188-"+platform)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO composite_model_routes (target_platform, marker) VALUES ($1, $2)`, platform, "before-188-"+platform)
		require.NoError(t, err)
	}

	migration188, err := dbmigrations.FS.ReadFile("188_add_openai_compatible_providers.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration188))
	require.NoError(t, err)

	assertMigration188RowsPreserved(t, ctx, tx, "user_platform_quotas", "platform", pre188Platforms)
	assertMigration188RowsPreserved(t, ctx, tx, "composite_model_routes", "target_platform", pre188Platforms)

	newPlatforms := []string{"qwen", "glm", "kimi", "doubao", "siliconflow", "openrouter", "minimax", "mimo"}
	for _, platform := range newPlatforms {
		_, err = tx.ExecContext(ctx, `INSERT INTO user_platform_quotas (platform, marker) VALUES ($1, $2)`, platform, "after-188-"+platform)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO composite_model_routes (target_platform, marker) VALUES ($1, $2)`, platform, "after-188-"+platform)
		require.NoError(t, err)
	}

	assertMigration188InvalidPlatformRejected(
		t, ctx, tx, "invalid_quota_platform",
		`INSERT INTO user_platform_quotas (platform, marker) VALUES ('invalid-provider', 'invalid')`,
		"user_platform_quotas_platform_check",
	)
	assertMigration188InvalidPlatformRejected(
		t, ctx, tx, "invalid_route_platform",
		`INSERT INTO composite_model_routes (target_platform, marker) VALUES ('invalid-provider', 'invalid')`,
		"composite_model_routes_target_platform_check",
	)

	expectedPlatforms := append(append([]string(nil), pre188Platforms...), newPlatforms...)
	assertMigration188Constraint(t, ctx, tx, "user_platform_quotas", "user_platform_quotas_platform_check", expectedPlatforms)
	assertMigration188Constraint(t, ctx, tx, "composite_model_routes", "composite_model_routes_target_platform_check", expectedPlatforms)
}

func assertMigration188RowsPreserved(t *testing.T, ctx context.Context, tx *sql.Tx, table, column string, expected []string) {
	t.Helper()

	rows, err := tx.QueryContext(ctx, `SELECT `+column+` FROM `+table+` WHERE marker LIKE 'before-188-%' ORDER BY `+column)
	require.NoError(t, err)
	defer rows.Close()

	var actual []string
	for rows.Next() {
		var platform string
		require.NoError(t, rows.Scan(&platform))
		actual = append(actual, platform)
	}
	require.NoError(t, rows.Err())
	require.ElementsMatch(t, expected, actual)
}

func assertMigration188InvalidPlatformRejected(t *testing.T, ctx context.Context, tx *sql.Tx, savepoint, statement, constraintName string) {
	t.Helper()

	_, err := tx.ExecContext(ctx, "SAVEPOINT "+savepoint)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, statement)
	require.ErrorContains(t, err, constraintName)
	_, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint)
	require.NoError(t, rollbackErr)
}

func assertMigration188Constraint(t *testing.T, ctx context.Context, tx *sql.Tx, table, constraintName string, expected []string) {
	t.Helper()

	var constraintType string
	var validated bool
	var definition string
	err := tx.QueryRowContext(ctx, `
SELECT con.contype::text, con.convalidated, pg_get_constraintdef(con.oid)
FROM pg_constraint AS con
JOIN pg_class AS relation ON relation.oid = con.conrelid
JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace
WHERE namespace.nspname = current_schema()
  AND relation.relname = $1
  AND con.conname = $2
`, table, constraintName).Scan(&constraintType, &validated, &definition)
	require.NoError(t, err)
	require.Equal(t, "c", constraintType)
	require.True(t, validated)

	literalPattern := regexp.MustCompile(`'([^']+)'::text`)
	matches := literalPattern.FindAllStringSubmatch(definition, -1)
	actual := make([]string, 0, len(matches))
	for _, match := range matches {
		actual = append(actual, match[1])
	}
	require.Equal(t, expected, actual)
}
