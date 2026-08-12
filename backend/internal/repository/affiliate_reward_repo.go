package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var errAffiliatePaymentOrderNotFound = errors.New("affiliate payment order not found")

var _ service.AffiliateRewardRepository = (*affiliateRepository)(nil)

func (r *affiliateRepository) BindInviterWithRewardProgram(
	ctx context.Context,
	userID, inviterID int64,
	config service.AffiliateRewardProgramConfig,
	meta service.AffiliateRegistrationMeta,
) (bool, error) {
	var bound bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, inviterID); err != nil {
			return err
		}

		res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET inviter_id = $1, updated_at = NOW()
WHERE user_id = $2 AND inviter_id IS NULL`, inviterID, userID)
		if err != nil {
			return fmt.Errorf("bind inviter with reward program: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return nil
		}
		if _, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_count = aff_count + 1, updated_at = NOW()
WHERE user_id = $1`, inviterID); err != nil {
			return fmt.Errorf("increment inviter affiliate count: %w", err)
		}

		bound = true
		return createRegistrationRewardReviews(txCtx, txClient, userID, inviterID, config, meta)
	})
	return bound, err
}

func (r *affiliateRepository) EnsureRegistrationRewardReviews(
	ctx context.Context,
	userID, inviterID int64,
	config service.AffiliateRewardProgramConfig,
	meta service.AffiliateRegistrationMeta,
) error {
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		return createRegistrationRewardReviews(txCtx, txClient, userID, inviterID, config, meta)
	})
}

func createRegistrationRewardReviews(
	ctx context.Context,
	client affiliateQueryExecer,
	userID, inviterID int64,
	config service.AffiliateRewardProgramConfig,
	meta service.AffiliateRegistrationMeta,
) error {
	if !config.Enabled || !config.Registration.Enabled {
		return nil
	}
	if meta.ClientIP != "" {
		if _, err := client.ExecContext(ctx, `
INSERT INTO referral.user_registration_ip_proxy (
    user_id, ip, source, first_seen_at, user_agent, created_at
)
SELECT id, $2::inet, $3, NOW(), $4, NOW()
FROM users
WHERE id = $1
  AND created_at >= NOW() - INTERVAL '24 hours'
ON CONFLICT (user_id) DO NOTHING`, userID, meta.ClientIP, firstNonEmpty(meta.Source, "auth_request"), meta.UserAgent); err != nil {
			return fmt.Errorf("record affiliate registration metadata: %w", err)
		}
	}

	risk, err := queryRegistrationRewardRisk(ctx, client, inviterID, userID)
	if err != nil {
		return err
	}

	registration := config.Registration
	if registration.InviteeTrialAmount > 0 {
		flags, err := service.MergeAffiliateRewardFlags(risk, map[string]any{
			"benefit":         "vip_trial_card",
			"group_id":        registration.InviteeTrialGroupID,
			"quota_amount":    registration.InviteeTrialAmount,
			"validity_days":   registration.InviteeTrialDays,
			"program_version": config.Version,
		})
		if err != nil {
			return fmt.Errorf("encode registration trial reward flags: %w", err)
		}
		if _, err := client.ExecContext(ctx, `
INSERT INTO referral.reward_reviews (
    inviter_user_id, invitee_user_id, reward_user_id, reward_type,
    reward_amount, payment_order_id, status, risk_flags, created_at, updated_at
)
VALUES ($1, $2, $2, $3, $4, NULL, 'pending', $5::jsonb, NOW(), NOW())
ON CONFLICT (invitee_user_id, reward_type) WHERE payment_order_id IS NULL DO NOTHING`,
			inviterID, userID, service.AffiliateRewardTypeRegistrationInviteeTrial,
			registration.InviteeTrialAmount, string(flags)); err != nil {
			return fmt.Errorf("create registration invitee trial review: %w", err)
		}
	}

	if registration.InviterBonus > 0 {
		flags, err := service.MergeAffiliateRewardFlags(risk, map[string]any{
			"benefit":         "inviter_registration_bonus",
			"quota_amount":    registration.InviterBonus,
			"sync_with":       service.AffiliateRewardTypeRegistrationInviteeTrial,
			"program_version": config.Version,
		})
		if err != nil {
			return fmt.Errorf("encode registration inviter reward flags: %w", err)
		}
		if _, err := client.ExecContext(ctx, `
INSERT INTO referral.reward_reviews (
    inviter_user_id, invitee_user_id, reward_user_id, reward_type,
    reward_amount, payment_order_id, status, risk_flags, created_at, updated_at
)
VALUES ($1, $2, $1, $3, $4, NULL, 'pending', $5::jsonb, NOW(), NOW())
ON CONFLICT (invitee_user_id, reward_type) WHERE payment_order_id IS NULL DO NOTHING`,
			inviterID, userID, service.AffiliateRewardTypeRegistrationInviterBonus,
			registration.InviterBonus, string(flags)); err != nil {
			return fmt.Errorf("create registration inviter bonus review: %w", err)
		}
	}
	return nil
}

