package securityaudit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeLegacyEngine struct {
	decision *LegacyDecision
	err      error
	calls    atomic.Int64
}

func (f *fakeLegacyEngine) Check(context.Context, Request) (*LegacyDecision, error) {
	f.calls.Add(1)
	return f.decision, f.err
}

type fakePromptEngine struct {
	mode      Mode
	decision  *PromptDecision
	err       error
	enqueues  atomic.Int64
	evaluates atomic.Int64
	request   Request
}

type fakeInstructionEngine struct {
	decision *InstructionDecision
	route    PromptAuditRoute
	calls    atomic.Int64
}

type fakeInstructionOnlyEngine struct {
	calls atomic.Int64
}

func (f *fakeInstructionEngine) EvaluateInstruction(context.Context, Request) *InstructionDecision {
	f.calls.Add(1)
	return f.decision
}

func (f *fakeInstructionEngine) ResolvePromptAuditRoute(Request) PromptAuditRoute {
	return f.route
}

func (f *fakeInstructionOnlyEngine) EvaluateInstruction(context.Context, Request) *InstructionDecision {
	f.calls.Add(1)
	return &InstructionDecision{Allow: true}
}

func testPromptAuditRoute() PromptAuditRoute {
	return PromptAuditRoute{
		Eligible: true, AuditSource: PromptAuditSourceInstructionV2,
		InstructionConfigVersion: 17, ClientProfileKey: InstructionClientOther,
		ClientProfileName: "Other", TriggerReason: PromptAuditTriggerNonResponses,
		ModelContractVersion: PromptAuditModelContractVersion,
	}
}

func newPromptEligibleCoordinator(legacy LegacyEngine, prompt PromptEngine) *Coordinator {
	return NewCoordinatorWithInstruction(legacy, prompt, &fakeInstructionEngine{
		decision: &InstructionDecision{Allow: true},
		route:    testPromptAuditRoute(),
	})
}

func (f *fakePromptEngine) EffectiveMode() Mode { return f.mode }
func (f *fakePromptEngine) Enqueue(_ context.Context, req Request) error {
	f.request = req
	f.enqueues.Add(1)
	return f.err
}
func (f *fakePromptEngine) Evaluate(_ context.Context, req Request) (*PromptDecision, error) {
	f.request = req
	f.evaluates.Add(1)
	return f.decision, f.err
}

func TestCoordinatorModesAndPriority(t *testing.T) {
	tests := []struct {
		name           string
		mode           Mode
		legacy         *LegacyDecision
		prompt         *PromptDecision
		promptErr      error
		wantKind       DecisionKind
		wantCode       string
		wantEnqueue    int64
		wantEvaluation int64
	}{
		{name: "off", mode: ModeOff, wantKind: DecisionAllow},
		{name: "async only enqueues", mode: ModeAsync, wantKind: DecisionAllow, wantEnqueue: 1},
		{name: "prompt block", mode: ModeBlocking, prompt: &PromptDecision{Kind: DecisionBlock}, wantKind: DecisionBlock, wantCode: ErrorCodeBlocked, wantEvaluation: 1},
		{name: "prompt unavailable", mode: ModeBlocking, promptErr: errors.New("down"), wantKind: DecisionUnavailable, wantCode: ErrorCodeUnavailable, wantEvaluation: 1},
		{name: "legacy wins both block", mode: ModeBlocking,
			legacy: &LegacyDecision{Blocked: true, StatusCode: http.StatusForbidden, ErrorCode: "content_policy_violation", Message: "legacy"},
			prompt: &PromptDecision{Kind: DecisionBlock}, wantKind: DecisionBlock, wantCode: "content_policy_violation", wantEvaluation: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacy := &fakeLegacyEngine{decision: tt.legacy}
			prompt := &fakePromptEngine{mode: tt.mode, decision: tt.prompt, err: tt.promptErr}
			decision := newPromptEligibleCoordinator(legacy, prompt).Check(context.Background(), Request{Body: []byte(`{}`)})
			require.Equal(t, tt.wantKind, decision.Kind)
			require.Equal(t, tt.wantCode, decision.ErrorCode)
			require.Equal(t, int64(1), legacy.calls.Load())
			require.Equal(t, tt.wantEnqueue, prompt.enqueues.Load())
			require.Equal(t, tt.wantEvaluation, prompt.evaluates.Load())
		})
	}
}

