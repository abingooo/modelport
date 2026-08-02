package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAffiliateRewardProgramMigrationAdoptsHistoryWithoutRewritingIt(t *testing.T) {
	content, err := FS.ReadFile("195_affiliate_reward_review_program.sql")
	require.NoError(t, err)
	sql := string(content)
	upperSQL := strings.ToUpper(sql)

	require.Contains(t, sql, "CREATE SCHEMA IF NOT EXISTS referral")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS referral.reward_reviews")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS referral.balance_grants")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS referral.user_registration_ip_proxy")
	require.Contains(t, sql, "WHERE EXISTS (SELECT 1 FROM referral.reward_reviews LIMIT 1)")
	require.Contains(t, sql, "'legacy_approval_cutoff', '2026-07-05T22:00:00Z'")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
	require.Contains(t, sql, "ALTER TABLE %I.%I DISABLE TRIGGER %I")
	require.NotContains(t, upperSQL, "UPDATE REFERRAL.REWARD_REVIEWS")
	require.NotContains(t, upperSQL, "DELETE FROM REFERRAL.REWARD_REVIEWS")
	require.NotContains(t, upperSQL, "UPDATE REFERRAL.BALANCE_GRANTS")
	require.NotContains(t, upperSQL, "DELETE FROM REFERRAL.BALANCE_GRANTS")
}

func TestAffiliateDefaultInviterMigrationOnlyAdoptsLegacyDatabases(t *testing.T) {
	content, err := FS.ReadFile("196_affiliate_default_inviter.sql")
	require.NoError(t, err)
	sql := string(content)
	upperSQL := strings.ToUpper(sql)

	require.Contains(t, sql, "WHERE EXISTS (SELECT 1 FROM referral.reward_reviews LIMIT 1)")
	require.Contains(t, sql, "'default_inviter_enabled', true")
	require.Contains(t, sql, "'default_inviter_user_id', 1")
	require.Contains(t, sql, "ON CONFLICT (key) DO UPDATE")
	require.NotContains(t, upperSQL, "UPDATE REFERRAL.REWARD_REVIEWS")
	require.NotContains(t, upperSQL, "DELETE FROM REFERRAL.REWARD_REVIEWS")
	require.NotContains(t, upperSQL, "UPDATE USER_AFFILIATES")
}
