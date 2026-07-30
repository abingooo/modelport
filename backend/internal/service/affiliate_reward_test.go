//go:build unit

package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAffiliateRewardProgramConfig(t *testing.T) {
	t.Parallel()

	valid := DefaultAffiliateRewardProgramConfig()
	valid.Enabled = true
	legacyCutoff := time.Date(2026, time.July, 6, 6, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	valid.LegacyApprovalCutoff = &legacyCutoff
	normalized, err := NormalizeAffiliateRewardProgramConfig(valid)
	require.NoError(t, err)
	require.Equal(t, AffiliateRewardProgramVersion, normalized.Version)
	require.True(t, normalized.Enabled)
	require.Equal(t, time.Date(2026, time.July, 5, 22, 0, 0, 0, time.UTC), *normalized.LegacyApprovalCutoff)

	cases := []struct {
		name   string
		mutate func(*AffiliateRewardProgramConfig)
	}{
		{"negative registration bonus", func(config *AffiliateRewardProgramConfig) { config.Registration.InviterBonus = -1 }},
		{"non-finite trial amount", func(config *AffiliateRewardProgramConfig) { config.Registration.InviteeTrialAmount = math.Inf(1) }},
		{"missing trial group", func(config *AffiliateRewardProgramConfig) { config.Registration.InviteeTrialGroupID = 0 }},
		{"invalid trial days", func(config *AffiliateRewardProgramConfig) { config.Registration.InviteeTrialDays = 3651 }},
		{"invalid recharge percent", func(config *AffiliateRewardProgramConfig) { config.FirstRecharge.InviteeBonusPercent = 100.01 }},
		{"zero legacy cutoff", func(config *AffiliateRewardProgramConfig) {
			zero := time.Time{}
			config.LegacyApprovalCutoff = &zero
		}},
		{"empty registration rewards", func(config *AffiliateRewardProgramConfig) {
			config.Registration.InviterBonus = 0
			config.Registration.InviteeTrialAmount = 0
		}},
		{"empty recharge rewards", func(config *AffiliateRewardProgramConfig) {
			config.FirstRecharge.InviterBonus = 0
			config.FirstRecharge.InviteeBonusPercent = 0
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			config := valid
			testCase.mutate(&config)
			_, err := NormalizeAffiliateRewardProgramConfig(config)
			require.ErrorIs(t, err, ErrAffiliateRewardProgramInvalid)
		})
	}
}

func TestAffiliateRewardProgramDefaultsDisabled(t *testing.T) {
	t.Parallel()

	config := DefaultAffiliateRewardProgramConfig()
	require.False(t, config.Enabled)
	require.True(t, config.Registration.Enabled)
	require.True(t, config.FirstRecharge.Enabled)
	require.Nil(t, config.LegacyApprovalCutoff)
	require.InDelta(t, 1, config.Registration.InviterBonus, 1e-9)
	require.InDelta(t, 3, config.Registration.InviteeTrialAmount, 1e-9)
	require.Equal(t, int64(50), config.Registration.InviteeTrialGroupID)
	require.Equal(t, 3, config.Registration.InviteeTrialDays)
	require.InDelta(t, 2, config.FirstRecharge.InviterBonus, 1e-9)
	require.InDelta(t, 10, config.FirstRecharge.InviteeBonusPercent, 1e-9)
}

func TestAffiliateRegistrationMetaNormalization(t *testing.T) {
	t.Parallel()

	longUserAgent := strings.Repeat("模", 400)
	ctx := WithSessionBinding(context.Background(), &SessionBinding{
		IP:        " 203.0.113.8 ",
		UserAgent: longUserAgent,
	})
	meta := AffiliateRegistrationMetaFromContext(ctx)
	require.Equal(t, "203.0.113.8", meta.ClientIP)
	require.Equal(t, "auth_request", meta.Source)
	require.LessOrEqual(t, len(meta.UserAgent), 1000)
	require.True(t, utf8.ValidString(meta.UserAgent))

	explicit := AffiliateRegistrationMetaFromContext(WithAffiliateRegistrationMeta(ctx, AffiliateRegistrationMeta{
		ClientIP:  "not-an-ip",
		UserAgent: longUserAgent,
		Source:    strings.Repeat("s", 80),
	}))
	require.Empty(t, explicit.ClientIP)
	require.Len(t, explicit.Source, 32)
	require.LessOrEqual(t, len(explicit.UserAgent), 1000)
	require.True(t, utf8.ValidString(explicit.UserAgent))
}

func TestEvaluateAffiliateRewardRisk(t *testing.T) {
	t.Parallel()

	minutes := 0.5
	payAmount := 3.0
	flags := EvaluateAffiliateRewardRisk(AffiliateRewardRiskInput{
		InviterInvites24H:           20,
		InviterTotalInvites:         50,
		InviterRejectedRewards:      3,
		MinutesToFirstRecharge:      &minutes,
		PayAmount:                   &payAmount,
		DuplicatePaymentTradeNumber: true,
		Source:                      "first_recharge",
	})
	require.Equal(t, "high", flags.RiskLevel)
	require.Equal(t, 12, flags.RiskScore)
	require.Contains(t, flags.Reasons, "duplicate_payment_trade_no")
	require.False(t, flags.OrderClientIPUsedForRisk)

	adminFlags := EvaluateAffiliateRewardRisk(AffiliateRewardRiskInput{
		AdminInviter:             true,
		RegistrationIP:           "198.51.100.9",
		RegistrationIPUsers24H:   2,
		RegistrationIPUsersTotal: 10,
		Source:                   "registration_ip_capture",
	})
	require.Equal(t, "medium", adminFlags.RiskLevel)
	require.Equal(t, 3, adminFlags.RiskScore)
	require.True(t, adminFlags.RegistrationIPCaptured)

	unknownFlags := EvaluateAffiliateRewardRisk(AffiliateRewardRiskInput{AdminInviter: true})
	require.Equal(t, "unknown", unknownFlags.RiskLevel)
	require.Equal(t, []string{"registration_ip_not_captured"}, unknownFlags.Reasons)
}

func TestMergeAffiliateRewardFlags(t *testing.T) {
	t.Parallel()

	risk := AffiliateRewardRiskFlags{Source: "registration", RiskLevel: "low", Reasons: []string{"new_user"}}
	encoded, err := MergeAffiliateRewardFlags(risk, map[string]any{"group_id": int64(50), "validity_days": 3})
	require.NoError(t, err)

	var flags map[string]any
	require.NoError(t, json.Unmarshal(encoded, &flags))
	require.Equal(t, "registration", flags["source"])
	require.Equal(t, "low", flags["risk_level"])
	require.Equal(t, float64(50), flags["group_id"])
	require.Equal(t, float64(3), flags["validity_days"])
}

type affiliateRewardCapabilityStub struct {
	AffiliateRepository
	orderID int64
	config  AffiliateRewardProgramConfig
}

func (stub *affiliateRewardCapabilityStub) BindInviterWithRewardProgram(context.Context, int64, int64, AffiliateRewardProgramConfig, AffiliateRegistrationMeta) (bool, error) {
	return false, nil
}

func (stub *affiliateRewardCapabilityStub) EnsureRegistrationRewardReviews(context.Context, int64, int64, AffiliateRewardProgramConfig, AffiliateRegistrationMeta) error {
	return nil
}

func (stub *affiliateRewardCapabilityStub) CreateFirstRechargeRewardReviews(_ context.Context, orderID int64, config AffiliateRewardProgramConfig) (bool, error) {
	stub.orderID = orderID
	stub.config = config
	return true, nil
}

func (stub *affiliateRewardCapabilityStub) ValidateAffiliateRewardProgram(context.Context, AffiliateRewardProgramConfig) error {
	return nil
}

func (stub *affiliateRewardCapabilityStub) GetAffiliateRewardDashboard(context.Context, int64) (*AffiliateRewardDashboard, error) {
	return &AffiliateRewardDashboard{}, nil
}

func (stub *affiliateRewardCapabilityStub) ListAffiliateRewardReviews(context.Context, AffiliateRewardReviewFilter) ([]AffiliateRewardReview, int64, error) {
	return nil, 0, nil
}

func (stub *affiliateRewardCapabilityStub) GetAffiliateRewardStats(context.Context) (*AffiliateRewardReviewStats, error) {
	return &AffiliateRewardReviewStats{}, nil
}

func (stub *affiliateRewardCapabilityStub) ReviewAffiliateReward(context.Context, int64, int64, string, string, *time.Time) (*AffiliateRewardReviewResult, error) {
	return &AffiliateRewardReviewResult{}, nil
}

func TestAffiliateServiceDetectsRewardRepositoryCapability(t *testing.T) {
	t.Parallel()

	config := DefaultAffiliateRewardProgramConfig()
	config.Enabled = true
	encoded, err := json.Marshal(config)
	require.NoError(t, err)

	repo := &affiliateRewardCapabilityStub{}
	settingService := NewSettingService(&paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateRewardProgramConfig: string(encoded),
	}}, nil)
	service := NewAffiliateService(repo, settingService, nil, nil)

	created, err := service.CreateFirstRechargeRewardReviews(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(42), repo.orderID)
	require.True(t, repo.config.Enabled)
	require.Equal(t, AffiliateRewardProgramVersion, repo.config.Version)

	baseOnlyService := NewAffiliateService(&paymentFulfillmentAffiliateRepoStub{}, nil, nil, nil)
	require.Nil(t, baseOnlyService.rewardRepository())
}