func TestCoordinatorInstructionAuditRunsFirstAndShortCircuits(t *testing.T) {
	legacy := &fakeLegacyEngine{decision: &LegacyDecision{Allowed: true}}
	prompt := &fakePromptEngine{mode: ModeBlocking, decision: &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}}
	instruction := &fakeInstructionEngine{decision: &InstructionDecision{Applicable: true, Allow: false, Reason: "hash_mismatch"}}

	decision := NewCoordinatorWithInstruction(legacy, prompt, instruction).Check(context.Background(), Request{})

	require.Equal(t, DecisionBlock, decision.Kind)
	require.Equal(t, http.StatusForbidden, decision.HTTPStatus)
	require.Equal(t, InstructionErrorCodeRejected, decision.ErrorCode)
	require.Equal(t, InstructionClientMessage, decision.ClientMessage)
	require.Same(t, instruction.decision, decision.Instruction)
	require.False(t, decision.AllowNextStage)
	require.Equal(t, int64(1), instruction.calls.Load())
	require.Zero(t, legacy.calls.Load())
	require.Zero(t, prompt.evaluates.Load())
}

func TestCoordinatorContinuesAfterInstructionAuditAllows(t *testing.T) {
	legacy := &fakeLegacyEngine{decision: &LegacyDecision{Allowed: true}}
	prompt := &fakePromptEngine{mode: ModeOff}
	instruction := &fakeInstructionEngine{decision: &InstructionDecision{Applicable: true, Allow: true}}

	decision := NewCoordinatorWithInstruction(legacy, prompt, instruction).Check(context.Background(), Request{})

	require.True(t, decision.AllowNextStage)
	require.Equal(t, int64(1), instruction.calls.Load())
	require.Equal(t, int64(1), legacy.calls.Load())
}

func TestCoordinatorSkipsInstructionAuditWhenAlreadyCompleted(t *testing.T) {
	legacy := &fakeLegacyEngine{decision: &LegacyDecision{Allowed: true}}
	prompt := &fakePromptEngine{mode: ModeOff}
	instruction := &fakeInstructionEngine{decision: &InstructionDecision{Applicable: true, Allow: false}}

	decision := NewCoordinatorWithInstruction(legacy, prompt, instruction).Check(context.Background(), Request{
		InstructionAuditCompleted: true,
	})

	require.True(t, decision.AllowNextStage)
	require.Zero(t, instruction.calls.Load())
	require.Equal(t, int64(1), legacy.calls.Load())
}

func TestCoordinatorDoesNotMutateRequestBody(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	original := append([]byte(nil), body...)
	prompt := &fakePromptEngine{mode: ModeAsync}
	decision := newPromptEligibleCoordinator(&fakeLegacyEngine{}, prompt).Check(context.Background(), Request{Body: body})
	require.True(t, decision.AllowNextStage)
	require.Equal(t, original, body)
}