func queryRegistrationRewardRisk(ctx context.Context, client affiliateQueryExecer, inviterID, inviteeID int64) (service.AffiliateRewardRiskFlags, error) {
	rows, err := client.QueryContext(ctx, `
SELECT COALESCE(inviter.role = 'admin', false),
       COALESCE(host(rip.ip), ''),
       rip.first_seen_at
FROM users inviter
LEFT JOIN referral.user_registration_ip_proxy rip ON rip.user_id = $2
WHERE inviter.id = $1
LIMIT 1`, inviterID, inviteeID)
	if err != nil {
		return service.AffiliateRewardRiskFlags{}, fmt.Errorf("query registration reward risk context: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return service.AffiliateRewardRiskFlags{}, service.ErrUserNotFound
	}
	var adminInviter bool
	var registrationIP string
	var firstSeenAt sql.NullTime
	if err := rows.Scan(&adminInviter, &registrationIP, &firstSeenAt); err != nil {
		return service.AffiliateRewardRiskFlags{}, err
	}
	if err := rows.Err(); err != nil {
		return service.AffiliateRewardRiskFlags{}, err
	}
	if err := rows.Close(); err != nil {
		return service.AffiliateRewardRiskFlags{}, err
	}

	if registrationIP == "" {
		return service.AffiliateRewardRiskFlags{
			Source:                   "registration_ip_pending",
			RiskLevel:                "unknown",
			Reasons:                  []string{"registration_ip_not_captured"},
			AdminInviter:             adminInviter,
			OrderClientIPUsedForRisk: false,
		}, nil
	}
	if !adminInviter {
		var capturedAt *time.Time
		if firstSeenAt.Valid {
			capturedAt = &firstSeenAt.Time
		}
		return service.AffiliateRewardRiskFlags{
			Source:                    "registration_ip_capture",
			RiskLevel:                 "low",
			Reasons:                   []string{"invited_new_user_registration_trial_review"},
			RegistrationIPCaptured:    true,
			RegistrationIP:            registrationIP,
			RegistrationIPFirstSeenAt: capturedAt,
			OrderClientIPUsedForRisk:  false,
		}, nil
	}

	users24H, usersTotal, err := queryRegistrationIPReuse(ctx, client, inviterID, registrationIP, firstSeenAt)
	if err != nil {
		return service.AffiliateRewardRiskFlags{}, err
	}
	var capturedAt *time.Time
	if firstSeenAt.Valid {
		capturedAt = &firstSeenAt.Time
	}
	return service.EvaluateAffiliateRewardRisk(service.AffiliateRewardRiskInput{
		AdminInviter:              true,
		RegistrationIP:            registrationIP,
		RegistrationIPFirstSeenAt: capturedAt,
		RegistrationIPUsers24H:    users24H,
		RegistrationIPUsersTotal:  usersTotal,
		Source:                    "registration_ip_capture",
	}), nil
}

func queryRegistrationIPReuse(
	ctx context.Context,
	client affiliateQueryExecer,
	inviterID int64,
	registrationIP string,
	firstSeenAt sql.NullTime,
) (int, int, error) {
	if registrationIP == "" || !firstSeenAt.Valid {
		return 0, 0, nil
	}
	rows, err := client.QueryContext(ctx, `
SELECT COUNT(DISTINCT ua.user_id) FILTER (
	           WHERE rip.first_seen_at >= $3::timestamptz - INTERVAL '24 hours'
	             AND rip.first_seen_at <= $3::timestamptz + INTERVAL '1 minute'
       )::integer,
       COUNT(DISTINCT ua.user_id)::integer
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id AND u.deleted_at IS NULL
JOIN referral.user_registration_ip_proxy rip ON rip.user_id = ua.user_id
WHERE ua.inviter_id = $1 AND rip.ip = $2::inet`, inviterID, registrationIP, firstSeenAt.Time)
	if err != nil {
		return 0, 0, fmt.Errorf("query registration IP reuse: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, 0, rows.Err()
	}
	var users24H, usersTotal int
	if err := rows.Scan(&users24H, &usersTotal); err != nil {
		return 0, 0, err
	}
	return users24H, usersTotal, rows.Err()
}

func (r *affiliateRepository) CreateFirstRechargeRewardReviews(
	ctx context.Context,
	orderID int64,
	config service.AffiliateRewardProgramConfig,
) (bool, error) {
	if !config.Enabled || !config.FirstRecharge.Enabled || orderID <= 0 {
		return false, nil
	}
	var created bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		order, err := queryAffiliateFirstRechargeOrder(txCtx, txClient, orderID)
		if err != nil {
			if errors.Is(err, errAffiliatePaymentOrderNotFound) {
				return nil
			}
			return err
		}
		if order.Status != "COMPLETED" || order.OrderType != "balance" || order.PayAmount <= 0 || order.RefundAmount != 0 || order.InviterID == nil {
			return nil
		}
		if order.FirstRechargeBenefitSettled {
			return nil
		}
		prior, err := hasPriorCompletedBalanceOrder(txCtx, txClient, order)
		if err != nil {
			return err
		}
		if prior {
			return nil
		}

		risk, err := queryFirstRechargeRewardRisk(txCtx, txClient, order)
		if err != nil {
			return err
		}
		firstRecharge := config.FirstRecharge
		nominalAmount, amount := calculateFirstRechargeInviteeReward(
			order.PayAmount,
			firstRecharge.InviteeBonusPercent,
			order.RechargeBonusAmount,
		)
		if firstRecharge.InviteeBonusPercent > 0 {
			if amount > 0 {
				flags, err := service.MergeAffiliateRewardFlags(risk, map[string]any{
					"benefit":                      "first_recharge_invitee_bonus",
					"bonus_percent":                firstRecharge.InviteeBonusPercent,
					"nominal_reward_amount":        nominalAmount,
					"recharge_bonus_offset_amount": order.RechargeBonusAmount,
					"net_reward_amount":            amount,
					"program_version":              config.Version,
				})
				if err != nil {
					return err
				}
				result, err := txClient.ExecContext(txCtx, `
INSERT INTO referral.reward_reviews (
    inviter_user_id, invitee_user_id, reward_user_id, reward_type,
    reward_amount, payment_order_id, status, risk_flags, created_at, updated_at
)
VALUES ($1, $2, $2, $3, $4, $5, 'pending', $6::jsonb, NOW(), NOW())
ON CONFLICT (payment_order_id, reward_type) WHERE payment_order_id IS NOT NULL DO NOTHING`,
					*order.InviterID, order.UserID, service.AffiliateRewardTypeFirstRechargeInviteeBonus,
					amount, order.ID, string(flags))
				if err != nil {
					return fmt.Errorf("create first recharge invitee bonus review: %w", err)
				}
				affected, _ := result.RowsAffected()
				created = created || affected > 0
			}
		}
		if firstRecharge.InviterBonus > 0 {
			flags, err := service.MergeAffiliateRewardFlags(risk, map[string]any{
				"benefit":         "first_recharge_inviter_bonus",
				"quota_amount":    firstRecharge.InviterBonus,
				"program_version": config.Version,
			})
			if err != nil {
				return err
			}
			result, err := txClient.ExecContext(txCtx, `
INSERT INTO referral.reward_reviews (
    inviter_user_id, invitee_user_id, reward_user_id, reward_type,
    reward_amount, payment_order_id, status, risk_flags, created_at, updated_at
)
VALUES ($1, $2, $1, $3, $4, $5, 'pending', $6::jsonb, NOW(), NOW())
ON CONFLICT (payment_order_id, reward_type) WHERE payment_order_id IS NOT NULL DO NOTHING`,
				*order.InviterID, order.UserID, service.AffiliateRewardTypeFirstRechargeInviterBonus,
				firstRecharge.InviterBonus, order.ID, string(flags))
			if err != nil {
				return fmt.Errorf("create first recharge inviter bonus review: %w", err)
			}
			affected, _ := result.RowsAffected()
			created = created || affected > 0
		}
		return markFirstRechargeBenefitSettled(
			txCtx,
			txClient,
			order.ID,
			firstRecharge.InviteeBonusPercent,
			nominalAmount,
			order.RechargeBonusAmount,
			amount,
			config.Version,
		)
	})
	return created, err
}

