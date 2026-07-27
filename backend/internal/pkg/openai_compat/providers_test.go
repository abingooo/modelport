package openai_compat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderRegistryCoversDedicatedOpenAICompatiblePlatforms(t *testing.T) {
	require.Equal(t, []string{
		"deepseek", "qwen", "glm", "kimi", "doubao", "minimax", "mimo",
	}, ProviderIDs())

	for _, id := range ProviderIDs() {
		preset, ok := LookupProvider(id)
		require.True(t, ok, id)
		require.Equal(t, id, preset.ID)
		require.NotEmpty(t, preset.DisplayName)
		require.NotEmpty(t, preset.DefaultBaseURL)
		require.True(t, preset.Capabilities.ChatCompletions)
		require.True(t, preset.Capabilities.DownstreamResponses)
		require.True(t, preset.Capabilities.DownstreamMessages)
		require.True(t, preset.Capabilities.Streaming)
		require.True(t, preset.Capabilities.Usage)
		require.False(t, preset.Capabilities.UpstreamResponses)
		require.False(t, preset.Capabilities.ResponsesWebSocket)
	}
}

func TestProviderRegistryReturnsDefensiveCopies(t *testing.T) {
	preset, ok := LookupProvider("qwen")
	require.True(t, ok)
	preset.DefaultModelIDs[0] = "changed"
	preset.RequestParameterRules["stream_options"] = ParameterReject
	preset.RequestIDHeaders[0] = "changed"
	minimax, ok := LookupProvider("minimax")
	require.True(t, ok)
	minimax.AllowedIntegerParameters["n"][0] = 2

	again, ok := LookupProvider("qwen")
	require.True(t, ok)
	require.Equal(t, "qwen3.8-max-preview", again.DefaultModelIDs[0])
	require.Equal(t, ParameterPass, again.RequestParameterRules["stream_options"])
	require.Equal(t, "x-request-id", again.RequestIDHeaders[0])
	minimaxAgain, ok := LookupProvider("minimax")
	require.True(t, ok)
	require.Equal(t, []int{1}, minimaxAgain.AllowedIntegerParameters["n"])
}

func TestProviderSpecificContracts(t *testing.T) {
	doubao, ok := LookupProvider("doubao")
	require.True(t, ok)
	require.Equal(t, "endpoint_or_model", doubao.ModelReferenceMode)
	require.Equal(t, "volcengine-ark", doubao.ProtocolAdapter)
	require.False(t, doubao.Capabilities.ModelList)
	require.Empty(t, doubao.DefaultTestModel)

	minimax, ok := LookupProvider("minimax")
	require.True(t, ok)
	require.Equal(t, ParameterDrop, minimax.RequestParameterRules["presence_penalty"])
	require.Equal(t, ParameterDrop, minimax.RequestParameterRules["frequency_penalty"])
	require.Equal(t, ParameterDrop, minimax.RequestParameterRules["logit_bias"])
	require.Equal(t, []int{1}, minimax.AllowedIntegerParameters["n"])
	require.Contains(t, minimax.RequestIDHeaders, "trace-id")
}
