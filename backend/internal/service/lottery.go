package service

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	LotteryModeInstant   = "instant"
	LotteryModeScheduled = "scheduled"

	LotteryCampaignDraft     = "draft"
	LotteryCampaignActive    = "active"
	LotteryCampaignPaused    = "paused"
	LotteryCampaignCompleted = "completed"

	LotteryEntryPending = "pending"
	LotteryEntryWon     = "won"
	LotteryEntryNotWon  = "not_won"

	LotteryPrizeBalance          = "balance"
	LotteryPrizeSubscriptionCode = "subscription_code"
)

var (
	ErrLotteryCampaignNotFound  = infraerrors.NotFound("LOTTERY_CAMPAIGN_NOT_FOUND", "lottery campaign not found")
	ErrLotteryEntryNotFound     = infraerrors.NotFound("LOTTERY_ENTRY_NOT_FOUND", "lottery entry not found")
	ErrLotteryInvalid           = infraerrors.BadRequest("LOTTERY_INVALID", "invalid lottery configuration")
	ErrLotteryNotStarted        = infraerrors.Conflict("LOTTERY_NOT_STARTED", "lottery campaign has not started")
	ErrLotteryEnded             = infraerrors.Conflict("LOTTERY_ENDED", "lottery campaign has ended")
	ErrLotteryUnavailable       = infraerrors.Conflict("LOTTERY_UNAVAILABLE", "lottery campaign is unavailable")
	ErrLotteryIneligible        = infraerrors.Forbidden("LOTTERY_INELIGIBLE", "user is not eligible for this lottery campaign")
	ErrLotteryLimitReached      = infraerrors.Conflict("LOTTERY_LIMIT_REACHED", "lottery participation limit reached")
	ErrLotteryAlreadyDrawn      = infraerrors.Conflict("LOTTERY_ALREADY_DRAWN", "scheduled lottery has already been drawn")
	ErrLotteryHasEntries        = infraerrors.Conflict("LOTTERY_HAS_ENTRIES", "lottery campaign with entries cannot change prizes or be deleted")
	ErrLotteryInvalidTransition = infraerrors.Conflict("LOTTERY_INVALID_TRANSITION", "lottery campaign status transition is not allowed")
)

type LotteryCampaign struct {
	ID                           int64          `json:"id"`
	Name                         string         `json:"name"`
	Description                  string         `json:"description"`
	Mode                         string         `json:"mode"`
	Status                       string         `json:"status"`
	State                        string         `json:"state"`
	StartsAt                     time.Time      `json:"starts_at"`
	EndsAt                       time.Time      `json:"ends_at"`
	DrawAt                       *time.Time     `json:"draw_at,omitempty"`
	PerUserLimit                 int            `json:"per_user_limit"`
	MinimumBalance               float64        `json:"minimum_balance"`
	RequiredSubscriptionGroupIDs []int64        `json:"required_subscription_group_ids"`
	Eligible                     bool           `json:"eligible"`
	EligibilityReason            string         `json:"eligibility_reason,omitempty"`
	UserEntryCount               int            `json:"user_entry_count"`
	EntryCount                   int            `json:"entry_count"`
	CreatedBy                    *int64         `json:"created_by,omitempty"`
	UpdatedBy                    *int64         `json:"updated_by,omitempty"`
	CreatedAt                    time.Time      `json:"created_at"`
	UpdatedAt                    time.Time      `json:"updated_at"`
	Prizes                       []LotteryPrize `json:"prizes"`
}

