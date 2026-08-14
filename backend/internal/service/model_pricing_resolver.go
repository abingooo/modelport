package service

import (
	"context"
	"log/slog"
	"strings"
)

// PricingSource 定价来源标识
const (
	PricingSourceGroup    = "group"
	PricingSourceChannel  = "channel"
	PricingSourceLiteLLM  = "litellm"
	PricingSourceFallback = "fallback"
)

// ResolvedPricing 统一定价解析结果
type ResolvedPricing struct {
	// Mode 计费模式
	Mode BillingMode

	// Token 模式：基础定价（来自 LiteLLM 或 fallback）
	BasePricing *ModelPricing

	// Token 模式：区间定价列表（如有，覆盖 BasePricing 中的对应字段）
	Intervals []PricingInterval

	// 按次/图片模式：分层定价
	RequestTiers []PricingInterval

	// 按次/图片模式：默认价格（未命中层级时使用）
	DefaultPerRequestPrice float64

	// 来源标识
	Source string // "channel", "litellm", "fallback"

	// 是否支持缓存细分
	SupportsCacheBreakdown bool

	// 渠道定价原始配置（用于区间模式下获取 ImageOutputPrice）
	channelPricing *ChannelModelPricing

	longContextPricingEnabled bool
}

// ModelPricingResolver 统一模型定价解析器。
// 解析链：Group → Channel → LiteLLM → Fallback。
type ModelPricingResolver struct {
	channelService *ChannelService
	billingService *BillingService
}

// NewModelPricingResolver 创建定价解析器实例
func NewModelPricingResolver(channelService *ChannelService, billingService *BillingService) *ModelPricingResolver {
	return &ModelPricingResolver{
		channelService: channelService,
		billingService: billingService,
	}
}

// PricingInput 定价解析输入
type PricingInput struct {
	Model   string
	GroupID *int64 // nil 表示不检查渠道
	Group   *Group
}

// Resolve 解析模型定价。
// 1. 获取基础定价（LiteLLM → Fallback）
// 2. 如果指定了 GroupID，查找渠道定价并覆盖
func (r *ModelPricingResolver) Resolve(ctx context.Context, input PricingInput) *ResolvedPricing {
	longContextPricingEnabled := input.Group == nil || input.Group.LongContextPricingEnabled
	var chPricing *ChannelModelPricing
	if input.GroupID != nil && r.channelService != nil {
		chPricing = r.channelService.GetChannelModelPricing(ctx, *input.GroupID, input.Model)
	}
	if groupPricing := matchGroupModelPricing(input.Group, input.Model); groupPricing != nil {
		// Group token cards only override the first-tier / flat rates.
		// Long-context ladders come from official presets, gated by the checkbox.
		if groupPricing.BillingMode == "" || groupPricing.BillingMode == BillingModeToken {
			stripped := groupPricing.Clone()
			stripped.Intervals = nil
			groupPricing = &stripped
		}
		resolved := r.resolveGroupPricing(groupPricing, chPricing, input.Model)
		resolved.longContextPricingEnabled = longContextPricingEnabled
		return resolved
	}

	if chPricing != nil {
		mode := chPricing.BillingMode
		if mode == "" {
			mode = BillingModeToken
		}
		if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo {
			resolved := &ResolvedPricing{
				Mode:           mode,
				Source:         PricingSourceChannel,
				channelPricing: chPricing,
			}
			resolved.longContextPricingEnabled = longContextPricingEnabled
			r.applyRequestTierOverrides(chPricing, resolved)
			return resolved
		}
	}

	// 1. 获取基础定价
	basePricing, source := r.resolveBasePricing(input.Model)

	resolved := &ResolvedPricing{
		Mode:                   BillingModeToken,
		BasePricing:            basePricing,
		Source:                 source,
		SupportsCacheBreakdown: basePricing != nil && basePricing.SupportsCacheBreakdown,
	}
	resolved.longContextPricingEnabled = longContextPricingEnabled

	// 2. 如果有 GroupID，尝试渠道覆盖
	if chPricing != nil {
		resolved.Source = PricingSourceChannel
		resolved.channelPricing = chPricing
		r.applyTokenOverrides(chPricing, resolved)
		if !longContextPricingEnabled {
			r.applyFirstTokenTier(resolved, chPricing)
		}
	} else if input.GroupID != nil && r.channelService != nil {
		r.applyChannelOverrides(ctx, *input.GroupID, input.Model, resolved)
		if resolved.Source == PricingSourceChannel && !longContextPricingEnabled {
			r.applyFirstTokenTier(resolved, resolved.channelPricing)
		}
	}

	return resolved
}

