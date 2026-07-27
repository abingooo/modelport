package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateAndCheckLimits_FreeGroupSkipsQuotaButEnforcesExpiry(t *testing.T) {
	svc := &SubscriptionService{}
	group := &Group{IsFree: true}
	limit := 1.0
	group.DailyLimitUSD = &limit

	active := &UserSubscription{
		Status:        SubscriptionStatusActive,
		ExpiresAt:     time.Now().Add(time.Hour),
		DailyUsageUSD: 99,
	}
	needsMaintenance, err := svc.ValidateAndCheckLimits(active, group)
	require.NoError(t, err)
	require.False(t, needsMaintenance)

	expired := &UserSubscription{
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	_, err = svc.ValidateAndCheckLimits(expired, group)
	require.ErrorIs(t, err, ErrSubscriptionExpired)
}
