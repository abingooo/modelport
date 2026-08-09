package securityaudit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInstructionV2ServiceUsesAuthenticatedGroupAndClientScope(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	trustedDigest := instructionV2TestDigest("trusted-template")
	snapshot.Hashes[trustedDigest] = instructionV2HashRuntime{
		ID: 501, SHA256: trustedDigest, ScopeIDs: map[int64]struct{}{101: {}, 102: {}},
	}
	service.snapshot.Store(snapshot)

	tests := []struct {
		name       string
		groupID    int64
		userAgent  string
		model      string
		applicable bool
		outcome    string
	}{
		{name: "all clients scope", groupID: 7, userAgent: "curl/8.0", model: "gpt-a", applicable: true, outcome: InstructionV2OutcomeHashPass},
		{name: "client-specific scope", groupID: 8, userAgent: "codex_cli_rs/0.145.0", model: "unrelated-model", applicable: true, outcome: InstructionV2OutcomeHashPass},
		{name: "different client", groupID: 8, userAgent: "opencode/1.0", model: "gpt-a", applicable: false},
		{name: "different group", groupID: 99, userAgent: "codex_cli_rs/0.145.0", model: "gpt-a", applicable: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := service.EvaluateInstruction(context.Background(), Request{
				Protocol:        instructionAuditProtocol,
				GroupID:         instructionV2TestInt64Pointer(test.groupID),
				UserAgent:       test.userAgent,
				Model:           test.model,
				InstructionBody: []byte(`{"instructions":"trusted-template"}`),
			})
			require.True(t, decision.Allow)
			require.Equal(t, test.applicable, decision.Applicable)
			if test.applicable {
				require.Equal(t, test.outcome, decision.FinalOutcome)
				require.Equal(t, "instructions_hash_match", decision.Reason)
			}
		})
	}
}

func TestInstructionV2ServiceChecksHashBeforeAI(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeObserve)
	digest := instructionV2TestDigest("trusted input1")
	snapshot.Hashes[digest] = instructionV2HashRuntime{
		ID: 502, SHA256: digest, ScopeIDs: map[int64]struct{}{101: {}},
	}
	service.snapshot.Store(snapshot)

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		GroupID:         instructionV2TestInt64Pointer(7),
		UserAgent:       "curl/8.0",
		InstructionBody: []byte(`{"instructions":"not trusted","input":[{}, {"content":[{"type":"input_text","text":"trusted input1"}]}]}`),
	})

	require.True(t, decision.Allow)
	require.True(t, decision.Applicable)
	require.Equal(t, InstructionV2OutcomeHashPass, decision.FinalOutcome)
	require.Equal(t, "input1_hash_match", decision.Reason)
	require.Len(t, service.asyncQueue, 0, "a hash match must not enqueue AI review")
	require.Len(t, service.passQueue, 1)
}

func TestInstructionV2ServiceAllowsConfiguredExceptionsBeforeAI(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	snapshot.AllowedUsers[77] = struct{}{}
	service.snapshot.Store(snapshot)

	allowlisted := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		UserID:          77,
		GroupID:         instructionV2TestInt64Pointer(7),
		InstructionBody: []byte(`{`),
	})
	require.True(t, allowlisted.Allow)
	require.Equal(t, InstructionV2OutcomeAllowlistPass, allowlisted.FinalOutcome)
	require.Equal(t, "user_allowlist", allowlisted.Reason)

	empty := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		GroupID:         instructionV2TestInt64Pointer(7),
		InstructionBody: []byte(`{"input":[]}`),
	})
	require.True(t, empty.Allow)
	require.Equal(t, InstructionV2OutcomeEmptyPass, empty.FinalOutcome)
	require.Equal(t, "fields_empty", empty.Reason)
}

func TestInstructionV2ServiceObserveModeQueuesReviewWithoutBlocking(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeObserve)
	service.snapshot.Store(snapshot)

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		RequestID:       "observe-1",
		GroupID:         instructionV2TestInt64Pointer(7),
		InstructionBody: []byte(`{"instructions":"new template"}`),
	})

	require.True(t, decision.Allow)
	require.True(t, decision.Applicable)
	require.Equal(t, "observe_ai_queued", decision.Reason)
	require.Len(t, service.asyncQueue, 1)
	require.Len(t, service.passQueue, 0)
}

func TestInstructionV2ServiceAIFailsOverAllConfiguredNodes(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	snapshot.AINodes = []*instructionV2AINodeRuntime{
		newInstructionV2UnavailableNode(1, "first"),
		newInstructionV2UnavailableNode(2, "second"),
	}
	field := prepareInstructionV2AISample(newInstructionV2TextField("review me", false), 64000)
	evaluation := instructionV2EvaluationContext{
		snapshot: snapshot,
		scope:    snapshot.ScopesByGroup[7][0],
		profile:  snapshot.ProfilesByKey[InstructionClientOther],
		fields:   instructionV2ParsedFields{Instructions: field, Input1: InstructionV2Field{State: "missing"}},
	}

	outcome := service.reviewInstructionV2FieldsShared(context.Background(), evaluation)
	require.Equal(t, "error", outcome.Result)
	require.Len(t, outcome.Attempts, 2)
	require.Equal(t, int64(1), *outcome.Attempts[0].NodeID)
	require.Equal(t, int64(2), *outcome.Attempts[1].NodeID)
}

