//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newPlazaChannelService 构造 ListPlazaGroups 测试用的 ChannelService。
func newPlazaChannelService(channels []Channel, groups []Group, pricing *PricingService) *ChannelService {
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return channels, nil },
	}
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{activeGroups: groups}, nil, nil)
	svc.pricingService = pricing
	return svc
}

func plazaPricedChannel(id int64, name string, groupIDs []int64, platform string, models ...string) Channel {
	return Channel{
		ID:       id,
		Name:     name,
		Status:   StatusActive,
		GroupIDs: groupIDs,
		ModelPricing: []ChannelModelPricing{{
			Platform:    platform,
			Models:      models,
			BillingMode: BillingModeToken,
			InputPrice:  testPtrFloat64(3e-6),
			OutputPrice: testPtrFloat64(1.5e-5),
			UserVisible: true,
		}},
	}
}

func TestListPlazaGroups_GroupCentricAggregation(t *testing.T) {
	// 两个渠道挂同一分组:模型并入同一 PlazaGroup;无模型的分组不返回。
	channels := []Channel{
		plazaPricedChannel(1, "chA", []int64{10}, "anthropic", "claude-sonnet"),
		plazaPricedChannel(2, "chB", []int64{10}, "anthropic", "claude-opus"),
	}
	groups := []Group{
		{ID: 10, Name: "g-main", Description: "desc", Platform: "anthropic", RateMultiplier: 1, IsFree: true},
		{ID: 20, Name: "g-empty", Platform: "anthropic", RateMultiplier: 0.5},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1, "无模型的分组不应返回")
	require.Equal(t, int64(10), out[0].ID)
	require.Equal(t, "desc", out[0].Description)
	require.True(t, out[0].IsFree)
	require.Len(t, out[0].Models, 2)
	// 组内模型按名称排序
	require.Equal(t, "claude-opus", out[0].Models[0].Name)
	require.Equal(t, "claude-sonnet", out[0].Models[1].Name)
}

func TestListPlazaGroups_DedupUsesOldestChannel(t *testing.T) {
	older := plazaPricedChannel(20, "z-older", []int64{10}, "anthropic", "claude-sonnet")
	older.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older.ModelPricing[0].InputPrice = testPtrFloat64(1e-6)
	younger := plazaPricedChannel(10, "a-younger", []int64{10}, "anthropic", "claude-sonnet")
	younger.CreatedAt = older.CreatedAt.Add(time.Hour)
	younger.ModelPricing[0].InputPrice = testPtrFloat64(9e-6)
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}

	// 输入顺序和名称顺序都与创建时间相反，仍应选择更早创建的渠道。
	svc := newPlazaChannelService([]Channel{younger, older}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.InDelta(t, 1e-6, *out[0].Models[0].Pricing.InputPrice, 1e-12)
}

func TestListPlazaGroups_PlatformIsolation(t *testing.T) {
	// 渠道同时有 anthropic/openai 定价,anthropic 分组只应看到 anthropic 模型。
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet"}, InputPrice: testPtrFloat64(3e-6), UserVisible: true},
			{Platform: "openai", Models: []string{"gpt-5"}, InputPrice: testPtrFloat64(2e-6), UserVisible: true},
		},
	}
	groups := []Group{
		{ID: 10, Name: "g-claude", Platform: "anthropic", RateMultiplier: 1},
		{ID: 20, Name: "g-gpt", Platform: "openai", RateMultiplier: 1},
	}
	svc := newPlazaChannelService([]Channel{ch}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	byName := map[string][]PlazaModel{}
	for _, g := range out {
		byName[g.Name] = g.Models
	}
	require.Len(t, byName["g-claude"], 1)
	require.Equal(t, "claude-sonnet", byName["g-claude"][0].Name)
	require.Len(t, byName["g-gpt"], 1)
	require.Equal(t, "gpt-5", byName["g-gpt"][0].Name)
}

func TestListPlazaGroups_UserVisibleFiltersPricingEntries(t *testing.T) {
	channel := plazaPricedChannel(1, "ch", []int64{10}, "anthropic", "visible-model")
	channel.ModelPricing = append(channel.ModelPricing, ChannelModelPricing{
		Platform:    "anthropic",
		Models:      []string{"hidden-model"},
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(1e-6),
		UserVisible: false,
	})
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}

	out, err := newPlazaChannelService([]Channel{channel}, groups, nil).ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.Equal(t, "visible-model", out[0].Models[0].Name)
}

func TestListPlazaGroups_ImageInputOnlyPricingIsConfigured(t *testing.T) {
	channel := plazaPricedChannel(1, "ch", []int64{10}, "openai", "image-edit")
	channel.ModelPricing[0] = ChannelModelPricing{
		Platform:        "openai",
		Models:          []string{"image-edit"},
		BillingMode:     BillingModeImage,
		ImageInputPrice: testPtrFloat64(1e-6),
		UserVisible:     true,
	}
	groups := []Group{{ID: 10, Name: "g", Platform: "openai", RateMultiplier: 1}}

	out, err := newPlazaChannelService([]Channel{channel}, groups, nil).ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
}

func TestListPlazaGroups_InactiveChannelSkipped(t *testing.T) {
	inactive := plazaPricedChannel(1, "off", []int64{10}, "anthropic", "claude-sonnet")
	inactive.Status = "inactive"
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}
	svc := newPlazaChannelService([]Channel{inactive}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestListPlazaGroups_SortedByRateMultiplierAsc(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10, 20, 30}, "anthropic", "claude-sonnet"),
	}
	groups := []Group{
		{ID: 10, Name: "b-standard", Platform: "anthropic", RateMultiplier: 1},
		{ID: 20, Name: "a-standard", Platform: "anthropic", RateMultiplier: 1},
		{ID: 30, Name: "cheap", Platform: "anthropic", RateMultiplier: 0.5},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "cheap", out[0].Name, "倍率低者在前")
	require.Equal(t, "a-standard", out[1].Name, "同倍率按名称")
	require.Equal(t, "b-standard", out[2].Name)
}

func TestListPlazaGroups_DoesNotUseLiteLLMFallback(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"fallback-only": {Mode: "chat", InputCostPerToken: 8e-6, OutputCostPerToken: 9e-6},
	})
	channel := plazaPricedChannel(1, "ch", []int64{10}, "anthropic", "configured")
	channel.ModelMapping = map[string]map[string]string{
		"anthropic": {"fallback-only": "fallback-only"},
	}
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}
	svc := newPlazaChannelService([]Channel{channel}, groups, pricingSvc)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.Equal(t, "configured", out[0].Models[0].Name)
}

func TestListPlazaGroups_RepoErrorsPropagate(t *testing.T) {
	sentinel := errors.New("boom")
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, sentinel },
	}
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{}, nil, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)

	svc2 := NewChannelService(
		&mockChannelRepository{listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, nil }},
		&stubGroupRepoForAvailable{listActiveErr: sentinel},
		nil, nil,
	)
	out2, err2 := svc2.ListPlazaGroups(context.Background())
	require.Nil(t, out2)
	require.ErrorIs(t, err2, sentinel)
}
