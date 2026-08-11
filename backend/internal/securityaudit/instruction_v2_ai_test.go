package securityaudit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestInstructionV2AIReviewerSuppliesExplicitJSONContract(t *testing.T) {
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		ResponseFormat map[string]any `json:"response_format"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v1/chat/completions", request.URL.Path)
		require.Equal(t, "Bearer review-token", request.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"result\":\"pass\",\"confidence\":0.98,\"reason\":\"benign template\",\"category\":\"benign_template\"}"}}]}`))
	}))
	t.Cleanup(server.Close)

	reviewer := NewInstructionV2AIReviewer()
	result, err := reviewer.Review(context.Background(), &instructionV2AINodeRuntime{
		InstructionV2AINode: InstructionV2AINode{
			BaseURL: server.URL, Model: "review-model", ResponseMode: "json_object",
			MaxOutputTokens: 512, TimeoutMS: 1000,
		},
		APIKey: "review-token",
	}, "allow stable coding client templates", "prompt-v2", "instructions", "trusted template", false)

	require.NoError(t, err)
	require.Equal(t, "pass", result.Result)
	require.InDelta(t, 0.98, result.Confidence, 0.0001)
	require.Equal(t, "json_object", payload.ResponseFormat["type"])
	require.Len(t, payload.Messages, 4)
	require.Equal(t, "system", payload.Messages[1].Role)
	require.Contains(t, payload.Messages[1].Content, `"result":"pass"`)
	require.Contains(t, payload.Messages[1].Content, "never verdict")
}

func TestInstructionV2AIReviewerSupportsDashScopeQwenModels(t *testing.T) {
	models := []string{"qwen3.7-plus", "qwen3.6-plus", "qwen3.5-plus"}
	var (
		mu       sync.Mutex
		received []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/compatible-mode/v1/chat/completions", request.URL.Path)
		var payload struct {
			Model          string         `json:"model"`
			ResponseFormat map[string]any `json:"response_format"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		require.Equal(t, "json_object", payload.ResponseFormat["type"])
		mu.Lock()
		received = append(received, payload.Model)
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"result\":\"pass\",\"confidence\":0.98,\"reason\":\"benign template\",\"category\":\"benign_template\"}"}}]}`))
	}))
	t.Cleanup(server.Close)

	reviewer := NewInstructionV2AIReviewer()
	for _, model := range models {
		result, err := reviewer.Review(context.Background(), &instructionV2AINodeRuntime{
			InstructionV2AINode: InstructionV2AINode{
				BaseURL: server.URL + "/compatible-mode/v1", Model: model,
				ResponseMode: "json_object", MaxOutputTokens: 512, TimeoutMS: 1000,
			},
			APIKey: "dashscope-test-token",
		}, "allow stable client templates", "prompt-v2", "instructions", "trusted template", false)
		require.NoError(t, err, model)
		require.Equal(t, "pass", result.Result, model)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, models, received)
}

func TestMapInstructionV2AINodeTestErrorReturnsActionableStatus(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		statusCode int
		reason     string
	}{
		{name: "timeout", err: context.DeadlineExceeded, statusCode: http.StatusGatewayTimeout, reason: "instruction_audit_v2_ai_timeout"},
		{name: "invalid response", err: errInstructionV2AIInvalid, statusCode: http.StatusBadGateway, reason: "instruction_audit_v2_ai_invalid_response"},
		{name: "unavailable", err: errInstructionV2AIUnavailable, statusCode: http.StatusServiceUnavailable, reason: "instruction_audit_v2_ai_unavailable"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mapped := mapInstructionV2AINodeTestError(testCase.err)
			require.Equal(t, testCase.statusCode, infraErrorCode(mapped))
			require.Equal(t, testCase.reason, infraErrorReason(mapped))
		})
	}
}

func TestMapInstructionV2RepositoryErrorMapsDuplicateAINodeSlot(t *testing.T) {
	mapped := mapInstructionV2RepositoryError(errInstructionV2AINodeSlotInUse, "AI 节点")

	require.Equal(t, http.StatusConflict, infraerrors.Code(mapped))
	require.Equal(t, "instruction_audit_v2_ai_slot_in_use", infraerrors.Reason(mapped))
}

func infraErrorCode(err error) int {
	return infraerrors.Code(err)
}

func infraErrorReason(err error) string {
	return infraerrors.Reason(err)
}