func TestCoordinatorBlockingPriorityCoversBothEngineDecisionMatrix(t *testing.T) {
	legacyCases := []struct {
		name     string
		decision *LegacyDecision
	}{
		{name: "allow", decision: &LegacyDecision{Allowed: true, StatusCode: http.StatusOK, Action: "allow"}},
		{name: "flag", decision: &LegacyDecision{Allowed: true, Flagged: true, StatusCode: http.StatusOK, Action: "flag"}},
		{name: "block", decision: &LegacyDecision{Blocked: true, StatusCode: http.StatusForbidden, ErrorCode: "legacy_exact_code", Message: "legacy exact message", Action: "block"}},
	}
	promptCases := []struct {
		name     string
		decision *PromptDecision
		wantKind DecisionKind
		wantCode string
	}{
		{name: "allow", decision: &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}, wantKind: DecisionAllow},
		{name: "flag", decision: &PromptDecision{Kind: DecisionFlag, AllowNextStage: true}, wantKind: DecisionFlag},
		{name: "block", decision: &PromptDecision{Kind: DecisionBlock}, wantKind: DecisionBlock, wantCode: ErrorCodeBlocked},
		{name: "unavailable", decision: &PromptDecision{Kind: DecisionUnavailable, ErrorCode: ErrorCodeUnavailable}, wantKind: DecisionUnavailable, wantCode: ErrorCodeUnavailable},
		{name: "invalid", decision: &PromptDecision{Kind: DecisionInvalid, ErrorCode: ErrorCodeInvalidResponse}, wantKind: DecisionInvalid, wantCode: ErrorCodeInvalidResponse},
	}

	for _, legacyCase := range legacyCases {
		for _, promptCase := range promptCases {
			t.Run(fmt.Sprintf("legacy_%s_prompt_%s", legacyCase.name, promptCase.name), func(t *testing.T) {
				legacy := &fakeLegacyEngine{decision: legacyCase.decision}
				prompt := &fakePromptEngine{mode: ModeBlocking, decision: promptCase.decision}
				decision := newPromptEligibleCoordinator(legacy, prompt).Check(context.Background(), Request{})

				require.Same(t, legacyCase.decision, decision.Legacy)
				require.Same(t, promptCase.decision, decision.Prompt)
				require.Equal(t, int64(1), legacy.calls.Load())
				require.Equal(t, int64(1), prompt.evaluates.Load())
				if legacyCase.name == "block" {
					require.Equal(t, DecisionBlock, decision.Kind)
					require.Equal(t, "legacy_exact_code", decision.ErrorCode)
					require.Equal(t, "legacy exact message", decision.ClientMessage)
					require.False(t, decision.AllowNextStage)
					return
				}
				require.Equal(t, promptCase.wantKind, decision.Kind)
				require.Equal(t, promptCase.wantCode, decision.ErrorCode)
				require.Equal(t, promptCase.decision.AllowNextStage, decision.AllowNextStage)
			})
		}
	}
}

func TestCoordinatorPreservesIndependentEngineFactsAndMapsOnlyGatewayOutcome(t *testing.T) {
	legacyDecision := &LegacyDecision{
		Allowed: true, Flagged: true, Message: "legacy finding", StatusCode: http.StatusAccepted,
		ErrorCode: "legacy_observation", Action: "legacy_action",
	}
	promptResult := &NormalizedResult{
		Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock,
		Categories: []string{"pii"}, ScannerScores: map[string]float64{"pii": 1},
	}
	promptDecision := &PromptDecision{Kind: DecisionBlock, Result: promptResult}
	decision := newPromptEligibleCoordinator(
		&fakeLegacyEngine{decision: legacyDecision},
		&fakePromptEngine{mode: ModeBlocking, decision: promptDecision},
	).Check(context.Background(), Request{})

	require.Same(t, legacyDecision, decision.Legacy)
	require.Same(t, promptDecision, decision.Prompt)
	require.Same(t, promptResult, decision.Prompt.Result)
	require.Equal(t, "legacy finding", decision.Legacy.Message)
	require.Equal(t, []string{"pii"}, decision.Prompt.Result.Categories)
	require.Equal(t, ErrorCodeBlocked, decision.ErrorCode)
}

func TestCoordinatorAsyncEnqueueFailuresNeverChangeResponseOrDownstreamDispatch(t *testing.T) {
	for _, enqueueErr := range []error{ErrQueueFull, ErrQueueAdmissionBusy, errors.New("redis unavailable"), errors.New("publish failed")} {
		prompt := &fakePromptEngine{mode: ModeAsync, err: enqueueErr}
		decision := newPromptEligibleCoordinator(&fakeLegacyEngine{decision: &LegacyDecision{Allowed: true}}, prompt).Check(context.Background(), Request{})
		downstreamDispatches := 0
		status := http.StatusOK
		responseBody := "unchanged-upstream-response"
		if decision.AllowNextStage {
			downstreamDispatches++
		} else {
			status = decision.HTTPStatus
			responseBody = decision.ClientMessage
		}
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, "unchanged-upstream-response", responseBody)
		require.Equal(t, 1, downstreamDispatches)
		require.Equal(t, int64(1), prompt.enqueues.Load())
		require.Zero(t, prompt.evaluates.Load())
	}
}

