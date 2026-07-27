package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type modelCatalogRepositoryStub struct {
	items   []ModelCatalogMetadata
	saved   *ModelCatalogMetadata
	deleted int64
}

func (repository *modelCatalogRepositoryStub) List(context.Context) ([]ModelCatalogMetadata, error) {
	return repository.items, nil
}

func (repository *modelCatalogRepositoryStub) Upsert(_ context.Context, metadata *ModelCatalogMetadata) error {
	copyValue := *metadata
	repository.saved = &copyValue
	return nil
}

func (repository *modelCatalogRepositoryStub) Delete(_ context.Context, id int64) error {
	repository.deleted = id
	return nil
}

type modelCatalogChannelSourceStub struct {
	channels []AvailableChannel
}

func (source *modelCatalogChannelSourceStub) ListAvailable(context.Context) ([]AvailableChannel, error) {
	return source.channels, nil
}

type modelCatalogGroupSourceStub struct {
	groups []Group
	rates  map[int64]float64
}

func (source *modelCatalogGroupSourceStub) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return source.groups, nil
}

func (source *modelCatalogGroupSourceStub) GetUserGroupRates(context.Context, int64) (map[int64]float64, error) {
	return source.rates, nil
}

func TestModelCatalogListForUserEnforcesAvailabilityVisibilityAndGroups(t *testing.T) {
	price := 0.000001
	repository := &modelCatalogRepositoryStub{items: []ModelCatalogMetadata{
		{ID: 1, Platform: PlatformAnthropic, ModelName: "claude-sonnet", IsVisible: false},
		{ID: 2, Platform: PlatformOpenAI, ModelName: "gpt-5", DisplayName: "GPT 5", IsVisible: true, IsRecommended: true},
	}}
	channels := &modelCatalogChannelSourceStub{channels: []AvailableChannel{
		{
			ID: 10, Name: "primary", Status: StatusActive,
			Groups: []AvailableGroupRef{
				{ID: 1, Name: "Claude", Platform: PlatformAnthropic},
				{ID: 2, Name: "Universal", Platform: PlatformComposite, IsFree: true},
			},
			SupportedModels: []SupportedModel{
				{Name: "claude-sonnet", Platform: PlatformAnthropic},
				{Name: "gpt-5", Platform: PlatformOpenAI, Pricing: &ChannelModelPricing{InputPrice: &price, UserVisible: true}},
				{Name: "gpt-hidden", Platform: PlatformOpenAI, Pricing: &ChannelModelPricing{InputPrice: &price, UserVisible: false}},
			},
		},
		{
			ID: 13, Name: "private-qwen", Status: StatusActive,
			Groups:          []AvailableGroupRef{{ID: 3, Name: "Private", Platform: PlatformQwen}},
			SupportedModels: []SupportedModel{{Name: "qwen-max", Platform: PlatformQwen}},
		},
		{
			ID: 11, Name: "disabled", Status: StatusDisabled,
			Groups:          []AvailableGroupRef{{ID: 2, Name: "Universal", Platform: PlatformComposite}},
			SupportedModels: []SupportedModel{{Name: "gpt-disabled", Platform: PlatformOpenAI}},
		},
	}}
	groups := &modelCatalogGroupSourceStub{groups: []Group{
		{ID: 1, Platform: PlatformAnthropic},
		{ID: 2, Platform: PlatformComposite},
	}, rates: map[int64]float64{2: 0.75}}
	modelCatalog := &ModelCatalogService{repo: repository, channels: channels, groups: groups}

	items, err := modelCatalog.ListForUser(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "gpt-5", items[0].Name)
	require.Equal(t, "GPT 5", items[0].DisplayName)
	require.True(t, items[0].IsRecommended)
	require.Equal(t, []string{"text"}, items[0].Capabilities)
	require.Equal(t, []string{"openai", "anthropic"}, items[0].InterfaceFormats)
	require.Equal(t, []string{"chat"}, items[0].Scenarios)
	require.Len(t, items[0].Offers, 1)
	require.Equal(t, int64(2), items[0].Offers[0].Groups[0].ID)
	require.True(t, items[0].Offers[0].Groups[0].IsFree)
	require.Zero(t, items[0].Offers[0].Groups[0].RateMultiplier)
	require.Equal(t, price, *items[0].Offers[0].Pricing.InputPrice)
}

func TestModelCatalogListForAdminIncludesUnavailableMetadata(t *testing.T) {
	repository := &modelCatalogRepositoryStub{items: []ModelCatalogMetadata{
		{ID: 8, Platform: PlatformMiMo, ModelName: "mimo-orphan", DisplayName: "MiMo Orphan", IsVisible: false},
	}}
	channels := &modelCatalogChannelSourceStub{channels: []AvailableChannel{
		{
			ID: 12, Name: "disabled", Status: StatusDisabled,
			SupportedModels: []SupportedModel{{Name: "gpt-disabled", Platform: PlatformOpenAI}},
		},
	}}
	modelCatalog := &ModelCatalogService{repo: repository, channels: channels, groups: &modelCatalogGroupSourceStub{}}

	items, err := modelCatalog.ListForAdmin(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	itemsByName := make(map[string]ModelCatalogItem, len(items))
	for _, item := range items {
		itemsByName[item.Name] = item
	}
	require.True(t, itemsByName["gpt-disabled"].Available)
	require.Len(t, itemsByName["gpt-disabled"].Offers, 1)
	require.Empty(t, itemsByName["gpt-disabled"].Offers[0].Groups)
	require.False(t, itemsByName["mimo-orphan"].Available)
	require.False(t, itemsByName["mimo-orphan"].IsVisible)
}

func TestModelCatalogUpsertNormalizesMetadata(t *testing.T) {
	repository := &modelCatalogRepositoryStub{}
	modelCatalog := &ModelCatalogService{repo: repository}
	longExample := strings.Repeat("x", 12010)
	metadata := &ModelCatalogMetadata{
		Platform:         " OPENAI ",
		ModelName:        "  gpt-5  ",
		DisplayName:      " GPT 5 ",
		Description:      " Primary model ",
		Capabilities:     []string{"TEXT", "text", " reasoning "},
		InterfaceFormats: []string{"OPENAI", "unsupported", "anthropic"},
		Scenarios:        []string{"CHAT", "chat"},
		ExampleOverrides: map[string]string{"openai": longExample, "unsupported": "ignored"},
	}

	require.NoError(t, modelCatalog.UpsertMetadata(context.Background(), metadata))
	require.NotNil(t, repository.saved)
	require.Equal(t, PlatformOpenAI, repository.saved.Platform)
	require.Equal(t, "gpt-5", repository.saved.ModelName)
	require.Equal(t, "GPT 5", repository.saved.DisplayName)
	require.Equal(t, []string{"text", "reasoning"}, repository.saved.Capabilities)
	require.Equal(t, []string{"openai", "anthropic"}, repository.saved.InterfaceFormats)
	require.Equal(t, []string{"chat"}, repository.saved.Scenarios)
	require.Len(t, repository.saved.ExampleOverrides["openai"], 12000)
	require.NotContains(t, repository.saved.ExampleOverrides, "unsupported")

	metadata.Platform = "unknown-provider"
	require.ErrorIs(t, modelCatalog.UpsertMetadata(context.Background(), metadata), ErrModelCatalogInvalid)
}

func TestModelCatalogDeleteRejectsInvalidID(t *testing.T) {
	modelCatalog := &ModelCatalogService{repo: &modelCatalogRepositoryStub{}}
	require.ErrorIs(t, modelCatalog.DeleteMetadata(context.Background(), 0), ErrModelCatalogMetadataNotFound)
}
