package securityaudit

import (
	"context"
	"errors"
	"strings"
	"time"
)

func instructionAIReviewEnabled(snapshot *instructionSnapshot, decision *InstructionDecision) bool {
	if snapshot == nil || decision == nil || !snapshot.Runtime.AIEnabled {
		return false
	}
	policy, ok := snapshot.ReasonPolicies[decision.InitialReason]
	if !ok || !policy.AIReviewEnabled {
		return false
	}
	if decision.InitialReason != "hash_mismatch" && decision.InitialReason != "field_invalid" {
		return false
	}
	return instructionFieldCanBeAIReviewed(decision.Instructions) || instructionFieldCanBeAIReviewed(decision.Input1)
}

func instructionFieldCanBeAIReviewed(field InstructionFieldResult) bool {
	return field.Plaintext != "" && instructionDigestPattern.MatchString(field.SHA256) &&
		(field.Result == "mismatch" || field.Result == "invalid")
}

func (s *InstructionService) evaluateInstructionWithAI(
	ctx context.Context,
	request Request,
	decision *InstructionDecision,
	snapshot *instructionSnapshot,
	startedAt time.Time,
) *InstructionDecision {
	runtime := snapshot.Runtime
	type reviewField struct {
		source string
		field  InstructionFieldResult
	}
	fields := []reviewField{
		{source: "instructions", field: decision.Instructions},
		{source: "input1", field: decision.Input1},
	}
	attempts := make([]instructionAIReviewAttempt, 0, 3)
	approvedIndex := -1
	approvedField := InstructionFieldResult{}
	hasUncertain := false
	for _, item := range fields {
		if !instructionFieldCanBeAIReviewed(item.field) {
			continue
		}
		attempt := s.reviewInstructionField(ctx, request.UserID, runtime, item.source, item.field)
		attempts = append(attempts, attempt)
		decision.AILatency += time.Duration(attempt.LatencyMS) * time.Millisecond
		switch attempt.Result {
		case "pass":
			approvedIndex = len(attempts) - 1
			approvedField = item.field
		case "uncertain":
			hasUncertain = true
		case "error":
			return s.finishInstructionAIFailure(ctx, request, decision, snapshot, startedAt, attempts, "ai_error")
		}
		if approvedIndex >= 0 {
			break
		}
	}
	if approvedIndex < 0 {
		finalReason := "ai_rejected"
		if hasUncertain {
			finalReason = "ai_uncertain"
		}
		return s.finishInstructionAIFailure(ctx, request, decision, snapshot, startedAt, attempts, finalReason)
	}
	if err := reserveInstructionAIAutomaticHash(
		ctx, s.redis, request.UserID, runtime.AIPerUserDailyLimit, runtime.AIGlobalDailyLimit,
	); err != nil {
		attempts = append(attempts, instructionAISystemErrorAttempt(
			approvedField, attempts[approvedIndex].ApprovedSource, runtime, instructionAIErrorCode(err),
		))
		return s.finishInstructionAIFailure(ctx, request, decision, snapshot, startedAt, attempts, "ai_error")
	}
	raw, err := s.prepareInstructionAutomaticHashRaw(approvedField, runtime)
	if err != nil {
		attempts = append(attempts, instructionAISystemErrorAttempt(
			approvedField, attempts[approvedIndex].ApprovedSource, runtime, instructionAIErrorCode(err),
		))
		return s.finishInstructionAIFailure(ctx, request, decision, snapshot, startedAt, attempts, "ai_error")
	}
	decision.Allow = true
	decision.Reason = instructionAIPassReason(attempts[approvedIndex].ApprovedSource)
	decision.FinalReason = decision.Reason
	decision.FinalOutcome = InstructionOutcomeAIPass
	decision.PolicyAction = "ai_review"
	decision.AlertEnabled = false
	decision.Latency = time.Since(startedAt)
	automaticUntil := time.Now().UTC().Add(24 * time.Hour)
	result, err := s.commitInstructionAIOutcome(ctx, request, decision, attempts, approvedIndex, raw, approvedField, automaticUntil)
	if err != nil {
		attempts = append(attempts, instructionAISystemErrorAttempt(
			approvedField, attempts[approvedIndex].ApprovedSource, runtime, instructionAIErrorCode(err),
		))
		return s.finishInstructionAIFailure(ctx, request, decision, snapshot, startedAt, attempts, "ai_error")
	}
	decision.EventID = result.EventID
	decision.AIReviewID = &result.AIReviewID
	if result.ConfigVersion > 0 {
		s.refreshAfterMutation(ctx, result.ConfigVersion)
	}
	return decision
}