// resolveGroupPricing overlays a token group card on the channel card selected
// for the same group/model. Media group cards remain complete replacements.
func (r *ModelPricingResolver) resolveGroupPricing(groupPricing, channelPricing *ChannelModelPricing, model string) *ResolvedPricing {
	mode := groupPricing.BillingMode
	if mode == "" {
		mode = BillingModeToken
	}
	if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo {
		configured := groupPricing.Clone()
		resolved := &ResolvedPricing{Mode: mode, Source: PricingSourceGroup, channelPricing: &configured}
		r.applyRequestTierOverrides(&configured, resolved)
		return resolved
	}

	basePricing, _ := r.resolveBasePricing(model)
	hasTokenChannel := channelPricing != nil && (channelPricing.BillingMode == "" || channelPricing.BillingMode == BillingModeToken)
	merged := overlayGroupTokenPricing(channelPricing, groupPricing)
	resolved := &ResolvedPricing{
		Mode:                   BillingModeToken,
		BasePricing:            basePricing,
		Source:                 PricingSourceGroup,
		SupportsCacheBreakdown: basePricing != nil && basePricing.SupportsCacheBreakdown,
		channelPricing:         merged,
	}
	r.applyGroupTokenOverrides(merged, resolved, hasTokenChannel)
	return resolved
}

// overlayGroupTokenPricing overlays the explicitly configured fields from a
// group token card onto a channel token card. Media channel cards are not a
// valid token-price base, and token intervals never carry over to group cards.
func overlayGroupTokenPricing(channelPricing, groupPricing *ChannelModelPricing) *ChannelModelPricing {
	if groupPricing == nil {
		return nil
	}

	merged := groupPricing.Clone()
	if channelPricing != nil && (channelPricing.BillingMode == "" || channelPricing.BillingMode == BillingModeToken) {
		merged = channelPricing.Clone()
		merged.Platform = groupPricing.Platform
		merged.Models = append([]string(nil), groupPricing.Models...)
	}
	merged.BillingMode = BillingModeToken
	merged.Intervals = nil

	override := groupPricing
	if override.InputPrice != nil {
		merged.InputPrice = override.InputPrice
	}
	if override.OutputPrice != nil {
		merged.OutputPrice = override.OutputPrice
	}
	if override.CacheWritePrice != nil {
		merged.CacheWritePrice = override.CacheWritePrice
	}
	if override.CacheReadPrice != nil {
		merged.CacheReadPrice = override.CacheReadPrice
	}
	if override.ImageInputPrice != nil {
		merged.ImageInputPrice = override.ImageInputPrice
	}
	if override.ImageOutputPrice != nil {
		merged.ImageOutputPrice = override.ImageOutputPrice
	}
	return &merged
}

// applyGroupTokenOverrides applies only fields present after the group/channel
// overlay. Unlike direct channel pricing, a field absent from both layers must
// retain the built-in value instead of being treated as an explicit zero.
func (r *ModelPricingResolver) applyGroupTokenOverrides(pricing *ChannelModelPricing, resolved *ResolvedPricing, hasTokenChannel bool) {
	if pricing == nil || resolved == nil {
		return
	}
	if resolved.BasePricing == nil {
		resolved.BasePricing = &ModelPricing{}
	} else {
		cloned := *resolved.BasePricing
		resolved.BasePricing = &cloned
	}

	if pricing.InputPrice != nil {
		resolved.BasePricing.InputPricePerToken = *pricing.InputPrice
		resolved.BasePricing.InputPricePerTokenPriority = *pricing.InputPrice
	}
	if pricing.OutputPrice != nil {
		resolved.BasePricing.OutputPricePerToken = *pricing.OutputPrice
		resolved.BasePricing.OutputPricePerTokenPriority = *pricing.OutputPrice
	}
	if pricing.CacheWritePrice != nil {
		resolved.BasePricing.CacheCreationPricePerToken = *pricing.CacheWritePrice
		resolved.BasePricing.CacheCreationPricePerTokenPriority = *pricing.CacheWritePrice
		resolved.BasePricing.CacheCreationPriceExplicit = true
		resolved.BasePricing.CacheCreation5mPrice = *pricing.CacheWritePrice
		resolved.BasePricing.CacheCreation1hPrice = *pricing.CacheWritePrice
	}
	if pricing.CacheReadPrice != nil {
		resolved.BasePricing.CacheReadPricePerToken = *pricing.CacheReadPrice
		resolved.BasePricing.CacheReadPricePerTokenPriority = *pricing.CacheReadPrice
	}
	if pricing.ImageInputPrice != nil {
		resolved.BasePricing.ImageInputPricePerToken = *pricing.ImageInputPrice
	} else if hasTokenChannel {
		resolved.BasePricing.ImageInputPricePerToken = 0
	}
	if pricing.ImageOutputPrice != nil {
		resolved.BasePricing.ImageOutputPricePerToken = *pricing.ImageOutputPrice
		resolved.BasePricing.ImageOutputPriceExplicit = true
	} else if hasTokenChannel {
		resolved.BasePricing.ImageOutputPricePerToken = 0
		resolved.BasePricing.ImageOutputPriceExplicit = true
	}
}

