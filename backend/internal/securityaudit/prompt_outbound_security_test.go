package securityaudit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBaseURLAllowsAdministratorConfiguredDestinations(t *testing.T) {
	allowed := []string{
		"https://guard.example.com", "https://guard.example.com/v1", "http://guard.example.com",
		"http://127.0.0.1:8080", "http://10.0.0.8:8080", "https://172.16.0.5",
		"http://169.254.169.254", "https://metadata.google.internal", "https://192.0.2.1",
		"http://internal-admin.local", "http://guard.local:8080",
	}
	for _, raw := range allowed {
		_, err := NormalizeBaseURL(raw)
		require.NoError(t, err, raw)
	}
	blocked := []string{
		"ftp://guard.example.com", "https://user:pass@guard.example.com",
		"https://guard.example.com?q=secret", "https://guard.example.com/#fragment",
	}
	for _, raw := range blocked {
		_, err := NormalizeBaseURL(raw)
		require.Error(t, err, raw)
	}
	chatURLTests := map[string]string{
		"https://guard.example.com/v1":                       "https://guard.example.com/v1/chat/completions",
		"https://dashscope.aliyuncs.com/compatible-mode/v1":  "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		"https://open.bigmodel.cn/api/paas/v4":               "https://open.bigmodel.cn/api/paas/v4/chat/completions",
		"https://guard.example.com/api":                      "https://guard.example.com/api/v1/chat/completions",
		"https://guard.example.com/proxy/models":             "https://guard.example.com/proxy/models/v1/chat/completions",
		"https://guard.example.com/v1/chat/completions":      "https://guard.example.com/v1/chat/completions",
		"https://dashscope.aliyuncs.com/compatible-mode/v1/": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
	}
	for baseURL, expected := range chatURLTests {
		actual, err := ChatCompletionsURL(baseURL)
		require.NoError(t, err, baseURL)
		require.Equal(t, expected, actual, baseURL)
	}

	modelsURL, err := ModelsURL("https://dashscope.aliyuncs.com/compatible-mode/v1")
	require.NoError(t, err)
	require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/models", modelsURL)
	modelsURL, err = ModelsURL("https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions")
	require.NoError(t, err)
	require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/models", modelsURL)
	chatURL, err := ChatCompletionsURL("https://dashscope.aliyuncs.com/compatible-mode/v1/models")
	require.NoError(t, err)
	require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", chatURL)
	modelsURL, err = ModelsURL("https://guard.example.com/proxy/chat/completions")
	require.NoError(t, err)
	require.Equal(t, "https://guard.example.com/proxy/chat/completions/v1/models", modelsURL)
}

func TestHTTPClientUsesDirectStandardDialer(t *testing.T) {
	client, err := NewSecureHTTPClient(ActiveEndpoint{BaseURL: "https://guard.example.com", TimeoutMS: 1000})
	require.NoError(t, err)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.Proxy)
	require.NotNil(t, transport.DialContext)
}

func TestOpenAICompatibleScannerRequestContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		require.Equal(t, promptAuditReviewPurposeHeader, r.Header.Get("X-ModelPort-Internal-Purpose"))
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, testReviewerModel, payload["model"])
		require.Equal(t, float64(0), payload["temperature"])
		require.Equal(t, float64(320), payload["max_tokens"])
		require.Equal(t, false, payload["stream"])
		require.NotContains(t, payload, "seed")
		messages := payload["messages"].([]any)
		require.Len(t, messages, 2)
		user := messages[1].(map[string]any)
		var envelope map[string]string
		require.NoError(t, json.Unmarshal([]byte(user["content"].(string)), &envelope))
		require.Equal(t, "hello", envelope["audit_text"])
		w.Header().Set("Content-Type", "application/json")
		writeReviewerResponse(w, "safe", nil)
	}))
	defer server.Close()
	scanner := NewOpenAICompatibleScanner()
	result, err := scanner.Scan(context.Background(), ActiveEndpoint{ID: "one", BaseURL: server.URL, Model: testReviewerModel, Token: "token", TimeoutMS: 1000, ResponseMode: ResponseModeTextJSON, MaxOutputTokens: 320}, "hello", AllScannerIDs)
	require.NoError(t, err)
	require.Equal(t, EventPass, result.Decision)
}

