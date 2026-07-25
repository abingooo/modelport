//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration191IsIdempotentAndEnforcesLotteryInvariants(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `
CREATE SCHEMA migration_191_test;
SET LOCAL search_path TO migration_191_test;
CREATE TABLE users (id BIGSERIAL PRIMARY KEY);
CREATE TABLE groups (id BIGSERIAL PRIMARY KEY);
CREATE TABLE redeem_codes (id BIGSERIAL PRIMARY KEY);
`)
	require.NoError(t, err)

	migration, err := dbmigrations.FS.ReadFile("191_create_lottery_system.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err, "migration 191 should be idempotent")

	_, err = tx.ExecContext(ctx, `INSERT INTO lottery_campaigns
(name,mode,status,starts_at,ends_at,per_user_limit)
VALUES ('instant','instant','active',NOW(),NOW()+INTERVAL '1 day',1)`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SAVEPOINT invalid_probability`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO lottery_prizes
(campaign_id,name,prize_type,balance_amount,probability_bps,inventory)
VALUES (1,'balance','balance',10,10001,1)`)
	require.Error(t, err, "probabilities above 10000 bps must be rejected")
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT invalid_probability`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `INSERT INTO users DEFAULT VALUES`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO lottery_entries
(campaign_id,user_id,idempotency_key,status) VALUES (1,1,'same-request','pending')`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SAVEPOINT duplicate_entry`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO lottery_entries
(campaign_id,user_id,idempotency_key,status) VALUES (1,1,'same-request','pending')`)
	require.Error(t, err, "duplicate participation requests must be rejected")
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT duplicate_entry`)
	require.NoError(t, err)
}
