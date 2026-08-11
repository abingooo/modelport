package service

import (
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokGatewayExplicitPricingGates(t *testing.T) {
	zero := 0.0
	positive := 1.25
	negative := -1.0
	nan := math.NaN()

	t.Run("search requires an explicit valid value", func(t *testing.T) {
		require.False(t, GroupHasExplicitGrokSearchPrice(nil))
		require.False(t, GroupHasExplicitGrokSearchPrice(&Group{}))
		require.False(t, GroupHasExplicitGrokSearchPrice(&Group{SearchPricePer1k: &negative}))
		require.False(t, GroupHasExplicitGrokSearchPrice(&Group{SearchPricePer1k: &nan}))
		require.True(t, GroupHasExplicitGrokSearchPrice(&Group{SearchPricePer1k: &zero}))
		require.True(t, GroupHasExplicitGrokSearchPrice(&Group{SearchPricePer1k: &positive}))
	})

	t.Run("voice modes are independently enabled", func(t *testing.T) {
		group := &Group{
			AudioRealtimePricePerMin:     &zero,
			AudioTTSPricePerMillionChars: &positive,
		}
		require.True(t, GroupHasExplicitGrokAudioPrice(group, "realtime"))
		require.True(t, GroupHasExplicitGrokAudioPrice(group, "TTS"))
		require.False(t, GroupHasExplicitGrokAudioPrice(group, "stt"))
		require.False(t, GroupHasExplicitGrokAudioPrice(group, "unknown"))
	})

	t.Run("custom voice requires both consuming modes", func(t *testing.T) {
		group := &Group{AudioTTSPricePerMillionChars: &zero}
		require.False(t, GroupHasExplicitGrokCustomVoicePricing(group))
		group.AudioRealtimePricePerMin = &positive
		require.True(t, GroupHasExplicitGrokCustomVoicePricing(group))
		group.AudioSTTPricePerHour = nil
		require.True(t, GroupHasExplicitGrokCustomVoicePricing(group), "STT does not consume custom voice profiles")
	})

	t.Run("video uses model price then flat resolution fallback", func(t *testing.T) {
		group := &Group{
			VideoPrice720P: &zero,
			VideoModelPrices: map[string]map[string]float64{
				VideoPriceFamilyGrokImagineVideo15: {VideoBillingResolution1080P: positive},
			},
		}
		require.True(t, GroupHasExplicitGrokVideoPrice(group, "grok-imagine-video", "720p"))
		require.True(t, GroupHasExplicitGrokVideoPrice(group, "grok-imagine-video-1.5-preview", "1080p"))
		require.False(t, GroupHasExplicitGrokVideoPrice(group, "grok-imagine-video", "480p"))
		require.True(t, GroupHasAnyExplicitGrokVideoPrice(group))
		require.False(t, GroupHasAnyExplicitGrokVideoPrice(&Group{
			VideoModelPrices: map[string]map[string]float64{"model": {"480p": negative}},
		}))
	})
}

func TestGrokSearchIntentDetection(t *testing.T) {
	t.Parallel()

	t.Run("responses", func(t *testing.T) {
		tests := []struct {
			name string
			body string
			want bool
		}{
			{name: "plain text", body: `{"model":"grok","input":"hello"}`},
			{name: "native web search", body: `{"tools":[{"type":"web_search"}]}`, want: true},
			{name: "native x search", body: `{"tools":[{"type":"x_search"}]}`, want: true},
			{name: "native tool search", body: `{"tools":[{"type":"tool_search"}]}`, want: true},
			{name: "function search", body: `{"tools":[{"type":"function","name":"web_search"}]}`, want: true},
			{name: "function tool search", body: `{"tools":[{"type":"function","name":"tool_search"}]}`, want: true},
			{name: "additional tools search", body: `{"input":[{"type":"additional_tools","tools":[{"type":"function","name":"x_search"}]}]}`, want: true},
			{name: "forced tool search", body: `{"tools":[{"type":"tool_search"}],"tool_choice":{"type":"tool_search"}}`, want: true},
			{name: "choice none", body: `{"tools":[{"type":"web_search"}],"tool_choice":"none"}`},
			{name: "forced non search", body: `{"tools":[{"type":"web_search"},{"type":"function","name":"lookup"}],"tool_choice":{"type":"function","name":"lookup"}}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				require.Equal(t, tt.want, GrokResponsesRequestHasSearchIntent([]byte(tt.body)))
			})
		}
	})

	t.Run("chat completions", func(t *testing.T) {
		tests := []struct {
			name string
			body string
			want bool
		}{
			{name: "plain text", body: `{"messages":[{"role":"user","content":"hello"}]}`},
			{name: "native search", body: `{"tools":[{"type":"web_search"}]}`, want: true},
			{name: "native tool search", body: `{"tools":[{"type":"tool_search"}]}`, want: true},
			{name: "function search", body: `{"tools":[{"type":"function","function":{"name":"web_search"}}]}`, want: true},
			{name: "function tool search", body: `{"tools":[{"type":"function","function":{"name":"tool_search"}}]}`, want: true},
			{name: "legacy function", body: `{"functions":[{"name":"x_search"}]}`, want: true},
			{name: "legacy function auto", body: `{"functions":[{"name":"web_search"}],"function_call":"auto"}`, want: true},
			{name: "legacy function none", body: `{"functions":[{"name":"web_search"}],"function_call":"none"}`},
			{name: "legacy forced search", body: `{"functions":[{"name":"web_search"},{"name":"lookup"}],"function_call":{"name":"web_search"}}`, want: true},
			{name: "legacy forced non search", body: `{"functions":[{"name":"web_search"},{"name":"lookup"}],"function_call":{"name":"lookup"}}`},
			{name: "forced tool search", body: `{"tools":[{"type":"tool_search"}],"tool_choice":{"type":"tool_search"}}`, want: true},
			{name: "choice none", body: `{"tools":[{"type":"web_search"}],"tool_choice":"none"}`},
			{name: "forced non search", body: `{"tools":[{"type":"web_search"},{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"function","function":{"name":"lookup"}}}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				require.Equal(t, tt.want, GrokChatRequestHasSearchIntent([]byte(tt.body)))
			})
		}
	})

	t.Run("messages", func(t *testing.T) {
		tests := []struct {
			name string
			body string
			want bool
		}{
			{name: "plain text", body: `{"messages":[{"role":"user","content":"hello"}]}`},
			{name: "server search", body: `{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`, want: true},
			{name: "tool search", body: `{"tools":[{"type":"tool_search"}]}`, want: true},
			{name: "function style", body: `{"tools":[{"name":"x_search","input_schema":{"type":"object"}}]}`, want: true},
			{name: "function tool search", body: `{"tools":[{"name":"tool_search","input_schema":{"type":"object"}}]}`, want: true},
			{name: "forced tool search", body: `{"tools":[{"type":"tool_search"}],"tool_choice":{"type":"tool_search"}}`, want: true},
			{name: "choice none", body: `{"tools":[{"type":"web_search_20250305","name":"web_search"}],"tool_choice":{"type":"none"}}`},
			{name: "forced non search", body: `{"tools":[{"type":"web_search_20250305","name":"web_search"},{"name":"lookup"}],"tool_choice":{"type":"tool","name":"lookup"}}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				require.Equal(t, tt.want, GrokMessagesRequestHasSearchIntent([]byte(tt.body)))
			})
		}
	})
}

func TestRequireExplicitGrokSearchPriceForResponsesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	searchBody := []byte(`{"tools":[{"type":"web_search"}],"tool_choice":"auto"}`)

	t.Run("missing price fails closed", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformGrok}})
		require.ErrorIs(t, requireExplicitGrokSearchPriceForResponsesBody(c, searchBody), errGrokSearchPriceUnavailable)
	})

	t.Run("explicit zero allows", func(t *testing.T) {
		zero := 0.0
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformGrok, SearchPricePer1k: &zero}})
		require.NoError(t, requireExplicitGrokSearchPriceForResponsesBody(c, searchBody))
	})

	t.Run("tool choice none allows", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformGrok}})
		require.NoError(t, requireExplicitGrokSearchPriceForResponsesBody(c, []byte(`{"tools":[{"type":"web_search"}],"tool_choice":"none"}`)))
	})

	t.Run("function declaration alone is not final native search", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformGrok}})
		require.NoError(t, requireExplicitGrokSearchPriceForResponsesBody(c, []byte(`{"tools":[{"type":"function","name":"web_search"}]}`)))
	})

	t.Run("adapted tool search function remains billable", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("api_key", &APIKey{Group: &Group{Platform: PlatformGrok}})
		require.ErrorIs(t, requireExplicitGrokSearchPriceForResponsesBody(c, []byte(`{"tools":[{"type":"function","name":"tool_search"}]}`)), errGrokSearchPriceUnavailable)
	})
}
