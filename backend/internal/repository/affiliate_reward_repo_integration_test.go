//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func affiliateRewardIntegrationConfig(groupID int64) service.AffiliateRewardProgramConfig {
	config := service.DefaultAffiliateRewardProgramConfig()
	config.Enabled = true
	config.Registration.InviteeTrialGroupID = groupID
	return config
}

func createAffiliateRewardPaymentOrder(
	t *testing.T,
	ctx context.Context,
	client *dbent.Client,
	user *service.User,
	completedAt time.Time,
	payAmount float64,
	suffix string,
) int64 {
	t.Helper()
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(payAmount).
		SetPayAmount(payAmount).
		SetFeeRate(0).
		SetRechargeCode("AFF-REWARD-" + suffix).
		SetOutTradeNo("aff-reward-" + suffix).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("aff-trade-" + suffix).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusCompleted).
		SetExpiresAt(completedAt.Add(time.Hour)).
		SetPaidAt(completedAt).
		SetCompletedAt(completedAt).
		SetClientIP("198.51.100.20").
		SetSrcHost("modelport.test").
		SetCreatedAt(completedAt.Add(-time.Minute)).
		SetUpdatedAt(completedAt).
		Save(ctx)
	require.NoError(t, err)
	return order.ID
}

func TestAffiliateRewardRepositoryRegistrationReviewsAreIdempotent(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB).(*affiliateRepository)

	inviter := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("reward-inviter-%d@example.com", time.Now().UnixNano())})
	invitee := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("reward-invitee-%d@example.com", time.Now().UnixNano())})
	group := mustCreateGroup(t, client, &service.Group{
		Name:           fmt.Sprintf("reward-trial-%d", time.Now().UnixNano()),
		RateMultiplier: 1,
	})
	config := affiliateRewardIntegrationConfig(group.ID)
	meta := service.AffiliateRegistrationMeta{ClientIP: "203.0.113.9", UserAgent: "ModelPort integration", Source: "auth_request"}

	bound, err := repo.BindInviterWithRewardProgram(ctx, invitee.ID, inviter.ID, config, meta)
	require.NoError(t, err)
	require.True(t, bound)

	bound, err = repo.BindInviterWithRewardProgram(ctx, invitee.ID, inviter.ID, config, meta)
	require.NoError(t, err)
	require.False(t, bound)
	require.NoError(t, repo.EnsureRegistrationRewardReviews(ctx, invitee.ID, inviter.ID, config, meta))

	require.Equal(t, 2, querySingleInt(t, ctx, client, `
SELECT COUNT(*) FROM referral.reward_reviews
WHERE inviter_user_id = $1 AND invitee_user_id = $2`, inviter.ID, invitee.ID))
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT aff_count FROM user_affiliates WHERE user_id = $1", inviter.ID))

	rows, err := client.QueryContext(ctx, `
SELECT host(ip), source, user_agent
FROM referral.user_registration_ip_proxy
WHERE user_id = $1`, invitee.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var clientIP, source, userAgent string
	require.NoError(t, rows.Scan(&clientIP, &source, &userAgent))
	require.NoError(t, rows.Close())
	require.Equal(t, "203.0.113.9", clientIP)
	require.Equal(t, "auth_request", source)
	require.Equal(t, "ModelPort integration", userAgent)

	rows, err = client.QueryContext(ctx, `
SELECT reward_type, reward_user_id, reward_amount::double precision,
       COALESCE(risk_flags->>'group_id', ''),
       COALESCE(risk_flags->>'validity_days', ''),
       COALESCE(risk_flags->>'risk_level', 'unknown')
FROM referral.reward_reviews
WHERE inviter_user_id = $1 AND invitee_user_id = $2
ORDER BY id`, inviter.ID, invitee.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	type reviewRow struct {
		rewardType string
		rewardUser int64
		amount     float64
		groupID    string
		days       string
		risk       string
	}
	items := make([]reviewRow, 0, 2)
	for rows.Next() {
		var item reviewRow
		require.NoError(t, rows.Scan(&item.rewardType, &item.rewardUser, &item.amount, &item.groupID, &item.days, &item.risk))
		items = append(items, item)
	}
	require.NoError(t, rows.Err())
	require.Len(t, items, 2)
	require.Equal(t, service.AffiliateRewardTypeRegistrationInviteeTrial, items[0].rewardType)
	require.Equal(t, invitee.ID, items[0].rewardUser)
	require.InDelta(t, 3, items[0].amount, 1e-9)
	require.Equal(t, fmt.Sprint(group.ID), items[0].groupID)
	require.Equal(t, "3", items[0].days)
	require.Equal(t, "low", items[0].risk)
	require.Equal(t, service.AffiliateRewardTypeRegistrationInviterBonus, items[1].rewardType)
	require.Equal(t, inviter.ID, items[1].rewardUser)
	require.InDelta(t, 1, items[1].amount, 1e-9)
}

func TestAffiliateRewardRepositoryAdminInviterEvaluatesRegistrationIPReuse(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB).(*affiliateRepository)

	inviter := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("reward-admin-inviter-%d@example.com", time.Now().UnixNano()),
		Role:  service.RoleAdmin,
	})
	invitee := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("reward-admin-invitee-%d@example.com", time.Now().UnixNano())})
	group := mustCreateGroup(t, client, &service.Group{
		Name:           fmt.Sprintf("reward-admin-trial-%d", time.Now().UnixNano()),
		RateMultiplier: 1,
	})

	bound, err := repo.BindInviterWithRewardProgram(
		ctx,
		invitee.ID,
		inviter.ID,
		affiliateRewardIntegrationConfig(group.ID),
		service.AffiliateRegistrationMeta{ClientIP: "203.0.113.10", Source: "auth_request"},
	)
	require.NoError(t, err)
	require.True(t, bound)
	require.Equal(t, 2, querySingleInt(t, ctx, client, `
SELECT COUNT(*)
FROM referral.reward_reviews
WHERE inviter_user_id = $1
  AND invitee_user_id = $2
  AND risk_flags->>'admin_inviter' = 'true'
  AND risk_flags->>'risk_level' = 'low'`, inviter.ID, invitee.ID))
}

