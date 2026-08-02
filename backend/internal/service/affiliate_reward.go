package service

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AffiliateRewardTypeRegistrationInviteeTrial  = "invite_register_invitee_pro_trial_card"
	AffiliateRewardTypeRegistrationInviteeBonus  = "invite_register_invitee_bonus"
	AffiliateRewardTypeRegistrationInviterBonus  = "invite_register_inviter_bonus"
	AffiliateRewardTypeFirstRechargeInviteeBonus = "first_recharge_invitee_bonus"
	AffiliateRewardTypeFirstRechargeInviterBonus = "first_recharge_inviter_bonus"
	AffiliateRewardTypeLimitedRechargeBonus      = "limited_recharge_bonus"

	AffiliateRewardStatusPending  = "pending"
	AffiliateRewardStatusApproved = "approved"
	AffiliateRewardStatusRejected = "rejected"
	AffiliateRewardStatusPaid     = "paid"

	AffiliateRewardActionApprove = "approve"
	AffiliateRewardActionReject  = "reject"

	AffiliateRewardProgramVersion = 1
)

var (
	ErrAffiliateRewardProgramInvalid = infraerrors.BadRequest("AFFILIATE_REWARD_PROGRAM_INVALID", "invalid affiliate reward program")
	ErrAffiliateRewardNotFound       = infraerrors.NotFound("AFFILIATE_REWARD_NOT_FOUND", "affiliate reward review not found")
	ErrAffiliateRewardFinal          = infraerrors.Conflict("AFFILIATE_REWARD_ALREADY_FINAL", "affiliate reward review is already final")
	ErrAffiliateRewardUnsupported    = infraerrors.BadRequest("AFFILIATE_REWARD_UNSUPPORTED", "unsupported affiliate reward type")
	ErrAffiliateRewardGroupInvalid   = infraerrors.BadRequest("AFFILIATE_REWARD_GROUP_INVALID", "affiliate trial group is invalid")
	ErrAffiliateRewardOutOfScope     = infraerrors.Conflict("AFFILIATE_REWARD_OUT_OF_SCOPE", "historical affiliate reward review cannot be approved")
)

type AffiliateRegistrationRewardConfig struct {
	Enabled               bool    `json:"enabled"`
	DefaultInviterEnabled bool    `json:"default_inviter_enabled"`
	DefaultInviterUserID  int64   `json:"default_inviter_user_id"`
	InviterBonus          float64 `json:"inviter_bonus"`
	InviteeTrialAmount    float64 `json:"invitee_trial_amount"`
	InviteeTrialGroupID   int64   `json:"invitee_trial_group_id"`
	InviteeTrialDays      int     `json:"invitee_trial_days"`
}

type AffiliateFirstRechargeRewardConfig struct {
	Enabled             bool    `json:"enabled"`
	InviterBonus        float64 `json:"inviter_bonus"`
	InviteeBonusPercent float64 `json:"invitee_bonus_percent"`
}

type AffiliateRewardProgramConfig struct {
	Version              int                                `json:"version"`
	Enabled              bool                               `json:"enabled"`
	LegacyApprovalCutoff *time.Time                         `json:"legacy_approval_cutoff,omitempty"`
	Registration         AffiliateRegistrationRewardConfig  `json:"registration"`
	FirstRecharge        AffiliateFirstRechargeRewardConfig `json:"first_recharge"`
}

func DefaultAffiliateRewardProgramConfig() AffiliateRewardProgramConfig {
	return AffiliateRewardProgramConfig{
		Version: AffiliateRewardProgramVersion,
		Enabled: false,
		Registration: AffiliateRegistrationRewardConfig{
			Enabled:               true,
			DefaultInviterEnabled: false,
			DefaultInviterUserID:  0,
			InviterBonus:          1,
			InviteeTrialAmount:    3,
			InviteeTrialGroupID:   50,
			InviteeTrialDays:      3,
		},
		FirstRecharge: AffiliateFirstRechargeRewardConfig{
			Enabled:             true,
			InviterBonus:        2,
			InviteeBonusPercent: 10,
		},
	}
}

