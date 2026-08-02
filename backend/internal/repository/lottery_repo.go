package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type lotteryRepository struct {
	db *sql.DB
}

func NewLotteryRepository(db *sql.DB) service.LotteryRepository {
	return &lotteryRepository{db: db}
}

type lotterySQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const lotteryCampaignColumns = `
id, name, description, mode, status, starts_at, ends_at, draw_at,
per_user_limit, minimum_balance, required_subscription_group_ids,
created_by, updated_by, created_at, updated_at`

func scanLotteryCampaign(scan func(...any) error) (*service.LotteryCampaign, error) {
	campaign := &service.LotteryCampaign{}
	var requiredGroupIDs pq.Int64Array
	var drawAt sql.NullTime
	var createdBy, updatedBy sql.NullInt64
	if err := scan(
		&campaign.ID, &campaign.Name, &campaign.Description, &campaign.Mode, &campaign.Status,
		&campaign.StartsAt, &campaign.EndsAt, &drawAt, &campaign.PerUserLimit,
		&campaign.MinimumBalance, &requiredGroupIDs, &createdBy, &updatedBy,
		&campaign.CreatedAt, &campaign.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if drawAt.Valid {
		value := drawAt.Time
		campaign.DrawAt = &value
	}
	if createdBy.Valid {
		value := createdBy.Int64
		campaign.CreatedBy = &value
	}
	if updatedBy.Valid {
		value := updatedBy.Int64
		campaign.UpdatedBy = &value
	}
	campaign.RequiredSubscriptionGroupIDs = []int64(requiredGroupIDs)
	return campaign, nil
}

func (r *lotteryRepository) ListForUser(ctx context.Context, userID int64, params service.LotteryListParams, now time.Time) ([]service.LotteryCampaign, int64, error) {
	return r.listCampaigns(ctx, userID, false, params, now)
}

func (r *lotteryRepository) ListForAdmin(ctx context.Context, params service.LotteryListParams, now time.Time) ([]service.LotteryCampaign, int64, error) {
	return r.listCampaigns(ctx, 0, true, params, now)
}

func (r *lotteryRepository) listCampaigns(ctx context.Context, userID int64, admin bool, params service.LotteryListParams, now time.Time) ([]service.LotteryCampaign, int64, error) {
	where := []string{"1=1"}
	args := make([]any, 0, 5)
	if !admin {
		where = append(where, "status <> 'draft'")
	}
	if params.Status != "" {
		args = append(args, params.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if params.Mode != "" {
		args = append(args, params.Mode)
		where = append(where, fmt.Sprintf("mode = $%d", len(args)))
	}
	if params.Search != "" {
		args = append(args, "%"+params.Search+"%")
		where = append(where, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d)", len(args), len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lottery_campaigns WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count lottery campaigns: %w", err)
	}
	args = append(args, params.PageSize, (params.Page-1)*params.PageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+lotteryCampaignColumns+`
FROM lottery_campaigns WHERE `+whereSQL+`
ORDER BY starts_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list lottery campaigns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	campaigns := make([]service.LotteryCampaign, 0)
	for rows.Next() {
		campaign, scanErr := scanLotteryCampaign(rows.Scan)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan lottery campaign: %w", scanErr)
		}
		if err := r.decorateCampaign(ctx, r.db, campaign, userID, admin, now); err != nil {
			return nil, 0, err
		}
		campaigns = append(campaigns, *campaign)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate lottery campaigns: %w", err)
	}
	return campaigns, total, nil
}

func (r *lotteryRepository) GetForUser(ctx context.Context, userID, campaignID int64, now time.Time) (*service.LotteryCampaign, error) {
	campaign, err := r.getCampaign(ctx, r.db, campaignID, false)
	if err != nil {
		return nil, err
	}
	if err := r.decorateCampaign(ctx, r.db, campaign, userID, false, now); err != nil {
		return nil, err
	}
	return campaign, nil
}

func (r *lotteryRepository) GetForAdmin(ctx context.Context, campaignID int64, now time.Time) (*service.LotteryCampaign, error) {
	campaign, err := r.getCampaign(ctx, r.db, campaignID, true)
	if err != nil {
		return nil, err
	}
	if err := r.decorateCampaign(ctx, r.db, campaign, 0, true, now); err != nil {
		return nil, err
	}
	return campaign, nil
}

func (r *lotteryRepository) getCampaign(ctx context.Context, queryer lotterySQL, campaignID int64, admin bool) (*service.LotteryCampaign, error) {
	query := `SELECT ` + lotteryCampaignColumns + ` FROM lottery_campaigns WHERE id = $1`
	if !admin {
		query += ` AND status <> 'draft'`
	}
	campaign, err := scanLotteryCampaign(queryer.QueryRowContext(ctx, query, campaignID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLotteryCampaignNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get lottery campaign: %w", err)
	}
	return campaign, nil
}

func (r *lotteryRepository) decorateCampaign(ctx context.Context, queryer lotterySQL, campaign *service.LotteryCampaign, userID int64, admin bool, now time.Time) error {
	prizes, err := loadLotteryPrizes(ctx, queryer, campaign.ID, false)
	if err != nil {
		return err
	}
	campaign.Prizes = prizes
	campaign.State = lotteryCampaignState(campaign, now)
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries WHERE campaign_id = $1`, campaign.ID).Scan(&campaign.EntryCount); err != nil {
		return fmt.Errorf("count lottery entries: %w", err)
	}
	if admin {
		campaign.Eligible = true
		return nil
	}
	eligible, reason, count, err := lotteryUserEligibility(ctx, queryer, campaign, userID, now)
	if err != nil {
		return err
	}
	campaign.Eligible = eligible
	campaign.EligibilityReason = reason
	campaign.UserEntryCount = count
	return nil
}

func lotteryCampaignState(campaign *service.LotteryCampaign, now time.Time) string {
	if campaign.Status == service.LotteryCampaignCompleted {
		return "completed"
	}
	if campaign.Status == service.LotteryCampaignPaused {
		return "paused"
	}
	if campaign.Status == service.LotteryCampaignDraft {
		return "draft"
	}
	if now.Before(campaign.StartsAt) {
		return "not_started"
	}
	if !now.Before(campaign.EndsAt) {
		if campaign.Mode == service.LotteryModeScheduled {
			return "awaiting_draw"
		}
		return "ended"
	}
	return "active"
}

func lotteryUserEligibility(ctx context.Context, queryer lotterySQL, campaign *service.LotteryCampaign, userID int64, now time.Time) (bool, string, int, error) {
	var balance float64
	if err := queryer.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = $1 AND status = 'active' AND deleted_at IS NULL`, userID).Scan(&balance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, "user_inactive", 0, service.ErrLotteryIneligible
		}
		return false, "", 0, fmt.Errorf("load lottery participant: %w", err)
	}
	var count int
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries WHERE campaign_id = $1 AND user_id = $2`, campaign.ID, userID).Scan(&count); err != nil {
		return false, "", 0, fmt.Errorf("count user lottery entries: %w", err)
	}
	state := lotteryCampaignState(campaign, now)
	if state != "active" {
		return false, state, count, nil
	}
	if count >= campaign.PerUserLimit {
		return false, "limit_reached", count, nil
	}
	if balance < campaign.MinimumBalance {
		return false, "minimum_balance", count, nil
	}
	if len(campaign.RequiredSubscriptionGroupIDs) > 0 {
		var eligible bool
		err := queryer.QueryRowContext(ctx, `SELECT EXISTS (
SELECT 1 FROM user_subscriptions
WHERE user_id = $1 AND group_id = ANY($2) AND status = 'active'
  AND expires_at > $3 AND deleted_at IS NULL
)`, userID, pq.Array(campaign.RequiredSubscriptionGroupIDs), now).Scan(&eligible)
		if err != nil {
			return false, "", count, fmt.Errorf("check lottery subscription eligibility: %w", err)
		}
		if !eligible {
			return false, "subscription_required", count, nil
		}
	}
	return true, "", count, nil
}

func loadLotteryPrizes(ctx context.Context, queryer lotterySQL, campaignID int64, forUpdate bool) ([]service.LotteryPrize, error) {
	query := `SELECT id, campaign_id, name, prize_type, balance_amount, subscription_group_id,
subscription_validity_days, probability_bps, inventory, awarded_count, is_enabled,
sort_order, created_at, updated_at FROM lottery_prizes WHERE campaign_id = $1 ORDER BY sort_order, id`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	rows, err := queryer.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list lottery prizes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	prizes := make([]service.LotteryPrize, 0)
	for rows.Next() {
		var prize service.LotteryPrize
		var groupID sql.NullInt64
		if err := rows.Scan(&prize.ID, &prize.CampaignID, &prize.Name, &prize.PrizeType,
			&prize.BalanceAmount, &groupID, &prize.SubscriptionValidityDays,
			&prize.ProbabilityBPS, &prize.Inventory, &prize.AwardedCount,
			&prize.IsEnabled, &prize.SortOrder, &prize.CreatedAt, &prize.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan lottery prize: %w", err)
		}
		if groupID.Valid {
			value := groupID.Int64
			prize.SubscriptionGroupID = &value
		}
		prizes = append(prizes, prize)
	}
	return prizes, rows.Err()
}

func (r *lotteryRepository) Create(ctx context.Context, actorUserID int64, input service.LotteryCampaignInput) (*service.LotteryCampaign, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create lottery campaign: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateLotterySubscriptionGroups(ctx, tx, input.RequiredSubscriptionGroupIDs, input.Prizes); err != nil {
		return nil, err
	}
	campaign, err := insertLotteryCampaign(ctx, tx, actorUserID, input)
	if err != nil {
		return nil, err
	}
	if err := insertLotteryPrizes(ctx, tx, campaign.ID, input.Prizes); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create lottery campaign: %w", err)
	}
	return r.GetForAdmin(ctx, campaign.ID, time.Now().UTC())
}

