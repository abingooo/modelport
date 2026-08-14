package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// PlazaModel 模型广场中单个模型条目。Pricing 是渠道配置的官方基础价；
// DisplayPricing 是应用分组逐模型价或旧媒体档位覆盖后的展示基础价，handler 会再按有效倍率
// 生成实际展示价。
type PlazaModel struct {
	Name           string
	Platform       string
	Pricing        *ChannelModelPricing
	DisplayPricing *ChannelModelPricing
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 来自该分组关联渠道的
// 支持模型（普通分组按分组平台隔离，Composite 分组展开关联渠道已配置的
// 具体平台），与「可用渠道」页口径一致。
type PlazaGroup struct {
	ID                   int64
	Name                 string
	Description          string
	Platform             string
	SubscriptionType     string
	RateMultiplier       float64
	PeakRateEnabled      bool
	PeakStart            string
	PeakEnd              string
	PeakRateMultiplier   float64
	ImageRateIndependent bool
	ImageRateMultiplier  float64
	VideoRateIndependent bool
	VideoRateMultiplier  float64
	IsFree               bool
	IsExclusive          bool
	Models               []PlazaModel
}

func (g *PlazaGroup) PeakMultiplierAt(now time.Time) float64 {
	if g == nil {
		return 1
	}
	group := Group{
		SubscriptionType:   g.SubscriptionType,
		PeakRateEnabled:    g.PeakRateEnabled,
		PeakStart:          g.PeakStart,
		PeakEnd:            g.PeakEnd,
		PeakRateMultiplier: g.PeakRateMultiplier,
	}
	return group.PeakMultiplierAt(now)
}

// ListPlazaGroups 返回模型广场数据：每个活跃分组附带其可用模型与定价。
//
// 模型广场只展示 Active 渠道中明确配置且允许用户查看的渠道定价，不使用全局
// LiteLLM 价格回落：
//   - 渠道按创建时间升序遍历（同时间按 ID），同名模型由最早渠道兜底；
//   - 普通分组按模型名去重；Composite 分组按平台和模型名去重并展开具体平台；
//   - 展示定价按“分组逐模型价 > 旧分组媒体价 > 渠道原价”合成；Pricing 始终保留
//     最早渠道的原始官方价，见 plazaDisplayPricing；
//   - 只返回 Models 非空的分组；分组按 RateMultiplier 升序（同倍率按名称），
//     组内模型按名称、平台排序。
//
// 可见性过滤（专属分组）不在此层做，由 handler 按登录态裁剪。
func (s *ChannelService) ListPlazaGroups(ctx context.Context) ([]PlazaGroup, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	sort.SliceStable(channels, func(i, j int) bool {
		if !channels[i].CreatedAt.Equal(channels[j].CreatedAt) {
			return channels[i].CreatedAt.Before(channels[j].CreatedAt)
		}
		return channels[i].ID < channels[j].ID
	})

	byGroup := make(map[int64]*PlazaGroup, len(groups))
	groupEnt := make(map[int64]*Group, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		byGroup[g.ID] = &PlazaGroup{
			ID:                   g.ID,
			Name:                 g.Name,
			Description:          g.Description,
			Platform:             g.Platform,
			SubscriptionType:     g.SubscriptionType,
			RateMultiplier:       g.RateMultiplier,
			PeakRateEnabled:      g.PeakRateEnabled,
			PeakStart:            g.PeakStart,
			PeakEnd:              g.PeakEnd,
			PeakRateMultiplier:   g.PeakRateMultiplier,
			ImageRateIndependent: g.ImageRateIndependent,
			ImageRateMultiplier:  g.ImageRateMultiplier,
			VideoRateIndependent: g.VideoRateIndependent,
			VideoRateMultiplier:  g.VideoRateMultiplier,
			IsFree:               g.IsFree,
			IsExclusive:          g.IsExclusive,
		}
		groupEnt[g.ID] = g
		order = append(order, g.ID)
	}

	type modelKey struct {
		platform string
		name     string
	}
	// modelIdx[groupID][platform+modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[modelKey]int, len(groups))
	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		ch.normalizeBillingModelSource()
		supported := ch.SupportedModels()

		for _, gid := range ch.GroupIDs {
			pg, ok := byGroup[gid]
			if !ok {
				continue
			}
			idx := modelIdx[gid]
			if idx == nil {
				idx = make(map[modelKey]int, len(supported))
				modelIdx[gid] = idx
			}
			for j := range supported {
				m := supported[j]
				if pg.Platform == PlatformComposite {
					if !isConcreteRequestPlatform(m.Platform) {
						continue
					}
				} else if m.Platform != pg.Platform {
					continue
				}
				if m.Pricing == nil || !m.Pricing.UserVisible || !plazaPricingConfigured(m.Pricing) {
					continue
				}
				displayPricing := plazaDisplayPricing(m.Pricing, groupEnt[gid], m.Platform, m.Name)
				key := modelKey{platform: m.Platform, name: m.Name}
				if pg.Platform != PlatformComposite {
					key.platform = ""
				}
				if _, seen := idx[key]; seen {
					continue
				}
				idx[key] = len(pg.Models)
				pg.Models = append(pg.Models, PlazaModel{
					Name:           m.Name,
					Platform:       m.Platform,
					Pricing:        m.Pricing,
					DisplayPricing: displayPricing,
				})
			}
		}
	}

	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool {
			if pg.Models[i].Name != pg.Models[j].Name {
				return pg.Models[i].Name < pg.Models[j].Name
			}
			return pg.Models[i].Platform < pg.Models[j].Platform
		})
		out = append(out, *pg)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RateMultiplier != out[j].RateMultiplier {
			return out[i].RateMultiplier < out[j].RateMultiplier
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// plazaDisplayPricing 合成分组内实际展示的基础价。分组逐模型价优先于旧图片/视频
// 分组价；两者均未配置时原样返回渠道价。调用方仍将原始 p 放入 PlazaModel.Pricing，
// 因而这里的任何覆盖都不得修改 p。
func plazaDisplayPricing(p *ChannelModelPricing, g *Group, platform, model string) *ChannelModelPricing {
	if p == nil || g == nil {
		return p
	}
	if groupPricing := plazaGroupModelPricing(g, platform, model); groupPricing != nil {
		mode := groupPricing.BillingMode
		if mode == "" || mode == BillingModeToken {
			return plazaTokenGroupDisplayPricing(p, groupPricing)
		}
		clone := groupPricing.Clone()
		return &clone
	}

	switch p.BillingMode {
	case BillingModeImage:
		return plazaImageDisplayPricing(p, g)
	case BillingModeVideo:
		return plazaVideoDisplayPricing(p, g, model)
	default:
		return p
	}
}

// plazaGroupModelPricing matches the displayed model within its concrete
// platform. Exact names always beat wildcard prefixes, independent of entry order.
func plazaGroupModelPricing(g *Group, platform, model string) *ChannelModelPricing {
	if g == nil {
		return nil
	}
	model = normalizeChannelPricingModelName(model)
	var wildcard *ChannelModelPricing
	for i := range g.ModelPricing {
		entry := &g.ModelPricing[i]
		entryPlatform := strings.TrimSpace(entry.Platform)
		if entryPlatform == "" {
			entryPlatform = g.Platform
		}
		if !strings.EqualFold(entryPlatform, platform) {
			continue
		}
		for _, pattern := range entry.Models {
			normalized := normalizeChannelPricingModelName(pattern)
			if normalized == model {
				return entry
			}
			if wildcard == nil && strings.HasSuffix(normalized, wildcardSuffix) &&
				strings.HasPrefix(model, strings.TrimSuffix(normalized, wildcardSuffix)) {
				wildcard = entry
			}
		}
	}
	return wildcard
}

// plazaTokenGroupDisplayPricing applies only explicitly configured token-card
// fields. Missing fields inherit the channel price; no LiteLLM/global lookup is
// involved. Group token cards use official long-context presets at billing time,
// so channel-defined token intervals must not leak into the group display price.
func plazaTokenGroupDisplayPricing(channelPricing, groupPricing *ChannelModelPricing) *ChannelModelPricing {
	if channelPricing == nil || groupPricing == nil {
		return channelPricing
	}
	clone := groupPricing.Clone()
	if channelPricing.BillingMode == "" || channelPricing.BillingMode == BillingModeToken {
		clone = channelPricing.Clone()
	}
	clone.BillingMode = BillingModeToken
	clone.Intervals = nil
	if groupPricing.InputPrice != nil {
		clone.InputPrice = groupPricing.InputPrice
	}
	if groupPricing.OutputPrice != nil {
		clone.OutputPrice = groupPricing.OutputPrice
	}
	if groupPricing.CacheWritePrice != nil {
		clone.CacheWritePrice = groupPricing.CacheWritePrice
	}
	if groupPricing.CacheReadPrice != nil {
		clone.CacheReadPrice = groupPricing.CacheReadPrice
	}
	if groupPricing.ImageInputPrice != nil {
		clone.ImageInputPrice = groupPricing.ImageInputPrice
	}
	if groupPricing.ImageOutputPrice != nil {
		clone.ImageOutputPrice = groupPricing.ImageOutputPrice
	}
	return &clone
}

func plazaPricingConfigured(p *ChannelModelPricing) bool {
	if p == nil {
		return false
	}
	if p.InputPrice != nil || p.OutputPrice != nil ||
		p.CacheWritePrice != nil || p.CacheReadPrice != nil ||
		p.ImageInputPrice != nil || p.ImageOutputPrice != nil || p.PerRequestPrice != nil {
		return true
	}
	for _, interval := range p.Intervals {
		if interval.InputPrice != nil || interval.OutputPrice != nil ||
			interval.CacheWritePrice != nil || interval.CacheReadPrice != nil ||
			interval.PerRequestPrice != nil {
			return true
		}
	}
	return false
}

// plazaImageDisplayPricing 为图片计费模型合成展示定价，使档位价与实收口径一致：
// 每档（1K/2K/4K）单价 = 分组图片价 > 渠道同档位价 > 渠道默认按次价，无价的档不展示。
// 分组未配任何图片价、或定价非图片模式时原样返回。返回克隆，不修改入参
// （渠道定价指针指向缓存共享数据）。
func plazaImageDisplayPricing(p *ChannelModelPricing, g *Group) *ChannelModelPricing {
	if p == nil || g == nil || p.BillingMode != BillingModeImage {
		return p
	}
	if g.ImagePrice1K == nil && g.ImagePrice2K == nil && g.ImagePrice4K == nil {
		return p
	}
	channelTierPrice := func(label string) *float64 {
		for i := range p.Intervals {
			if p.Intervals[i].TierLabel == label && p.Intervals[i].PerRequestPrice != nil {
				return p.Intervals[i].PerRequestPrice
			}
		}
		return p.PerRequestPrice
	}
	tiers := []struct {
		label      string
		groupPrice *float64
	}{
		{"1K", g.ImagePrice1K},
		{"2K", g.ImagePrice2K},
		{"4K", g.ImagePrice4K},
	}
	clone := *p
	clone.Intervals = make([]PricingInterval, 0, len(tiers))
	for i, t := range tiers {
		price := t.groupPrice
		if price == nil {
			price = channelTierPrice(t.label)
		}
		if price == nil {
			continue
		}
		v := *price
		clone.Intervals = append(clone.Intervals, PricingInterval{
			TierLabel:       t.label,
			PerRequestPrice: &v,
			SortOrder:       i,
		})
	}
	return &clone
}

// plazaVideoDisplayPricing mirrors legacy video billing precedence for the
// three supported per-second tiers: model-specific/flat group price > channel
// tier > channel default price. It returns the original channel object when no
// legacy group video price applies.
func plazaVideoDisplayPricing(p *ChannelModelPricing, g *Group, model string) *ChannelModelPricing {
	if p == nil || g == nil || p.BillingMode != BillingModeVideo {
		return p
	}
	tiers := []string{
		VideoBillingResolution480P,
		VideoBillingResolution720P,
		VideoBillingResolution1080P,
	}
	hasGroupPrice := false
	for _, tier := range tiers {
		if g.GetVideoPriceForModel(model, tier) != nil {
			hasGroupPrice = true
			break
		}
	}
	if !hasGroupPrice {
		return p
	}

	channelTierPrice := func(label string) *float64 {
		for i := range p.Intervals {
			if normalized, ok := LookupVideoBillingResolution(p.Intervals[i].TierLabel); ok &&
				normalized == label && p.Intervals[i].PerRequestPrice != nil {
				return p.Intervals[i].PerRequestPrice
			}
		}
		return p.PerRequestPrice
	}
	clone := p.Clone()
	clone.Intervals = make([]PricingInterval, 0, len(tiers))
	for i, tier := range tiers {
		price := g.GetVideoPriceForModel(model, tier)
		if price == nil {
			price = channelTierPrice(tier)
		}
		if price == nil {
			continue
		}
		value := *price
		clone.Intervals = append(clone.Intervals, PricingInterval{
			TierLabel:       tier,
			PerRequestPrice: &value,
			SortOrder:       i,
		})
	}
	return &clone
}