type affiliateFirstRechargeOrder struct {
	ID                          int64
	UserID                      int64
	InviterID                   *int64
	Status                      string
	OrderType                   string
	PayAmount                   float64
	Amount                      float64
	RefundAmount                float64
	RechargeBonusAmount         float64
	FirstRechargeBenefitSettled bool
	OccurredAt                  time.Time
	UserCreatedAt               time.Time
	PaymentTradeNumber          string
}

func queryAffiliateFirstRechargeOrder(ctx context.Context, client affiliateQueryExecer, orderID int64) (*affiliateFirstRechargeOrder, error) {
	rows, err := client.QueryContext(ctx, `
SELECT po.id,
       po.user_id,
       ua.inviter_id,
       po.status,
       po.order_type,
       po.pay_amount::double precision,
       po.amount::double precision,
       po.refund_amount::double precision,
       COALESCE(po.provider_snapshot::text, '{}'),
       COALESCE(po.completed_at, po.paid_at, po.created_at),
       u.created_at,
       COALESCE(po.payment_trade_no, '')
FROM payment_orders po
JOIN users u ON u.id = po.user_id
LEFT JOIN user_affiliates ua ON ua.user_id = po.user_id
WHERE po.id = $1
FOR NO KEY UPDATE OF po, u`, orderID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errAffiliatePaymentOrderNotFound
	}
	var order affiliateFirstRechargeOrder
	var inviterID sql.NullInt64
	var providerSnapshot []byte
	if err := rows.Scan(
		&order.ID,
		&order.UserID,
		&inviterID,
		&order.Status,
		&order.OrderType,
		&order.PayAmount,
		&order.Amount,
		&order.RefundAmount,
		&providerSnapshot,
		&order.OccurredAt,
		&order.UserCreatedAt,
		&order.PaymentTradeNumber,
	); err != nil {
		return nil, err
	}
	if inviterID.Valid {
		order.InviterID = &inviterID.Int64
	}
	order.RechargeBonusAmount, order.FirstRechargeBenefitSettled = parseFirstRechargeBenefitSnapshot(providerSnapshot, order.Amount)
	return &order, rows.Err()
}

func parseFirstRechargeBenefitSnapshot(raw []byte, creditedAmount float64) (float64, bool) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return 0, false
	}
	var settled bool
	if value, ok := snapshot["first_recharge_benefit_settled"]; ok {
		_ = json.Unmarshal(value, &settled)
	}
	bonusAmount := parseFirstRechargeSnapshotAmount(snapshot["recharge_bonus_amount"])
	if creditedAmount <= 0 || bonusAmount > creditedAmount {
		bonusAmount = 0
	}
	return bonusAmount, settled
}

func parseFirstRechargeSnapshotAmount(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var amount float64
	if err := json.Unmarshal(raw, &amount); err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return 0
	}
	return amount
}

func markFirstRechargeBenefitSettled(
	ctx context.Context,
	client affiliateQueryExecer,
	orderID int64,
	bonusPercent, nominalAmount, rechargeBonusAmount, netAmount float64,
	programVersion int,
) error {
	_, err := client.ExecContext(ctx, `
UPDATE payment_orders
SET provider_snapshot = (
        CASE
            WHEN jsonb_typeof(provider_snapshot) = 'object' THEN provider_snapshot
            ELSE '{}'::jsonb
        END
    ) || jsonb_build_object(
        'first_recharge_benefit_settled', true,
        'first_recharge_bonus_percent', $2::double precision,
        'first_recharge_nominal_reward_amount', $3::double precision,
        'first_recharge_tier_offset_amount', $4::double precision,
        'first_recharge_net_reward_amount', $5::double precision,
        'first_recharge_program_version', $6::integer,
        'first_recharge_benefit_settled_at', NOW()
    )
WHERE id = $1`, orderID, bonusPercent, nominalAmount, rechargeBonusAmount, netAmount, programVersion)
	if err != nil {
		return fmt.Errorf("mark first recharge benefit settled: %w", err)
	}
	return nil
}

func hasPriorCompletedBalanceOrder(ctx context.Context, client affiliateQueryExecer, order *affiliateFirstRechargeOrder) (bool, error) {
	rows, err := client.QueryContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM payment_orders prior
    WHERE prior.user_id = $1
      AND prior.id <> $2
      AND prior.status = 'COMPLETED'
      AND prior.order_type = 'balance'
      AND prior.pay_amount > 0
      AND prior.refund_amount = 0
      AND (
          COALESCE(prior.completed_at, prior.paid_at, prior.created_at) < $3
          OR (COALESCE(prior.completed_at, prior.paid_at, prior.created_at) = $3 AND prior.id < $2)
      )
)`, order.UserID, order.ID, order.OccurredAt)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false, rows.Err()
	}
	var exists bool
	if err := rows.Scan(&exists); err != nil {
		return false, err
	}
	return exists, rows.Err()
}

func queryFirstRechargeRewardRisk(
	ctx context.Context,
	client affiliateQueryExecer,
	order *affiliateFirstRechargeOrder,
) (service.AffiliateRewardRiskFlags, error) {
	if order.InviterID == nil {
		return service.AffiliateRewardRiskFlags{}, service.ErrAffiliateProfileNotFound
	}
	registrationRisk, err := queryRegistrationRewardRisk(ctx, client, *order.InviterID, order.UserID)
	if err != nil {
		return service.AffiliateRewardRiskFlags{}, err
	}
	if registrationRisk.AdminInviter {
		registrationRisk.Source = "first_recharge"
		return registrationRisk, nil
	}

	rows, err := client.QueryContext(ctx, `
SELECT COUNT(*) FILTER (WHERE invited.created_at >= NOW() - INTERVAL '24 hours')::integer,
       COUNT(*)::integer,
       COUNT(DISTINCT paid.user_id)::integer,
       (SELECT COUNT(*)::integer
        FROM referral.reward_reviews rejected
        WHERE rejected.inviter_user_id = $1 AND rejected.status = 'rejected'),
       EXISTS (
           SELECT 1
           FROM payment_orders duplicate_order
	           WHERE $2 <> ''
	             AND duplicate_order.id <> $3
	             AND duplicate_order.payment_trade_no = $2
       )