func (s *InstructionService) reviewInstructionField(
	ctx context.Context,
	userID int64,
	runtime InstructionRuntimeConfig,
	source string,
	field InstructionFieldResult,
) instructionAIReviewAttempt {
	attempt := instructionAIReviewAttempt{
		ReviewedSource: source, ReviewedSHA256: field.SHA256,
		Result: "error", Reason: "unavailable", ReviewerModel: runtime.AIModel,
		PromptVersion: runtime.AIPromptVersion,
	}
	if err := reserveInstructionAIReview(ctx, s.redis, userID, runtime.AIPerUserRPM); err != nil {
		attempt.Reason = instructionAIErrorCode(err)
		return attempt
	}
	timeout := time.Duration(runtime.AITimeoutMS) * time.Millisecond
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	release, err := s.acquireInstructionAI(callCtx, runtime.AIMaxConcurrency)
	if err != nil {
		attempt.Reason = instructionAIErrorCode(err)
		return attempt
	}
	defer release()
	if s.aiReviewer == nil {
		attempt.Reason = "reviewer_unavailable"
		return attempt
	}
	startedAt := time.Now()
	result, err := s.aiReviewer.Review(callCtx, runtime, source, field.Plaintext)
	attempt.LatencyMS = int(time.Since(startedAt).Milliseconds())
	if err != nil {
		attempt.Reason = instructionAIErrorCode(err)
		return attempt
	}
	if result.Result == "pass" && result.Confidence < runtime.AIMinConfidence {
		result.Result = "uncertain"
		result.ApprovedSource = ""
		result.Reason = "confidence_below_threshold"
	}
	attempt.Result = result.Result
	attempt.ApprovedSource = result.ApprovedSource
	attempt.Confidence = result.Confidence
	attempt.Reason = strings.TrimSpace(result.Reason)
	return attempt
}

func (s *InstructionService) prepareInstructionAutomaticHashRaw(
	field InstructionFieldResult,
	runtime InstructionRuntimeConfig,
) (*instructionHashRawStorage, error) {
	if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
		return nil, errInstructionEvidenceEncryptionUnavailable
	}
	if field.Plaintext == "" || sha256Hex(field.Plaintext) != field.SHA256 {
		return nil, errors.New("instruction audit AI approved digest mismatch")
	}
	ciphertext, err := s.evidenceCipher.EncryptHashRaw(field.SHA256, field.Plaintext)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(time.Duration(runtime.RawContentRetentionDays) * 24 * time.Hour)
	return &instructionHashRawStorage{
		Ciphertext: ciphertext, Status: "stored", ContentBytes: len([]byte(field.Plaintext)),
		HashAlgorithm: InstructionHashAlgorithmSHA256,
		Normalization: InstructionHashNormalizationIdentityV1,
		KeyVersion:    instructionHashRawKeyVersion, ExpiresAt: &expiresAt,
	}, nil
}