func insertLotteryCampaign(ctx context.Context, tx *sql.Tx, actorUserID int64, input service.LotteryCampaignInput) (*service.LotteryCampaign, error) {
	row := tx.QueryRowContext(ctx, `INSERT INTO lottery_campaigns (
name, description, mode, status, starts_at, ends_at, draw_at, per_user_limit,
minimum_balance, required_subscription_group_ids, created_by, updated_by
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11) RETURNING `+lotteryCampaignColumns,
		input.Name, input.Description, input.Mode, input.Status, input.StartsAt, input.EndsAt,
		input.DrawAt, input.PerUserLimit, input.MinimumBalance,
		pq.Array(nonNilLotteryGroupIDs(input.RequiredSubscriptionGroupIDs)), actorUserID)
	campaign, err := scanLotteryCampaign(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("insert lottery campaign: %w", err)
	}
	return campaign, nil
}

func insertLotteryPrizes(ctx context.Context, tx *sql.Tx, campaignID int64, prizes []service.LotteryPrizeInput) error {
	for _, prize := range prizes {
		_, err := tx.ExecContext(ctx, `INSERT INTO lottery_prizes (
campaign_id, name, prize_type, balance_amount, subscription_group_id,
subscription_validity_days, probability_bps, inventory, is_enabled, sort_order
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, campaignID, prize.Name, prize.PrizeType,
			prize.BalanceAmount, prize.SubscriptionGroupID, prize.SubscriptionValidityDays,
			prize.ProbabilityBPS, prize.Inventory, prize.IsEnabled, prize.SortOrder)
		if err != nil {
			return fmt.Errorf("insert lottery prize: %w", err)
		}
	}
	return nil
}

func validateLotterySubscriptionGroups(ctx context.Context, tx *sql.Tx, requiredGroupIDs []int64, prizes []service.LotteryPrizeInput) error {
	groupIDs := make(map[int64]struct{}, len(requiredGroupIDs)+len(prizes))
	for _, groupID := range requiredGroupIDs {
		groupIDs[groupID] = struct{}{}
	}
	for _, prize := range prizes {
		if prize.PrizeType != service.LotteryPrizeSubscriptionCode || prize.SubscriptionGroupID == nil {
			continue
		}
		groupIDs[*prize.SubscriptionGroupID] = struct{}{}
	}
	if len(groupIDs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(groupIDs))
	for groupID := range groupIDs {
		ids = append(ids, groupID)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups
	WHERE id = ANY($1) AND subscription_type = 'subscription'
	  AND status = 'active' AND deleted_at IS NULL`, pq.Array(ids)).Scan(&count); err != nil {
		return fmt.Errorf("validate lottery subscription groups: %w", err)
	}
	if count != len(ids) {
		return service.ErrLotteryInvalid
	}
	return nil
}

func (r *lotteryRepository) Update(ctx context.Context, campaignID, actorUserID int64, input service.LotteryCampaignInput) (*service.LotteryCampaign, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin update lottery campaign: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var lockedID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM lottery_campaigns WHERE id = $1 FOR UPDATE`, campaignID).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrLotteryCampaignNotFound
		}
		return nil, fmt.Errorf("lock lottery campaign: %w", err)
	}
	var entryCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries WHERE campaign_id = $1`, campaignID).Scan(&entryCount); err != nil {
		return nil, fmt.Errorf("count lottery campaign entries: %w", err)
	}
	if entryCount > 0 {
		return nil, service.ErrLotteryHasEntries
	}
	if err := validateLotterySubscriptionGroups(ctx, tx, input.RequiredSubscriptionGroupIDs, input.Prizes); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE lottery_campaigns SET
name=$2, description=$3, mode=$4, status=$5, starts_at=$6, ends_at=$7, draw_at=$8,
per_user_limit=$9, minimum_balance=$10, required_subscription_group_ids=$11,
updated_by=$12, updated_at=NOW() WHERE id=$1`, campaignID, input.Name, input.Description,
		input.Mode, input.Status, input.StartsAt, input.EndsAt, input.DrawAt,
		input.PerUserLimit, input.MinimumBalance, pq.Array(nonNilLotteryGroupIDs(input.RequiredSubscriptionGroupIDs)), actorUserID)
	if err != nil {
		return nil, fmt.Errorf("update lottery campaign: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, service.ErrLotteryCampaignNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM lottery_prizes WHERE campaign_id = $1`, campaignID); err != nil {
		return nil, fmt.Errorf("replace lottery prizes: %w", err)
	}
	if err := insertLotteryPrizes(ctx, tx, campaignID, input.Prizes); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update lottery campaign: %w", err)
	}
	return r.GetForAdmin(ctx, campaignID, time.Now().UTC())
}

func nonNilLotteryGroupIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

func (r *lotteryRepository) SetStatus(ctx context.Context, campaignID, actorUserID int64, status string, now time.Time) (*service.LotteryCampaign, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin lottery status update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentStatus string
	var mode string
	var endsAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT status, mode, ends_at FROM lottery_campaigns WHERE id=$1 FOR UPDATE`, campaignID).Scan(&currentStatus, &mode, &endsAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrLotteryCampaignNotFound
		}
		return nil, fmt.Errorf("lock lottery campaign status: %w", err)
	}
	if currentStatus == service.LotteryCampaignCompleted && status != service.LotteryCampaignCompleted {
		return nil, service.ErrLotteryInvalidTransition
	}
	if status == service.LotteryCampaignCompleted && mode == service.LotteryModeScheduled {
		return nil, service.ErrLotteryInvalidTransition
	}
	if status == service.LotteryCampaignActive && mode != service.LotteryModeScheduled && !now.Before(endsAt) {
		return nil, service.ErrLotteryEnded
	}
	if _, err := tx.ExecContext(ctx, `UPDATE lottery_campaigns SET status=$2, updated_by=$3, updated_at=NOW() WHERE id=$1`, campaignID, status, actorUserID); err != nil {
		return nil, fmt.Errorf("update lottery status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit lottery status update: %w", err)
	}
	return r.GetForAdmin(ctx, campaignID, now)
}

func (r *lotteryRepository) Delete(ctx context.Context, campaignID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete lottery campaign: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var lockedID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM lottery_campaigns WHERE id=$1 FOR UPDATE`, campaignID).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrLotteryCampaignNotFound
		}
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries WHERE campaign_id=$1`, campaignID).Scan(&count); err != nil {
		return fmt.Errorf("count lottery campaign entries: %w", err)
	}
	if count > 0 {
		return service.ErrLotteryHasEntries
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM lottery_campaigns WHERE id=$1`, campaignID)
	if err != nil {
		return fmt.Errorf("delete lottery campaign: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrLotteryCampaignNotFound
	}
	return tx.Commit()
}

