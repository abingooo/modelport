//go:build unit

package service

import (
	"context"
	"testing"
)

func TestDomesticChannelMonitorProvidersUseOpenAICompatibleChat(t *testing.T) {
	providers := []string{
		MonitorProviderDeepSeek,
		MonitorProviderQwen,
		MonitorProviderGLM,
		MonitorProviderKimi,
		MonitorProviderDoubao,
		MonitorProviderMiniMax,
		MonitorProviderMiMo,
	}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			handler := &openAICaptureHandler{}
			endpoint := setupFakeOpenAI(t, handler)
			result := runCheckForModel(context.Background(), provider, endpoint, "sk-test", "model-test", nil)

			if result.Status != MonitorStatusOperational {
				t.Fatalf("provider %s should pass an OpenAI-compatible check: status=%s message=%q", provider, result.Status, result.Message)
			}
			if err := validateProvider(provider); err != nil {
				t.Fatalf("provider %s should be accepted: %v", provider, err)
			}
			if err := validateAPIMode(provider, MonitorAPIModeResponses); err == nil {
				t.Fatalf("provider %s must reject responses mode", provider)
			}
			if err := validateReplaceRequestBody(provider, MonitorAPIModeChatCompletions, map[string]any{}); err == nil {
				t.Fatalf("provider %s replace mode must require messages", provider)
			}
		})
	}
}
