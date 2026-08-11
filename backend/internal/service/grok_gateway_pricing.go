package service

import (
	"errors"
	"math"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errGrokSearchPriceUnavailable = errors.New("grok web search is not available for this group")

// GroupHasExplicitGrokSearchPrice reports whether the standalone Grok search
// gateway may be exposed for this group. An explicit zero keeps the endpoint
// enabled and makes the search surcharge free.
func GroupHasExplicitGrokSearchPrice(group *Group) bool {
	return group != nil && isExplicitGatewayPrice(group.SearchPricePer1k)
}

// GrokResponsesRequestHasSearchIntent reports explicit downstream search intent
// before account-specific Grok request rewriting. Function-form search tools are
// included because the Free OAuth cache route can promote them to native tools.
func GrokResponsesRequestHasSearchIntent(body []byte) bool {
	if grokSearchToolChoiceDisablesAll(gjson.GetBytes(body, "tool_choice")) {
		return false
	}
	if selected, forced := grokSearchToolChoiceSelection(gjson.GetBytes(body, "tool_choice")); forced {
		return selected
	}
	if grokToolArrayHasSearchIntent(gjson.GetBytes(body, "tools"), true, false) {
		return true
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	for _, item := range input.Array() {
		if strings.TrimSpace(item.Get("type").String()) == "additional_tools" &&
			grokToolArrayHasSearchIntent(item.Get("tools"), true, false) {
			return true
		}
	}
	return false
}

// GrokChatRequestHasSearchIntent recognizes both native xAI declarations and
// OpenAI function declarations that can be promoted by the Grok Responses bridge.
func GrokChatRequestHasSearchIntent(body []byte) bool {
	choice := gjson.GetBytes(body, "tool_choice")
	if grokSearchToolChoiceDisablesAll(choice) {
		return false
	}
	if selected, forced := grokSearchToolChoiceSelection(choice); forced {
		return selected
	}
	if grokToolArrayHasSearchIntent(gjson.GetBytes(body, "tools"), true, true) {
		return true
	}
	if selected, forced := grokLegacySearchFunctionSelection(gjson.GetBytes(body, "function_call")); forced {
		return selected
	}
	for _, function := range gjson.GetBytes(body, "functions").Array() {
		if isGrokSearchToolName(function.Get("name").String()) {
			return true
		}
	}
	return false
}

// GrokMessagesRequestHasSearchIntent recognizes Anthropic server-side search
// tools and same-named function declarations that the Grok bridge may promote.
func GrokMessagesRequestHasSearchIntent(body []byte) bool {
	choice := gjson.GetBytes(body, "tool_choice")
	if grokSearchToolChoiceDisablesAll(choice) {
		return false
	}
	if selected, forced := grokSearchToolChoiceSelection(choice); forced {
		return selected
	}
	return grokToolArrayHasSearchIntent(gjson.GetBytes(body, "tools"), true, false)
}

// requireExplicitGrokSearchPriceForResponsesBody is a final egress guard. It
// runs after Grok cache routing because that routing can add native search tools
// to an otherwise client-tool-only request. tool_choice=none remains allowed.
func requireExplicitGrokSearchPriceForResponsesBody(c *gin.Context, body []byte) error {
	if !grokToolArrayHasSearchIntent(gjson.GetBytes(body, "tools"), false, false) ||
		grokSearchToolChoiceDisablesAll(gjson.GetBytes(body, "tool_choice")) {
		return nil
	}
	if selected, forced := grokSearchToolChoiceSelection(gjson.GetBytes(body, "tool_choice")); forced && !selected {
		return nil
	}
	apiKey := getAPIKeyFromContext(c)
	if apiKey != nil && GroupHasExplicitGrokSearchPrice(apiKey.Group) {
		return nil
	}
	MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
	return errGrokSearchPriceUnavailable
}

func grokToolArrayHasSearchIntent(tools gjson.Result, includeFunctionNames, chatShape bool) bool {
	if !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		toolType := strings.ToLower(strings.TrimSpace(tool.Get("type").String()))
		if toolType == "web_search" || toolType == "x_search" || toolType == "tool_search" || strings.HasPrefix(toolType, "web_search_") {
			return true
		}
		name := tool.Get("name").String()
		if chatShape {
			name = tool.Get("function.name").String()
		}
		// The Responses client-tool adapter lowers the built-in tool_search to an
		// exact function of the same name before the final egress guard. It remains
		// billable search intent even when other client function names are ignored.
		if !includeFunctionNames {
			if toolType == "function" && strings.EqualFold(strings.TrimSpace(name), "tool_search") {
				return true
			}
			continue
		}
		if isGrokSearchToolName(name) {
			return true
		}
	}
	return false
}

func grokSearchToolChoiceDisablesAll(choice gjson.Result) bool {
	if !choice.Exists() {
		return false
	}
	if choice.Type == gjson.String {
		return strings.EqualFold(strings.TrimSpace(choice.String()), "none")
	}
	return strings.EqualFold(strings.TrimSpace(choice.Get("type").String()), "none")
}

// grokSearchToolChoiceSelection returns forced=true only for a choice that
// selects one concrete tool. A forced non-search tool prevents native search
// execution even when search declarations remain in the tools array.
func grokSearchToolChoiceSelection(choice gjson.Result) (selected, forced bool) {
	if !choice.Exists() || !choice.IsObject() {
		return false, false
	}
	choiceType := strings.ToLower(strings.TrimSpace(choice.Get("type").String()))
	if choiceType == "auto" || choiceType == "any" || choiceType == "required" || choiceType == "none" || choiceType == "" {
		return false, false
	}
	if choiceType == "web_search" || choiceType == "x_search" || choiceType == "tool_search" || strings.HasPrefix(choiceType, "web_search_") {
		return true, true
	}
	name := strings.TrimSpace(choice.Get("name").String())
	if name == "" {
		name = strings.TrimSpace(choice.Get("function.name").String())
	}
	if choiceType == "function" || choiceType == "tool" || name != "" {
		return isGrokSearchToolName(name), true
	}
	return false, false
}

// grokLegacySearchFunctionSelection handles Chat Completions' deprecated
// functions/function_call pair. Non-empty legacy functions stay on raw Chat,
// where "none" and a named function are authoritative selections.
func grokLegacySearchFunctionSelection(choice gjson.Result) (selected, forced bool) {
	if !choice.Exists() {
		return false, false
	}
	if choice.Type == gjson.String {
		switch strings.ToLower(strings.TrimSpace(choice.String())) {
		case "none":
			return false, true
		case "auto", "":
			return false, false
		default:
			return false, false
		}
	}
	if !choice.IsObject() {
		return false, false
	}
	name := strings.TrimSpace(choice.Get("name").String())
	if name == "" {
		return false, false
	}
	return isGrokSearchToolName(name), true
}

func isGrokSearchToolName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "web_search" || name == "x_search" || name == "tool_search" || strings.HasPrefix(name, "web_search_")
}