const lotteryEntryColumns = `
e.id, e.campaign_id, c.name, c.mode, e.user_id, COALESCE(u.email, ''), e.idempotency_key,
e.status, e.prize_id, e.prize_name, e.prize_type, e.balance_amount,
e.subscription_group_id, e.subscription_validity_days, e.reward_redeem_code_id,
COALESCE(rc.code, ''), e.created_at, e.resolved_at`

func scanLotteryEntry(scan func(...any) error) (*service.LotteryEntry, error) {
	entry := &service.LotteryEntry{}
	var prizeID, groupID, codeID sql.NullInt64
	var resolvedAt sql.NullTime
	if err := scan(&entry.ID, &entry.CampaignID, &entry.CampaignName, &entry.CampaignMode,
		&entry.UserID, &entry.UserEmail, &entry.IdempotencyKey, &entry.Status,
		&prizeID, &entry.PrizeName, &entry.PrizeType, &entry.BalanceAmount,
		&groupID, &entry.SubscriptionValidityDays, &codeID, &entry.RewardCode,
		&entry.CreatedAt, &resolvedAt); err != nil {
		return nil, err
	}
	if prizeID.Valid {
		value := prizeID.Int64
		entry.PrizeID = &value
	}
	if groupID.Valid {
		value := groupID.Int64
		entry.SubscriptionGroupID = &value
	}
	if codeID.Valid {
		value := codeID.Int64
		entry.RewardRedeemCodeID = &value
	}
	if resolvedAt.Valid {
		value := resolvedAt.Time
		entry.ResolvedAt = &value
	}
	return entry, nil
}