func TestAffiliateServicePreservesLegacyApprovalCutoffOnAdminUpdate(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, time.July, 5, 22, 0, 0, 0, time.UTC)
	stored := DefaultAffiliateRewardProgramConfig()
	stored.Enabled = true
	stored.LegacyApprovalCutoff = &cutoff
	encoded, err := json.Marshal(stored)
	require.NoError(t, err)

	settingRepo := &paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateRewardProgramConfig: string(encoded),
	}}
	affiliateService := NewAffiliateService(&affiliateRewardCapabilityStub{}, NewSettingService(settingRepo, nil), nil, nil)
	update := DefaultAffiliateRewardProgramConfig()
	update.Enabled = true
	update.Registration.InviterBonus = 1.5

	updated, err := affiliateService.AdminSetAffiliateRewardProgram(context.Background(), update)
	require.NoError(t, err)
	require.NotNil(t, updated.LegacyApprovalCutoff)
	require.Equal(t, cutoff, *updated.LegacyApprovalCutoff)
	require.InDelta(t, 1.5, updated.Registration.InviterBonus, 1e-9)

	var persisted AffiliateRewardProgramConfig
	require.NoError(t, json.Unmarshal([]byte(settingRepo.values[SettingKeyAffiliateRewardProgramConfig]), &persisted))
	require.NotNil(t, persisted.LegacyApprovalCutoff)
	require.Equal(t, cutoff, *persisted.LegacyApprovalCutoff)
}