func TestOpenAICompatibleScannerFollowsRedirectAndRejectsOversize(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReviewerResponse(w, "safe", nil)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	result, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "redirect", BaseURL: redirect.URL, Model: testReviewerModel, TimeoutMS: 1000, ResponseMode: ResponseModeTextJSON}, "hello", AllScannerIDs)
	require.NoError(t, err)
	require.Equal(t, EventPass, result.Decision)
	oversize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxGuardResponseBytes)+1)))
	}))
	defer oversize.Close()
	_, err = NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "large", BaseURL: oversize.URL, Model: testReviewerModel, TimeoutMS: 1000, ResponseMode: ResponseModeTextJSON}, "hello", AllScannerIDs)
	require.Error(t, err)
}

func TestOpenAICompatibleScannerClassifiesHTTPConnectionAndTimeoutFailures(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "authentication", status: http.StatusUnauthorized, retryable: false},
		{name: "forbidden", status: http.StatusForbidden, retryable: false},
		{name: "rate limited", status: http.StatusTooManyRequests, retryable: true},
		{name: "server failure", status: http.StatusBadGateway, retryable: true},
		{name: "other client error", status: http.StatusBadRequest, retryable: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			_, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "status", BaseURL: server.URL, Model: testReviewerModel, TimeoutMS: 1000, ResponseMode: ResponseModeTextJSON}, "hello", AllScannerIDs)
			var guardErr *GuardError
			require.ErrorAs(t, err, &guardErr)
			require.Equal(t, ErrorCodeUnavailable, guardErr.Code)
			require.Equal(t, tt.status, guardErr.HTTPStatus)
			require.Equal(t, tt.retryable, guardErr.Retryable)
			require.NotContains(t, err.Error(), server.URL)
		})
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	_, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "closed", BaseURL: closedURL, Model: testReviewerModel, TimeoutMS: 100, ResponseMode: ResponseModeTextJSON}, "hello", AllScannerIDs)
	var connectionErr *GuardError
	require.ErrorAs(t, err, &connectionErr)
	require.True(t, connectionErr.Retryable)

	timeout := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer timeout.Close()
	_, err = NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "timeout", BaseURL: timeout.URL, Model: testReviewerModel, TimeoutMS: 20, ResponseMode: ResponseModeTextJSON}, "hello", AllScannerIDs)
	var timeoutErr *GuardError
	require.ErrorAs(t, err, &timeoutErr)
	require.True(t, timeoutErr.Retryable)
	require.True(t, timeoutErr.Timeout)
}

