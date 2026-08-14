package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotGroupModelPricingRoundTrip(t *testing.T) {
	groupID := int64(91)
	zero := 0.0
	outputPrice := 7.5e-6
	tierPrice := 0.25
	apiKey := &APIKey{
		ID: 7, UserID: 8, GroupID: &groupID, Key: "sk-group-pricing", Status: StatusActive,
		User: &User{ID: 8, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "priced", Platform: PlatformComposite, Status: StatusActive,
			LongContextPricingEnabled: false,
			ModelPricing: []ChannelModelPricing{
				{
					Platform: PlatformOpenAI, Models: []string{"gpt-5*"}, BillingMode: BillingModeToken,
					InputPrice: &zero, OutputPrice: &outputPrice,
				},
				{
					Platform: PlatformGrok, Models: []string{"grok-imagine-*"}, BillingMode: BillingModeImage,
					Intervals: []PricingInterval{{TierLabel: ImageBillingSize1K, PerRequestPrice: &tierPrice}},
				},
			},
		},
	}
	svc := &APIKeyService{}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var restored APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &restored))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &restored)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized)
	require.NotNil(t, materialized.Group)
	require.False(t, materialized.Group.LongContextPricingEnabled)
	require.Equal(t, apiKey.Group.ModelPricing, materialized.Group.ModelPricing)
	require.NotNil(t, materialized.Group.ModelPricing[0].InputPrice)
	require.Zero(t, *materialized.Group.ModelPricing[0].InputPrice)
}

func TestAPIKeyAuthSnapshotBeforeGroupModelPricingIsRejected(t *testing.T) {
	svc := &APIKeyService{}
	snapshot := &APIKeyAuthSnapshot{Version: apiKeyAuthSnapshotVersion - 1}

	materialized, used, err := svc.applyAuthCacheEntry("sk-stale-group-pricing", &APIKeyAuthCacheEntry{Snapshot: snapshot})

	require.NoError(t, err)
	require.False(t, used)
	require.Nil(t, materialized)
}