func matchGroupModelPricing(group *Group, model string) *ChannelModelPricing {
	if group == nil {
		return nil
	}
	groupPlatform := NormalizeGroupPlatform(strings.TrimSpace(group.Platform))
	model = normalizeChannelPricingModelName(model)
	var wildcard *ChannelModelPricing
	for i := range group.ModelPricing {
		entry := &group.ModelPricing[i]
		entryPlatform := strings.TrimSpace(entry.Platform)
		if entryPlatform == "" {
			entryPlatform = groupPlatform
		}
		if groupPlatform != PlatformComposite && !strings.EqualFold(entryPlatform, groupPlatform) {
			continue
		}
		for _, pattern := range entry.Models {
			normalized := normalizeChannelPricingModelName(pattern)
			if normalized == model {
				cp := entry.Clone()
				return &cp
			}
			if strings.HasSuffix(normalized, "*") && strings.HasPrefix(model, strings.TrimSuffix(normalized, "*")) && wildcard == nil {
				cp := entry.Clone()
				wildcard = &cp
			}
		}
	}
	return wildcard
}

func (r *ModelPricingResolver) applyFirstTokenTier(resolved *ResolvedPricing, config *ChannelModelPricing) {
	if resolved == nil || len(resolved.Intervals) == 0 {
		return
	}
	first := resolved.Intervals[0]
	for _, interval := range resolved.Intervals[1:] {
		if interval.MinTokens < first.MinTokens {
			first = interval
		}
	}
	resolved.BasePricing = intervalToModelPricing(resolved.BasePricing, &first, resolved.SupportsCacheBreakdown, config)
	resolved.Intervals = nil
}

// resolveBasePricing 从 LiteLLM 或 Fallback 获取基础定价
func (r *ModelPricingResolver) resolveBasePricing(model string) (*ModelPricing, string) {
	pricing, err := r.billingService.GetModelPricing(model)
	if err != nil {
		slog.Debug("failed to get model pricing from LiteLLM, using fallback",
			"model", model, "error", err)
		return nil, PricingSourceFallback
	}
	return pricing, PricingSourceLiteLLM
}

// applyChannelOverrides 应用渠道定价覆盖
func (r *ModelPricingResolver) applyChannelOverrides(ctx context.Context, groupID int64, model string, resolved *ResolvedPricing) {
	chPricing := r.channelService.GetChannelModelPricing(ctx, groupID, model)
	if chPricing == nil {
		return
	}

	resolved.Source = PricingSourceChannel
	resolved.channelPricing = chPricing
	resolved.Mode = chPricing.BillingMode
	if resolved.Mode == "" {
		resolved.Mode = BillingModeToken
	}

	switch resolved.Mode {
	case BillingModeToken:
		r.applyTokenOverrides(chPricing, resolved)
	case BillingModePerRequest, BillingModeImage, BillingModeVideo:
		r.applyRequestTierOverrides(chPricing, resolved)
	}
}

