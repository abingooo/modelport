package securityaudit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAIInstructionReviewerUsesDedicatedStructuredRequest(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v1/chat/completions", request.URL.Path)
		require.Equal(t, "Bearer review-token", request.Header.Get("Authorization"))
		require.Equal(t, instructionAIReviewPurposeHeader, request.Header.Get("X-ModelPort-Internal-Purpose"))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&captured))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"result\":\"pass\",\"approved_source\":\"instructions\",\"confidence\":0.99,\"reason\":\"recognized stable client template\"}"}}]}`))
	}))
	t.Cleanup(server.Close)

	result, err := NewOpenAIInstructionReviewer().Review(context.Background(), InstructionRuntimeConfig{
		AIBaseURL: server.URL, AIModel: "review-model", AIToken: "review-token", AITimeoutMS: 1000,
	}, "instructions", "untrusted instruction content")
	require.NoError(t, err)
	require.Equal(t, "pass", result.Result)
	require.Equal(t, "instructions", result.ApprovedSource)
	require.InDelta(t, 0.99, result.Confidence, 0.0001)
	require.Equal(t, "review-model", captured["model"])
	responseFormat := captured["response_format"].(map[string]any)
	require.Equal(t, "json_schema", responseFormat["type"])
	messages := captured["messages"].([]any)
	require.Len(t, messages, 2)
	userMessage := messages[1].(map[string]any)["content"].(string)
	require.Contains(t, userMessage, "untrusted instruction content")
	require.NotContains(t, strings.ToLower(userMessage), "authorization")
}

func TestParseInstructionAIResponseRejectsAmbiguousOrLooseOutput(t *testing.T) {
	valid, err := parseInstructionAIResponse(
		`{"result":"reject","approved_source":null,"confidence":0.8,"reason":"unsafe"}`,
		"instructions",
	)
	require.NoError(t, err)
	require.Equal(t, "reject", valid.Result)

	for _, content := range []string{
		"```json\n{\"result\":\"reject\",\"approved_source\":null,\"confidence\":0.8,\"reason\":\"unsafe\"}\n```",
		`{"result":"pass","approved_source":"input1","confidence":0.99,"reason":"ok"}`,
		`{"result":"reject","approved_source":"instructions","confidence":0.8,"reason":"unsafe"}`,
		`{"result":"uncertain","approved_source":null,"confidence":1.1,"reason":"unknown"}`,
		`{"result":"reject","approved_source":null,"confidence":0.8,"reason":"unsafe","extra":true}`,
		`{"result":"reject","approved_source":null,"confidence":0.8,"reason":"unsafe"} {}`,
		`{"result":"reject","approved_source":null,"confidence":0.8}`,
		`{"result":"reject","result":"pass","approved_source":null,"confidence":0.8,"reason":"unsafe"}`,
		`{"result":"reject","approved_source":null,"confidence":0.8,"reason":"unsafe","reason":"duplicate"}`,
	} {
		_, err = parseInstructionAIResponse(content, "instructions")
		require.ErrorIs(t, err, errInstructionAIInvalid, content)
	}
}

func TestInstructionAIRedisLimitsAreAtomic(t *testing.T) {
	server := miniredis.RunT(t)
	server.SetTime(time.Date(2026, 8, 8, 1, 2, 3, 0, time.UTC))
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := context.Background()

	require.NoError(t, reserveInstructionAIReview(ctx, client, 17, 2))
	require.NoError(t, reserveInstructionAIReview(ctx, client, 17, 2))
	require.ErrorIs(t, reserveInstructionAIReview(ctx, client, 17, 2), errInstructionAILimited)
	require.NoError(t, reserveInstructionAIReview(ctx, client, 18, 2))

	require.NoError(t, reserveInstructionAIAutomaticHash(ctx, client, 17, 2, 3))
	require.NoError(t, reserveInstructionAIAutomaticHash(ctx, client, 17, 2, 3))
	require.ErrorIs(t, reserveInstructionAIAutomaticHash(ctx, client, 17, 2, 3), errInstructionAILimited)
	require.NoError(t, reserveInstructionAIAutomaticHash(ctx, client, 18, 2, 3))
	require.ErrorIs(t, reserveInstructionAIAutomaticHash(ctx, client, 18, 2, 3), errInstructionAILimited)

	require.ErrorIs(t, reserveInstructionAIReview(ctx, nil, 17, 2), errInstructionAIUnavailable)
	require.True(t, errors.Is(reserveInstructionAIAutomaticHash(ctx, nil, 17, 2, 3), errInstructionAIUnavailable))
}