func TestInstructionV2ServiceRejectsAIWhenGlobalQueueIsFull(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	snapshot.Config.AIQueueWaitMS = 0
	snapshot.GlobalSemaphore = make(chan struct{}, 1)
	snapshot.GlobalSemaphore <- struct{}{}
	field := prepareInstructionV2AISample(newInstructionV2TextField("review me", false), 64000)
	evaluation := instructionV2EvaluationContext{
		snapshot: snapshot,
		scope:    snapshot.ScopesByGroup[7][0],
		profile:  snapshot.ProfilesByKey[InstructionClientOther],
		fields:   instructionV2ParsedFields{Instructions: field, Input1: InstructionV2Field{State: "missing"}},
	}

	outcome := service.reviewInstructionV2FieldsShared(context.Background(), evaluation)
	require.Equal(t, "queue_full", outcome.Result)
}

func TestInstructionV2ServiceReviewsInput1AfterInstructionsReject(t *testing.T) {
	requestedFields := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		require.NotEmpty(t, payload.Messages)
		var reviewInput struct {
			Field string `json:"field"`
		}
		require.NoError(t, json.Unmarshal([]byte(payload.Messages[len(payload.Messages)-1].Content), &reviewInput))
		requestedFields = append(requestedFields, reviewInput.Field)
		result := "reject"
		if reviewInput.Field == "input1" {
			result = "pass"
		}
		content, err := json.Marshal(map[string]any{
			"result": result, "confidence": 0.99, "reason": "test result", "category": "test",
		})
		require.NoError(t, err)
		response := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}}}
		writer.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(writer).Encode(response))
	}))
	t.Cleanup(server.Close)

	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeEnforce)
	snapshot.AINodes = []*instructionV2AINodeRuntime{{
		InstructionV2AINode: InstructionV2AINode{
			ID: 1, Name: "reviewer", BaseURL: server.URL, Model: "reviewer",
			TimeoutMS: 1000, MaxConcurrency: 1, Enabled: true,
		},
		APIKey: "test-key", semaphore: make(chan struct{}, 1),
	}}
	evaluation := instructionV2EvaluationContext{
		snapshot: snapshot,
		scope:    snapshot.ScopesByGroup[7][0],
		profile:  snapshot.ProfilesByKey[InstructionClientOther],
		fields: instructionV2ParsedFields{
			Instructions: prepareInstructionV2AISample(newInstructionV2TextField("reject this", false), 64000),
			Input1:       prepareInstructionV2AISample(newInstructionV2TextField("trusted fallback", false), 64000),
		},
	}

	outcome := service.reviewInstructionV2FieldsShared(context.Background(), evaluation)

	require.Equal(t, "pass", outcome.Result)
	require.Equal(t, "input1", outcome.ReviewedField)
	require.Equal(t, []string{"instructions", "input1"}, requestedFields)
	require.Len(t, outcome.Attempts, 2)
}

func TestInstructionV2ObserveQueueDoesNotRetainRequestBodies(t *testing.T) {
	service, snapshot := newInstructionV2TestService(t, InstructionV2ModeObserve)
	groupID := int64(7)
	evaluation := instructionV2EvaluationContext{
		request: Request{
			GroupID: &groupID,
			Body:    []byte("large gateway body"), InstructionBody: []byte("large instruction body"),
		},
		snapshot: snapshot,
		profile:  snapshot.ProfilesByKey[InstructionClientOther],
		scopes:   snapshot.ScopesByGroup[7], scope: snapshot.ScopesByGroup[7][0],
		fields: instructionV2ParsedFields{
			Instructions: prepareInstructionV2AISample(newInstructionV2TextField("review me", false), 64000),
			Input1:       InstructionV2Field{State: "missing"},
		},
	}

	require.True(t, service.enqueueObserveJob(evaluation))
	job := <-service.asyncQueue
	require.Nil(t, job.request.Body)
	require.Nil(t, job.request.InstructionBody)
	require.NotSame(t, evaluation.request.GroupID, job.request.GroupID)
	require.Equal(t, []byte("large gateway body"), evaluation.request.Body)
	require.Equal(t, []byte("large instruction body"), evaluation.request.InstructionBody)
}