func (s *InstructionService) commitInstructionAIOutcome(
	ctx context.Context,
	request Request,
	decision *InstructionDecision,
	attempts []instructionAIReviewAttempt,
	finalAttempt int,
	raw *instructionHashRawStorage,
	approvedField InstructionFieldResult,
	automaticUntil time.Time,
) (*instructionAIOutcomeCommitResult, error) {
	if s.repository == nil {
		return nil, errors.New("instruction audit repository unavailable")
	}
	evidenceStatus, evidenceExpiresAt, evidence := s.prepareEvidence(ctx, decision)
	eventRequest := instructionEventRequest(request)
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instructionBlockedEventPersistenceTimeout)
	defer cancel()
	return s.repository.CommitAIOutcome(recordCtx, instructionAIOutcomeCommit{
		Request: eventRequest, Decision: decision, EvidenceStatus: evidenceStatus,
		EvidenceExpiresAt: evidenceExpiresAt, Evidence: evidence, Attempts: attempts,
		FinalAttempt: finalAttempt, ApprovedRaw: raw, ApprovedField: approvedField,
		AutomaticUntil: automaticUntil,
	})
}

func (s *InstructionService) finishInstructionAIFailure(
	ctx context.Context,
	request Request,
	decision *InstructionDecision,
	snapshot *instructionSnapshot,
	startedAt time.Time,
	attempts []instructionAIReviewAttempt,
	finalReason string,
) *InstructionDecision {
	if len(attempts) == 0 {
		attempts = append(attempts, instructionAISystemErrorAttempt(
			decision.Instructions, "instructions", snapshot.Runtime, "review_unavailable",
		))
		finalReason = "ai_error"
	}
	decision.Allow = false
	decision.Reason = finalReason
	decision.FinalReason = finalReason
	decision.FinalOutcome = InstructionOutcomeBlocked
	decision.PolicyAction = InstructionPolicyActionBlock
	decision.AlertEnabled = true
	applyInstructionPolicyForReason(snapshot, decision, finalReason, time.Now().UTC())
	decision.Latency = time.Since(startedAt)
	finalAttempt := len(attempts) - 1
	result, err := s.commitInstructionAIOutcome(
		ctx, request, decision, attempts, finalAttempt, nil, InstructionFieldResult{}, time.Time{},
	)
	if err == nil {
		decision.EventID = result.EventID
		decision.AIReviewID = &result.AIReviewID
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instructionBlockedEventPersistenceTimeout)
		defer cancel()
		s.enqueueInstructionOutcomeNotifications(recordCtx, instructionEventRequest(request), decision, result.EventID)
		return decision
	}
	decision.Allow = false
	decision.Reason = "ai_error"
	decision.FinalReason = "ai_error"
	decision.FinalOutcome = InstructionOutcomeBlocked
	decision.PolicyAction = InstructionPolicyActionBlock
	decision.AlertEnabled = true
	decision.AIReviewID = nil
	_ = s.recordOutcome(ctx, request, decision)
	return decision
}

func instructionEventRequest(request Request) Request {
	result := request
	if request.GroupID != nil {
		groupID := *request.GroupID
		result.GroupID = &groupID
	}
	result.Body = nil
	result.InstructionBody = nil
	return result
}

func instructionAISystemErrorAttempt(
	field InstructionFieldResult,
	source string,
	runtime InstructionRuntimeConfig,
	errorCode string,
) instructionAIReviewAttempt {
	if source != "instructions" && source != "input1" {
		source = "instructions"
	}
	digest := field.SHA256
	if !instructionDigestPattern.MatchString(digest) {
		digest = sha256Hex("instruction-ai-system-error:" + source)
	}
	return instructionAIReviewAttempt{
		ReviewedSource: source, ReviewedSHA256: digest, Result: "error",
		Reason: errorCode, ReviewerModel: runtime.AIModel, PromptVersion: runtime.AIPromptVersion,
	}
}

func instructionAIErrorCode(err error) string {
	switch {
	case errors.Is(err, errInstructionAILimited):
		return "rate_limited"
	case errors.Is(err, errInstructionAIInvalid):
		return "invalid_response"
	case errors.Is(err, errInstructionAIAutomaticHashUnavailable):
		return "hash_disabled"
	case errors.Is(err, errInstructionEvidenceEncryptionUnavailable):
		return "encryption_unavailable"
	default:
		return "unavailable"
	}
}

func instructionAIPassReason(source string) string {
	if source == "input1" {
		return "input1_match"
	}
	return "instructions_match"
}
