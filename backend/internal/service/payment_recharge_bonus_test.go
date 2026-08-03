package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestRechargeBonusTierBoundaries(t *testing.T) {
	cfg := &PaymentConfig{
		RechargeBonusEnabled: true,
		RechargeBonusTiers:   cloneRechargeBonusTiers(defaultRechargeBonusTiers),
	}
	tests := []struct {
		principal float64
		base      float64
		percent   float64
		bonus     float64
	}{
		{principal: 99.99, base: 99.99, percent: 3, bonus: 3},
		{principal: 100, base: 100, percent: 5, bonus: 5},
		{principal: 499.99, base: 249.995, percent: 5, bonus: 12.5},
		{principal: 500, base: 500, percent: 8, bonus: 40},
		{principal: 999.99, base: 999.99, percent: 8, bonus: 80},
		{principal: 1000, base: 1000, percent: 10, bonus: 100},
	}

	for _, test := range tests {
		got := calculateRechargeBonus(test.principal, test.base, cfg)
		require.Equal(t, test.percent, got.BonusPercent)
		require.Equal(t, test.bonus, got.BonusAmount)
	}
}

func TestRechargeBonusDisabledKeepsBaseAmount(t *testing.T) {
	got := calculateRechargeBonus(1000, 500, &PaymentConfig{
		RechargeBonusEnabled: false,
		RechargeBonusTiers:   cloneRechargeBonusTiers(defaultRechargeBonusTiers),
	})
	require.Zero(t, got.BonusPercent)
	require.Zero(t, got.BonusAmount)
	require.Equal(t, float64(500), got.BaseAmount)
}

func TestNormalizeRechargeBonusTiersSortsAndRejectsDuplicates(t *testing.T) {
	got, err := normalizeRechargeBonusTiers([]RechargeBonusTier{
		{MinAmount: 500, BonusPercent: 8},
		{MinAmount: 0, BonusPercent: 3},
		{MinAmount: 100, BonusPercent: 5},
	})
	require.NoError(t, err)
	require.Equal(t, []float64{0, 100, 500}, []float64{got[0].MinAmount, got[1].MinAmount, got[2].MinAmount})

	_, err = normalizeRechargeBonusTiers([]RechargeBonusTier{
		{MinAmount: 100, BonusPercent: 3},
		{MinAmount: 100, BonusPercent: 5},
	})
	require.Error(t, err)
}

func TestPaymentConfigRechargeBonusDefaultsAndPersistence(t *testing.T) {
	svc := &PaymentConfigService{}
	cfg := svc.parsePaymentConfig(map[string]string{})
	require.False(t, cfg.RechargeBonusEnabled)
	require.Equal(t, defaultRechargeBonusTiers, cfg.RechargeBonusTiers)

	repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
	svc.settingRepo = repo
	enabled := true
	tiers := []RechargeBonusTier{
		{MinAmount: 100, BonusPercent: 5},
		{MinAmount: 0, BonusPercent: 3},
	}
	require.NoError(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{
		RechargeBonusEnabled: &enabled,
		RechargeBonusTiers:   &tiers,
	}))
	require.Equal(t, "true", repo.values[SettingRechargeBonusEnabled])
	require.JSONEq(t, `[{"min_amount":0,"bonus_percent":3},{"min_amount":100,"bonus_percent":5}]`, repo.values[SettingRechargeBonusTiers])
}

func TestPaymentOrderRechargeBonusReadsDurableSnapshot(t *testing.T) {
	order := &dbent.PaymentOrder{ProviderSnapshot: map[string]any{
		"recharge_principal_amount": 500.0,
		"recharge_base_amount":      500.0,
		"recharge_bonus_percent":    8.0,
		"recharge_bonus_amount":     40.0,
	}}
	require.Equal(t, RechargeBonusSnapshot{
		PrincipalAmount: 500,
		BaseAmount:      500,
		BonusPercent:    8,
		BonusAmount:     40,
	}, PaymentOrderRechargeBonus(order))
}
