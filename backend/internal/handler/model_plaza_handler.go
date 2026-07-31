package handler

import (
	"log/slog"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelPlazaHandler 处理「模型广场」查询。
//
// 广场路由挂 OptionalJWT 中间件：匿名可访问（除非 require_auth 开启），带 token 则
// 识别用户。可见性规则（橱窗语义，与「可用渠道」的可绑定语义不同）：
//   - 匿名：仅非专属分组（订阅型照常展示）；
//   - 登录：非专属分组 + user_allowed_groups 授权的专属分组（不检查订阅有效性）。
type ModelPlazaHandler struct {
	channelService *service.ChannelService
	apiKeyService  *service.APIKeyService
	settingService *service.SettingService
}

// NewModelPlazaHandler 创建模型广场 handler。
func NewModelPlazaHandler(
	channelService *service.ChannelService,
	apiKeyService *service.APIKeyService,
	settingService *service.SettingService,
) *ModelPlazaHandler {
	return &ModelPlazaHandler{
		channelService: channelService,
		apiKeyService:  apiKeyService,
		settingService: settingService,
	}
}

// modelPlazaOfficialPricing LiteLLM 官方参考价（USD per token）。
type modelPlazaOfficialPricing struct {
	InputPrice        *float64 `json:"input_price"`
	OutputPrice       *float64 `json:"output_price"`
	CacheWritePrice   *float64 `json:"cache_write_price"`
	CacheWrite1hPrice *float64 `json:"cache_write_1h_price,omitempty"`
	CacheReadPrice    *float64 `json:"cache_read_price"`
}

// modelPlazaModel 广场模型条目：渠道定价（白名单形态）+ 官方参考价。
type modelPlazaModel struct {
	Name                string                     `json:"name"`
	Platform            string                     `json:"platform"`
	Pricing             *userSupportedModelPricing `json:"pricing"`
	DisplayPricing      *userSupportedModelPricing `json:"display_pricing"`
	EffectiveMultiplier float64                    `json:"effective_multiplier"`
	OfficialPricing     *modelPlazaOfficialPricing `json:"official_pricing"`
}

// modelPlazaGroup 广场分组条目（白名单字段）。
type modelPlazaGroup struct {
	ID                           int64             `json:"id"`
	Name                         string            `json:"name"`
	Description                  string            `json:"description"`
	Platform                     string            `json:"platform"`
	SubscriptionType             string            `json:"subscription_type"`
	RateMultiplier               float64           `json:"rate_multiplier"`
	UserRateMultiplier           *float64          `json:"user_rate_multiplier,omitempty"`
	PeakRateEnabled              bool              `json:"peak_rate_enabled"`
	PeakStart                    string            `json:"peak_start"`
	PeakEnd                      string            `json:"peak_end"`
	PeakRateMultiplier           float64           `json:"peak_rate_multiplier"`
	AppliedPeakMultiplier        float64           `json:"applied_peak_multiplier"`
	EffectiveRateMultiplier      float64           `json:"effective_rate_multiplier"`
	EffectiveImageRateMultiplier float64           `json:"effective_image_rate_multiplier"`
	IsFree                       bool              `json:"is_free"`
	IsExclusive                  bool              `json:"is_exclusive"`
	Models                       []modelPlazaModel `json:"models"`
}

// modelPlazaResponse 广场页响应。
type modelPlazaResponse struct {
	Description              string            `json:"description"`
	Currency                 string            `json:"currency"`
	OfficialPricingSource    string            `json:"official_pricing_source"`
	OfficialPricingUpdatedAt string            `json:"official_pricing_updated_at,omitempty"`
	Groups                   []modelPlazaGroup `json:"groups"`
}

// Get 返回模型广场数据。
// GET /api/v1/model-plaza
func (h *ModelPlazaHandler) Get(c *gin.Context) {
	if h.settingService == nil {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	if !rt.Enabled {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}

	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if rt.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return
	}

	groups, err := h.channelService.ListPlazaGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// allowedExclusive == nil 表示匿名；登录用户恒为非 nil（可能为空集合）。
	var allowedExclusive map[int64]struct{}
	var userRates map[int64]float64
	if authed {
		allowedExclusive, err = h.apiKeyService.GetUserAllowedGroupIDSet(c.Request.Context(), subject.UserID)
		if err != nil {
			// 可见性数据拿不到时不能静默降级成匿名视图（会错漏专属分组），直接报错。
			response.ErrorFrom(c, err)
			return
		}
		userRates, err = h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
		if err != nil {
			// 专属倍率仅是展示增强，失败降级为分组默认倍率。
			slog.Warn("model_plaza_user_rates_failed", "error", err, "user_id", subject.UserID)
			userRates = nil
		}
	}

	visible := filterPlazaVisibleGroups(groups, allowedExclusive)

	out := make([]modelPlazaGroup, 0, len(visible))
	for i := range visible {
		out = append(out, toModelPlazaGroupDTO(&visible[i], userRates))
	}
	officialUpdatedAt := ""
	if updatedAt := h.channelService.PlazaOfficialPricingUpdatedAt(); !updatedAt.IsZero() {
		officialUpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	}
	response.Success(c, modelPlazaResponse{
		Description:              rt.Description,
		Currency:                 "CNY",
		OfficialPricingSource:    "LiteLLM",
		OfficialPricingUpdatedAt: officialUpdatedAt,
		Groups:                   out,
	})
}

// filterPlazaVisibleGroups 按登录态裁剪分组可见性。
// allowedExclusive == nil 表示匿名（仅非专属）；非 nil 表示登录（非专属 + 授权专属）。
func filterPlazaVisibleGroups(
	groups []service.PlazaGroup,
	allowedExclusive map[int64]struct{},
) []service.PlazaGroup {
	visible := make([]service.PlazaGroup, 0, len(groups))
	for _, g := range groups {
		if g.IsExclusive {
			if allowedExclusive == nil {
				continue
			}
			if _, ok := allowedExclusive[g.ID]; !ok {
				continue
			}
		}
		visible = append(visible, g)
	}
	return visible
}

// toModelPlazaGroupDTO 将 service 层广场分组映射为白名单 DTO,并合并用户专属倍率。
func toModelPlazaGroupDTO(g *service.PlazaGroup, userRates map[int64]float64) modelPlazaGroup {
	return toModelPlazaGroupDTOAt(g, userRates, time.Now())
}

func toModelPlazaGroupDTOAt(g *service.PlazaGroup, userRates map[int64]float64, now time.Time) modelPlazaGroup {
	baseMultiplier := g.RateMultiplier
	if rate, ok := userRates[g.ID]; ok {
		baseMultiplier = rate
	}
	baseMultiplier = validDisplayMultiplier(baseMultiplier)
	if g.IsFree {
		baseMultiplier = 0
	}
	appliedPeakMultiplier := g.PeakMultiplierAt(now)
	tokenMultiplier := baseMultiplier * appliedPeakMultiplier
	imageMultiplier := baseMultiplier
	if !g.IsFree && g.ImageRateIndependent {
		imageMultiplier = validDisplayMultiplier(g.ImageRateMultiplier)
	}

	models := make([]modelPlazaModel, 0, len(g.Models))
	for i := range g.Models {
		m := &g.Models[i]
		effectiveMultiplier := baseMultiplier
		if m.Pricing != nil {
			switch m.Pricing.BillingMode {
			case service.BillingModeToken, "":
				effectiveMultiplier = tokenMultiplier
			case service.BillingModeImage:
				effectiveMultiplier = imageMultiplier
			}
		}
		models = append(models, modelPlazaModel{
			Name:                m.Name,
			Platform:            m.Platform,
			Pricing:             toUserPricing(m.Pricing),
			DisplayPricing:      toDisplayPricing(m.Pricing, effectiveMultiplier),
			EffectiveMultiplier: effectiveMultiplier,
			OfficialPricing:     toModelPlazaOfficialPricing(m.OfficialPricing),
		})
	}
	dto := modelPlazaGroup{
		ID:                           g.ID,
		Name:                         g.Name,
		Description:                  g.Description,
		Platform:                     g.Platform,
		SubscriptionType:             g.SubscriptionType,
		RateMultiplier:               g.RateMultiplier,
		PeakRateEnabled:              g.PeakRateEnabled,
		PeakStart:                    g.PeakStart,
		PeakEnd:                      g.PeakEnd,
		PeakRateMultiplier:           g.PeakRateMultiplier,
		AppliedPeakMultiplier:        appliedPeakMultiplier,
		EffectiveRateMultiplier:      baseMultiplier,
		EffectiveImageRateMultiplier: imageMultiplier,
		IsFree:                       g.IsFree,
		IsExclusive:                  g.IsExclusive,
		Models:                       models,
	}
	if rate, ok := userRates[g.ID]; ok {
		dto.UserRateMultiplier = &rate
	}
	return dto
}

func validDisplayMultiplier(value float64) float64 {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func toDisplayPricing(p *service.ChannelModelPricing, multiplier float64) *userSupportedModelPricing {
	pricing := toUserPricing(p)
	if pricing == nil {
		return nil
	}
	pricing.InputPrice = scaledPrice(pricing.InputPrice, multiplier)
	pricing.OutputPrice = scaledPrice(pricing.OutputPrice, multiplier)
	pricing.CacheWritePrice = scaledPrice(pricing.CacheWritePrice, multiplier)
	pricing.CacheReadPrice = scaledPrice(pricing.CacheReadPrice, multiplier)
	pricing.ImageInputPrice = scaledPrice(pricing.ImageInputPrice, multiplier)
	pricing.ImageOutputPrice = scaledPrice(pricing.ImageOutputPrice, multiplier)
	pricing.PerRequestPrice = scaledPrice(pricing.PerRequestPrice, multiplier)
	for i := range pricing.Intervals {
		interval := &pricing.Intervals[i]
		interval.InputPrice = scaledPrice(interval.InputPrice, multiplier)
		interval.OutputPrice = scaledPrice(interval.OutputPrice, multiplier)
		interval.CacheWritePrice = scaledPrice(interval.CacheWritePrice, multiplier)
		interval.CacheReadPrice = scaledPrice(interval.CacheReadPrice, multiplier)
		interval.PerRequestPrice = scaledPrice(interval.PerRequestPrice, multiplier)
	}
	return pricing
}

func scaledPrice(value *float64, multiplier float64) *float64 {
	if value == nil {
		return nil
	}
	scaled := *value * multiplier
	return &scaled
}

// toModelPlazaOfficialPricing 转换官方参考价；nil 透传（前端显示 "-"）。
func toModelPlazaOfficialPricing(p *service.PlazaOfficialPricing) *modelPlazaOfficialPricing {
	if p == nil {
		return nil
	}
	return &modelPlazaOfficialPricing{
		InputPrice:        p.InputPrice,
		OutputPrice:       p.OutputPrice,
		CacheWritePrice:   p.CacheWritePrice,
		CacheWrite1hPrice: p.CacheWrite1hPrice,
		CacheReadPrice:    p.CacheReadPrice,
	}
}