func getLotteryEntryByKey(ctx context.Context, queryer lotterySQL, campaignID, userID int64, key string) (*service.LotteryEntry, error) {
	entry, err := scanLotteryEntry(queryer.QueryRowContext(ctx, `SELECT `+lotteryEntryColumns+`
FROM lottery_entries e JOIN lottery_campaigns c ON c.id=e.campaign_id
JOIN users u ON u.id=e.user_id LEFT JOIN redeem_codes rc ON rc.id=e.reward_redeem_code_id
WHERE e.campaign_id=$1 AND e.user_id=$2 AND e.idempotency_key=$3`, campaignID, userID, key).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLotteryEntryNotFound
	}
	return entry, err
}

func (r *lotteryRepository) Participate(ctx context.Context, userID, campaignID int64, key string, now time.Time, random service.LotteryRandomSource) (*service.LotteryEntry, bool, error) {
	if existing, err := getLotteryEntryByKey(ctx, r.db, campaignID, userID, key); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, service.ErrLotteryEntryNotFound) {
		return nil, false, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, false, fmt.Errorf("begin lottery participation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	campaign, err := scanLotteryCampaign(tx.QueryRowContext(ctx, `SELECT `+lotteryCampaignColumns+` FROM lottery_campaigns WHERE id=$1 FOR UPDATE`, campaignID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, service.ErrLotteryCampaignNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("lock lottery campaign: %w", err)
	}
	if existing, err := getLotteryEntryByKey(ctx, tx, campaignID, userID, key); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, service.ErrLotteryEntryNotFound) {
		return nil, false, err
	}
	if campaign.Status != service.LotteryCampaignActive {
		return nil, false, service.ErrLotteryUnavailable
	}
	if now.Before(campaign.StartsAt) {
		return nil, false, service.ErrLotteryNotStarted
	}
	if !now.Before(campaign.EndsAt) {
		return nil, false, service.ErrLotteryEnded
	}
	eligible, _, count, err := lotteryUserEligibility(ctx, tx, campaign, userID, now)
	if err != nil {
		return nil, false, err
	}
	if !eligible {
		if count >= campaign.PerUserLimit {
			return nil, false, service.ErrLotteryLimitReached
		}
		return nil, false, service.ErrLotteryIneligible
	}
	status := service.LotteryEntryPending
	var prize *service.LotteryPrize
	if campaign.Mode == service.LotteryModeInstant {
		prizes, loadErr := loadLotteryPrizes(ctx, tx, campaignID, true)
		if loadErr != nil {
			return nil, false, loadErr
		}
		roll, randomErr := random.Intn(10000)
		if randomErr != nil {
			return nil, false, fmt.Errorf("generate lottery draw: %w", randomErr)
		}
		prize = service.SelectLotteryPrize(prizes, roll)
		if prize == nil {
			status = service.LotteryEntryNotWon
		} else {
			status = service.LotteryEntryWon
		}
	}
	entry, err := fulfillLotteryEntry(ctx, tx, campaign, userID, key, status, prize, now, random)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit lottery participation: %w", err)
	}
	return entry, false, nil
}