// applyTokenOverrides 应用 token 模式的渠道覆盖
func (r *ModelPricingResolver) applyTokenOverrides(chPricing *ChannelModelPricing, resolved *ResolvedPricing) {
	// 过滤掉所有价格字段都为空的无效 interval
	validIntervals := filterValidIntervals(chPricing.Intervals)

	// 如果有有效的区间定价，使用区间
	if len(validIntervals) > 0 {
		resolved.Intervals = validIntervals
		// 区间不匹配时回退到 BasePricing，也需要覆盖图片价格
		if resolved.BasePricing == nil {
			resolved.BasePricing = &ModelPricing{}
		} else {
			// 防止修改 fallbackPrices 中的共享指针
			cloned := *resolved.BasePricing
			resolved.BasePricing = &cloned
		}
		if chPricing.ImageOutputPrice != nil {
			resolved.BasePricing.ImageOutputPricePerToken = *chPricing.ImageOutputPrice
		} else {
			resolved.BasePricing.ImageOutputPricePerToken = 0
		}
		resolved.BasePricing.ImageOutputPriceExplicit = true
		applyChannelImageInputPrice(chPricing, resolved.BasePricing)
		return
	}

	// 否则用 flat 字段覆盖 BasePricing
	if resolved.BasePricing == nil {
		resolved.BasePricing = &ModelPricing{}
	} else {
		// 防止修改 fallbackPrices 中的共享指针
		cloned := *resolved.BasePricing
		resolved.BasePricing = &cloned
	}

	if chPricing.InputPrice != nil {
		resolved.BasePricing.InputPricePerToken = *chPricing.InputPrice
		resolved.BasePricing.InputPricePerTokenPriority = *chPricing.InputPrice
	}
	if chPricing.OutputPrice != nil {
		resolved.BasePricing.OutputPricePerToken = *chPricing.OutputPrice
		resolved.BasePricing.OutputPricePerTokenPriority = *chPricing.OutputPrice
	}
	if chPricing.CacheWritePrice != nil {
		resolved.BasePricing.CacheCreationPricePerToken = *chPricing.CacheWritePrice
		resolved.BasePricing.CacheCreationPricePerTokenPriority = *chPricing.CacheWritePrice
		resolved.BasePricing.CacheCreationPriceExplicit = true
		resolved.BasePricing.CacheCreation5mPrice = *chPricing.CacheWritePrice
		resolved.BasePricing.CacheCreation1hPrice = *chPricing.CacheWritePrice
	}
	if chPricing.CacheReadPrice != nil {
		resolved.BasePricing.CacheReadPricePerToken = *chPricing.CacheReadPrice
		resolved.BasePricing.CacheReadPricePerTokenPriority = *chPricing.CacheReadPrice
	}
	// 渠道定价覆盖一切：显式配置则用配置值，未配置则归零（不回退到 LiteLLM）
	if chPricing.ImageOutputPrice != nil {
		resolved.BasePricing.ImageOutputPricePerToken = *chPricing.ImageOutputPrice
	} else {
		resolved.BasePricing.ImageOutputPricePerToken = 0
	}
	resolved.BasePricing.ImageOutputPriceExplicit = true
	applyChannelImageInputPrice(chPricing, resolved.BasePricing)
}

// applyChannelImageInputPrice 应用渠道图片输入价：显式配置则用配置值；
// 未配置时归零，使 computeTokenBreakdown 回退到文本输入价（向后兼容，
// 避免 commit 引入的 LiteLLM 图片输入价泄漏进渠道自定义定价）。
// 与 image_output 不同，此处不设 Explicit 标志——图片输入未配置应回退文本价，
// 而非硬置 0。
func applyChannelImageInputPrice(chPricing *ChannelModelPricing, pricing *ModelPricing) {
	if chPricing != nil && chPricing.ImageInputPrice != nil {
		pricing.ImageInputPricePerToken = *chPricing.ImageInputPrice
	} else {
		pricing.ImageInputPricePerToken = 0
	}
}

// applyRequestTierOverrides 应用按次/图片模式的渠道覆盖
func (r *ModelPricingResolver) applyRequestTierOverrides(chPricing *ChannelModelPricing, resolved *ResolvedPricing) {
	resolved.RequestTiers = filterValidIntervals(chPricing.Intervals)
	if chPricing.PerRequestPrice != nil {
		resolved.DefaultPerRequestPrice = *chPricing.PerRequestPrice
	}
}

// filterValidIntervals 过滤掉所有价格字段都为空的无效 interval。
// 前端可能创建了只有 min/max 但无价格的空 interval。
func filterValidIntervals(intervals []PricingInterval) []PricingInterval {
	var valid []PricingInterval
	for _, iv := range intervals {
		if iv.InputPrice != nil || iv.OutputPrice != nil ||
			iv.CacheWritePrice != nil || iv.CacheReadPrice != nil ||
			iv.PerRequestPrice != nil {
			valid = append(valid, iv)
		}
	}
	return valid
}

