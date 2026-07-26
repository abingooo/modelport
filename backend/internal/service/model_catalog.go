package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrModelCatalogMetadataNotFound = infraerrors.NotFound("MODEL_CATALOG_METADATA_NOT_FOUND", "model catalog metadata not found")
	ErrModelCatalogInvalid          = infraerrors.BadRequest("MODEL_CATALOG_INVALID", "invalid model catalog metadata")
)

type ModelCatalogMetadata struct {
	ID               int64             `json:"id"`
	Platform         string            `json:"platform"`
	ModelName        string            `json:"model_name"`
	DisplayName      string            `json:"display_name"`
	Description      string            `json:"description"`
	Capabilities     []string          `json:"capabilities"`
	ContextWindow    int64             `json:"context_window"`
	InterfaceFormats []string          `json:"interface_formats"`
	Scenarios        []string          `json:"scenarios"`
	ExampleOverrides map[string]string `json:"example_overrides"`
	IsRecommended    bool              `json:"is_recommended"`
	IsVisible        bool              `json:"is_visible"`
	SortOrder        int               `json:"sort_order"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type ModelCatalogRepository interface {
	List(ctx context.Context) ([]ModelCatalogMetadata, error)
	Upsert(ctx context.Context, metadata *ModelCatalogMetadata) error
	Delete(ctx context.Context, id int64) error
}

type ModelCatalogGroup struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	RateMultiplier     float64 `json:"rate_multiplier"`
	IsFree             bool    `json:"is_free"`
	PeakRateEnabled    bool    `json:"peak_rate_enabled"`
	PeakRateMultiplier float64 `json:"peak_rate_multiplier"`
	SubscriptionType   string  `json:"subscription_type"`
	IsExclusive        bool    `json:"is_exclusive"`
}

type ModelCatalogPricingInterval struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label,omitempty"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
}

type ModelCatalogPricing struct {
	BillingMode      string                        `json:"billing_mode"`
	InputPrice       *float64                      `json:"input_price"`
	OutputPrice      *float64                      `json:"output_price"`
	CacheWritePrice  *float64                      `json:"cache_write_price"`
	CacheReadPrice   *float64                      `json:"cache_read_price"`
	ImageInputPrice  *float64                      `json:"image_input_price"`
	ImageOutputPrice *float64                      `json:"image_output_price"`
	PerRequestPrice  *float64                      `json:"per_request_price"`
	Intervals        []ModelCatalogPricingInterval `json:"intervals"`
}

type ModelCatalogOffer struct {
	ChannelID   int64                `json:"channel_id"`
	ChannelName string               `json:"channel_name"`
	Groups      []ModelCatalogGroup  `json:"groups"`
	Pricing     *ModelCatalogPricing `json:"pricing"`
}

type ModelCatalogItem struct {
	MetadataID       int64               `json:"metadata_id"`
	Platform         string              `json:"platform"`
	Name             string              `json:"name"`
	DisplayName      string              `json:"display_name"`
	Description      string              `json:"description"`
	Capabilities     []string            `json:"capabilities"`
	ContextWindow    int64               `json:"context_window"`
	InterfaceFormats []string            `json:"interface_formats"`
	Scenarios        []string            `json:"scenarios"`
	ExampleOverrides map[string]string   `json:"example_overrides"`
	IsRecommended    bool                `json:"is_recommended"`
	IsVisible        bool                `json:"is_visible"`
	SortOrder        int                 `json:"sort_order"`
	Available        bool                `json:"available"`
	Offers           []ModelCatalogOffer `json:"offers"`
}

type modelCatalogChannelSource interface {
	ListAvailable(ctx context.Context) ([]AvailableChannel, error)
}

type modelCatalogGroupSource interface {
	GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error)
	GetUserGroupRates(ctx context.Context, userID int64) (map[int64]float64, error)
}

type ModelCatalogService struct {
	repo     ModelCatalogRepository
	channels modelCatalogChannelSource
	groups   modelCatalogGroupSource
}

func NewModelCatalogService(repo ModelCatalogRepository, channels *ChannelService, groups *APIKeyService) *ModelCatalogService {
	return &ModelCatalogService{repo: repo, channels: channels, groups: groups}
}

func (s *ModelCatalogService) ListForUser(ctx context.Context, userID int64) ([]ModelCatalogItem, error) {
	groups, err := s.groups.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user model catalog groups: %w", err)
	}
	allowed := make(map[int64]struct{}, len(groups))
	for i := range groups {
		allowed[groups[i].ID] = struct{}{}
	}
	userRates, err := s.groups.GetUserGroupRates(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user model catalog group rates: %w", err)
	}

	items, err := s.build(ctx, allowed, userRates, true)
	if err != nil {
		return nil, err
	}
	visible := items[:0]
	for i := range items {
		if items[i].Available && items[i].IsVisible {
			visible = append(visible, items[i])
		}
	}
	return visible, nil
}

func (s *ModelCatalogService) ListForAdmin(ctx context.Context) ([]ModelCatalogItem, error) {
	return s.build(ctx, nil, nil, false)
}

func (s *ModelCatalogService) UpsertMetadata(ctx context.Context, metadata *ModelCatalogMetadata) error {
	if metadata == nil {
		return ErrModelCatalogInvalid
	}
	metadata.Platform = strings.ToLower(strings.TrimSpace(metadata.Platform))
	metadata.ModelName = strings.TrimSpace(metadata.ModelName)
	metadata.DisplayName = strings.TrimSpace(metadata.DisplayName)
	metadata.Description = strings.TrimSpace(metadata.Description)
	if !isModelCatalogPlatform(metadata.Platform) || metadata.ModelName == "" || len(metadata.ModelName) > 255 ||
		len(metadata.DisplayName) > 255 || len(metadata.Description) > 8000 || metadata.ContextWindow < 0 {
		return ErrModelCatalogInvalid
	}
	metadata.Capabilities = normalizeCatalogStrings(metadata.Capabilities, 16, 40)
	metadata.InterfaceFormats = normalizeCatalogInterfaces(metadata.InterfaceFormats)
	metadata.Scenarios = normalizeCatalogStrings(metadata.Scenarios, 16, 40)
	metadata.ExampleOverrides = normalizeCatalogExamples(metadata.ExampleOverrides)
	return s.repo.Upsert(ctx, metadata)
}

func (s *ModelCatalogService) DeleteMetadata(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrModelCatalogMetadataNotFound
	}
	return s.repo.Delete(ctx, id)
}

func (s *ModelCatalogService) build(ctx context.Context, allowedGroupIDs map[int64]struct{}, userRates map[int64]float64, activeOnly bool) ([]ModelCatalogItem, error) {
	metadata, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model catalog metadata: %w", err)
	}
	metadataByKey := make(map[string]ModelCatalogMetadata, len(metadata))
	for i := range metadata {
		metadataByKey[modelCatalogKey(metadata[i].Platform, metadata[i].ModelName)] = metadata[i]
	}

	channels, err := s.channels.ListAvailable(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model catalog channels: %w", err)
	}
	items := make(map[string]*ModelCatalogItem)
	for i := range channels {
		channel := channels[i]
		if activeOnly && channel.Status != StatusActive {
			continue
		}
		for j := range channel.SupportedModels {
			model := channel.SupportedModels[j]
			if activeOnly && model.Pricing != nil && !model.Pricing.UserVisible {
				continue
			}
			visibleGroups := catalogGroups(channel.Groups, model.Platform, allowedGroupIDs, userRates)
			if activeOnly && len(visibleGroups) == 0 {
				continue
			}
			key := modelCatalogKey(model.Platform, model.Name)
			item := items[key]
			if item == nil {
				itemValue := catalogItemFromMetadata(model.Platform, model.Name, metadataByKey[key])
				item = &itemValue
				items[key] = item
			}
			item.Available = true
			item.Offers = append(item.Offers, ModelCatalogOffer{
				ChannelID: channel.ID, ChannelName: channel.Name, Groups: visibleGroups, Pricing: toModelCatalogPricing(model.Pricing),
			})
		}
	}

	if !activeOnly {
		for key, meta := range metadataByKey {
			if _, exists := items[key]; exists {
				continue
			}
			item := catalogItemFromMetadata(meta.Platform, meta.ModelName, meta)
			items[key] = &item
		}
	}

	result := make([]ModelCatalogItem, 0, len(items))
	for _, item := range items {
		result = append(result, *item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].IsRecommended != result[j].IsRecommended {
			return result[i].IsRecommended
		}
		if result[i].SortOrder != result[j].SortOrder {
			return result[i].SortOrder < result[j].SortOrder
		}
		if result[i].Platform != result[j].Platform {
			return result[i].Platform < result[j].Platform
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func modelCatalogKey(platform, model string) string {
	return strings.ToLower(strings.TrimSpace(platform)) + "\x00" + strings.ToLower(strings.TrimSpace(model))
}

func catalogGroups(groups []AvailableGroupRef, platform string, allowed map[int64]struct{}, userRates map[int64]float64) []ModelCatalogGroup {
	out := make([]ModelCatalogGroup, 0, len(groups))
	for i := range groups {
		group := groups[i]
		if group.Platform != platform && group.Platform != PlatformComposite {
			continue
		}
		if allowed != nil {
			if _, ok := allowed[group.ID]; !ok {
				continue
			}
		}
		rateMultiplier := group.RateMultiplier
		if userRate, ok := userRates[group.ID]; ok {
			rateMultiplier = userRate
		}
		if group.IsFree {
			rateMultiplier = 0
		}
		out = append(out, ModelCatalogGroup{
			ID: group.ID, Name: group.Name, RateMultiplier: rateMultiplier, IsFree: group.IsFree,
			PeakRateEnabled: group.PeakRateEnabled, PeakRateMultiplier: group.PeakRateMultiplier,
			SubscriptionType: group.SubscriptionType, IsExclusive: group.IsExclusive,
		})
	}
	return out
}

func isModelCatalogPlatform(platform string) bool {
	if platform == PlatformComposite {
		return true
	}
	for _, supported := range ConcretePlatforms {
		if platform == supported {
			return true
		}
	}
	return false
}

func catalogItemFromMetadata(platform, model string, metadata ModelCatalogMetadata) ModelCatalogItem {
	capabilities := metadata.Capabilities
	if len(capabilities) == 0 {
		capabilities = inferModelCapabilities(model)
	}
	formats := metadata.InterfaceFormats
	if len(formats) == 0 {
		formats = inferModelInterfaces(platform, capabilities)
	}
	scenarios := metadata.Scenarios
	if len(scenarios) == 0 {
		scenarios = inferModelScenarios(capabilities)
	}
	displayName := metadata.DisplayName
	if displayName == "" {
		displayName = model
	}
	visible := true
	if metadata.ID > 0 {
		visible = metadata.IsVisible
	}
	return ModelCatalogItem{
		MetadataID: metadata.ID, Platform: platform, Name: model, DisplayName: displayName,
		Description: metadata.Description, Capabilities: capabilities, ContextWindow: metadata.ContextWindow,
		InterfaceFormats: formats, Scenarios: scenarios, ExampleOverrides: metadata.ExampleOverrides,
		IsRecommended: metadata.IsRecommended, IsVisible: visible, SortOrder: metadata.SortOrder,
		Offers: make([]ModelCatalogOffer, 0),
	}
}

func inferModelCapabilities(model string) []string {
	name := strings.ToLower(model)
	switch {
	case strings.Contains(name, "embed"):
		return []string{"embedding"}
	case strings.Contains(name, "video") || strings.Contains(name, "veo"):
		return []string{"video"}
	case strings.Contains(name, "image") || strings.Contains(name, "dall-e"):
		return []string{"image"}
	default:
		return []string{"text"}
	}
}

func inferModelInterfaces(platform string, capabilities []string) []string {
	formats := []string{"openai"}
	if len(capabilities) == 0 || capabilities[0] == "text" {
		formats = append(formats, "anthropic")
	}
	if platform == PlatformGemini || platform == PlatformAntigravity || platform == PlatformComposite {
		formats = append(formats, "google")
	}
	return formats
}

func inferModelScenarios(capabilities []string) []string {
	for _, capability := range capabilities {
		switch capability {
		case "image", "video", "embedding":
			return []string{capability}
		}
	}
	return []string{"chat"}
}

func normalizeCatalogStrings(values []string, maxItems, maxLength int) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len(value) > maxLength {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) == maxItems {
			break
		}
	}
	return out
}

func normalizeCatalogInterfaces(values []string) []string {
	allowed := map[string]struct{}{"openai": {}, "anthropic": {}, "google": {}}
	values = normalizeCatalogStrings(values, 3, 20)
	out := values[:0]
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func normalizeCatalogExamples(examples map[string]string) map[string]string {
	if len(examples) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, 3)
	for _, format := range []string{"openai", "anthropic", "google"} {
		value := strings.TrimSpace(examples[format])
		if value == "" {
			continue
		}
		if len(value) > 12000 {
			value = value[:12000]
		}
		out[format] = value
	}
	return out
}

func toModelCatalogPricing(pricing *ChannelModelPricing) *ModelCatalogPricing {
	if pricing == nil {
		return nil
	}
	mode := string(pricing.BillingMode)
	if mode == "" {
		mode = string(BillingModeToken)
	}
	intervals := make([]ModelCatalogPricingInterval, 0, len(pricing.Intervals))
	for i := range pricing.Intervals {
		interval := pricing.Intervals[i]
		intervals = append(intervals, ModelCatalogPricingInterval{
			MinTokens: interval.MinTokens, MaxTokens: interval.MaxTokens, TierLabel: interval.TierLabel,
			InputPrice: interval.InputPrice, OutputPrice: interval.OutputPrice,
			CacheWritePrice: interval.CacheWritePrice, CacheReadPrice: interval.CacheReadPrice,
			PerRequestPrice: interval.PerRequestPrice,
		})
	}
	return &ModelCatalogPricing{
		BillingMode: mode, InputPrice: pricing.InputPrice, OutputPrice: pricing.OutputPrice,
		CacheWritePrice: pricing.CacheWritePrice, CacheReadPrice: pricing.CacheReadPrice,
		ImageInputPrice: pricing.ImageInputPrice, ImageOutputPrice: pricing.ImageOutputPrice,
		PerRequestPrice: pricing.PerRequestPrice, Intervals: intervals,
	}
}