type LotteryPrize struct {
	ID                       int64     `json:"id"`
	CampaignID               int64     `json:"campaign_id"`
	Name                     string    `json:"name"`
	PrizeType                string    `json:"prize_type"`
	BalanceAmount            float64   `json:"balance_amount"`
	SubscriptionGroupID      *int64    `json:"subscription_group_id,omitempty"`
	SubscriptionValidityDays int       `json:"subscription_validity_days"`
	ProbabilityBPS           int       `json:"probability_bps"`
	Inventory                int       `json:"inventory"`
	AwardedCount             int       `json:"awarded_count"`
	IsEnabled                bool      `json:"is_enabled"`
	SortOrder                int       `json:"sort_order"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type LotteryEntry struct {
	ID                       int64      `json:"id"`
	CampaignID               int64      `json:"campaign_id"`
	CampaignName             string     `json:"campaign_name,omitempty"`
	CampaignMode             string     `json:"campaign_mode,omitempty"`
	UserID                   int64      `json:"user_id"`
	UserEmail                string     `json:"user_email,omitempty"`
	IdempotencyKey           string     `json:"-"`
	Status                   string     `json:"status"`
	PrizeID                  *int64     `json:"prize_id,omitempty"`
	PrizeName                string     `json:"prize_name,omitempty"`
	PrizeType                string     `json:"prize_type,omitempty"`
	BalanceAmount            float64    `json:"balance_amount"`
	SubscriptionGroupID      *int64     `json:"subscription_group_id,omitempty"`
	SubscriptionValidityDays int        `json:"subscription_validity_days"`
	RewardRedeemCodeID       *int64     `json:"reward_redeem_code_id,omitempty"`
	RewardCode               string     `json:"reward_code,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	ResolvedAt               *time.Time `json:"resolved_at,omitempty"`
	Replayed                 bool       `json:"replayed,omitempty"`
}

type LotteryPrizeInput struct {
	Name                     string  `json:"name"`
	PrizeType                string  `json:"prize_type"`
	BalanceAmount            float64 `json:"balance_amount"`
	SubscriptionGroupID      *int64  `json:"subscription_group_id"`
	SubscriptionValidityDays int     `json:"subscription_validity_days"`
	ProbabilityBPS           int     `json:"probability_bps"`
	Inventory                int     `json:"inventory"`
	IsEnabled                bool    `json:"is_enabled"`
	SortOrder                int     `json:"sort_order"`
}

type LotteryCampaignInput struct {
	Name                         string              `json:"name"`
	Description                  string              `json:"description"`
	Mode                         string              `json:"mode"`
	Status                       string              `json:"status"`
	StartsAt                     time.Time           `json:"starts_at"`
	EndsAt                       time.Time           `json:"ends_at"`
	DrawAt                       *time.Time          `json:"draw_at"`
	PerUserLimit                 int                 `json:"per_user_limit"`
	MinimumBalance               float64             `json:"minimum_balance"`
	RequiredSubscriptionGroupIDs []int64             `json:"required_subscription_group_ids"`
	Prizes                       []LotteryPrizeInput `json:"prizes"`
}

type LotteryListParams struct {
	Page     int
	PageSize int
	Status   string
	Mode     string
	Search   string
}

func (p LotteryListParams) Normalized() LotteryListParams {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	p.Status = strings.ToLower(strings.TrimSpace(p.Status))
	p.Mode = strings.ToLower(strings.TrimSpace(p.Mode))
	p.Search = strings.TrimSpace(p.Search)
	return p
}

type LotteryDrawResult struct {
	CampaignID           int64   `json:"campaign_id"`
	ParticipantCount     int     `json:"participant_count"`
	WinnerCount          int     `json:"winner_count"`
	AlreadyCompleted     bool    `json:"already_completed"`
	BalanceRewardUserIDs []int64 `json:"-"`
}

type LotteryRandomSource interface {
	Intn(max int) (int, error)
	RedeemCode() (string, error)
}