func TestAffiliateRewardRepositoryOnlyCreatesFirstRechargeReviewsOnce(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB).(*affiliateRepository)

	inviter := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("first-inviter-%d@example.com", time.Now().UnixNano())})
	invitee := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("first-invitee-%d@example.com", time.Now().UnixNano())})
	_, err := repo.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	_, err = repo.EnsureUserAffiliate(ctx, invitee.ID)
	require.NoError(t, err)
	bound, err := repo.BindInviter(ctx, invitee.ID, inviter.ID)
	require.NoError(t, err)
	require.True(t, bound)

	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprint(now.UnixNano())
	firstOrderID := createAffiliateRewardPaymentOrder(t, ctx, client, invitee, now, 100, suffix+"-first")
	secondOrderID := createAffiliateRewardPaymentOrder(t, ctx, client, invitee, now.Add(time.Minute), 200, suffix+"-second")
	config := affiliateRewardIntegrationConfig(50)
	config.Registration.Enabled = false

	created, err := repo.CreateFirstRechargeRewardReviews(ctx, firstOrderID, config)
	require.NoError(t, err)
	require.True(t, created)
	created, err = repo.CreateFirstRechargeRewardReviews(ctx, firstOrderID, config)
	require.NoError(t, err)
	require.False(t, created)
	created, err = repo.CreateFirstRechargeRewardReviews(ctx, secondOrderID, config)
	require.NoError(t, err)
	require.False(t, created)

	require.Equal(t, 2, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.reward_reviews WHERE payment_order_id = $1", firstOrderID))
	require.Equal(t, 0, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.reward_reviews WHERE payment_order_id = $1", secondOrderID))

	rows, err := client.QueryContext(ctx, `
SELECT reward_type, reward_amount::double precision
FROM referral.reward_reviews
WHERE payment_order_id = $1
ORDER BY reward_type`, firstOrderID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var rewardType string
	var amount float64
	require.NoError(t, rows.Scan(&rewardType, &amount))
	require.Equal(t, service.AffiliateRewardTypeFirstRechargeInviteeBonus, rewardType)
	require.InDelta(t, 10, amount, 1e-9)
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&rewardType, &amount))
	require.Equal(t, service.AffiliateRewardTypeFirstRechargeInviterBonus, rewardType)
	require.InDelta(t, 2, amount, 1e-9)
	require.NoError(t, rows.Close())

	reviewID := querySingleInt(t, ctx, client, `
SELECT id FROM referral.reward_reviews
WHERE payment_order_id = $1 AND reward_type = $2`, firstOrderID, service.AffiliateRewardTypeFirstRechargeInviteeBonus)
	result, err := repo.ReviewAffiliateReward(ctx, int64(reviewID), inviter.ID, service.AffiliateRewardActionReject, "integration rejection", nil)
	require.NoError(t, err)
	require.Equal(t, service.AffiliateRewardStatusRejected, result.Status)
	require.Len(t, result.ReviewIDs, 2)
	require.Equal(t, 2, querySingleInt(t, ctx, client, `
SELECT COUNT(*) FROM referral.reward_reviews
WHERE payment_order_id = $1 AND status = 'rejected'`, firstOrderID))
}

