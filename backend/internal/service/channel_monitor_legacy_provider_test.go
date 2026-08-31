//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitorrequesttemplate"
	"github.com/stretchr/testify/require"
)

func TestLegacyModelPortMonitorProvidersRemainProbeCapable(t *testing.T) {
	tests := []struct {
		provider string
		path     string
	}{
		{provider: MonitorProviderQwen, path: providerQwenPath},
		{provider: MonitorProviderGLM, path: providerZhipuPath},
		{provider: MonitorProviderDoubao, path: providerDoubaoPath},
		{provider: MonitorProviderMiniMax, path: providerOpenAIPath},
		{provider: MonitorProviderMiMo, path: providerOpenAIPath},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			require.NoError(t, validateProvider(tt.provider))
			require.NoError(t, validateCheckMode(tt.provider, MonitorCheckModeProbe))
			require.ErrorIs(t, validateCheckMode(tt.provider, MonitorCheckModeQuota), ErrChannelMonitorInvalidCheckMode)
			require.ErrorIs(t, validateCheckMode(tt.provider, MonitorCheckModeQuotaProbe), ErrChannelMonitorInvalidCheckMode)
			require.ErrorIs(t, validateAPIMode(tt.provider, MonitorAPIModeResponses), ErrChannelMonitorInvalidAPIMode)

			adapter, apiMode, ok := providerAdapterFor(tt.provider, MonitorAPIModeChatCompletions)
			require.True(t, ok)
			require.Equal(t, MonitorAPIModeChatCompletions, apiMode)
			require.Equal(t, tt.path, adapter.buildPath("model-test"))

			require.NoError(t, channelmonitor.ProviderValidator(channelmonitor.Provider(tt.provider)))
			require.NoError(t, channelmonitorrequesttemplate.ProviderValidator(channelmonitorrequesttemplate.Provider(tt.provider)))
			require.NoError(t, validateTemplateCreateParams(ChannelMonitorRequestTemplateCreateParams{
				Name:     "legacy probe",
				Provider: tt.provider,
			}))

			handler := &openAICaptureHandler{}
			endpoint := setupFakeOpenAI(t, handler)
			result := runCheckForModelAgainstTestServer(context.Background(), tt.provider, endpoint, "legacy-key", "model-test", nil)
			require.Equal(t, MonitorStatusOperational, result.Status, result.Message)
			captured := handler.snapshot()
			require.Equal(t, tt.path, captured.path)
			require.Equal(t, "Bearer legacy-key", captured.headers.Get("Authorization"))
		})
	}
}

func TestRemovedLegacyModelPortProvidersDoNotGainMonitorRuntime(t *testing.T) {
	for _, provider := range []string{"siliconflow", "openrouter"} {
		t.Run(provider, func(t *testing.T) {
			require.ErrorIs(t, validateProvider(provider), ErrChannelMonitorInvalidProvider)
			require.ErrorIs(t, validateCheckMode(provider, MonitorCheckModeProbe), ErrChannelMonitorInvalidCheckMode)

			_, _, ok := providerAdapterFor(provider, MonitorAPIModeChatCompletions)
			require.False(t, ok)
			require.Error(t, channelmonitor.ProviderValidator(channelmonitor.Provider(provider)))
			require.Error(t, channelmonitorrequesttemplate.ProviderValidator(
				channelmonitorrequesttemplate.Provider(provider),
			))
			require.ErrorIs(t, validateTemplateCreateParams(ChannelMonitorRequestTemplateCreateParams{
				Name:     "removed legacy provider",
				Provider: provider,
			}), ErrChannelMonitorTemplateInvalidProvider)
		})
	}
}

func TestLegacyModelPortMonitorProviderTablesStayAligned(t *testing.T) {
	for provider := range probeCapableProviders {
		_, hasAdapter := providerAdapters[provider]
		require.True(t, hasAdapter, "probe provider %q has no runtime adapter", provider)
	}
	for provider := range providerAdapters {
		require.True(t, providerSupportsProbe(provider), "adapter provider %q is rejected by probe validation", provider)
	}
}