type LotteryRepository interface {
	ListForUser(ctx context.Context, userID int64, params LotteryListParams, now time.Time) ([]LotteryCampaign, int64, error)
	GetForUser(ctx context.Context, userID, campaignID int64, now time.Time) (*LotteryCampaign, error)
	ListForAdmin(ctx context.Context, params LotteryListParams, now time.Time) ([]LotteryCampaign, int64, error)
	GetForAdmin(ctx context.Context, campaignID int64, now time.Time) (*LotteryCampaign, error)
	Create(ctx context.Context, actorUserID int64, input LotteryCampaignInput) (*LotteryCampaign, error)
	Update(ctx context.Context, campaignID, actorUserID int64, input LotteryCampaignInput) (*LotteryCampaign, error)
	SetStatus(ctx context.Context, campaignID, actorUserID int64, status string, now time.Time) (*LotteryCampaign, error)
	Delete(ctx context.Context, campaignID int64) error
	Participate(ctx context.Context, userID, campaignID int64, idempotencyKey string, now time.Time, random LotteryRandomSource) (*LotteryEntry, bool, error)
	ListUserEntries(ctx context.Context, userID int64, params LotteryListParams) ([]LotteryEntry, int64, error)
	ListAdminEntries(ctx context.Context, campaignID int64, params LotteryListParams) ([]LotteryEntry, int64, error)
	ListDueScheduledCampaignIDs(ctx context.Context, now time.Time, limit int) ([]int64, error)
	DrawScheduled(ctx context.Context, campaignID int64, triggeredBy *int64, now time.Time, random LotteryRandomSource) (*LotteryDrawResult, error)
}

type cryptoLotteryRandom struct{}

func (cryptoLotteryRandom) Intn(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("random upper bound must be positive")
	}
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func (cryptoLotteryRandom) RedeemCode() (string, error) {
	return GenerateRedeemCode()
}

type LotteryService struct {
	repo                 LotteryRepository
	random               LotteryRandomSource
	billingCache         *BillingCacheService
	authCacheInvalidator APIKeyAuthCacheInvalidator

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
}

func NewLotteryService(repo LotteryRepository, billingCache *BillingCacheService, authCacheInvalidator APIKeyAuthCacheInvalidator) *LotteryService {
	return &LotteryService{
		repo: repo, random: cryptoLotteryRandom{}, billingCache: billingCache,
		authCacheInvalidator: authCacheInvalidator, stopCh: make(chan struct{}), doneCh: make(chan struct{}),
	}
}

func (s *LotteryService) ListForUser(ctx context.Context, userID int64, params LotteryListParams) ([]LotteryCampaign, int64, error) {
	return s.repo.ListForUser(ctx, userID, params.Normalized(), time.Now().UTC())
}

func (s *LotteryService) GetForUser(ctx context.Context, userID, campaignID int64) (*LotteryCampaign, error) {
	if campaignID <= 0 {
		return nil, ErrLotteryCampaignNotFound
	}
	return s.repo.GetForUser(ctx, userID, campaignID, time.Now().UTC())
}

func (s *LotteryService) ListForAdmin(ctx context.Context, params LotteryListParams) ([]LotteryCampaign, int64, error) {
	return s.repo.ListForAdmin(ctx, params.Normalized(), time.Now().UTC())
}

func (s *LotteryService) GetForAdmin(ctx context.Context, campaignID int64) (*LotteryCampaign, error) {
	if campaignID <= 0 {
		return nil, ErrLotteryCampaignNotFound
	}
	return s.repo.GetForAdmin(ctx, campaignID, time.Now().UTC())
}

func (s *LotteryService) Create(ctx context.Context, actorUserID int64, input LotteryCampaignInput) (*LotteryCampaign, error) {
	if actorUserID <= 0 {
		return nil, ErrLotteryInvalid
	}
	if err := normalizeLotteryCampaignInput(&input); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, actorUserID, input)
}

func (s *LotteryService) Update(ctx context.Context, campaignID, actorUserID int64, input LotteryCampaignInput) (*LotteryCampaign, error) {
	if campaignID <= 0 || actorUserID <= 0 {
		return nil, ErrLotteryInvalid
	}
	if err := normalizeLotteryCampaignInput(&input); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, campaignID, actorUserID, input)
}