func fulfillLotteryEntry(ctx context.Context, tx *sql.Tx, campaign *service.LotteryCampaign, userID int64, key, status string, prize *service.LotteryPrize, now time.Time, random service.LotteryRandomSource) (*service.LotteryEntry, error) {
	var codeID *int64
	if prize != nil {
		result, err := tx.ExecContext(ctx, `UPDATE lottery_prizes SET awarded_count=awarded_count+1, updated_at=NOW()
WHERE id=$1 AND is_enabled=TRUE AND awarded_count < inventory`, prize.ID)
		if err != nil {
			return nil, fmt.Errorf("reserve lottery prize: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			prize = nil
			status = service.LotteryEntryNotWon
		}
	}
	if prize != nil && prize.PrizeType == service.LotteryPrizeBalance {
		result, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1, updated_at=NOW() WHERE id=$2 AND status='active' AND deleted_at IS NULL`, prize.BalanceAmount, userID)
		if err != nil {
			return nil, fmt.Errorf("credit lottery balance: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			return nil, service.ErrLotteryIneligible
		}
	}
	if prize != nil && prize.PrizeType == service.LotteryPrizeSubscriptionCode {
		value, err := insertLotteryRedeemCode(ctx, tx, campaign.ID, prize, random)
		if err != nil {
			return nil, err
		}
		codeID = &value
	}
	resolvedAt := any(nil)
	if status != service.LotteryEntryPending {
		resolvedAt = now
	}
	var prizeID any
	var prizeName, prizeType string
	var balanceAmount float64
	var groupID any
	var validityDays int
	if prize != nil {
		prizeID, prizeName, prizeType = prize.ID, prize.Name, prize.PrizeType
		balanceAmount, groupID, validityDays = prize.BalanceAmount, prize.SubscriptionGroupID, prize.SubscriptionValidityDays
	}
	var entryID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO lottery_entries (
campaign_id,user_id,idempotency_key,status,prize_id,prize_name,prize_type,balance_amount,
subscription_group_id,subscription_validity_days,reward_redeem_code_id,created_at,resolved_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`, campaign.ID, userID, key,
		status, prizeID, prizeName, prizeType, balanceAmount, groupID, validityDays, codeID, now, resolvedAt).Scan(&entryID)
	if err != nil {
		return nil, fmt.Errorf("insert lottery entry: %w", err)
	}
	if err := insertLotteryEntryEvents(ctx, tx, campaign.ID, entryID, userID, status, prize, codeID, now, true); err != nil {
		return nil, err
	}
	entry, err := scanLotteryEntry(tx.QueryRowContext(ctx, `SELECT `+lotteryEntryColumns+`
FROM lottery_entries e JOIN lottery_campaigns c ON c.id=e.campaign_id JOIN users u ON u.id=e.user_id
LEFT JOIN redeem_codes rc ON rc.id=e.reward_redeem_code_id WHERE e.id=$1`, entryID).Scan)
	if err != nil {
		return nil, fmt.Errorf("load created lottery entry: %w", err)
	}
	return entry, nil
}

func insertLotteryRedeemCode(ctx context.Context, tx *sql.Tx, campaignID int64, prize *service.LotteryPrize, random service.LotteryRandomSource) (int64, error) {
	for attempt := 0; attempt < 3; attempt++ {
		code, err := random.RedeemCode()
		if err != nil {
			return 0, fmt.Errorf("generate lottery redeem code: %w", err)
		}
		var codeID int64
		err = tx.QueryRowContext(ctx, `INSERT INTO redeem_codes (
code,type,value,status,notes,group_id,validity_days,created_at
) VALUES ($1,'subscription',0,'unused',$2,$3,$4,NOW()) RETURNING id`, code,
			fmt.Sprintf("Lottery campaign %d reward", campaignID), prize.SubscriptionGroupID,
			prize.SubscriptionValidityDays).Scan(&codeID)
		if err == nil {
			return codeID, nil
		}
		var pqErr *pq.Error
		if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
			return 0, fmt.Errorf("create lottery subscription redeem code: %w", err)
		}
	}
	return 0, fmt.Errorf("create unique lottery subscription redeem code")
}

func insertLotteryEntryEvents(ctx context.Context, tx *sql.Tx, campaignID, entryID, userID int64, status string, prize *service.LotteryPrize, codeID *int64, now time.Time, includeParticipation bool) error {
	if includeParticipation {
		if _, err := tx.ExecContext(ctx, `INSERT INTO lottery_events (campaign_id,entry_id,user_id,event_type,created_at)
VALUES ($1,$2,$3,'participated',$4)`, campaignID, entryID, userID, now); err != nil {
			return fmt.Errorf("audit lottery participation: %w", err)
		}
	}
	eventType := "entry_not_won"
	if status == service.LotteryEntryWon {
		eventType = "entry_won"
	}
	if status != service.LotteryEntryPending {
		if _, err := tx.ExecContext(ctx, `INSERT INTO lottery_events (campaign_id,entry_id,user_id,event_type,created_at)
VALUES ($1,$2,$3,$4,$5)`, campaignID, entryID, userID, eventType, now); err != nil {
			return fmt.Errorf("audit lottery result: %w", err)
		}
	}
	if prize != nil && prize.PrizeType == service.LotteryPrizeBalance {
		_, err := tx.ExecContext(ctx, `INSERT INTO lottery_events (campaign_id,entry_id,user_id,event_type,balance_amount,created_at)
VALUES ($1,$2,$3,'balance_credited',$4,$5)`, campaignID, entryID, userID, prize.BalanceAmount, now)
		if err != nil {
			return fmt.Errorf("audit lottery balance reward: %w", err)
		}
	}
	if prize != nil && prize.PrizeType == service.LotteryPrizeSubscriptionCode && codeID != nil {
		_, err := tx.ExecContext(ctx, `INSERT INTO lottery_events (campaign_id,entry_id,user_id,event_type,redeem_code_id,created_at)
VALUES ($1,$2,$3,'subscription_code_issued',$4,$5)`, campaignID, entryID, userID, *codeID, now)
		if err != nil {
			return fmt.Errorf("audit lottery subscription reward: %w", err)
		}
	}
	return nil
}

