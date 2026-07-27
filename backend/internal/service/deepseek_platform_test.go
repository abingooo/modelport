package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDedicatedOpenAICompatibleAccountCreateValidation(t *testing.T) {
	for _, platform := range openai_compat.ProviderIDs() {
		t.Run(platform, func(t *testing.T) {
			_, err := buildAccountForCreate(&CreateAccountInput{
				Platform: platform,
				Type:     AccountTypeOAuth,
			}, nil)
			require.ErrorContains(t, err, "only support apikey type")

			_, err = buildAccountForCreate(&CreateAccountInput{
				Platform:    platform,
				Type:        AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "  "},
			}, nil)
			require.ErrorContains(t, err, "require an API key")

			_, err = buildAccountForCreate(&CreateAccountInput{
				Platform: platform,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "file:///tmp/upstream",
				},
			}, nil)
			require.ErrorContains(t, err, "valid HTTP(S) URL")

			account, err := buildAccountForCreate(&CreateAccountInput{
				Platform: platform,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "https://upstream.example/v1",
				},
			}, nil)
			require.NoError(t, err)
			require.Equal(t, platform, account.Platform)
			require.Equal(t, AccountTypeAPIKey, account.Type)
		})
	}
}

func TestAccountCreateRejectsUnsupportedPlatform(t *testing.T) {
	_, err := buildAccountForCreate(&CreateAccountInput{
		Platform:    "retired-provider",
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
	}, nil)

	require.ErrorContains(t, err, "unsupported platform")
}

func TestDedicatedProviderCredentialValidationAppliesToEdits(t *testing.T) {
	for _, platform := range openai_compat.ProviderIDs() {
		t.Run(platform, func(t *testing.T) {
			require.Error(t, validateDedicatedProviderCredentials(platform, AccountTypeOAuth, map[string]any{"api_key": "key"}))
			require.Error(t, validateDedicatedProviderCredentials(platform, AccountTypeAPIKey, map[string]any{"api_key": "key", "base_url": "file:///tmp/upstream"}))
			require.NoError(t, validateDedicatedProviderCredentials(platform, AccountTypeAPIKey, map[string]any{"api_key": "key", "base_url": "https://upstream.example/v1"}))
		})
	}
	require.ErrorContains(t, validateDedicatedProviderCredentials(PlatformDoubao, AccountTypeAPIKey, map[string]any{
		"api_key": "key", "endpoint_id": 123,
	}), "endpoint_id must be a string")
}