func TestCoordinatorPromptPatchEligibilityAndResponsesExclusion(t *testing.T) {
	eligibleRoute := testPromptAuditRoute()
	tests := []struct {
		name                 string
		request              Request
		route                PromptAuditRoute
		wantEnqueue          int64
		wantInstructionCalls int64
	}{
		{name: "eligible non responses", request: Request{Protocol: "openai_chat_completions"}, route: eligibleRoute, wantEnqueue: 1, wantInstructionCalls: 1},
		{name: "ineligible non responses", request: Request{Protocol: "openai_chat_completions"}, wantInstructionCalls: 1},
		{name: "responses http is always excluded", request: Request{Protocol: "openai_responses"}, route: eligibleRoute, wantInstructionCalls: 1},
		{name: "responses websocket alias is always excluded", request: Request{Protocol: "responses_websocket"}, route: eligibleRoute, wantInstructionCalls: 1},
		{name: "responses endpoint is always excluded", request: Request{Protocol: "openai_chat", Endpoint: "/v1/responses"}, route: eligibleRoute, wantInstructionCalls: 1},
		{name: "completed instruction still permits eligible patch", request: Request{Protocol: "anthropic_messages", InstructionAuditCompleted: true}, route: eligibleRoute, wantEnqueue: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prompt := &fakePromptEngine{mode: ModeAsync}
			instruction := &fakeInstructionEngine{
				decision: &InstructionDecision{Allow: true}, route: test.route,
			}
			decision := NewCoordinatorWithInstruction(
				&fakeLegacyEngine{}, prompt, instruction,
			).Check(context.Background(), test.request)

			require.True(t, decision.AllowNextStage)
			require.Equal(t, test.wantEnqueue, prompt.enqueues.Load())
			require.Equal(t, test.wantInstructionCalls, instruction.calls.Load())
			if test.wantEnqueue > 0 {
				require.True(t, prompt.request.PromptAuditRouteStamped)
				require.Equal(t, int64(17), prompt.request.InstructionConfigVersion)
				require.Equal(t, InstructionClientOther, prompt.request.PromptClientProfileKey)
			}
		})
	}
}

func TestCoordinatorPromptPatchRequiresEligibilityProvider(t *testing.T) {
	tests := []struct {
		name        string
		instruction InstructionEngine
	}{
		{name: "no instruction engine"},
		{name: "instruction engine without eligibility provider", instruction: &fakeInstructionOnlyEngine{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := &fakeLegacyEngine{decision: &LegacyDecision{Allowed: true}}
			prompt := &fakePromptEngine{
				mode: ModeBlocking, decision: &PromptDecision{Kind: DecisionBlock},
			}
			decision := NewCoordinatorWithInstruction(legacy, prompt, test.instruction).
				Check(context.Background(), Request{Protocol: "anthropic_messages"})

			require.Equal(t, DecisionAllow, decision.Kind)
			require.True(t, decision.AllowNextStage)
			require.Equal(t, int64(1), legacy.calls.Load())
			require.Zero(t, prompt.enqueues.Load())
			require.Zero(t, prompt.evaluates.Load())
		})
	}
}

func TestCoordinatorPromptPatchFailsClosedWhenInstructionConfigIsUnavailable(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		request  Request
		wantKind DecisionKind
		wantCode string
	}{
		{name: "blocking", mode: ModeBlocking, request: Request{Protocol: "anthropic_messages"}, wantKind: DecisionUnavailable, wantCode: ErrorCodeUnavailable},
		{name: "async remains best effort", mode: ModeAsync, request: Request{Protocol: "anthropic_messages"}, wantKind: DecisionAllow},
		{name: "responses remains excluded", mode: ModeBlocking, request: Request{Protocol: "openai_responses"}, wantKind: DecisionAllow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := &fakeLegacyEngine{decision: &LegacyDecision{Allowed: true}}
			prompt := &fakePromptEngine{mode: test.mode, decision: &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}}
			route := PromptAuditRoute{InstructionConfigUnavailable: true}
			instruction := &fakeInstructionEngine{decision: &InstructionDecision{Allow: true}, route: route}

			decision := NewCoordinatorWithInstruction(legacy, prompt, instruction).Check(context.Background(), test.request)

			require.Equal(t, test.wantKind, decision.Kind)
			require.Equal(t, test.wantCode, decision.ErrorCode)
			require.Equal(t, int64(1), legacy.calls.Load())
			require.Zero(t, prompt.enqueues.Load())
			require.Zero(t, prompt.evaluates.Load())
		})
	}
}