FROM user_affiliates ua
JOIN users invited ON invited.id = ua.user_id AND invited.deleted_at IS NULL
LEFT JOIN (
    SELECT DISTINCT po.user_id
    FROM payment_orders po
    WHERE po.status = 'COMPLETED'
      AND po.order_type = 'balance'
      AND po.pay_amount > 0
      AND po.refund_amount = 0
) paid ON paid.user_id = ua.user_id
	WHERE ua.inviter_id = $1`, *order.InviterID, order.PaymentTradeNumber, order.ID)
	if err != nil {
		return service.AffiliateRewardRiskFlags{}, fmt.Errorf("query first recharge risk metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return service.AffiliateRewardRiskFlags{}, rows.Err()
	}
	var invites24H, totalInvites, paidInvitees, rejectedRewards int
	var duplicateTradeNumber bool
	if err := rows.Scan(&invites24H, &totalInvites, &paidInvitees, &rejectedRewards, &duplicateTradeNumber); err != nil {
		return service.AffiliateRewardRiskFlags{}, err
	}
	minutes := order.OccurredAt.Sub(order.UserCreatedAt).Minutes()
	payAmount := order.PayAmount
	creditedQuota := order.Amount
	return service.EvaluateAffiliateRewardRisk(service.AffiliateRewardRiskInput{
		InviterInvites24H:           invites24H,
		InviterTotalInvites:         totalInvites,
		InviterPaidInvitees:         paidInvitees,
		InviterRejectedRewards:      rejectedRewards,
		MinutesToFirstRecharge:      &minutes,
		PayAmount:                   &payAmount,
		CreditedQuota:               &creditedQuota,
		DuplicatePaymentTradeNumber: duplicateTradeNumber,
		Source:                      "first_recharge",
	}), rows.Err()
}

func roundAffiliateReward(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func calculateFirstRechargeInviteeReward(payAmount, bonusPercent, rechargeBonusAmount float64) (float64, float64) {
	nominalAmount := roundAffiliateReward(payAmount * bonusPercent / 100)
	netAmount := roundAffiliateReward(math.Max(0, nominalAmount-math.Max(0, rechargeBonusAmount)))
	return nominalAmount, netAmount
}

func (r *affiliateRepository) ValidateAffiliateRewardProgram(ctx context.Context, config service.AffiliateRewardProgramConfig) error {
	client := clientFromContext(ctx, r.client)
	if config.Registration.DefaultInviterEnabled {
		rows, err := client.QueryContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM users
    WHERE id = $1 AND deleted_at IS NULL AND status = 'active'
)`, config.Registration.DefaultInviterUserID)
		if err != nil {
			return err
		}
		if !rows.Next() {
			_ = rows.Close()
			return service.ErrAffiliateDefaultInviter
		}
		var valid bool
		if err := rows.Scan(&valid); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if !valid {
			return service.ErrAffiliateDefaultInviter
		}
	}
	if !config.Enabled || !config.Registration.Enabled || config.Registration.InviteeTrialAmount <= 0 {
		return nil
	}
	rows, err := client.QueryContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM groups
    WHERE id = $1 AND deleted_at IS NULL AND status = 'active'
)`, config.Registration.InviteeTrialGroupID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return service.ErrAffiliateRewardGroupInvalid
	}
	var valid bool
	if err := rows.Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return service.ErrAffiliateRewardGroupInvalid
	}
	return rows.Err()
}

func (r *affiliateRepository) GetAffiliateRewardDashboard(ctx context.Context, inviterID int64) (*service.AffiliateRewardDashboard, error) {
	client := clientFromContext(ctx, r.client)
	dashboard := &service.AffiliateRewardDashboard{Invitees: make([]service.AffiliateRewardInvitee, 0)}

	rows, err := client.QueryContext(ctx, `
SELECT COALESCE(SUM(reward_amount) FILTER (WHERE status = 'paid'), 0)::double precision,
       COALESCE(SUM(reward_amount) FILTER (WHERE status IN ('pending', 'approved')), 0)::double precision,
       COALESCE(SUM(reward_amount) FILTER (WHERE status = 'rejected'), 0)::double precision