func NormalizeAffiliateRewardProgramConfig(config AffiliateRewardProgramConfig) (AffiliateRewardProgramConfig, error) {
	config.Version = AffiliateRewardProgramVersion
	if config.LegacyApprovalCutoff != nil {
		if config.LegacyApprovalCutoff.IsZero() {
			return AffiliateRewardProgramConfig{}, ErrAffiliateRewardProgramInvalid
		}
		cutoff := config.LegacyApprovalCutoff.UTC()
		config.LegacyApprovalCutoff = &cutoff
	}
	if !validAffiliateRewardAmount(config.Registration.InviterBonus) ||
		!validAffiliateRewardAmount(config.Registration.InviteeTrialAmount) ||
		!validAffiliateRewardAmount(config.FirstRecharge.InviterBonus) ||
		math.IsNaN(config.FirstRecharge.InviteeBonusPercent) ||
		math.IsInf(config.FirstRecharge.InviteeBonusPercent, 0) ||
		config.FirstRecharge.InviteeBonusPercent < 0 || config.FirstRecharge.InviteeBonusPercent > 100 {
		return AffiliateRewardProgramConfig{}, ErrAffiliateRewardProgramInvalid
	}
	if config.Registration.DefaultInviterUserID < 0 ||
		(config.Registration.DefaultInviterEnabled && config.Registration.DefaultInviterUserID <= 0) {
		return AffiliateRewardProgramConfig{}, ErrAffiliateRewardProgramInvalid
	}
	if config.Registration.Enabled {
		if config.Registration.InviteeTrialAmount > 0 &&
			(config.Registration.InviteeTrialGroupID <= 0 || config.Registration.InviteeTrialDays <= 0 || config.Registration.InviteeTrialDays > 3650) {
			return AffiliateRewardProgramConfig{}, ErrAffiliateRewardProgramInvalid
		}
		if config.Registration.InviterBonus <= 0 && config.Registration.InviteeTrialAmount <= 0 {
			return AffiliateRewardProgramConfig{}, ErrAffiliateRewardProgramInvalid
		}
	}
	if config.FirstRecharge.Enabled && config.FirstRecharge.InviterBonus <= 0 && config.FirstRecharge.InviteeBonusPercent <= 0 {
		return AffiliateRewardProgramConfig{}, ErrAffiliateRewardProgramInvalid
	}
	return config, nil
}

func validAffiliateRewardAmount(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1_000_000
}

type AffiliateRegistrationMeta struct {
	ClientIP  string
	UserAgent string
	Source    string
}

type affiliateRegistrationMetaContextKey struct{}

func WithAffiliateRegistrationMeta(ctx context.Context, meta AffiliateRegistrationMeta) context.Context {
	return context.WithValue(ctx, affiliateRegistrationMetaContextKey{}, normalizeAffiliateRegistrationMeta(meta))
}

func normalizeAffiliateRegistrationMeta(meta AffiliateRegistrationMeta) AffiliateRegistrationMeta {
	meta.ClientIP = normalizeAffiliateClientIP(meta.ClientIP)
	meta.UserAgent = strings.TrimSpace(strings.ToValidUTF8(meta.UserAgent, ""))
	meta.UserAgent = truncateUTF8(meta.UserAgent, 1000)
	meta.Source = truncateUTF8(strings.TrimSpace(strings.ToValidUTF8(meta.Source, "")), 32)
	if meta.Source == "" {
		meta.Source = "auth_request"
	}
	return meta
}

func AffiliateRegistrationMetaFromContext(ctx context.Context) AffiliateRegistrationMeta {
	if ctx == nil {
		return AffiliateRegistrationMeta{}
	}
	meta, ok := ctx.Value(affiliateRegistrationMetaContextKey{}).(AffiliateRegistrationMeta)
	if !ok {
		if binding := SessionBindingFromContext(ctx); binding != nil {
			meta = normalizeAffiliateRegistrationMeta(AffiliateRegistrationMeta{
				ClientIP:  binding.IP,
				UserAgent: binding.UserAgent,
				Source:    "auth_request",
			})
		}
	}
	return meta
}

func normalizeAffiliateClientIP(value string) string {
	parsed := net.ParseIP(strings.TrimSpace(value))
	if parsed == nil {
		return ""
	}
	return parsed.String()
}