func (s *LotteryService) SetStatus(ctx context.Context, campaignID, actorUserID int64, status string) (*LotteryCampaign, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if campaignID <= 0 || actorUserID <= 0 || (status != LotteryCampaignDraft && status != LotteryCampaignActive && status != LotteryCampaignPaused && status != LotteryCampaignCompleted) {
		return nil, ErrLotteryInvalid
	}
	return s.repo.SetStatus(ctx, campaignID, actorUserID, status, time.Now().UTC())
}

func (s *LotteryService) Delete(ctx context.Context, campaignID int64) error {
	if campaignID <= 0 {
		return ErrLotteryCampaignNotFound
	}
	return s.repo.Delete(ctx, campaignID)
}

func (s *LotteryService) Participate(ctx context.Context, userID, campaignID int64, idempotencyKey string) (*LotteryEntry, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if userID <= 0 || campaignID <= 0 || idempotencyKey == "" || len(idempotencyKey) > 128 {
		return nil, infraerrors.BadRequest("LOTTERY_IDEMPOTENCY_KEY_INVALID", "a valid idempotency key is required")
	}
	entry, replayed, err := s.repo.Participate(ctx, userID, campaignID, idempotencyKey, time.Now().UTC(), s.random)
	if err != nil {
		return nil, err
	}
	entry.Replayed = replayed
	if !replayed && entry.Status == LotteryEntryWon && entry.PrizeType == LotteryPrizeBalance {
		s.invalidateBalance(ctx, userID)
	}
	return entry, nil
}

func (s *LotteryService) ListUserEntries(ctx context.Context, userID int64, params LotteryListParams) ([]LotteryEntry, int64, error) {
	return s.repo.ListUserEntries(ctx, userID, params.Normalized())
}

func (s *LotteryService) ListAdminEntries(ctx context.Context, campaignID int64, params LotteryListParams) ([]LotteryEntry, int64, error) {
	if campaignID <= 0 {
		return nil, 0, ErrLotteryCampaignNotFound
	}
	return s.repo.ListAdminEntries(ctx, campaignID, params.Normalized())
}

func (s *LotteryService) DrawScheduled(ctx context.Context, campaignID int64, triggeredBy *int64) (*LotteryDrawResult, error) {
	result, err := s.repo.DrawScheduled(ctx, campaignID, triggeredBy, time.Now().UTC(), s.random)
	if err != nil {
		return nil, err
	}
	for _, userID := range result.BalanceRewardUserIDs {
		s.invalidateBalance(ctx, userID)
	}
	return result, nil
}

func (s *LotteryService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.repo == nil {
			close(s.doneCh)
			return
		}
		go s.runScheduler()
	})
}

func (s *LotteryService) Stop() {
	if s == nil {
		return
	}
	s.Start()
	s.stopOnce.Do(func() { close(s.stopCh) })
	<-s.doneCh
}

func (s *LotteryService) runScheduler() {
	defer close(s.doneCh)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	s.drawDueCampaigns()
	for {
		select {
		case <-ticker.C:
			s.drawDueCampaigns()
		case <-s.stopCh:
			return
		}
	}
}

func (s *LotteryService) drawDueCampaigns() {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	ids, err := s.repo.ListDueScheduledCampaignIDs(ctx, time.Now().UTC(), 20)
	if err != nil {
		return
	}
	for _, campaignID := range ids {
		_, _ = s.DrawScheduled(ctx, campaignID, nil)
	}
}

func (s *LotteryService) invalidateBalance(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCache != nil {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.billingCache.InvalidateUserBalance(cacheCtx, userID)
	}
}

