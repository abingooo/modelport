package openai_compat

import "sort"

type CapabilityLevel string

const (
	CapabilityUnsupported    CapabilityLevel = "unsupported"
	CapabilitySupported      CapabilityLevel = "supported"
	CapabilityModelDependent CapabilityLevel = "model_dependent"
)

type ParameterAction string

const (
	ParameterPass   ParameterAction = "pass"
	ParameterDrop   ParameterAction = "drop"
	ParameterReject ParameterAction = "reject"
)

type ProviderCapabilities struct {
	ChatCompletions     bool
	UpstreamResponses   bool
	DownstreamResponses bool
	DownstreamMessages  bool
	ResponsesWebSocket  bool
	Streaming           bool
	Usage               bool
	ModelList           bool
	Reasoning           CapabilityLevel
	Tools               CapabilityLevel
	Vision              CapabilityLevel
	JSONSchema          CapabilityLevel
	PromptCache         CapabilityLevel
}

type ProviderPreset struct {
	ID                       string
	DisplayName              string
	DefaultBaseURL           string
	DefaultTestModel         string
	DefaultModelIDs          []string
	AuthHeader               string
	AuthScheme               string
	DefaultHeaders           map[string]string
	RequestIDHeaders         []string
	ModelListPath            string
	ModelReferenceMode       string
	ProtocolAdapter          string
	Capabilities             ProviderCapabilities
	RequestParameterRules    map[string]ParameterAction
	ForcedIntegerParameters  map[string]int
	AllowedIntegerParameters map[string][]int
}

var providerOrder = []string{
	"deepseek",
	"qwen",
	"glm",
	"kimi",
	"doubao",
	"minimax",
	"mimo",
}

var providerPresets = map[string]ProviderPreset{
	"deepseek": newPreset(
		"deepseek", "DeepSeek", "https://api.deepseek.com", "deepseek-chat",
		[]string{"deepseek-chat", "deepseek-reasoner"}, true,
		ProviderCapabilities{
			Reasoning: CapabilitySupported, Tools: CapabilitySupported,
			Vision: CapabilityUnsupported, JSONSchema: CapabilityModelDependent,
			PromptCache: CapabilitySupported,
		},
	),
	"qwen": newPreset(
		"qwen", "Alibaba Cloud Model Studio / Qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen3.7-plus",
		[]string{"qwen3.8-max-preview", "qwen3.7-max", "qwen3.7-plus", "qwen3.7-flash", "qwen3-coder-plus", "qwen-vl-plus"}, true,
		ProviderCapabilities{
			Reasoning: CapabilityModelDependent, Tools: CapabilityModelDependent,
			Vision: CapabilityModelDependent, JSONSchema: CapabilityModelDependent,
			PromptCache: CapabilityModelDependent,
		},
	),
	"glm": newPreset(
		"glm", "Zhipu AI / GLM", "https://open.bigmodel.cn/api/paas/v4", "glm-5.2",
		[]string{"glm-5.2", "glm-5.1", "glm-5-turbo", "glm-4.7", "glm-4.6v"}, true,
		ProviderCapabilities{
			Reasoning: CapabilitySupported, Tools: CapabilitySupported,
			Vision: CapabilityModelDependent, JSONSchema: CapabilityModelDependent,
			PromptCache: CapabilityModelDependent,
		},
	),
	"kimi": newPreset(
		"kimi", "Moonshot AI / Kimi", "https://api.moonshot.cn/v1", "kimi-k3",
		[]string{"kimi-k3", "kimi-k2.7-code-highspeed", "kimi-k2.6", "kimi-k2.5", "moonshot-v1-128k"}, true,
		ProviderCapabilities{
			Reasoning: CapabilitySupported, Tools: CapabilitySupported,
			Vision: CapabilityModelDependent, JSONSchema: CapabilityModelDependent,
			PromptCache: CapabilityModelDependent,
		},
	),
	"doubao": func() ProviderPreset {
		preset := newPreset(
			"doubao", "ByteDance", "https://ark.cn-beijing.volces.com/api/v3", "",
			[]string{"doubao-seed-1.8", "doubao-seed-code", "doubao-seed-1.6-vision"}, false,
			ProviderCapabilities{
				Reasoning: CapabilityModelDependent, Tools: CapabilityModelDependent,
				Vision: CapabilityModelDependent, JSONSchema: CapabilityModelDependent,
				PromptCache: CapabilityModelDependent,
			},
		)
		preset.ModelReferenceMode = "endpoint_or_model"
		preset.ProtocolAdapter = "volcengine-ark"
		preset.RequestIDHeaders = []string{"x-request-id", "x-tt-logid", "request-id"}
		return preset
	}(),
	"minimax": func() ProviderPreset {
		preset := newPreset(
			"minimax", "MiniMax", "https://api.minimaxi.com/v1", "MiniMax-M3",
			[]string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5"}, false,
			ProviderCapabilities{
				Reasoning: CapabilitySupported, Tools: CapabilitySupported,
				Vision: CapabilityModelDependent, JSONSchema: CapabilityModelDependent,
				PromptCache: CapabilitySupported,
			},
		)
		preset.ProtocolAdapter = "minimax-openai"
		preset.RequestIDHeaders = []string{"x-request-id", "trace-id", "request-id"}
		preset.RequestParameterRules["presence_penalty"] = ParameterDrop
		preset.RequestParameterRules["frequency_penalty"] = ParameterDrop
		preset.RequestParameterRules["logit_bias"] = ParameterDrop
		preset.AllowedIntegerParameters = map[string][]int{"n": {1}}
		return preset
	}(),
	"mimo": func() ProviderPreset {
		preset := newPreset(
			"mimo", "Xiaomi MiMo", "https://api.xiaomimimo.com/v1", "mimo-v2.5",
			[]string{"mimo-v2.5-pro", "mimo-v2.5", "mimo-v2-pro", "mimo-v2-omni", "mimo-v2-flash"}, false,
			ProviderCapabilities{
				Reasoning: CapabilitySupported, Tools: CapabilitySupported,
				Vision: CapabilityModelDependent, JSONSchema: CapabilityModelDependent,
				PromptCache: CapabilityModelDependent,
			},
		)
		preset.ProtocolAdapter = "xiaomi-mimo"
		return preset
	}(),
}