type AffiliateRewardRiskInput struct {
	AdminInviter                bool
	RegistrationIP              string
	RegistrationIPFirstSeenAt   *time.Time
	RegistrationIPUsers24H      int
	RegistrationIPUsersTotal    int
	InviterInvites24H           int
	InviterTotalInvites         int
	InviterPaidInvitees         int
	InviterRejectedRewards      int
	MinutesToFirstRecharge      *float64
	PayAmount                   *float64
	CreditedQuota               *float64
	DuplicatePaymentTradeNumber bool
	Source                      string
}

type AffiliateRewardRiskFlags struct {
	Source                      string     `json:"source"`
	RiskLevel                   string     `json:"risk_level"`
	RiskScore                   int        `json:"risk_score"`
	Reasons                     []string   `json:"reasons"`
	AdminInviter                bool       `json:"admin_inviter"`
	RegistrationIPCaptured      bool       `json:"registration_ip_captured,omitempty"`
	RegistrationIP              string     `json:"registration_ip,omitempty"`
	RegistrationIPFirstSeenAt   *time.Time `json:"registration_ip_first_seen_at,omitempty"`
	RegistrationIPUsers24H      int        `json:"registration_ip_24h_users,omitempty"`
	RegistrationIPUsersTotal    int        `json:"registration_ip_total_users,omitempty"`
	InviterInvites24H           int        `json:"inviter_invites_24h,omitempty"`
	InviterTotalInvites         int        `json:"inviter_total_invites,omitempty"`
	InviterPaidInvitees         int        `json:"inviter_paid_invitees,omitempty"`
	InviterRejectedRewards      int        `json:"inviter_rejected_rewards,omitempty"`
	MinutesToFirstRecharge      *float64   `json:"minutes_to_first_recharge,omitempty"`
	PayAmount                   *float64   `json:"pay_amount,omitempty"`
	CreditedQuota               *float64   `json:"credited_quota,omitempty"`
	DuplicatePaymentTradeNumber bool       `json:"duplicate_payment_trade_no,omitempty"`
	OrderClientIPUsedForRisk    bool       `json:"order_client_ip_used_for_risk"`
}