FROM referral.reward_reviews
WHERE reward_user_id = $1
  AND reward_type IN ($2, $3)`, inviterID,
		service.AffiliateRewardTypeRegistrationInviterBonus,
		service.AffiliateRewardTypeFirstRechargeInviterBonus)
	if err != nil {
		return nil, err
	}
	if rows.Next() {
		if err := rows.Scan(&dashboard.PaidAmount, &dashboard.PendingAmount, &dashboard.RejectedAmount); err != nil {
			_ = rows.Close()
			return nil, err
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = client.QueryContext(ctx, `
SELECT ua.user_id,
       COALESCE(invitee.email, ''),
       invitee.created_at,
       COALESCE(registration_trial.status, 'none'),
       COALESCE(registration_reward.status, 'none'),
       COALESCE(registration_reward.reward_amount, 0)::double precision,
       COALESCE(first_recharge_benefit.status, 'none'),
       COALESCE(first_recharge_reward.status, 'none'),
       COALESCE(first_recharge_reward.reward_amount, 0)::double precision,
       GREATEST(
           registration_trial.updated_at,
           registration_reward.updated_at,
           first_recharge_benefit.updated_at,
           first_recharge_reward.updated_at
       )
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id AND invitee.deleted_at IS NULL
LEFT JOIN LATERAL (
    SELECT rr.status, rr.updated_at
    FROM referral.reward_reviews rr
    WHERE rr.inviter_user_id = $1
      AND rr.invitee_user_id = ua.user_id
      AND rr.reward_type = $2
    ORDER BY rr.id DESC LIMIT 1
) registration_trial ON true
LEFT JOIN LATERAL (
    SELECT rr.status, rr.reward_amount, rr.updated_at
    FROM referral.reward_reviews rr
    WHERE rr.inviter_user_id = $1
      AND rr.invitee_user_id = ua.user_id
      AND rr.reward_user_id = $1
      AND rr.reward_type = $3
    ORDER BY rr.id DESC LIMIT 1
) registration_reward ON true
LEFT JOIN LATERAL (
    SELECT rr.status, rr.updated_at
    FROM referral.reward_reviews rr
    WHERE rr.inviter_user_id = $1
      AND rr.invitee_user_id = ua.user_id
      AND rr.reward_type = $4
    ORDER BY rr.id DESC LIMIT 1
) first_recharge_benefit ON true
LEFT JOIN LATERAL (
    SELECT rr.status, rr.reward_amount, rr.updated_at
    FROM referral.reward_reviews rr
    WHERE rr.inviter_user_id = $1
      AND rr.invitee_user_id = ua.user_id
      AND rr.reward_user_id = $1
      AND rr.reward_type = $5
    ORDER BY rr.id DESC LIMIT 1
) first_recharge_reward ON true
WHERE ua.inviter_id = $1
ORDER BY ua.created_at DESC, ua.user_id DESC
LIMIT 200`, inviterID,
		service.AffiliateRewardTypeRegistrationInviteeTrial,
		service.AffiliateRewardTypeRegistrationInviterBonus,
		service.AffiliateRewardTypeFirstRechargeInviteeBonus,
		service.AffiliateRewardTypeFirstRechargeInviterBonus)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item service.AffiliateRewardInvitee
		var email string
		var updatedAt sql.NullTime
		if err := rows.Scan(
			&item.UserID,
			&email,
			&item.RegisteredAt,
			&item.RegistrationStatus,
			&item.RegistrationRewardStatus,
			&item.RegistrationRewardAmount,
			&item.FirstRechargeStatus,
			&item.FirstRechargeRewardStatus,
			&item.FirstRechargeRewardAmount,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		item.EmailMasked = service.MaskEmail(email)
		if updatedAt.Valid {
			item.UpdatedAt = &updatedAt.Time
		}
		dashboard.Invitees = append(dashboard.Invitees, item)
		if item.FirstRechargeStatus != "none" || item.FirstRechargeRewardStatus != "none" {
			dashboard.FirstRechargeUsers++
		}
	}
	dashboard.InvitedUsers = len(dashboard.Invitees)
	return dashboard, rows.Err()
}

func (r *affiliateRepository) ListAffiliateRewardReviews(
	ctx context.Context,
	filter service.AffiliateRewardReviewFilter,
) ([]service.AffiliateRewardReview, int64, error) {
	filter.Page, filter.PageSize = normalizeAffiliateRewardReviewPage(filter.Page, filter.PageSize)
	where, args := buildAffiliateRewardReviewWhere(filter)
	client := clientFromContext(ctx, r.client)
	total, err := queryAffiliateRecordCount(ctx, client, `
SELECT COUNT(*)
FROM referral.reward_reviews rr
LEFT JOIN users inviter ON inviter.id = rr.inviter_user_id
LEFT JOIN users invitee ON invitee.id = rr.invitee_user_id
LEFT JOIN users reward_user ON reward_user.id = rr.reward_user_id
`+where, args...)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT rr.id,
       rr.inviter_user_id,
       rr.invitee_user_id,
       rr.reward_user_id,
       rr.reward_type,
       rr.reward_amount::double precision,
       rr.payment_order_id,
       rr.status,
       rr.risk_flags,
       COALESCE(rr.risk_flags->>'risk_level', 'unknown'),
       COALESCE(NULLIF(rr.risk_flags->>'risk_score', '')::integer, 0),
       rr.reviewed_by,
       rr.reviewed_at,
       rr.review_note,
       rr.paid_at,
       rr.created_at,
       rr.updated_at,
       COALESCE(inviter.email, ''),
       COALESCE(invitee.email, ''),
       COALESCE(reward_user.email, ''),
       COALESCE(reviewer.email, ''),
       po.amount::double precision,
       po.pay_amount::double precision,
       COALESCE(po.status, ''),
       COALESCE(host(registration_ip.ip), ''),
       registration_ip.first_seen_at
FROM referral.reward_reviews rr
LEFT JOIN users inviter ON inviter.id = rr.inviter_user_id
LEFT JOIN users invitee ON invitee.id = rr.invitee_user_id
LEFT JOIN users reward_user ON reward_user.id = rr.reward_user_id
LEFT JOIN users reviewer ON reviewer.id = rr.reviewed_by
LEFT JOIN payment_orders po ON po.id = rr.payment_order_id
LEFT JOIN referral.user_registration_ip_proxy registration_ip ON registration_ip.user_id = rr.invitee_user_id
`+where+`
ORDER BY rr.created_at DESC, rr.id DESC
LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateRewardReview, 0, filter.PageSize)
	for rows.Next() {
		var item service.AffiliateRewardReview
		var paymentOrderID, reviewedBy sql.NullInt64
		var reviewedAt, paidAt, registrationIPFirstSeenAt sql.NullTime
		var orderAmount, orderPayAmount sql.NullFloat64
		var riskFlags []byte
		if err := rows.Scan(
			&item.ID,
			&item.InviterUserID,
			&item.InviteeUserID,
			&item.RewardUserID,
			&item.RewardType,
			&item.RewardAmount,
			&paymentOrderID,
			&item.Status,
			&riskFlags,
			&item.RiskLevel,
			&item.RiskScore,
			&reviewedBy,
			&reviewedAt,
			&item.ReviewNote,
			&paidAt,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.InviterEmail,
			&item.InviteeEmail,
			&item.RewardUserEmail,
			&item.ReviewedByEmail,
			&orderAmount,
			&orderPayAmount,
			&item.OrderStatus,
			&item.RegistrationIP,
			&registrationIPFirstSeenAt,
		); err != nil {
			return nil, 0, err
		}
		item.RiskFlags = append(json.RawMessage(nil), riskFlags...)
		item.PaymentOrderID = nullInt64Pointer(paymentOrderID)
		item.ReviewedBy = nullInt64Pointer(reviewedBy)
		item.ReviewedAt = nullTimePointer(reviewedAt)
		item.PaidAt = nullTimePointer(paidAt)
		item.OrderAmount = nullableFloat64Ptr(orderAmount)
		item.OrderPayAmount = nullableFloat64Ptr(orderPayAmount)
		item.RegistrationIPFirstSeenAt = nullTimePointer(registrationIPFirstSeenAt)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func normalizeAffiliateRewardReviewPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func buildAffiliateRewardReviewWhere(filter service.AffiliateRewardReviewFilter) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	switch filter.Status {
	case "pending_review", "":
		clauses = append(clauses, "rr.status IN ('pending', 'approved')")
	case "final":
		clauses = append(clauses, "rr.status IN ('paid', 'rejected')")
	case service.AffiliateRewardStatusPending, service.AffiliateRewardStatusApproved,
		service.AffiliateRewardStatusRejected, service.AffiliateRewardStatusPaid:
		add("rr.status = $%d", filter.Status)
	}
	if rewardType := strings.TrimSpace(filter.RewardType); rewardType != "" && rewardType != "all" {
		add("rr.reward_type = $%d", rewardType)
	}
	switch filter.Risk {
	case "low", "medium", "high", "unknown":
		add("COALESCE(rr.risk_flags->>'risk_level', 'unknown') = $%d", filter.Risk)
	case "attention":
		clauses = append(clauses, "COALESCE(rr.risk_flags->>'risk_level', 'unknown') <> 'low'")
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		placeholder := "$" + strconv.Itoa(len(args))
		clauses = append(clauses, "("+strings.Join([]string{
			"LOWER(COALESCE(inviter.email, '')) LIKE " + placeholder,
			"LOWER(COALESCE(invitee.email, '')) LIKE " + placeholder,
			"LOWER(COALESCE(reward_user.email, '')) LIKE " + placeholder,
			"rr.id::text LIKE " + placeholder,
			"rr.inviter_user_id::text LIKE " + placeholder,
			"rr.invitee_user_id::text LIKE " + placeholder,
			"COALESCE(rr.payment_order_id::text, '') LIKE " + placeholder,
		}, " OR ")+")")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (r *affiliateRepository) GetAffiliateRewardStats(ctx context.Context) (*service.AffiliateRewardReviewStats, error) {
	client := clientFromContext(ctx, r.client)
	stats := &service.AffiliateRewardReviewStats{ByType: make(map[string]any)}
	rows, err := client.QueryContext(ctx, `