func TestReuseInstructionV2SemaphoresPreservesInFlightLimits(t *testing.T) {
	previousGlobal := make(chan struct{}, 64)
	previousGlobal <- struct{}{}
	previousNode := make(chan struct{}, 16)
	previousNode <- struct{}{}
	previous := &instructionV2Snapshot{
		GlobalSemaphore: previousGlobal,
		AINodes: []*instructionV2AINodeRuntime{{
			InstructionV2AINode: InstructionV2AINode{ID: 1},
			semaphore:           previousNode,
		}},
	}
	next := &instructionV2Snapshot{
		GlobalSemaphore: make(chan struct{}, 64),
		AINodes: []*instructionV2AINodeRuntime{
			{InstructionV2AINode: InstructionV2AINode{ID: 1}, semaphore: make(chan struct{}, 16)},
			{InstructionV2AINode: InstructionV2AINode{ID: 2}, semaphore: make(chan struct{}, 8)},
		},
	}

	reuseInstructionV2Semaphores(previous, next)

	require.Equal(t, previousGlobal, next.GlobalSemaphore)
	require.Len(t, next.GlobalSemaphore, 1)
	require.Equal(t, previousNode, next.AINodes[0].semaphore)
	require.Len(t, next.AINodes[0].semaphore, 1)
	require.NotEqual(t, previousNode, next.AINodes[1].semaphore)
}

func TestReuseInstructionV2SemaphoresReplacesChangedLimits(t *testing.T) {
	previous := &instructionV2Snapshot{
		GlobalSemaphore: make(chan struct{}, 64),
		AINodes: []*instructionV2AINodeRuntime{{
			InstructionV2AINode: InstructionV2AINode{ID: 1},
			semaphore:           make(chan struct{}, 16),
		}},
	}
	nextGlobal := make(chan struct{}, 32)
	nextNode := make(chan struct{}, 8)
	next := &instructionV2Snapshot{
		GlobalSemaphore: nextGlobal,
		AINodes: []*instructionV2AINodeRuntime{{
			InstructionV2AINode: InstructionV2AINode{ID: 1},
			semaphore:           nextNode,
		}},
	}

	reuseInstructionV2Semaphores(previous, next)

	require.Equal(t, nextGlobal, next.GlobalSemaphore)
	require.Equal(t, nextNode, next.AINodes[0].semaphore)
}

func newInstructionV2TestService(t *testing.T, mode string) (*InstructionV2Service, *instructionV2Snapshot) {
	t.Helper()
	profiles, byKey, err := normalizeInstructionV2ClientProfiles([]InstructionV2ClientProfile{
		{ID: 1, ProfileKey: InstructionClientCodexCLI, Name: "Codex CLI", Enabled: true, Priority: 10, Matchers: []InstructionV2ClientMatcher{{Type: "prefix", Value: "codex_cli_rs/"}, {Type: "prefix", Value: "codex-tui/"}}},
		{ID: 2, ProfileKey: InstructionClientOpenCode, Name: "OpenCode", Enabled: true, Priority: 20, Matchers: []InstructionV2ClientMatcher{{Type: "prefix", Value: "opencode/"}}},
		{ID: 3, ProfileKey: InstructionClientModelPortInternal, Name: "ModelPort Internal", Enabled: true, BuiltIn: true, ImmutableInternal: true},
		{ID: 4, ProfileKey: InstructionClientOther, Name: "Other", Enabled: true, BuiltIn: true, Priority: 100000},
		{ID: 5, ProfileKey: InstructionClientUnknown, Name: "Unknown", Enabled: true, BuiltIn: true, Priority: 100000},
	})
	require.NoError(t, err)
	codexID := int64(1)
	snapshot := &instructionV2Snapshot{
		Config: InstructionV2Config{
			Mode: mode, EffectiveMode: mode, ConfigVersion: 1,
			AIInputMaxChars: 64000, AIQueueWaitMS: 10, AITotalTimeoutMS: 1000,
			RawFullMaxBytes: 4 << 20,
		},
		PromptVersion: "test-v1",
		Profiles:      profiles,
		ProfilesByKey: byKey,
		ScopesByGroup: map[int64][]instructionV2ScopeRuntime{
			7: {{ID: 101, GroupID: 7}},
			8: {{ID: 102, GroupID: 8, ClientProfileID: &codexID, ClientProfileKey: InstructionClientCodexCLI}},
		},
		Hashes:          map[string]instructionV2HashRuntime{},
		AllowedUsers:    map[int64]struct{}{},
		GlobalSemaphore: make(chan struct{}, InstructionV2DefaultGlobalConcurrency),
		LoadedAt:        time.Now().UTC(),
	}
	return &InstructionV2Service{
		reviewer:   NewInstructionV2AIReviewer(),
		asyncQueue: make(chan instructionV2AsyncJob, 8),
		passQueue:  make(chan InstructionV2Event, 8),
	}, snapshot
}

func newInstructionV2UnavailableNode(id int64, name string) *instructionV2AINodeRuntime {
	return &instructionV2AINodeRuntime{
		InstructionV2AINode: InstructionV2AINode{
			ID: id, Name: name, BaseURL: "://invalid", Model: "reviewer",
			TimeoutMS: 50, MaxConcurrency: 1, Enabled: true,
		},
		APIKey:    "test-key",
		semaphore: make(chan struct{}, 1),
	}
}

func instructionV2TestInt64Pointer(value int64) *int64 {
	return &value
}