func normalizeLotteryCampaignInput(input *LotteryCampaignInput) error {
	if input == nil {
		return ErrLotteryInvalid
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = LotteryCampaignDraft
	}
	if input.Name == "" || len(input.Name) > 160 || len(input.Description) > 8000 {
		return ErrLotteryInvalid
	}
	if input.Mode != LotteryModeInstant && input.Mode != LotteryModeScheduled {
		return ErrLotteryInvalid
	}
	if input.Status != LotteryCampaignDraft && input.Status != LotteryCampaignActive && input.Status != LotteryCampaignPaused && input.Status != LotteryCampaignCompleted {
		return ErrLotteryInvalid
	}
	if input.StartsAt.IsZero() || input.EndsAt.IsZero() || !input.EndsAt.After(input.StartsAt) {
		return ErrLotteryInvalid
	}
	input.StartsAt = input.StartsAt.UTC()
	input.EndsAt = input.EndsAt.UTC()
	if input.Mode == LotteryModeInstant {
		input.DrawAt = nil
	} else if input.DrawAt == nil || input.DrawAt.IsZero() || input.DrawAt.Before(input.EndsAt) {
		return ErrLotteryInvalid
	} else {
		drawAt := input.DrawAt.UTC()
		input.DrawAt = &drawAt
	}
	if input.PerUserLimit < 1 || input.PerUserLimit > 1000 || math.IsNaN(input.MinimumBalance) || math.IsInf(input.MinimumBalance, 0) || input.MinimumBalance < 0 {
		return ErrLotteryInvalid
	}
	groups, err := normalizePositiveIDs(input.RequiredSubscriptionGroupIDs, 100)
	if err != nil {
		return ErrLotteryInvalid
	}
	input.RequiredSubscriptionGroupIDs = groups
	if len(input.Prizes) == 0 || len(input.Prizes) > 100 {
		return ErrLotteryInvalid
	}
	totalProbability := 0
	for i := range input.Prizes {
		prize := &input.Prizes[i]
		prize.Name = strings.TrimSpace(prize.Name)
		prize.PrizeType = strings.ToLower(strings.TrimSpace(prize.PrizeType))
		if prize.Name == "" || len(prize.Name) > 160 || prize.ProbabilityBPS < 1 || prize.ProbabilityBPS > 10000 || prize.Inventory < 1 {
			return ErrLotteryInvalid
		}
		if prize.IsEnabled {
			totalProbability += prize.ProbabilityBPS
		}
		switch prize.PrizeType {
		case LotteryPrizeBalance:
			if prize.SubscriptionGroupID != nil || prize.SubscriptionValidityDays != 0 || math.IsNaN(prize.BalanceAmount) || math.IsInf(prize.BalanceAmount, 0) || prize.BalanceAmount <= 0 {
				return ErrLotteryInvalid
			}
		case LotteryPrizeSubscriptionCode:
			if prize.BalanceAmount != 0 || prize.SubscriptionGroupID == nil || *prize.SubscriptionGroupID <= 0 || prize.SubscriptionValidityDays < 1 || prize.SubscriptionValidityDays > 3650 {
				return ErrLotteryInvalid
			}
		default:
			return ErrLotteryInvalid
		}
	}
	if totalProbability > 10000 {
		return infraerrors.BadRequest("LOTTERY_PROBABILITY_EXCEEDED", "enabled prize probabilities must not exceed 10000 basis points")
	}
	sort.SliceStable(input.Prizes, func(i, j int) bool {
		if input.Prizes[i].SortOrder == input.Prizes[j].SortOrder {
			return i < j
		}
		return input.Prizes[i].SortOrder < input.Prizes[j].SortOrder
	})
	return nil
}

func normalizePositiveIDs(ids []int64, max int) ([]int64, error) {
	if len(ids) > max {
		return nil, fmt.Errorf("too many IDs")
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("IDs must be positive")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func SelectLotteryPrize(prizes []LotteryPrize, roll int) *LotteryPrize {
	if roll < 0 || roll >= 10000 {
		return nil
	}
	cursor := 0
	for i := range prizes {
		prize := &prizes[i]
		if !prize.IsEnabled {
			continue
		}
		cursor += prize.ProbabilityBPS
		if roll < cursor {
			if prize.AwardedCount >= prize.Inventory {
				return nil
			}
			return prize
		}
	}
	return nil
}