// GetIntervalPricing 根据 context token 数获取区间定价。
// 如果有区间列表，找到匹配区间并构造 ModelPricing；否则直接返回 BasePricing。
func (r *ModelPricingResolver) GetIntervalPricing(resolved *ResolvedPricing, totalContextTokens int) *ModelPricing {
	if len(resolved.Intervals) == 0 {
		return resolved.BasePricing
	}

	iv := FindMatchingInterval(resolved.Intervals, totalContextTokens)
	if iv == nil {
		return resolved.BasePricing
	}

	return intervalToModelPricing(resolved.BasePricing, iv, resolved.SupportsCacheBreakdown, resolved.channelPricing)
}

// intervalToModelPricing 将区间定价覆盖到基础定价。区间未配置的字段继承基础价，
// 指向零值的字段仍视为显式覆盖。
func intervalToModelPricing(base *ModelPricing, iv *PricingInterval, supportsCacheBreakdown bool, chPricing *ChannelModelPricing) *ModelPricing {
	pricing := &ModelPricing{}
	if base != nil {
		*pricing = *base
	}
	pricing.SupportsCacheBreakdown = supportsCacheBreakdown
	if iv.InputPrice != nil {
		pricing.InputPricePerToken = *iv.InputPrice
		pricing.InputPricePerTokenPriority = *iv.InputPrice
	}
	if iv.OutputPrice != nil {
		pricing.OutputPricePerToken = *iv.OutputPrice
		pricing.OutputPricePerTokenPriority = *iv.OutputPrice
	}
	if iv.CacheWritePrice != nil {
		pricing.CacheCreationPricePerToken = *iv.CacheWritePrice
		pricing.CacheCreationPricePerTokenPriority = *iv.CacheWritePrice
		pricing.CacheCreationPriceExplicit = true
		pricing.CacheCreation5mPrice = *iv.CacheWritePrice
		pricing.CacheCreation1hPrice = *iv.CacheWritePrice
	}
	if iv.CacheReadPrice != nil {
		pricing.CacheReadPricePerToken = *iv.CacheReadPrice
		pricing.CacheReadPricePerTokenPriority = *iv.CacheReadPrice
	}
	// 渠道定价存在时，ImageOutputPrice 显式覆盖；图片输入价用渠道级配置
	// （区间不携带图片输入价，与 image_output 一致）。
	if chPricing != nil {
		pricing.ImageOutputPriceExplicit = true
		if chPricing.ImageOutputPrice != nil {
			pricing.ImageOutputPricePerToken = *chPricing.ImageOutputPrice
		}
		applyChannelImageInputPrice(chPricing, pricing)
	}
	return pricing
}

// LookupRequestTierPrice 根据层级标签获取按次价格，并区分显式零价和未命中。
func (r *ModelPricingResolver) LookupRequestTierPrice(resolved *ResolvedPricing, tierLabel string) (float64, bool) {
	if resolved == nil {
		return 0, false
	}
	for _, tier := range resolved.RequestTiers {
		if tier.TierLabel == tierLabel && tier.PerRequestPrice != nil {
			return *tier.PerRequestPrice, true
		}
	}
	return 0, false
}

// GetRequestTierPrice 保留只返回价格的兼容接口；未命中时返回 0。
func (r *ModelPricingResolver) GetRequestTierPrice(resolved *ResolvedPricing, tierLabel string) float64 {
	price, _ := r.LookupRequestTierPrice(resolved, tierLabel)
	return price
}

// LookupRequestTierPriceByContext 根据 context token 数获取按次价格，并区分显式零价和未命中。
func (r *ModelPricingResolver) LookupRequestTierPriceByContext(resolved *ResolvedPricing, totalContextTokens int) (float64, bool) {
	if resolved == nil {
		return 0, false
	}
	iv := FindMatchingInterval(resolved.RequestTiers, totalContextTokens)
	if iv != nil && iv.PerRequestPrice != nil {
		return *iv.PerRequestPrice, true
	}
	return 0, false
}

// GetRequestTierPriceByContext 保留只返回价格的兼容接口；未命中时返回 0。
func (r *ModelPricingResolver) GetRequestTierPriceByContext(resolved *ResolvedPricing, totalContextTokens int) float64 {
	price, _ := r.LookupRequestTierPriceByContext(resolved, totalContextTokens)
	return price
}