func TestAccountTestServiceDedicatedProvidersUsePresetConnectionContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range openai_compat.ProviderIDs() {
		t.Run(platform, func(t *testing.T) {
			preset, ok := openai_compat.LookupProvider(platform)
			require.True(t, ok)
			upstreamBody := strings.Join([]string{
				`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":null}]}`,
				"",
				`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				"",
				"data: [DONE]",
				"",
			}, "\n")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(upstreamBody)),
			}}
			svc := &AccountTestService{
				httpUpstream: upstream,
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
			}
			credentials := map[string]any{"api_key": "sk-provider-test"}
			if platform == PlatformDoubao {
				credentials["endpoint_id"] = "ep-provider-test"
			}
			account := &Account{ID: 91, Platform: platform, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: credentials}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/91/test", bytes.NewReader(nil))

			err := svc.testOpenAIAccountConnection(c, account, "", "hello", "")
			require.NoError(t, err)
			require.Equal(t, buildOpenAIChatCompletionsURL(preset.DefaultBaseURL), upstream.lastReq.URL.String())
			require.Equal(t, "Bearer sk-provider-test", upstream.lastReq.Header.Get("Authorization"))
			for key, value := range preset.DefaultHeaders {
				require.Equal(t, value, upstream.lastReq.Header.Get(key), key)
			}
			expectedModel := preset.DefaultTestModel
			if platform == PlatformDoubao {
				expectedModel = "ep-provider-test"
			}
			require.Equal(t, expectedModel, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
		})
	}
}

func TestAccountTestServiceDedicatedProviderErrorsAreRedacted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "sk-secret-must-not-leak"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-safe"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"invalid api_key ` + secret + `"}}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{ID: 92, Platform: PlatformQwen, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": secret}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/92/test", nil)

	err := svc.testOpenAIAccountConnection(c, account, "qwen-test", "hello", "")
	require.Error(t, err)
	require.NotContains(t, rec.Body.String(), secret)
	require.NotContains(t, err.Error(), secret)
	require.Contains(t, rec.Body.String(), "rid-safe")
}

func TestDedicatedProviderOfficialErrorShapes(t *testing.T) {
	tests := map[string]string{
		PlatformQwen:    `{"code":"InvalidApiKey","message":"invalid qwen key"}`,
		PlatformGLM:     `{"error":{"code":"1210","message":"invalid glm key"}}`,
		PlatformKimi:    `{"error":{"type":"invalid_authentication_error","message":"invalid kimi key"}}`,
		PlatformDoubao:  `{"error":{"code":"AuthenticationError","message":"invalid ByteDance key"}}`,
		PlatformMiniMax: `{"base_resp":{"status_code":1004,"status_msg":"invalid MiniMax key"}}`,
		PlatformMiMo:    `{"error":{"message":"invalid MiMo key"}}`,
	}
	for platform, body := range tests {
		t.Run(platform, func(t *testing.T) {
			require.Contains(t, strings.ToLower(ExtractUpstreamErrorMessage([]byte(body))), "invalid")
		})
	}
}

func TestDedicatedProvidersAreIndependentlySchedulable(t *testing.T) {
	for _, platform := range openai_compat.ProviderIDs() {
		t.Run(platform, func(t *testing.T) {
			preset, ok := openai_compat.LookupProvider(platform)
			require.True(t, ok)
			model := preset.DefaultTestModel
			if model == "" {
				model = preset.DefaultModelIDs[0]
			}
			account := &Account{
				Platform: platform, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "key"},
			}
			require.True(t, isOpenAICompatibleAccountEligibleForRequest(
				context.Background(), account, platform, model, false, OpenAIEndpointCapabilityChatCompletions,
			))
			for _, other := range openai_compat.ProviderIDs() {
				if other == platform {
					continue
				}
				require.False(t, isOpenAICompatibleAccountEligibleForRequest(
					context.Background(), account, other, model, false, OpenAIEndpointCapabilityChatCompletions,
				))
			}
		})
	}
}

func TestDeepSeekAccountContract(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepSeek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-deepseek-test",
		},
	}

	require.True(t, account.IsDeepSeek())
	require.True(t, account.IsOpenAICompatible())
	require.Equal(t, "https://api.deepseek.com", account.GetOpenAICompatibleBaseURL())
	require.Equal(t, "sk-deepseek-test", account.GetOpenAICompatibleAPIKey())
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))
}

func TestDeepSeekCustomBaseURLAndModelDetection(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepSeek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://relay.example.com/deepseek/v1",
		},
	}

	require.Equal(t, "https://relay.example.com/deepseek/v1", account.GetOpenAICompatibleBaseURL())
	require.Equal(t, PlatformDeepSeek, normalizeOpenAICompatiblePlatform(PlatformDeepSeek))

	platform, ok := DetectModelPlatform("deepseek/deepseek-reasoner")
	require.True(t, ok)
	require.Equal(t, PlatformDeepSeek, platform)

	platform, ok = DetectModelPlatform("deepseek-chat")
	require.True(t, ok)
	require.Equal(t, PlatformDeepSeek, platform)
}

func TestBuildDeepSeekChatCompletionsURL(t *testing.T) {
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", buildOpenAIChatCompletionsURL("https://api.deepseek.com"))
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", buildOpenAIChatCompletionsURL("https://api.deepseek.com/v1"))
}