SELECT COUNT(*) FILTER (WHERE status IN ('pending', 'approved')),
       COALESCE(SUM(reward_amount) FILTER (WHERE status IN ('pending', 'approved')), 0)::double precision,
       COUNT(*) FILTER (WHERE status = 'paid'),
       COALESCE(SUM(reward_amount) FILTER (WHERE status = 'paid'), 0)::double precision,
       COUNT(*) FILTER (WHERE status = 'rejected'),
       COUNT(*) FILTER (
           WHERE status IN ('pending', 'approved')
             AND COALESCE(risk_flags->>'risk_level', 'unknown') <> 'low'
       ),
       COUNT(*) FILTER (
           WHERE status = 'paid'
             AND paid_at >= date_trunc('day', NOW() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai'
       )
FROM referral.reward_reviews`)
	if err != nil {
		return nil, err
	}
	if rows.Next() {
		if err := rows.Scan(
			&stats.PendingCount,
			&stats.PendingAmount,
			&stats.PaidCount,
			&stats.PaidAmount,
			&stats.RejectedCount,
			&stats.HighRiskPendingCount,
			&stats.TodayPaidCount,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = client.QueryContext(ctx, `
SELECT reward_type,
       COUNT(*) FILTER (WHERE status IN ('pending', 'approved')),
       COALESCE(SUM(reward_amount) FILTER (WHERE status IN ('pending', 'approved')), 0)::double precision,
       COUNT(*) FILTER (WHERE status = 'paid'),
       COALESCE(SUM(reward_amount) FILTER (WHERE status = 'paid'), 0)::double precision,
       COUNT(*) FILTER (WHERE status = 'rejected')
FROM referral.reward_reviews
GROUP BY reward_type
ORDER BY reward_type`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rewardType string
		var pendingCount, paidCount, rejectedCount int64
		var pendingAmount, paidAmount float64
		if err := rows.Scan(&rewardType, &pendingCount, &pendingAmount, &paidCount, &paidAmount, &rejectedCount); err != nil {
			return nil, err
		}
		stats.ByType[rewardType] = map[string]any{
			"pending_count":  pendingCount,
			"pending_amount": pendingAmount,
			"paid_count":     paidCount,
			"paid_amount":    paidAmount,
			"rejected_count": rejectedCount,
		}
	}
	return stats, rows.Err()
}

func (r *affiliateRepository) ReviewAffiliateReward(
	ctx context.Context,
	reviewID, adminID int64,
	action, note string,
	legacyApprovalCutoff *time.Time,
) (*service.AffiliateRewardReviewResult, error) {
	if reviewID <= 0 || adminID <= 0 {
		return nil, service.ErrAffiliateRewardNotFound
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != service.AffiliateRewardActionApprove && action != service.AffiliateRewardActionReject {
		return nil, service.ErrAffiliateRewardProgramInvalid
	}
	note = truncateAffiliateUTF8(strings.TrimSpace(strings.ToValidUTF8(note, "")), 2000)

	result := &service.AffiliateRewardReviewResult{
		ReviewIDs: make([]int64, 0, 2),
		Effects:   make([]service.AffiliateRewardGrantEffect, 0, 2),
	}
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		reviews, err := lockAffiliateRewardGroup(txCtx, txClient, reviewID)
		if err != nil {
			return err
		}
		if action == service.AffiliateRewardActionApprove && legacyApprovalCutoff != nil {
			for _, review := range reviews {
				if review.CreatedAt.Before(*legacyApprovalCutoff) {
					return service.ErrAffiliateRewardOutOfScope
				}
			}
		}
		for _, review := range reviews {
			if review.Status == service.AffiliateRewardStatusPaid {
				if action == service.AffiliateRewardActionReject {
					return service.ErrAffiliateRewardFinal
				}
				result.ReviewIDs = append(result.ReviewIDs, review.ID)
				continue
			}
			if review.Status == service.AffiliateRewardStatusRejected {
				if action == service.AffiliateRewardActionApprove {
					return service.ErrAffiliateRewardFinal
				}
				result.ReviewIDs = append(result.ReviewIDs, review.ID)
				continue
			}

			if action == service.AffiliateRewardActionApprove {
				effect, err := approveAffiliateRewardReview(txCtx, txClient, review, adminID, note)
				if err != nil {
					return err
				}
				if effect != nil {
					result.Effects = append(result.Effects, *effect)
				}
			} else if _, err := txClient.ExecContext(txCtx, `
UPDATE referral.reward_reviews
SET status = 'rejected',
    reviewed_by = $2,
    reviewed_at = NOW(),
    review_note = $3,
    updated_at = NOW()
WHERE id = $1`, review.ID, adminID, note); err != nil {
				return fmt.Errorf("reject affiliate reward review: %w", err)
			}
			result.ReviewIDs = append(result.ReviewIDs, review.ID)
		}
		if action == service.AffiliateRewardActionApprove {
			result.Status = service.AffiliateRewardStatusPaid
		} else {
			result.Status = service.AffiliateRewardStatusRejected
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type lockedAffiliateRewardReview struct {
	ID             int64
	InviterUserID  int64
	InviteeUserID  int64
	RewardUserID   int64
	RewardType     string
	RewardAmount   float64
	PaymentOrderID *int64
	Status         string
	RiskFlags      []byte
	CreatedAt      time.Time
}

func lockAffiliateRewardGroup(ctx context.Context, client affiliateQueryExecer, reviewID int64) ([]lockedAffiliateRewardReview, error) {
	rows, err := client.QueryContext(ctx, `
SELECT inviter_user_id, invitee_user_id, payment_order_id, reward_type
FROM referral.reward_reviews
WHERE id = $1
`, reviewID)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		_ = rows.Close()
		return nil, service.ErrAffiliateRewardNotFound
	}
	var inviterID, inviteeID int64
	var paymentOrderID sql.NullInt64
	var rewardType string
	if err := rows.Scan(&inviterID, &inviteeID, &paymentOrderID, &rewardType); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	query := `
SELECT id, inviter_user_id, invitee_user_id, reward_user_id, reward_type,
	       reward_amount::double precision, payment_order_id, status, risk_flags, created_at
FROM referral.reward_reviews
WHERE id = $1
FOR UPDATE`
	args := []any{reviewID}
	if isAffiliateRegistrationRewardType(rewardType) {
		query = `
SELECT id, inviter_user_id, invitee_user_id, reward_user_id, reward_type,
	       reward_amount::double precision, payment_order_id, status, risk_flags, created_at
FROM referral.reward_reviews
WHERE inviter_user_id = $1
  AND invitee_user_id = $2
  AND payment_order_id IS NULL
  AND reward_type IN ($3, $4, $5)
ORDER BY id
FOR UPDATE`
		args = []any{
			inviterID,
			inviteeID,
			service.AffiliateRewardTypeRegistrationInviteeTrial,
			service.AffiliateRewardTypeRegistrationInviteeBonus,
			service.AffiliateRewardTypeRegistrationInviterBonus,
		}
	} else if isAffiliateFirstRechargeRewardType(rewardType) && paymentOrderID.Valid {
		query = `
SELECT id, inviter_user_id, invitee_user_id, reward_user_id, reward_type,
	       reward_amount::double precision, payment_order_id, status, risk_flags, created_at
FROM referral.reward_reviews
WHERE payment_order_id = $1
  AND reward_type IN ($2, $3)
ORDER BY id
FOR UPDATE`
		args = []any{
			paymentOrderID.Int64,
			service.AffiliateRewardTypeFirstRechargeInviteeBonus,
			service.AffiliateRewardTypeFirstRechargeInviterBonus,
		}
	}

	rows, err = client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	reviews := make([]lockedAffiliateRewardReview, 0, 2)
	for rows.Next() {
		var review lockedAffiliateRewardReview
		var orderID sql.NullInt64
		if err := rows.Scan(
			&review.ID,
			&review.InviterUserID,
			&review.InviteeUserID,
			&review.RewardUserID,
			&review.RewardType,
			&review.RewardAmount,
			&orderID,
			&review.Status,
			&review.RiskFlags,
			&review.CreatedAt,
		); err != nil {
			return nil, err
		}
		if orderID.Valid {
			review.PaymentOrderID = &orderID.Int64
		}
		reviews = append(reviews, review)
	}
	if len(reviews) == 0 {
		return nil, service.ErrAffiliateRewardNotFound
	}
	return reviews, rows.Err()
}

func approveAffiliateRewardReview(
	ctx context.Context,
	client affiliateQueryExecer,
	review lockedAffiliateRewardReview,
	adminID int64,
	note string,
) (*service.AffiliateRewardGrantEffect, error) {
	switch review.RewardType {
	case service.AffiliateRewardTypeRegistrationInviteeTrial:
		return grantAffiliateTrialReview(ctx, client, review, adminID, note)
	case service.AffiliateRewardTypeRegistrationInviteeBonus,
		service.AffiliateRewardTypeRegistrationInviterBonus,
		service.AffiliateRewardTypeFirstRechargeInviteeBonus,
		service.AffiliateRewardTypeFirstRechargeInviterBonus,
		service.AffiliateRewardTypeLimitedRechargeBonus:
		return grantAffiliateBalanceReview(ctx, client, review, adminID, note)
	default:
		return nil, service.ErrAffiliateRewardUnsupported
	}
}

func grantAffiliateTrialReview(
	ctx context.Context,
	client affiliateQueryExecer,
	review lockedAffiliateRewardReview,
	adminID int64,
	note string,
) (*service.AffiliateRewardGrantEffect, error) {
	var flags struct {
		GroupID      int64 `json:"group_id"`
		ValidityDays int   `json:"validity_days"`
	}
	if err := json.Unmarshal(review.RiskFlags, &flags); err != nil {
		return nil, service.ErrAffiliateRewardProgramInvalid
	}
	if flags.GroupID <= 0 || flags.ValidityDays <= 0 || flags.ValidityDays > 3650 {
		return nil, service.ErrAffiliateRewardProgramInvalid
	}
	valid, err := affiliateRewardGroupExists(ctx, client, flags.GroupID)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, service.ErrAffiliateRewardGroupInvalid
	}

	now := time.Now()
	code := fmt.Sprintf("REFERRAL-PRO-TRIAL-%d", review.ID)
	grantNote := fmt.Sprintf("ModelPort邀请注册体验卡，审核单 #%d", review.ID)
	if note != "" {
		grantNote += "，" + note
	}
	if _, err := client.ExecContext(ctx, `
INSERT INTO redeem_codes (
    code, type, value, status, used_by, used_at, created_at,
    notes, group_id, validity_days
)
VALUES ($1, 'subscription', $2, 'used', $3, $4, $4, $5, $6, $7)
ON CONFLICT (code) DO NOTHING`, code, review.RewardAmount, review.RewardUserID, now, grantNote, flags.GroupID, flags.ValidityDays); err != nil {
		return nil, fmt.Errorf("record affiliate trial grant: %w", err)
	}
	if _, err := client.ExecContext(ctx, `
INSERT INTO user_subscriptions (
    user_id, group_id, starts_at, expires_at, status,
    assigned_by, assigned_at, notes, created_at, updated_at
)
VALUES (
	    $1, $2, $3::timestamptz, $3::timestamptz + make_interval(days => $4::integer), 'active',
	    $5, $3::timestamptz, $6, $3::timestamptz, $3::timestamptz
)
ON CONFLICT (user_id, group_id) WHERE deleted_at IS NULL
DO UPDATE SET
    status = 'active',
    starts_at = LEAST(user_subscriptions.starts_at, EXCLUDED.starts_at),
	    expires_at = GREATEST(user_subscriptions.expires_at, EXCLUDED.starts_at) + make_interval(days => $4::integer),
    assigned_by = EXCLUDED.assigned_by,
    assigned_at = EXCLUDED.assigned_at,
    notes = EXCLUDED.notes,
    updated_at = EXCLUDED.updated_at`, review.RewardUserID, flags.GroupID, now, flags.ValidityDays, adminID, grantNote); err != nil {
		return nil, fmt.Errorf("grant affiliate trial subscription: %w", err)
	}
	if err := markAffiliateRewardPaid(ctx, client, review.ID, adminID, note, now); err != nil {
		return nil, err
	}
	return &service.AffiliateRewardGrantEffect{UserID: review.RewardUserID, Kind: "subscription", GroupID: flags.GroupID}, nil
}

func grantAffiliateBalanceReview(
	ctx context.Context,
	client affiliateQueryExecer,
	review lockedAffiliateRewardReview,
	adminID int64,
	note string,
) (*service.AffiliateRewardGrantEffect, error) {
	rows, err := client.QueryContext(ctx, `
SELECT balance::double precision
FROM users
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE`, review.RewardUserID)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		_ = rows.Close()
		return nil, service.ErrUserNotFound
	}
	var balanceBefore float64
	if err := rows.Scan(&balanceBefore); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	granted, err := affiliateBalanceGrantExists(ctx, client, review.ID, review.RewardUserID, review.RewardAmount)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if !granted {
		balanceAfter := balanceBefore + review.RewardAmount
		if _, err := client.ExecContext(ctx, `
UPDATE users
SET balance = balance + $2, updated_at = $3
WHERE id = $1`, review.RewardUserID, review.RewardAmount, now); err != nil {
			return nil, fmt.Errorf("apply affiliate reward balance: %w", err)
		}
		if _, err := client.ExecContext(ctx, `
INSERT INTO referral.balance_grants (
    review_id, user_id, amount, balance_before, balance_after, created_at
)
VALUES ($1, $2, $3, $4, $5, $6)`, review.ID, review.RewardUserID, review.RewardAmount, balanceBefore, balanceAfter, now); err != nil {
			return nil, fmt.Errorf("record affiliate balance grant: %w", err)
		}
	}

	code := fmt.Sprintf("ADMIN-REFERRAL-REWARD-%d", review.ID)
	grantNote := fmt.Sprintf("ModelPort邀请奖励 +%.8f，类型 %s，审核单 #%d", review.RewardAmount, review.RewardType, review.ID)
	if review.PaymentOrderID != nil {
		grantNote += fmt.Sprintf("，来源订单 #%d", *review.PaymentOrderID)
	}
	if note != "" {
		grantNote += "，" + note
	}
	if _, err := client.ExecContext(ctx, `
INSERT INTO redeem_codes (
    code, type, value, status, used_by, used_at, created_at, notes, validity_days
)
VALUES ($1, 'admin_balance', $2, 'used', $3, $4, $4, $5, 0)
ON CONFLICT (code) DO NOTHING`, code, review.RewardAmount, review.RewardUserID, now, grantNote); err != nil {
		return nil, fmt.Errorf("record affiliate reward balance history: %w", err)
	}
	if err := markAffiliateRewardPaid(ctx, client, review.ID, adminID, note, now); err != nil {
		return nil, err
	}
	return &service.AffiliateRewardGrantEffect{UserID: review.RewardUserID, Kind: "balance"}, nil
}

func affiliateRewardGroupExists(ctx context.Context, client affiliateQueryExecer, groupID int64) (bool, error) {
	rows, err := client.QueryContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM groups
    WHERE id = $1 AND deleted_at IS NULL AND status = 'active'
)`, groupID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false, rows.Err()
	}
	var exists bool
	if err := rows.Scan(&exists); err != nil {
		return false, err
	}
	return exists, rows.Err()
}