func EvaluateAffiliateRewardRisk(input AffiliateRewardRiskInput) AffiliateRewardRiskFlags {
	flags := AffiliateRewardRiskFlags{
		Source:                   strings.TrimSpace(input.Source),
		RiskLevel:                "low",
		Reasons:                  make([]string, 0),
		AdminInviter:             input.AdminInviter,
		OrderClientIPUsedForRisk: false,
	}
	if flags.Source == "" {
		flags.Source = "modelport"
	}

	if input.AdminInviter {
		flags.RegistrationIP = normalizeAffiliateClientIP(input.RegistrationIP)
		flags.RegistrationIPCaptured = flags.RegistrationIP != ""
		flags.RegistrationIPFirstSeenAt = input.RegistrationIPFirstSeenAt
		flags.RegistrationIPUsers24H = input.RegistrationIPUsers24H
		flags.RegistrationIPUsersTotal = input.RegistrationIPUsersTotal
		if !flags.RegistrationIPCaptured {
			flags.RiskLevel = "unknown"
			flags.Reasons = append(flags.Reasons, "registration_ip_not_captured")
			return flags
		}
		if input.RegistrationIPUsers24H >= 5 {
			flags.RiskScore += 4
			flags.Reasons = append(flags.Reasons, "registration_ip_24h_users_ge_5")
		} else if input.RegistrationIPUsers24H >= 3 {
			flags.RiskScore += 2
			flags.Reasons = append(flags.Reasons, "registration_ip_24h_users_ge_3")
		} else if input.RegistrationIPUsers24H >= 2 {
			flags.RiskScore++
			flags.Reasons = append(flags.Reasons, "registration_ip_24h_users_ge_2")
		}
		if input.RegistrationIPUsersTotal >= 10 {
			flags.RiskScore += 2
			flags.Reasons = append(flags.Reasons, "registration_ip_total_users_ge_10")
		}
		flags.RiskLevel = affiliateRiskLevel(flags.RiskScore)
		return flags
	}

	flags.InviterInvites24H = input.InviterInvites24H
	flags.InviterTotalInvites = input.InviterTotalInvites
	flags.InviterPaidInvitees = input.InviterPaidInvitees
	flags.InviterRejectedRewards = input.InviterRejectedRewards
	flags.MinutesToFirstRecharge = input.MinutesToFirstRecharge
	flags.PayAmount = input.PayAmount
	flags.CreditedQuota = input.CreditedQuota
	flags.DuplicatePaymentTradeNumber = input.DuplicatePaymentTradeNumber

	if input.InviterInvites24H >= 20 {
		flags.RiskScore += 2
		flags.Reasons = append(flags.Reasons, "inviter_invites_24h_ge_20")
	} else if input.InviterInvites24H >= 5 {
		flags.RiskScore++
		flags.Reasons = append(flags.Reasons, "inviter_invites_24h_ge_5")
	}
	if input.InviterTotalInvites >= 50 {
		flags.RiskScore++
		flags.Reasons = append(flags.Reasons, "inviter_total_invites_ge_50")
	}
	if input.InviterRejectedRewards >= 3 {
		flags.RiskScore += 2
		flags.Reasons = append(flags.Reasons, "inviter_rejected_rewards_ge_3")
	}
	if input.MinutesToFirstRecharge != nil {
		if *input.MinutesToFirstRecharge < 1 {
			flags.RiskScore += 2
			flags.Reasons = append(flags.Reasons, "first_recharge_under_1_minute")
		} else if *input.MinutesToFirstRecharge < 5 {
			flags.RiskScore++
			flags.Reasons = append(flags.Reasons, "first_recharge_under_5_minutes")
		}
	}
	if input.PayAmount != nil {
		if *input.PayAmount < 5 {
			flags.RiskScore += 2
			flags.Reasons = append(flags.Reasons, "low_first_recharge_under_5_cny")
		} else if *input.PayAmount == 5 || *input.PayAmount == 10 {
			flags.RiskScore++
			flags.Reasons = append(flags.Reasons, "round_or_minimum_like_recharge_amount")
		}
	}
	if input.DuplicatePaymentTradeNumber {
		flags.RiskScore += 3
		flags.Reasons = append(flags.Reasons, "duplicate_payment_trade_no")
	}
	flags.RiskLevel = affiliateRiskLevel(flags.RiskScore)
	return flags
}

func affiliateRiskLevel(score int) string {
	if score >= 4 {
		return "high"
	}
	if score >= 2 {
		return "medium"
	}
	return "low"
}

func MergeAffiliateRewardFlags(risk AffiliateRewardRiskFlags, benefit map[string]any) ([]byte, error) {
	flags := make(map[string]any, len(benefit)+8)
	riskJSON, err := json.Marshal(risk)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(riskJSON, &flags); err != nil {
		return nil, err
	}
	for key, value := range benefit {
		flags[key] = value
	}
	return json.Marshal(flags)
}

type AffiliateRewardRepository interface {
	BindInviterWithRewardProgram(ctx context.Context, userID, inviterID int64, config AffiliateRewardProgramConfig, meta AffiliateRegistrationMeta) (bool, error)
	EnsureRegistrationRewardReviews(ctx context.Context, userID, inviterID int64, config AffiliateRewardProgramConfig, meta AffiliateRegistrationMeta) error
	CreateFirstRechargeRewardReviews(ctx context.Context, orderID int64, config AffiliateRewardProgramConfig) (bool, error)
	ValidateAffiliateRewardProgram(ctx context.Context, config AffiliateRewardProgramConfig) error
	GetAffiliateRewardDashboard(ctx context.Context, inviterID int64) (*AffiliateRewardDashboard, error)
	ListAffiliateRewardReviews(ctx context.Context, filter AffiliateRewardReviewFilter) ([]AffiliateRewardReview, int64, error)
	GetAffiliateRewardStats(ctx context.Context) (*AffiliateRewardReviewStats, error)
	ReviewAffiliateReward(ctx context.Context, reviewID, adminID int64, action, note string, legacyApprovalCutoff *time.Time) (*AffiliateRewardReviewResult, error)
}

