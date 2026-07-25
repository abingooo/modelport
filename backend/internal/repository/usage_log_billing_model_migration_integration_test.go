//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration189AddsWritableBillingModelWithoutChangingExistingRows(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx, `
CREATE SCHEMA migration_189_test;
SET LOCAL search_path TO migration_189_test;

CREATE TABLE usage_logs (
    id BIGSERIAL PRIMARY KEY,
    model VARCHAR(100) NOT NULL
);

INSERT INTO usage_logs (model) VALUES ('downstream-model');
`)
	require.NoError(t, err)

	migration189, err := dbmigrations.FS.ReadFile("189_add_usage_log_billing_model.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration189))
	require.NoError(t, err)

	var legacyBillingModel sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT billing_model FROM usage_logs WHERE id = 1`).Scan(&legacyBillingModel)
	require.NoError(t, err)
	require.False(t, legacyBillingModel.Valid)

	const billingModel = "provider/model-priced-alias"
	var insertedID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO usage_logs (model, billing_model)
VALUES ('downstream-model', $1)
RETURNING id
`, billingModel).Scan(&insertedID)
	require.NoError(t, err)

	var storedBillingModel string
	err = tx.QueryRowContext(ctx, `SELECT billing_model FROM usage_logs WHERE id = $1`, insertedID).Scan(&storedBillingModel)
	require.NoError(t, err)
	require.Equal(t, billingModel, storedBillingModel)

	requireColumn(t, tx, "usage_logs", "billing_model", "character varying", 100, true)

	_, err = tx.ExecContext(ctx, string(migration189))
	require.NoError(t, err, "migration 189 should be idempotent")
}
