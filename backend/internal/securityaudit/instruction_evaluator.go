package securityaudit

import (
	"context"
	"errors"
	"strings"
	"time"
)

func instructionRequestBody(request Request) []byte {
	if len(request.InstructionBody) > 0 {
		return request.InstructionBody
	}
	return request.Body
}

func (s *InstructionService) parseInstructionRoot(
	ctx context.Context,
	body []byte,
	runtime InstructionRuntimeConfig,
) (map[string]any, error) {
	limits := defaultInstructionAuditParserLimits()
	if runtime.MaxBodyBytes > 0 {
		limits.MaxBodyBytes = int(runtime.MaxBodyBytes)
	}
	if runtime.ParseTimeoutMS > 0 {
		limits.ParseTimeout = time.Duration(runtime.ParseTimeoutMS) * time.Millisecond
	}
	if len(body) > limits.MaxBodyBytes {
		return nil, errInstructionAuditBodyTooLarge
	}
	capacity := runtime.MaxInflightBodyBytes
	if capacity <= 0 {
		capacity = InstructionDefaultMaxInflightBodyBytes
	}
	budget := s.requestBodyBudget.Load()
	if budget == nil || budget.Capacity() != capacity {
		s.configureInstructionRequestBodyBudget(capacity)
		budget = s.requestBodyBudget.Load()
	}
	if budget == nil {
		return nil, errors.New("instruction audit parser budget unavailable")
	}
	weight := int64(len(body))
	if weight < 1 {
		weight = 1
	}
	if weight > budget.Capacity() {
		return nil, errInstructionAuditBodyTooLarge
	}
	waitCtx, cancel := context.WithTimeout(ctx, limits.ParseTimeout)
	defer cancel()
	lease, err := budget.Acquire(waitCtx, weight)
	if err != nil {
		return nil, errInstructionAuditParseTimeout
	}
	defer lease.Release()
	return decodeStrictJSONObjectWithLimits(body, limits)
}

func applyInstructionReasonPolicy(snapshot *instructionSnapshot, decision *InstructionDecision, evaluatedAt time.Time) {
	if decision == nil {
		return
	}
	reason := decision.InitialReason
	if reason == "" {
		reason = decision.Reason
		decision.InitialReason = reason
	}
	applyInstructionPolicyForReason(snapshot, decision, reason, evaluatedAt)
}

func applyInstructionPolicyForReason(
	snapshot *instructionSnapshot,
	decision *InstructionDecision,
	reason string,
	evaluatedAt time.Time,
) {
	if decision == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = decision.InitialReason
	}
	decision.Allow = false
	decision.FinalReason = reason
	decision.Reason = reason
	decision.FinalOutcome = InstructionOutcomeBlocked
	decision.PolicyAction = InstructionPolicyActionBlock
	decision.AlertEnabled = true
	if reason == "config_unavailable" || reason == "ai_error" || snapshot == nil {
		return
	}
	policy, exists := snapshot.ReasonPolicies[reason]
	if !exists {
		return
	}
	decision.AlertEnabled = policy.AlertEnabled
	if policy.Action != InstructionPolicyActionAllowAndRecord {
		return
	}
	if policy.AllowUntil != nil && !evaluatedAt.Before(*policy.AllowUntil) {
		return
	}
	decision.Allow = true
	decision.FinalOutcome = InstructionOutcomePolicyAllow
	decision.PolicyAction = InstructionPolicyActionAllowAndRecord
}
