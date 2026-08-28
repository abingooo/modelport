//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// TestMigration232BackfillsOnlyUnsetFreeBillingValues exercises the bridge on
// a legacy-shaped schema. Existing explicit TRUE/FALSE values must remain
// unchanged; only NULL values are converted to the paid default.
func TestMigration232BackfillsOnlyUnsetFreeBillingValues(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	_, err := tx.ExecContext(ctx, `
CREATE SCHEMA migration_232_backfill_test;
SET LOCAL search_path TO migration_232_backfill_test;
CREATE TABLE groups (
    id BIGSERIAL PRIMARY KEY,
    is_free BOOLEAN
);
CREATE TABLE batch_image_jobs (
    batch_id VARCHAR(64) PRIMARY KEY,
    is_free_billing BOOLEAN
);
INSERT INTO groups (is_free) VALUES (TRUE), (NULL), (FALSE);
INSERT INTO batch_image_jobs (batch_id, is_free_billing)
VALUES ('free', TRUE), ('unset', NULL), ('paid', FALSE);
`)
	require.NoError(t, err)

	migration, err := dbmigrations.FS.ReadFile("232_modelport_free_group_bridge.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	assertFreeFlagAuthCacheTriggerCondition(t, tx)
	// The bridge is intentionally rerunnable for operators retrying a failed
	// deployment or upgrading a database that already contains the columns.
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	require.Equal(t, []bool{true, false, false}, queryBoolColumn(t, tx,
		`SELECT is_free FROM groups ORDER BY id`))
	require.Equal(t, []bool{true, false, false}, queryBoolColumn(t, tx,
		`SELECT is_free_billing FROM batch_image_jobs ORDER BY batch_id`))

	assertBooleanColumnRequiredWithFalseDefault(t, tx, "groups", "is_free")
	assertBooleanColumnRequiredWithFalseDefault(t, tx, "batch_image_jobs", "is_free_billing")
	assertNullInsertRejected(t, tx, `INSERT INTO groups (is_free) VALUES (NULL)`)
	assertNullInsertRejected(t, tx,
		`INSERT INTO batch_image_jobs (batch_id, is_free_billing) VALUES ('null-insert', NULL)`)

	// Omitted values use the paid default after the bridge.
	_, err = tx.ExecContext(ctx, `INSERT INTO groups DEFAULT VALUES`)
	require.NoError(t, err)
	var groupDefault bool
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT is_free FROM groups ORDER BY id DESC LIMIT 1`,
	).Scan(&groupDefault))
	require.False(t, groupDefault)

	_, err = tx.ExecContext(ctx, `INSERT INTO batch_image_jobs (batch_id) VALUES ('defaulted')`)
	require.NoError(t, err)
	var batchDefault bool
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT is_free_billing FROM batch_image_jobs WHERE batch_id = 'defaulted'`,
	).Scan(&batchDefault))
	require.False(t, batchDefault)
}

// TestMigration232AddsAndBackfillsFreeBillingColumns covers a clean
// Sub2API-shaped schema where neither bridge column exists yet. It also
// verifies that old batch jobs are treated as paid when the snapshot is added.
func TestMigration232AddsAndBackfillsFreeBillingColumns(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	_, err := tx.ExecContext(ctx, `
CREATE SCHEMA migration_232_add_test;
SET LOCAL search_path TO migration_232_add_test;
CREATE TABLE groups (id BIGSERIAL PRIMARY KEY);
CREATE TABLE batch_image_jobs (batch_id VARCHAR(64) PRIMARY KEY);
INSERT INTO groups DEFAULT VALUES;
INSERT INTO batch_image_jobs (batch_id) VALUES ('legacy-job');
`)
	require.NoError(t, err)

	migration, err := dbmigrations.FS.ReadFile("232_modelport_free_group_bridge.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	var groupFree, batchFree bool
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT is_free FROM groups LIMIT 1`).Scan(&groupFree))
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT is_free_billing FROM batch_image_jobs WHERE batch_id = 'legacy-job'`,
	).Scan(&batchFree))
	require.False(t, groupFree)
	require.False(t, batchFree)
	assertBooleanColumnRequiredWithFalseDefault(t, tx, "groups", "is_free")
	assertBooleanColumnRequiredWithFalseDefault(t, tx, "batch_image_jobs", "is_free_billing")
}

func queryBoolColumn(t *testing.T, tx *sql.Tx, query string) []bool {
	t.Helper()
	rows, err := tx.QueryContext(context.Background(), query)
	require.NoError(t, err)
	defer rows.Close()

	var values []bool
	for rows.Next() {
		var value bool
		require.NoError(t, rows.Scan(&value))
		values = append(values, value)
	}
	require.NoError(t, rows.Err())
	return values
}

func assertBooleanColumnRequiredWithFalseDefault(t *testing.T, tx *sql.Tx, table, column string) {
	t.Helper()
	var nullable string
	var defaultValue sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), `
SELECT is_nullable, column_default
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2
`, table, column).Scan(&nullable, &defaultValue))
	require.Equal(t, "NO", nullable, "%s.%s must reject NULL", table, column)
	require.True(t, defaultValue.Valid)
	require.Contains(t, strings.ToLower(defaultValue.String), "false")
}

func assertFreeFlagAuthCacheTriggerCondition(t *testing.T, tx *sql.Tx) {
	t.Helper()
	var functionDefinition string
	err := tx.QueryRowContext(context.Background(),
		`SELECT pg_get_functiondef('enqueue_group_auth_cache_invalidation()'::regprocedure)`,
	).Scan(&functionDefinition)
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(functionDefinition),
		"old.is_free is not distinct from new.is_free",
		"auth-cache invalidation must react to free-mode changes")
}

// A failed INSERT aborts the current PostgreSQL transaction. Savepoints keep
// the remainder of this invariant test usable while still asserting rejection.
func assertNullInsertRejected(t *testing.T, tx *sql.Tx, query string) {
	t.Helper()
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `SAVEPOINT migration_232_null_insert`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, query)
	require.Error(t, err)
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT migration_232_null_insert`)
	require.NoError(t, err)
}