func TestAffiliateRewardRepositoryGroupedApprovalIsIdempotent(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB).(*affiliateRepository)

	admin := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("reward-admin-%d@example.com", time.Now().UnixNano()), Role: service.RoleAdmin})
	inviter := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("approve-inviter-%d@example.com", time.Now().UnixNano()), Balance: 5})
	invitee := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("approve-invitee-%d@example.com", time.Now().UnixNano())})
	group := mustCreateGroup(t, client, &service.Group{Name: fmt.Sprintf("approve-trial-%d", time.Now().UnixNano()), RateMultiplier: 1})
	config := affiliateRewardIntegrationConfig(group.ID)

	bound, err := repo.BindInviterWithRewardProgram(ctx, invitee.ID, inviter.ID, config, service.AffiliateRegistrationMeta{ClientIP: "192.0.2.10"})
	require.NoError(t, err)
	require.True(t, bound)
	reviewID := querySingleInt(t, ctx, client, `
SELECT id FROM referral.reward_reviews
WHERE invitee_user_id = $1 AND reward_type = $2`, invitee.ID, service.AffiliateRewardTypeRegistrationInviterBonus)

	result, err := repo.ReviewAffiliateReward(ctx, int64(reviewID), admin.ID, service.AffiliateRewardActionApprove, "verified", nil)
	require.NoError(t, err)
	require.Equal(t, service.AffiliateRewardStatusPaid, result.Status)
	require.Len(t, result.ReviewIDs, 2)
	require.Len(t, result.Effects, 2)
	require.InDelta(t, 6, querySingleFloat(t, ctx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", inviter.ID), 1e-9)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.balance_grants WHERE user_id = $1", inviter.ID))
	require.Equal(t, 1, querySingleInt(t, ctx, client, `
SELECT COUNT(*) FROM user_subscriptions
WHERE user_id = $1 AND group_id = $2 AND status = 'active' AND deleted_at IS NULL`, invitee.ID, group.ID))
	require.Equal(t, 2, querySingleInt(t, ctx, client, `
SELECT COUNT(*) FROM redeem_codes
WHERE used_by IN ($1, $2) AND status = 'used'`, inviter.ID, invitee.ID))

	rows, err := client.QueryContext(ctx, `
SELECT expires_at FROM user_subscriptions
WHERE user_id = $1 AND group_id = $2 AND deleted_at IS NULL`, invitee.ID, group.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var firstExpiry time.Time
	require.NoError(t, rows.Scan(&firstExpiry))
	require.NoError(t, rows.Close())

	replayed, err := repo.ReviewAffiliateReward(ctx, int64(reviewID), admin.ID, service.AffiliateRewardActionApprove, "replayed", nil)
	require.NoError(t, err)
	require.Empty(t, replayed.Effects)
	require.InDelta(t, 6, querySingleFloat(t, ctx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", inviter.ID), 1e-9)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.balance_grants WHERE user_id = $1", inviter.ID))

	rows, err = client.QueryContext(ctx, `
SELECT expires_at FROM user_subscriptions
WHERE user_id = $1 AND group_id = $2 AND deleted_at IS NULL`, invitee.ID, group.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var replayedExpiry time.Time
	require.NoError(t, rows.Scan(&replayedExpiry))
	require.NoError(t, rows.Close())
	require.True(t, firstExpiry.Equal(replayedExpiry))
}

func TestAffiliateRewardRepositoryUnknownHistoryCanOnlyBeRejected(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB).(*affiliateRepository)

	admin := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("unknown-admin-%d@example.com", time.Now().UnixNano()), Role: service.RoleAdmin})
	inviter := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("unknown-inviter-%d@example.com", time.Now().UnixNano())})
	invitee := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("unknown-invitee-%d@example.com", time.Now().UnixNano()), Balance: 8})

	rows, err := client.QueryContext(ctx, `
INSERT INTO referral.reward_reviews (
    inviter_user_id, invitee_user_id, reward_user_id, reward_type,
    reward_amount, status, risk_flags, created_at, updated_at
)
VALUES ($1, $2, $2, 'legacy_unknown_reward', 9, 'pending', '{"risk_level":"unknown"}'::jsonb, NOW(), NOW())
RETURNING id`, inviter.ID, invitee.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var reviewID int64
	require.NoError(t, rows.Scan(&reviewID))
	require.NoError(t, rows.Close())

	_, err = repo.ReviewAffiliateReward(ctx, reviewID, admin.ID, service.AffiliateRewardActionApprove, "", nil)
	require.ErrorIs(t, err, service.ErrAffiliateRewardUnsupported)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.reward_reviews WHERE id = $1 AND status = 'pending'", reviewID))
	require.InDelta(t, 8, querySingleFloat(t, ctx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", invitee.ID), 1e-9)

	result, err := repo.ReviewAffiliateReward(ctx, reviewID, admin.ID, service.AffiliateRewardActionReject, "unsupported legacy type", nil)
	require.NoError(t, err)
	require.Equal(t, service.AffiliateRewardStatusRejected, result.Status)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.reward_reviews WHERE id = $1 AND status = 'rejected'", reviewID))
}

func TestAffiliateRewardRepositoryLegacyCutoffPreventsHistoricalPayout(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB).(*affiliateRepository)

	admin := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("cutoff-admin-%d@example.com", time.Now().UnixNano()), Role: service.RoleAdmin})
	inviter := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("cutoff-inviter-%d@example.com", time.Now().UnixNano()), Balance: 11})
	invitee := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("cutoff-invitee-%d@example.com", time.Now().UnixNano())})
	createdAt := time.Date(2026, time.July, 5, 21, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, time.July, 5, 22, 0, 0, 0, time.UTC)

	rows, err := client.QueryContext(ctx, `
INSERT INTO referral.reward_reviews (
    inviter_user_id, invitee_user_id, reward_user_id, reward_type,
    reward_amount, status, risk_flags, created_at, updated_at
)
VALUES ($1, $2, $1, $3, 2, 'pending', '{}'::jsonb, $4, $4)
RETURNING id`, inviter.ID, invitee.ID, service.AffiliateRewardTypeFirstRechargeInviterBonus, createdAt)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var reviewID int64
	require.NoError(t, rows.Scan(&reviewID))
	require.NoError(t, rows.Close())

	_, err = repo.ReviewAffiliateReward(ctx, reviewID, admin.ID, service.AffiliateRewardActionApprove, "", &cutoff)
	require.ErrorIs(t, err, service.ErrAffiliateRewardOutOfScope)
	require.InDelta(t, 11, querySingleFloat(t, ctx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", inviter.ID), 1e-9)
	require.Equal(t, 0, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.balance_grants WHERE review_id = $1", reviewID))

	result, err := repo.ReviewAffiliateReward(ctx, reviewID, admin.ID, service.AffiliateRewardActionReject, "legacy cleanup", &cutoff)
	require.NoError(t, err)
	require.Equal(t, service.AffiliateRewardStatusRejected, result.Status)
}

func TestAffiliateRewardMigrationIsRepeatableAndPreservesHistoricalRows(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()

	inviter := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("migration-inviter-%d@example.com", time.Now().UnixNano())})
	invitee := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("migration-invitee-%d@example.com", time.Now().UnixNano())})
	_, err := client.ExecContext(ctx, "DELETE FROM settings WHERE key = $1", service.SettingKeyAffiliateRewardProgramConfig)
	require.NoError(t, err)
	rows, err := client.QueryContext(ctx, `
INSERT INTO referral.reward_reviews (
    inviter_user_id, invitee_user_id, reward_user_id, reward_type,
    reward_amount, status, risk_flags, review_note, created_at, updated_at
)
VALUES ($1, $2, $1, 'historical_custom_reward', 7.25, 'paid',
        '{"legacy":true,"nested":{"value":"unchanged"}}'::jsonb,
        'historical note', NOW() - INTERVAL '30 days', NOW() - INTERVAL '20 days')
RETURNING id`, inviter.ID, invitee.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var reviewID int64
	require.NoError(t, rows.Scan(&reviewID))
	require.NoError(t, rows.Close())

	_, err = client.ExecContext(ctx, `
CREATE OR REPLACE FUNCTION public.modelport_affiliate_migration_test_trigger()
RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;
CREATE TRIGGER trg_referral_first_recharge_update
AFTER UPDATE OF status ON payment_orders
FOR EACH ROW EXECUTE FUNCTION public.modelport_affiliate_migration_test_trigger();
CREATE TRIGGER trg_referral_registration_rewards_update
AFTER UPDATE OF inviter_id ON user_affiliates
FOR EACH ROW EXECUTE FUNCTION public.modelport_affiliate_migration_test_trigger();
CREATE TRIGGER trg_reward_reviews_notify_changed
AFTER UPDATE ON referral.reward_reviews
FOR EACH ROW EXECUTE FUNCTION public.modelport_affiliate_migration_test_trigger();
CREATE TRIGGER trg_referral_refresh_admin_registration_ip_risk_flags
AFTER UPDATE OF ip ON referral.user_registration_ip_proxy
FOR EACH ROW EXECUTE FUNCTION public.modelport_affiliate_migration_test_trigger();`)
	require.NoError(t, err)

	rowSnapshot := func() (string, string) {
		resultRows, queryErr := client.QueryContext(ctx, `
SELECT ctid::text, to_jsonb(rr)::text
FROM referral.reward_reviews rr
WHERE id = $1`, reviewID)
		require.NoError(t, queryErr)
		defer func() { _ = resultRows.Close() }()
		require.True(t, resultRows.Next())
		var tupleID, payload string
		require.NoError(t, resultRows.Scan(&tupleID, &payload))
		return tupleID, payload
	}
	originalTupleID, originalPayload := rowSnapshot()

	migrationSQL, err := migrations.FS.ReadFile("195_affiliate_reward_review_program.sql")
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	currentTupleID, currentPayload := rowSnapshot()
	require.Equal(t, originalTupleID, currentTupleID)
	require.JSONEq(t, originalPayload, currentPayload)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM referral.reward_reviews WHERE id = $1", reviewID))
	require.Equal(t, 4, querySingleInt(t, ctx, client, `
SELECT COUNT(*) FROM pg_trigger
WHERE tgname IN (
    'trg_referral_first_recharge_update',
    'trg_referral_registration_rewards_update',
    'trg_reward_reviews_notify_changed',
    'trg_referral_refresh_admin_registration_ip_risk_flags'
) AND tgenabled = 'D'`))

	rows, err = client.QueryContext(ctx,
		"SELECT value FROM settings WHERE key = $1", service.SettingKeyAffiliateRewardProgramConfig)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var rawConfig string
	require.NoError(t, rows.Scan(&rawConfig))
	require.NoError(t, rows.Close())
	var config service.AffiliateRewardProgramConfig
	require.NoError(t, json.Unmarshal([]byte(rawConfig), &config))
	require.True(t, config.Enabled)
	require.True(t, config.Registration.Enabled)
	require.True(t, config.FirstRecharge.Enabled)
	require.NotNil(t, config.LegacyApprovalCutoff)
	require.Equal(t, time.Date(2026, time.July, 5, 22, 0, 0, 0, time.UTC), *config.LegacyApprovalCutoff)
}