func TestOpenAICompatibleScannerAutoDowngradesOnlyCapabilityErrors(t *testing.T) {
	t.Run("explicit schema capability error downgrades", func(t *testing.T) {
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":{"param":"response_format","message":"json_schema is not supported"}}`))
				return
			}
			writeReviewerResponse(w, "safe", nil)
		}))
		defer server.Close()
		result, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{
			ID: "auto", BaseURL: server.URL, Model: testReviewerModel, TimeoutMS: 1000,
			ResponseMode: ResponseModeAuto, MaxOutputTokens: DefaultMaxOutputTokens,
		}, "hello", AllScannerIDs)
		require.NoError(t, err)
		require.Equal(t, ResponseModeJSONObject, result.EffectiveResponseMode)
		require.Equal(t, int64(2), calls.Load())
	})

	t.Run("generic client error does not downgrade", func(t *testing.T) {
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"request rejected by content policy"}}`))
		}))
		defer server.Close()
		_, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{
			ID: "auto-policy", BaseURL: server.URL, Model: testReviewerModel, TimeoutMS: 1000,
			ResponseMode: ResponseModeAuto, MaxOutputTokens: DefaultMaxOutputTokens,
		}, "hello", AllScannerIDs)
		require.Error(t, err)
		require.Equal(t, int64(1), calls.Load())
	})
}

func TestOpenAICompatibleScannerParentCancellationIsTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewOpenAICompatibleScanner().Scan(ctx, ActiveEndpoint{
		ID: "canceled", BaseURL: "http://127.0.0.1:1", Model: testReviewerModel,
		TimeoutMS: 1000, ResponseMode: ResponseModeTextJSON, MaxOutputTokens: DefaultMaxOutputTokens,
	}, "hello", AllScannerIDs)
	var guardErr *GuardError
	require.ErrorAs(t, err, &guardErr)
	require.False(t, guardErr.Retryable)
	require.False(t, guardErr.Failoverable)
}

func TestPromptAuditProbeRequiresTwoRealClassifications(t *testing.T) {
	t.Run("models response never short circuits", func(t *testing.T) {
		var chatCalls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer temporary-token", r.Header.Get("Authorization"))
			if r.URL.Path == "/v1/models" {
				t.Fatal("probe must not call /models")
				return
			}
			call := chatCalls.Add(1)
			if call == 1 {
				writeReviewerResponse(w, "safe", nil)
			} else {
				writeReviewerResponse(w, "unsafe", []string{"jailbreak"})
			}
		}))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")})
		require.True(t, result.OK)
		require.True(t, result.TokenApplied)
		require.Equal(t, http.StatusOK, result.HTTPStatus)
		require.Equal(t, int64(2), chatCalls.Load())
		require.Equal(t, ResponseModeJSONSchema, result.EffectiveResponseMode)
	})

	t.Run("risky canary must not be allowed", func(t *testing.T) {
		var chatCalls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			chatCalls.Add(1)
			writeReviewerResponse(w, "safe", nil)
		}))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")})
		require.False(t, result.OK)
		require.Equal(t, ErrorCodeInvalidResponse, result.ErrorCode)
		require.Equal(t, int64(2), chatCalls.Load())
	})

	t.Run("fallback authentication failure is stable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")})
		require.False(t, result.OK)
		require.Equal(t, ErrorCodeUnavailable, result.ErrorCode)
		require.Equal(t, http.StatusUnauthorized, result.HTTPStatus)
		require.False(t, result.Retryable)
	})

	t.Run("oversized reviewer response is rejected", func(t *testing.T) {
		var chatCalls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			chatCalls.Add(1)
			_, _ = w.Write([]byte(strings.Repeat("x", int(maxGuardResponseBytes)+1)))
		}))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")})
		require.False(t, result.OK)
		require.Equal(t, ErrorCodeInvalidResponse, result.ErrorCode)
		require.Equal(t, int64(1), chatCalls.Load())
	})
}

func TestResolveProbeEndpointReusesTokenOnlyForMatchingBaseURL(t *testing.T) {
	manager := &ConfigManager{}
	manager.snapshot.Store(&activeConfigSnapshot{active: ActiveConfig{Endpoints: []ActiveEndpoint{{
		ID: "guard-1", BaseURL: "https://guard.example.com", Token: "STORED_GUARD_TOKEN", TimeoutMS: 1000, InputLimit: 1024, Enabled: true,
	}}}})
	service := &PromptService{config: manager}

	matched, applied, err := service.resolveProbeEndpoint(UpdateEndpoint{
		ID: "guard-1", BaseURL: "https://guard.example.com/v1", Model: testReviewerModel, TimeoutMS: 1000, InputLimit: 1024,
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, "STORED_GUARD_TOKEN", matched.Token)

	cleared, applied, err := service.resolveProbeEndpoint(UpdateEndpoint{
		ID: "guard-1", BaseURL: "https://guard.example.com/v1", Model: testReviewerModel,
		TimeoutMS: 1000, InputLimit: 1024, ClearToken: true,
	})
	require.NoError(t, err)
	require.False(t, applied)
	require.Empty(t, cleared.Token)

	mismatched, applied, err := service.resolveProbeEndpoint(UpdateEndpoint{
		ID: "guard-1", BaseURL: "https://attacker.example.com", Model: testReviewerModel, TimeoutMS: 1000, InputLimit: 1024,
	})
	require.NoError(t, err)
	require.False(t, applied)
	require.Empty(t, mismatched.Token)
}

func writeReviewerResponse(w http.ResponseWriter, safety string, categories []string) {
	if categories == nil {
		categories = []string{}
	}
	content, _ := json.Marshal(map[string]any{"safety": safety, "categories": categories})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(content)}}},
	})
}

func newProbeTestService() *PromptService {
	return &PromptService{
		config: &ConfigManager{}, scanner: NewOpenAICompatibleScanner(), clock: realClock{},
		probes: map[string]ProbeResult{},
	}
}

func probeEndpoint(baseURL, token string) UpdateEndpoint {
	return UpdateEndpoint{
		ID: "probe-one", Name: "Probe One", Protocol: "openai_compatible", BaseURL: baseURL,
		Model: testReviewerModel, Token: token, TimeoutMS: 1000, InputLimit: 1024, Enabled: true,
	}
}