func newPreset(
	id string,
	displayName string,
	defaultBaseURL string,
	defaultTestModel string,
	defaultModelIDs []string,
	supportsModelList bool,
	capabilities ProviderCapabilities,
) ProviderPreset {
	capabilities.ChatCompletions = true
	capabilities.DownstreamResponses = true
	capabilities.DownstreamMessages = true
	capabilities.Streaming = true
	capabilities.Usage = true
	capabilities.ModelList = supportsModelList
	return ProviderPreset{
		ID:                 id,
		DisplayName:        displayName,
		DefaultBaseURL:     defaultBaseURL,
		DefaultTestModel:   defaultTestModel,
		DefaultModelIDs:    defaultModelIDs,
		AuthHeader:         "Authorization",
		AuthScheme:         "Bearer",
		RequestIDHeaders:   []string{"x-request-id", "request-id"},
		ModelListPath:      "/models",
		ModelReferenceMode: "model_id",
		ProtocolAdapter:    "openai-chat",
		Capabilities:       capabilities,
		RequestParameterRules: map[string]ParameterAction{
			"stream_options":      ParameterPass,
			"reasoning_effort":    ParameterPass,
			"response_format":     ParameterPass,
			"parallel_tool_calls": ParameterPass,
		},
	}
}

func LookupProvider(id string) (ProviderPreset, bool) {
	preset, ok := providerPresets[id]
	if !ok {
		return ProviderPreset{}, false
	}
	return cloneProviderPreset(preset), true
}

func IsProvider(id string) bool {
	_, ok := providerPresets[id]
	return ok
}

func ProviderIDs() []string {
	ids := append([]string(nil), providerOrder...)
	return ids
}

func AllProviders() []ProviderPreset {
	providers := make([]ProviderPreset, 0, len(providerPresets))
	for _, id := range providerOrder {
		providers = append(providers, cloneProviderPreset(providerPresets[id]))
	}
	return providers
}

func DefaultModelIDs(id string) []string {
	preset, ok := providerPresets[id]
	if !ok {
		return nil
	}
	return append([]string(nil), preset.DefaultModelIDs...)
}

func SortedProviderIDs() []string {
	ids := ProviderIDs()
	sort.Strings(ids)
	return ids
}

func cloneProviderPreset(preset ProviderPreset) ProviderPreset {
	preset.DefaultModelIDs = append([]string(nil), preset.DefaultModelIDs...)
	preset.DefaultHeaders = cloneStringMap(preset.DefaultHeaders)
	preset.RequestIDHeaders = append([]string(nil), preset.RequestIDHeaders...)
	if preset.RequestParameterRules != nil {
		rules := make(map[string]ParameterAction, len(preset.RequestParameterRules))
		for name, action := range preset.RequestParameterRules {
			rules[name] = action
		}
		preset.RequestParameterRules = rules
	}
	if preset.ForcedIntegerParameters != nil {
		values := make(map[string]int, len(preset.ForcedIntegerParameters))
		for name, value := range preset.ForcedIntegerParameters {
			values[name] = value
		}
		preset.ForcedIntegerParameters = values
	}
	if preset.AllowedIntegerParameters != nil {
		values := make(map[string][]int, len(preset.AllowedIntegerParameters))
		for name, allowed := range preset.AllowedIntegerParameters {
			values[name] = append([]int(nil), allowed...)
		}
		preset.AllowedIntegerParameters = values
	}
	return preset
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
