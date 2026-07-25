//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration190IsIdempotentAndEnforcesCaseInsensitiveIdentity(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	_, err := tx.ExecContext(ctx, `
CREATE SCHEMA migration_190_test;
SET LOCAL search_path TO migration_190_test;
`)
	require.NoError(t, err)

	migration190, err := dbmigrations.FS.ReadFile("190_create_model_catalog_metadata.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration190))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration190))
	require.NoError(t, err, "migration 190 should be idempotent")

	_, err = tx.ExecContext(ctx, `
INSERT INTO model_catalog_metadata (platform, model_name, display_name)
VALUES ('openai', 'GPT-5', 'Initial')
`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
INSERT INTO model_catalog_metadata (platform, model_name, display_name)
VALUES ('openai', 'gpt-5', 'Updated')
ON CONFLICT (platform, LOWER(model_name)) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    updated_at = NOW()
`)
	require.NoError(t, err)

	var count int
	var displayName string
	err = tx.QueryRowContext(ctx, `
SELECT COUNT(*), MAX(display_name)
FROM model_catalog_metadata
WHERE platform = 'openai' AND LOWER(model_name) = 'gpt-5'
`).Scan(&count, &displayName)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, "Updated", displayName)
}
