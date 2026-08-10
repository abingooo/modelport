package service

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// PlazaModel 模型广场中单个模型条目。Pricing 是渠道配置的官方基础价；
// handler 会按分组和用户倍率另行生成实际展示价。
type PlazaModel struct {
	Name     string
	Platform string
	Pricing  *ChannelModelPricing
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 来自该分组关联渠道的
// 支持模型（按分组平台隔离，防跨平台泄漏），与「可用渠道」页口径一致。
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
//   - 同分组同名模型「先见者胜」；
//   - 只返回 Models 非空的分组；分组按 RateMultiplier 升序（同倍率按名称），
//     组内模型按名称排序。
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
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := groups[i]
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
			IsFree:               g.IsFree,
			IsExclusive:          g.IsExclusive,
		}
		order = append(order, g.ID)
	}

	// modelIdx[groupID][modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[string]int, len(groups))
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
				idx = make(map[string]int, len(supported))
				modelIdx[gid] = idx
			}
			for j := range supported {
				m := supported[j]
				if m.Platform != pg.Platform || m.Pricing == nil ||
					!m.Pricing.UserVisible || !plazaPricingConfigured(m.Pricing) {
					continue
				}
				if _, seen := idx[m.Name]; seen {
					continue
				}
				idx[m.Name] = len(pg.Models)
				pg.Models = append(pg.Models, PlazaModel(m))
			}
		}
	}

	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool { return pg.Models[i].Name < pg.Models[j].Name })
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