func (r *lotteryRepository) ListUserEntries(ctx context.Context, userID int64, params service.LotteryListParams) ([]service.LotteryEntry, int64, error) {
	return r.listEntries(ctx, userID, 0, params)
}

func (r *lotteryRepository) ListAdminEntries(ctx context.Context, campaignID int64, params service.LotteryListParams) ([]service.LotteryEntry, int64, error) {
	return r.listEntries(ctx, 0, campaignID, params)
}

func (r *lotteryRepository) listEntries(ctx context.Context, userID, campaignID int64, params service.LotteryListParams) ([]service.LotteryEntry, int64, error) {
	where := "1=1"
	args := make([]any, 0, 4)
	if userID > 0 {
		args = append(args, userID)
		where += fmt.Sprintf(" AND e.user_id=$%d", len(args))
	}
	if campaignID > 0 {
		args = append(args, campaignID)
		where += fmt.Sprintf(" AND e.campaign_id=$%d", len(args))
	}
	if params.Status != "" {
		args = append(args, params.Status)
		where += fmt.Sprintf(" AND e.status=$%d", len(args))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries e WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count lottery entries: %w", err)
	}
	args = append(args, params.PageSize, (params.Page-1)*params.PageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+lotteryEntryColumns+`
FROM lottery_entries e JOIN lottery_campaigns c ON c.id=e.campaign_id JOIN users u ON u.id=e.user_id
LEFT JOIN redeem_codes rc ON rc.id=e.reward_redeem_code_id WHERE `+where+`
ORDER BY e.created_at DESC,e.id DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list lottery entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	entries := make([]service.LotteryEntry, 0)
	for rows.Next() {
		entry, err := scanLotteryEntry(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan lottery entry: %w", err)
		}
		entries = append(entries, *entry)
	}
	return entries, total, rows.Err()
}

func (r *lotteryRepository) ListDueScheduledCampaignIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.id FROM lottery_campaigns c
WHERE c.mode='scheduled' AND c.status='active' AND c.draw_at <= $1
AND NOT EXISTS (SELECT 1 FROM lottery_draw_runs d WHERE d.campaign_id=c.id)
ORDER BY c.draw_at,c.id LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due lottery campaigns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *lotteryRepository) DrawScheduled(ctx context.Context, campaignID int64, triggeredBy *int64, now time.Time, random service.LotteryRandomSource) (*service.LotteryDrawResult, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin scheduled lottery draw: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	campaign, err := scanLotteryCampaign(tx.QueryRowContext(ctx, `SELECT `+lotteryCampaignColumns+` FROM lottery_campaigns WHERE id=$1 FOR UPDATE`, campaignID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrLotteryCampaignNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock scheduled lottery: %w", err)
	}
	var existing service.LotteryDrawResult
	err = tx.QueryRowContext(ctx, `SELECT participant_count,winner_count FROM lottery_draw_runs WHERE campaign_id=$1`, campaignID).Scan(&existing.ParticipantCount, &existing.WinnerCount)
	if err == nil {
		existing.CampaignID = campaignID
		existing.AlreadyCompleted = true
		return &existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read scheduled lottery draw: %w", err)
	}
	if campaign.Mode != service.LotteryModeScheduled || campaign.Status != service.LotteryCampaignActive {
		return nil, service.ErrLotteryUnavailable
	}
	if campaign.DrawAt == nil || now.Before(*campaign.DrawAt) {
		return nil, service.ErrLotteryNotStarted
	}
	prizes, err := loadLotteryPrizes(ctx, tx, campaignID, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,user_id,idempotency_key FROM lottery_entries
WHERE campaign_id=$1 AND status='pending' ORDER BY id FOR UPDATE`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("lock scheduled lottery entries: %w", err)
	}
	type pendingEntry struct {
		id, userID int64
		key        string
	}
	pending := make([]pendingEntry, 0)
	for rows.Next() {
		var entry pendingEntry
		if err := rows.Scan(&entry.id, &entry.userID, &entry.key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		pending = append(pending, entry)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	assignments, err := service.AllocateScheduledLotteryPrizes(prizes, len(pending), random)
	if err != nil {
		return nil, fmt.Errorf("generate scheduled lottery draw: %w", err)
	}
	result := &service.LotteryDrawResult{CampaignID: campaignID, ParticipantCount: len(pending)}
	for pendingIndex, pendingEntry := range pending {
		prize := assignments[pendingIndex]
		status := service.LotteryEntryNotWon
		if prize != nil {
			status = service.LotteryEntryWon
		}
		entry, err := resolveScheduledLotteryEntry(ctx, tx, campaign, pendingEntry.id, pendingEntry.userID, status, prize, now, random)
		if err != nil {
			return nil, err
		}
		if entry.Status == service.LotteryEntryWon {
			result.WinnerCount++
			if entry.PrizeType == service.LotteryPrizeBalance {
				result.BalanceRewardUserIDs = append(result.BalanceRewardUserIDs, entry.UserID)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO lottery_draw_runs (
campaign_id,participant_count,winner_count,started_at,completed_at,triggered_by
) VALUES ($1,$2,$3,$4,$4,$5)`, campaignID, result.ParticipantCount, result.WinnerCount, now, triggeredBy); err != nil {
		return nil, fmt.Errorf("record scheduled lottery draw: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE lottery_campaigns SET status='completed',updated_at=NOW(),updated_by=COALESCE($2,updated_by) WHERE id=$1`, campaignID, triggeredBy); err != nil {
		return nil, fmt.Errorf("complete scheduled lottery campaign: %w", err)
	}
	metadata, _ := json.Marshal(map[string]int{"participants": result.ParticipantCount, "winners": result.WinnerCount})
	if _, err := tx.ExecContext(ctx, `INSERT INTO lottery_events (campaign_id,event_type,metadata,created_at)
VALUES ($1,'scheduled_draw_completed',$2,$3)`, campaignID, metadata, now); err != nil {
		return nil, fmt.Errorf("audit scheduled lottery draw: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit scheduled lottery draw: %w", err)
	}
	return result, nil
}

func resolveScheduledLotteryEntry(ctx context.Context, tx *sql.Tx, campaign *service.LotteryCampaign, entryID, userID int64, status string, prize *service.LotteryPrize, now time.Time, random service.LotteryRandomSource) (*service.LotteryEntry, error) {
	var codeID *int64
	if prize != nil {
		result, err := tx.ExecContext(ctx, `UPDATE lottery_prizes SET awarded_count=awarded_count+1,updated_at=NOW()
WHERE id=$1 AND is_enabled=TRUE AND awarded_count<inventory`, prize.ID)
		if err != nil {
			return nil, err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			prize = nil
			status = service.LotteryEntryNotWon
		}
	}
	if prize != nil && prize.PrizeType == service.LotteryPrizeBalance {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,updated_at=NOW() WHERE id=$2`, prize.BalanceAmount, userID); err != nil {
			return nil, fmt.Errorf("credit scheduled lottery balance: %w", err)
		}
	}
	if prize != nil && prize.PrizeType == service.LotteryPrizeSubscriptionCode {
		value, err := insertLotteryRedeemCode(ctx, tx, campaign.ID, prize, random)
		if err != nil {
			return nil, err
		}
		codeID = &value
	}
	var prizeID any
	var prizeName, prizeType string
	var balanceAmount float64
	var groupID any
	var validityDays int
	if prize != nil {
		prizeID, prizeName, prizeType = prize.ID, prize.Name, prize.PrizeType
		balanceAmount, groupID, validityDays = prize.BalanceAmount, prize.SubscriptionGroupID, prize.SubscriptionValidityDays
	}
	result, err := tx.ExecContext(ctx, `UPDATE lottery_entries SET status=$2,prize_id=$3,prize_name=$4,
prize_type=$5,balance_amount=$6,subscription_group_id=$7,subscription_validity_days=$8,
reward_redeem_code_id=$9,resolved_at=$10 WHERE id=$1 AND status='pending'`, entryID, status,
		prizeID, prizeName, prizeType, balanceAmount, groupID, validityDays, codeID, now)
	if err != nil {
		return nil, fmt.Errorf("resolve scheduled lottery entry: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return nil, service.ErrLotteryAlreadyDrawn
	}
	if err := insertLotteryEntryEvents(ctx, tx, campaign.ID, entryID, userID, status, prize, codeID, now, false); err != nil {
		return nil, err
	}
	entry, err := scanLotteryEntry(tx.QueryRowContext(ctx, `SELECT `+lotteryEntryColumns+`
FROM lottery_entries e JOIN lottery_campaigns c ON c.id=e.campaign_id JOIN users u ON u.id=e.user_id
LEFT JOIN redeem_codes rc ON rc.id=e.reward_redeem_code_id WHERE e.id=$1`, entryID).Scan)
	if err != nil {
		return nil, err
	}
	return entry, nil
}
