package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newGatewayFreeGroupRecordUsageServiceForTest(usageRepo UsageLogRepository, billingRepo UsageBillingRepository) *GatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	svc := NewGatewayService(
		nil, nil, usageRepo, billingRepo, nil, nil, nil, nil, cfg,
		nil, nil, NewBillingService(cfg, nil), nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	svc.deferredService = &DeferredService{}
	return svc
}

func TestGatewayServiceRecordUsage_FreeGroupKeepsRawAndAccountCost(t *testing.T) {
	groupID := int64(44)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}

	svc := newGatewayFreeGroupRecordUsageServiceForTest(usageRepo, billingRepo)
	svc.userRepo = userRepo
	svc.userSubRepo = subRepo

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_free_group",
			Usage: ClaudeUsage{
				InputTokens:              1200,
				OutputTokens:             300,
				CacheReadInputTokens:     100,
				CacheCreationInputTokens: 50,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:      1003,
			GroupID: &groupID,
			Quota:   1,
			Group: &Group{
				ID:             groupID,
				Platform:       PlatformAnthropic,
				RateMultiplier: 2,
				IsFree:         true,
			},
		},
		User: &User{ID: 2003},
		Account: &Account{
			ID:       3003,
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"quota_limit": 100.0},
		},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 1200, usageRepo.lastLog.InputTokens)
	require.Equal(t, 300, usageRepo.lastLog.OutputTokens)
	require.Equal(t, 100, usageRepo.lastLog.CacheReadTokens)
	require.Equal(t, 50, usageRepo.lastLog.CacheCreationTokens)
	require.Greater(t, usageRepo.lastLog.TotalCost, 0.0)
	require.Zero(t, usageRepo.lastLog.ActualCost)
	require.Zero(t, usageRepo.lastLog.RateMultiplier)
	require.NotNil(t, billingRepo.lastCmd)
	require.Zero(t, billingRepo.lastCmd.BalanceCost)
	require.Zero(t, billingRepo.lastCmd.SubscriptionCost)
	require.Zero(t, billingRepo.lastCmd.APIKeyQuotaCost)
	require.Zero(t, billingRepo.lastCmd.APIKeyRateLimitCost)
	require.InDelta(t, usageRepo.lastLog.TotalCost, billingRepo.lastCmd.AccountQuotaCost, 1e-12)
	require.Zero(t, userRepo.deductCalls)
	require.Zero(t, subRepo.incrementCalls)
	require.Zero(t, quotaSvc.quotaCalls)
	require.Zero(t, quotaSvc.rateLimitCalls)
}