// GroupHasExplicitGrokAudioPrice reports whether one billable Voice mode has
// an explicit group price. Defaults remain billing fallbacks, not feature
// enablement: an operator must opt the group into each exposed mode.
func GroupHasExplicitGrokAudioPrice(group *Group, mode string) bool {
	if group == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "tts":
		return isExplicitGatewayPrice(group.AudioTTSPricePerMillionChars)
	case "stt":
		return isExplicitGatewayPrice(group.AudioSTTPricePerHour)
	case "realtime":
		return isExplicitGatewayPrice(group.AudioRealtimePricePerMin)
	default:
		return false
	}
}

// GroupHasExplicitGrokCustomVoicePricing gates custom-voice management on all
// priced modes that can consume a custom voice. Custom-voice CRUD has no
// independent billing unit, so requiring both TTS and Realtime fails closed
// without inventing a fourth price field. STT does not consume a voice profile.
func GroupHasExplicitGrokCustomVoicePricing(group *Group) bool {
	return GroupHasExplicitGrokAudioPrice(group, "tts") &&
		GroupHasExplicitGrokAudioPrice(group, "realtime")
}

// GroupHasExplicitGrokVideoPrice reports whether this model and resolution
// resolve to an explicit per-second group price. Model-specific prices take
// precedence and flat resolution prices remain the fallback.
func GroupHasExplicitGrokVideoPrice(group *Group, model, resolution string) bool {
	if group == nil {
		return false
	}
	return isExplicitGatewayPrice(group.GetVideoPriceForModel(model, resolution))
}

// GroupHasAnyExplicitGrokVideoPrice is used by status/content requests, which
// carry a task ID but no model or resolution. It only controls endpoint
// exposure; completion billing still uses the create-time task snapshot.
func GroupHasAnyExplicitGrokVideoPrice(group *Group) bool {
	if group == nil {
		return false
	}
	if isExplicitGatewayPrice(group.VideoPrice480P) ||
		isExplicitGatewayPrice(group.VideoPrice720P) ||
		isExplicitGatewayPrice(group.VideoPrice1080P) {
		return true
	}
	for _, prices := range group.VideoModelPrices {
		for _, price := range prices {
			if isExplicitGatewayPrice(&price) {
				return true
			}
		}
	}
	return false
}

func isExplicitGatewayPrice(price *float64) bool {
	return price != nil && !math.IsNaN(*price) && !math.IsInf(*price, 0) && *price >= 0
}
