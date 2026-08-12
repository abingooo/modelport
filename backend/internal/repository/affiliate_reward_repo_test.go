package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateFirstRechargeInviteeRewardOffsetsTierBonus(t *testing.T) {
	tests := []struct {
		name         string
		payAmount    float64
		bonusPercent float64
		tierBonus    float64
		wantNominal  float64
		wantNet      float64
	}{
		{name: "no tier bonus", payAmount: 100, bonusPercent: 10, wantNominal: 10, wantNet: 10},
		{name: "partial offset", payAmount: 100, bonusPercent: 10, tierBonus: 5, wantNominal: 10, wantNet: 5},
		{name: "full offset", payAmount: 100, bonusPercent: 10, tierBonus: 10, wantNominal: 10, wantNet: 0},
		{name: "tier exceeds reward", payAmount: 100, bonusPercent: 10, tierBonus: 12, wantNominal: 10, wantNet: 0},
		{name: "negative snapshot is ignored", payAmount: 100, bonusPercent: 10, tierBonus: -5, wantNominal: 10, wantNet: 10},
		{name: "currency precision", payAmount: 99.99, bonusPercent: 10, tierBonus: 3, wantNominal: 9.999, wantNet: 6.999},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nominal, net := calculateFirstRechargeInviteeReward(test.payAmount, test.bonusPercent, test.tierBonus)
			require.InDelta(t, test.wantNominal, nominal, 1e-9)
			require.InDelta(t, test.wantNet, net, 1e-9)
		})
	}
}

func TestParseFirstRechargeBenefitSnapshot(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		credited    float64
		wantBonus   float64
		wantSettled bool
	}{
		{name: "valid snapshot", raw: `{"recharge_bonus_amount":5,"first_recharge_benefit_settled":true}`, credited: 105, wantBonus: 5, wantSettled: true},
		{name: "missing fields", raw: `{}`, credited: 100},
		{name: "null snapshot", raw: `null`, credited: 100},
		{name: "string amount", raw: `{"recharge_bonus_amount":"5"}`, credited: 105},
		{name: "negative amount", raw: `{"recharge_bonus_amount":-5}`, credited: 100},
		{name: "overflow amount", raw: `{"recharge_bonus_amount":1e400}`, credited: 100},
		{name: "amount exceeds credited total", raw: `{"recharge_bonus_amount":101}`, credited: 100},
		{name: "invalid json", raw: `{`, credited: 100},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bonus, settled := parseFirstRechargeBenefitSnapshot([]byte(test.raw), test.credited)
			require.InDelta(t, test.wantBonus, bonus, 1e-9)
			require.Equal(t, test.wantSettled, settled)
		})
	}
}