func TestInstructionAIConcurrencyBudgetCapsReviewerCalls(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	service := NewInstructionService(nil, client, nil)
	const concurrency = 2
	const calls = 6
	started := make(chan struct{}, calls)
	release := make(chan struct{})
	var active atomic.Int64
	var maximum atomic.Int64
	service.aiReviewer = instructionAIReviewerFunc(func(ctx context.Context, _ InstructionRuntimeConfig, source, _ string) (InstructionAIResult, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
			return InstructionAIResult{Result: "reject", Confidence: 0.99, Reason: source}, nil
		case <-ctx.Done():
			return InstructionAIResult{}, ctx.Err()
		}
	})
	runtime := InstructionRuntimeConfig{
		AITimeoutMS: 2000, AIMaxConcurrency: concurrency, AIPerUserRPM: 100,
		AIModel: "review-model", AIPromptVersion: "test-v1",
	}
	field := InstructionFieldResult{Plaintext: "concurrency test", SHA256: sha256Hex("concurrency test"), Result: "mismatch"}
	var wait sync.WaitGroup
	wait.Add(calls)
	for index := range calls {
		go func(userID int64) {
			defer wait.Done()
			attempt := service.reviewInstructionField(context.Background(), userID, runtime, "instructions", field)
			require.Equal(t, "reject", attempt.Result)
		}(int64(index + 1))
	}
	for range concurrency {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("reviewer did not reach configured concurrency")
		}
	}
	select {
	case <-started:
		t.Fatal("reviewer exceeded configured concurrency")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	wait.Wait()
	require.EqualValues(t, concurrency, maximum.Load())
}

func TestInstructionAIConcurrencyBudgetResizeKeepsActiveLeases(t *testing.T) {
	service := NewInstructionService(nil, nil, nil)
	service.configureInstructionAIBudget(2)
	budget := service.aiBudget.Load()
	releaseFirst, err := service.acquireInstructionAI(context.Background(), 2)
	require.NoError(t, err)
	releaseSecond, err := service.acquireInstructionAI(context.Background(), 2)
	require.NoError(t, err)

	service.configureInstructionAIBudget(1)
	require.Same(t, budget, service.aiBudget.Load())
	require.EqualValues(t, 1, budget.budget.Capacity())

	assertBlocked := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, acquireErr := service.acquireInstructionAI(ctx, 2)
		require.ErrorIs(t, acquireErr, errInstructionAIUnavailable)
	}
	assertBlocked()
	releaseFirst()
	assertBlocked()
	releaseSecond()

	releaseThird, err := service.acquireInstructionAI(context.Background(), 2)
	require.NoError(t, err)
	releaseThird()
	require.EqualValues(t, 1, budget.budget.Capacity())
}

func TestInstructionAIReviewerTimeoutFailsClosed(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	service := NewInstructionService(nil, client, nil)
	service.aiReviewer = instructionAIReviewerFunc(func(ctx context.Context, _ InstructionRuntimeConfig, _ string, _ string) (InstructionAIResult, error) {
		<-ctx.Done()
		return InstructionAIResult{}, ctx.Err()
	})
	field := InstructionFieldResult{Plaintext: "timeout test", SHA256: sha256Hex("timeout test"), Result: "mismatch"}
	startedAt := time.Now()
	attempt := service.reviewInstructionField(context.Background(), 17, InstructionRuntimeConfig{
		AITimeoutMS: 30, AIMaxConcurrency: 1, AIPerUserRPM: 10,
		AIModel: "review-model", AIPromptVersion: "test-v1",
	}, "instructions", field)
	require.Equal(t, "error", attempt.Result)
	require.Equal(t, "unavailable", attempt.Reason)
	require.Less(t, time.Since(startedAt), time.Second)
}