func TestPaymentServiceEnsuresFirstRechargeReviewsAfterCompletion(t *testing.T) {
	t.Parallel()

	config := DefaultAffiliateRewardProgramConfig()
	config.Enabled = true
	encoded, err := json.Marshal(config)
	require.NoError(t, err)

	repo := &affiliateRewardCapabilityStub{}
	affiliateService := NewAffiliateService(repo, NewSettingService(&paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateRewardProgramConfig: string(encoded),
	}}, nil), nil, nil)
	paymentService := &PaymentService{affiliateService: affiliateService}

	require.NoError(t, paymentService.ensureFirstRechargeRewardReviews(context.Background(), &dbent.PaymentOrder{
		ID:        87,
		OrderType: payment.OrderTypeBalance,
	}))
	require.Equal(t, int64(87), repo.orderID)

	repo.orderID = 0
	require.NoError(t, paymentService.ensureFirstRechargeRewardReviews(context.Background(), &dbent.PaymentOrder{
		ID:        88,
		OrderType: payment.OrderTypeSubscription,
	}))
	require.Zero(t, repo.orderID)
}

func TestAffiliateRewardRiskPreservesRegistrationTimestamp(t *testing.T) {
	t.Parallel()

	capturedAt := time.Now().UTC().Truncate(time.Second)
	flags := EvaluateAffiliateRewardRisk(AffiliateRewardRiskInput{
		AdminInviter:              true,
		RegistrationIP:            "192.0.2.20",
		RegistrationIPFirstSeenAt: &capturedAt,
	})
	require.NotNil(t, flags.RegistrationIPFirstSeenAt)
	require.True(t, capturedAt.Equal(*flags.RegistrationIPFirstSeenAt))
}