type AffiliateRewardDashboard struct {
	Program            AffiliateRewardProgramConfig `json:"program"`
	PaidAmount         float64                      `json:"paid_amount"`
	PendingAmount      float64                      `json:"pending_amount"`
	RejectedAmount     float64                      `json:"rejected_amount"`
	InvitedUsers       int                          `json:"invited_users"`
	FirstRechargeUsers int                          `json:"first_recharge_users"`
	Invitees           []AffiliateRewardInvitee     `json:"invitees"`
}

type AffiliateRewardInvitee struct {
	UserID                    int64      `json:"user_id"`
	EmailMasked               string     `json:"email_masked"`
	RegisteredAt              time.Time  `json:"registered_at"`
	RegistrationStatus        string     `json:"registration_status"`
	RegistrationRewardStatus  string     `json:"registration_reward_status"`
	RegistrationRewardAmount  float64    `json:"registration_reward_amount"`
	FirstRechargeStatus       string     `json:"first_recharge_status"`
	FirstRechargeRewardStatus string     `json:"first_recharge_reward_status"`
	FirstRechargeRewardAmount float64    `json:"first_recharge_reward_amount"`
	UpdatedAt                 *time.Time `json:"updated_at,omitempty"`
}

type AffiliateRewardReviewFilter struct {
	Search     string
	Status     string
	RewardType string
	Risk       string
	Page       int
	PageSize   int
}

type AffiliateRewardReview struct {
	ID                        int64           `json:"id"`
	InviterUserID             int64           `json:"inviter_user_id"`
	InviteeUserID             int64           `json:"invitee_user_id"`
	RewardUserID              int64           `json:"reward_user_id"`
	RewardType                string          `json:"reward_type"`
	RewardAmount              float64         `json:"reward_amount"`
	PaymentOrderID            *int64          `json:"payment_order_id,omitempty"`
	Status                    string          `json:"status"`
	RiskFlags                 json.RawMessage `json:"risk_flags"`
	RiskLevel                 string          `json:"risk_level"`
	RiskScore                 int             `json:"risk_score"`
	ReviewedBy                *int64          `json:"reviewed_by,omitempty"`
	ReviewedAt                *time.Time      `json:"reviewed_at,omitempty"`
	ReviewNote                string          `json:"review_note"`
	PaidAt                    *time.Time      `json:"paid_at,omitempty"`
	CreatedAt                 time.Time       `json:"created_at"`
	UpdatedAt                 time.Time       `json:"updated_at"`
	InviterEmail              string          `json:"inviter_email"`
	InviteeEmail              string          `json:"invitee_email"`
	RewardUserEmail           string          `json:"reward_user_email"`
	ReviewedByEmail           string          `json:"reviewed_by_email"`
	OrderAmount               *float64        `json:"order_amount,omitempty"`
	OrderPayAmount            *float64        `json:"order_pay_amount,omitempty"`
	OrderStatus               string          `json:"order_status"`
	RegistrationIP            string          `json:"registration_ip"`
	RegistrationIPFirstSeenAt *time.Time      `json:"registration_ip_first_seen_at,omitempty"`
	ApprovalBlockedReason     string          `json:"approval_blocked_reason,omitempty"`
}

type AffiliateRewardReviewStats struct {
	PendingCount         int64          `json:"pending_count"`
	PendingAmount        float64        `json:"pending_amount"`
	PaidCount            int64          `json:"paid_count"`
	PaidAmount           float64        `json:"paid_amount"`
	RejectedCount        int64          `json:"rejected_count"`
	HighRiskPendingCount int64          `json:"high_risk_pending_count"`
	TodayPaidCount       int64          `json:"today_paid_count"`
	ByType               map[string]any `json:"by_type"`
}

type AffiliateRewardGrantEffect struct {
	UserID  int64  `json:"user_id"`
	Kind    string `json:"kind"`
	GroupID int64  `json:"group_id,omitempty"`
}

type AffiliateRewardReviewResult struct {
	ReviewIDs []int64                      `json:"review_ids"`
	Status    string                       `json:"status"`
	Effects   []AffiliateRewardGrantEffect `json:"effects"`
}
