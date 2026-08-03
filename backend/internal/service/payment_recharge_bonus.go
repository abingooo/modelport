package service

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const maxRechargeBonusTiers = 20

type RechargeBonusTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}

type RechargeBonusSnapshot struct {
	PrincipalAmount float64 `json:"principal_amount"`
	BaseAmount      float64 `json:"base_amount"`
	BonusPercent    float64 `json:"bonus_percent"`
	BonusAmount     float64 `json:"bonus_amount"`
}

var defaultRechargeBonusTiers = []RechargeBonusTier{
	{MinAmount: 0, BonusPercent: 3},
	{MinAmount: 100, BonusPercent: 5},
	{MinAmount: 500, BonusPercent: 8},
	{MinAmount: 1000, BonusPercent: 10},
}

func parseRechargeBonusTiers(raw string) []RechargeBonusTier {
	if strings.TrimSpace(raw) == "" {
		return cloneRechargeBonusTiers(defaultRechargeBonusTiers)
	}
	var tiers []RechargeBonusTier
	if err := json.Unmarshal([]byte(raw), &tiers); err != nil {
		return cloneRechargeBonusTiers(defaultRechargeBonusTiers)
	}
	normalized, err := normalizeRechargeBonusTiers(tiers)
	if err != nil {
		return cloneRechargeBonusTiers(defaultRechargeBonusTiers)
	}
	return normalized
}

func encodeRechargeBonusTiers(tiers []RechargeBonusTier) (string, error) {
	encoded, err := json.Marshal(tiers)
	return string(encoded), err
}

func normalizeRechargeBonusTiers(tiers []RechargeBonusTier) ([]RechargeBonusTier, error) {
	if len(tiers) == 0 || len(tiers) > maxRechargeBonusTiers {
		return nil, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", "recharge bonus tiers must contain between 1 and 20 entries")
	}
	normalized := cloneRechargeBonusTiers(tiers)
	for _, tier := range normalized {
		if math.IsNaN(tier.MinAmount) || math.IsInf(tier.MinAmount, 0) || tier.MinAmount < 0 ||
			math.IsNaN(tier.BonusPercent) || math.IsInf(tier.BonusPercent, 0) || tier.BonusPercent < 0 || tier.BonusPercent > 100 {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", "tier amounts must be non-negative and bonus percentages must be between 0 and 100")
		}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].MinAmount < normalized[j].MinAmount })
	for i := 1; i < len(normalized); i++ {
		if normalized[i].MinAmount == normalized[i-1].MinAmount {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", "tier minimum amounts must be unique")
		}
	}
	return normalized, nil
}

func cloneRechargeBonusTiers(tiers []RechargeBonusTier) []RechargeBonusTier {
	return append([]RechargeBonusTier(nil), tiers...)
}

func calculateRechargeBonus(principalAmount, baseCreditedAmount float64, cfg *PaymentConfig) RechargeBonusSnapshot {
	snapshot := RechargeBonusSnapshot{
		PrincipalAmount: principalAmount,
		BaseAmount:      baseCreditedAmount,
	}
	if cfg == nil || !cfg.RechargeBonusEnabled || principalAmount <= 0 || baseCreditedAmount <= 0 {
		return snapshot
	}
	for _, tier := range cfg.RechargeBonusTiers {
		if principalAmount < tier.MinAmount {
			break
		}
		snapshot.BonusPercent = tier.BonusPercent
	}
	if snapshot.BonusPercent <= 0 {
		return snapshot
	}
	snapshot.BonusAmount = decimal.NewFromFloat(baseCreditedAmount).
		Mul(decimal.NewFromFloat(snapshot.BonusPercent)).
		Div(decimal.NewFromInt(100)).
		Round(2).
		InexactFloat64()
	return snapshot
}

func addRechargeBonusSnapshot(snapshot map[string]any, bonus RechargeBonusSnapshot) map[string]any {
	if bonus.BonusAmount <= 0 {
		return snapshot
	}
	if snapshot == nil {
		snapshot = map[string]any{"schema_version": 2}
	}
	snapshot["recharge_principal_amount"] = bonus.PrincipalAmount
	snapshot["recharge_base_amount"] = bonus.BaseAmount
	snapshot["recharge_bonus_percent"] = bonus.BonusPercent
	snapshot["recharge_bonus_amount"] = bonus.BonusAmount
	return snapshot
}

func PaymentOrderRechargeBonus(order *dbent.PaymentOrder) RechargeBonusSnapshot {
	if order == nil || len(order.ProviderSnapshot) == 0 {
		return RechargeBonusSnapshot{}
	}
	return RechargeBonusSnapshot{
		PrincipalAmount: psSnapshotFloatValue(order.ProviderSnapshot["recharge_principal_amount"]),
		BaseAmount:      psSnapshotFloatValue(order.ProviderSnapshot["recharge_base_amount"]),
		BonusPercent:    psSnapshotFloatValue(order.ProviderSnapshot["recharge_bonus_percent"]),
		BonusAmount:     psSnapshotFloatValue(order.ProviderSnapshot["recharge_bonus_amount"]),
	}
}

func psSnapshotFloatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}