func affiliateBalanceGrantExists(ctx context.Context, client affiliateQueryExecer, reviewID, userID int64, amount float64) (bool, error) {
	rows, err := client.QueryContext(ctx, `
SELECT user_id, amount::double precision
FROM referral.balance_grants
WHERE review_id = $1`, reviewID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false, rows.Err()
	}
	var existingUserID int64
	var existingAmount float64
	if err := rows.Scan(&existingUserID, &existingAmount); err != nil {
		return false, err
	}
	if existingUserID != userID || math.Abs(existingAmount-amount) > 0.00000001 {
		return false, errors.New("affiliate reward grant does not match review")
	}
	return true, rows.Err()
}

func markAffiliateRewardPaid(ctx context.Context, client affiliateQueryExecer, reviewID, adminID int64, note string, paidAt time.Time) error {
	_, err := client.ExecContext(ctx, `
UPDATE referral.reward_reviews
SET status = 'paid',
    reviewed_by = $2,
    reviewed_at = $3,
    review_note = $4,
    paid_at = $3,
    updated_at = $3
WHERE id = $1`, reviewID, adminID, paidAt, note)
	if err != nil {
		return fmt.Errorf("mark affiliate reward paid: %w", err)
	}
	return nil
}

func isAffiliateRegistrationRewardType(rewardType string) bool {
	switch rewardType {
	case service.AffiliateRewardTypeRegistrationInviteeTrial,
		service.AffiliateRewardTypeRegistrationInviteeBonus,
		service.AffiliateRewardTypeRegistrationInviterBonus:
		return true
	default:
		return false
	}
}

func isAffiliateFirstRechargeRewardType(rewardType string) bool {
	return rewardType == service.AffiliateRewardTypeFirstRechargeInviteeBonus ||
		rewardType == service.AffiliateRewardTypeFirstRechargeInviterBonus
}

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func truncateAffiliateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.ValidString(value[:maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}
