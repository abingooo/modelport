package securityaudit

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestApplyInstructionReasonPolicyHonorsAllowExpiryAndForcedBlocks(t *testing.T) {
	now := time.Now().UTC()
	activeUntil := now.Add(time.Hour)
	expiredAt := now.Add(-time.Minute)
	snapshot := &instructionSnapshot{ReasonPolicies: map[string]InstructionReasonPolicy{
		"hash_mismatch": {
			Reason: "hash_mismatch", Action: InstructionPolicyActionAllowAndRecord,
			AlertEnabled: false, AllowUntil: &activeUntil,
		},
		"request_too_large": {
			Reason: "request_too_large", Action: InstructionPolicyActionAllowAndRecord,
			AlertEnabled: false, AllowUntil: &expiredAt,
		},
		"config_unavailable": {
			Reason: "config_unavailable", Action: InstructionPolicyActionAllowAndRecord,
			AlertEnabled: false,
		},
	}}

	allowed := &InstructionDecision{InitialReason: "hash_mismatch"}
	applyInstructionReasonPolicy(snapshot, allowed, now)
	require.True(t, allowed.Allow)
	require.Equal(t, InstructionOutcomePolicyAllow, allowed.FinalOutcome)
	require.Equal(t, InstructionPolicyActionAllowAndRecord, allowed.PolicyAction)
	require.False(t, allowed.AlertEnabled)

	expired := &InstructionDecision{InitialReason: "request_too_large"}
	applyInstructionReasonPolicy(snapshot, expired, now)
	require.False(t, expired.Allow)
	require.Equal(t, InstructionOutcomeBlocked, expired.FinalOutcome)
	require.Equal(t, InstructionPolicyActionBlock, expired.PolicyAction)

	forced := &InstructionDecision{InitialReason: "config_unavailable"}
	applyInstructionReasonPolicy(snapshot, forced, now)
	require.False(t, forced.Allow)
	require.Equal(t, InstructionOutcomeBlocked, forced.FinalOutcome)
	require.True(t, forced.AlertEnabled)
}

func TestUpdateInstructionReasonPolicyRejectsUnsafeConfigurationBeforePersistence(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name    string
		reason  string
		request UpdateInstructionReasonPolicyRequest
		want    string
	}{
		{
			name: "configuration failure cannot allow", reason: "config_unavailable",
			request: UpdateInstructionReasonPolicyRequest{Action: InstructionPolicyActionAllowAndRecord, Confirmed: true},
			want:    "instruction_audit_reason_must_block",
		},
		{
			name: "AI failure cannot allow", reason: "ai_error",
			request: UpdateInstructionReasonPolicyRequest{Action: InstructionPolicyActionAllowAndRecord, Confirmed: true},
			want:    "instruction_audit_reason_must_block",
		},
		{
			name: "AI derived reason cannot recurse", reason: "ai_rejected",
			request: UpdateInstructionReasonPolicyRequest{Action: InstructionPolicyActionBlock, AIReviewEnabled: true},
			want:    "instruction_audit_ai_review_recursive",
		},
		{
			name: "reason without safe plaintext cannot use AI", reason: "invalid_json",
			request: UpdateInstructionReasonPolicyRequest{Action: InstructionPolicyActionBlock, AIReviewEnabled: true},
			want:    "instruction_audit_ai_review_unsupported_reason",
		},
		{
			name: "high risk allow requires confirmation", reason: "hash_mismatch",
			request: UpdateInstructionReasonPolicyRequest{Action: InstructionPolicyActionAllowAndRecord},
			want:    "instruction_audit_high_risk_confirmation_required",
		},
		{
			name: "every allow policy requires confirmation", reason: "fields_missing",
			request: UpdateInstructionReasonPolicyRequest{Action: InstructionPolicyActionAllowAndRecord},
			want:    "instruction_audit_high_risk_confirmation_required",
		},
		{
			name: "large request allow requires expiry", reason: "request_too_large",
			request: UpdateInstructionReasonPolicyRequest{Action: InstructionPolicyActionAllowAndRecord, Confirmed: true},
			want:    "instruction_audit_temporary_allow_required",
		},
		{
			name: "large request allow is bounded to one day", reason: "request_too_large",
			request: UpdateInstructionReasonPolicyRequest{
				Action: InstructionPolicyActionAllowAndRecord, Confirmed: true,
				AllowUntil: pointerToTime(now.Add(25 * time.Hour)),
			},
			want: "instruction_audit_temporary_allow_too_long",
		},
	}

	service := &InstructionService{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.UpdateReasonPolicy(context.Background(), test.reason, test.request, 1)
			require.Equal(t, test.want, infraerrors.Reason(err))
		})
	}
}

func TestInstructionRuntimeConfigPreservesLegacyAggregateRetention(t *testing.T) {
	current := InstructionRuntimeConfig{
		MaxBodyBytes: 1 << 20, ParseTimeoutMS: 500, MaxInflightBodyBytes: 1 << 20,
		PassEventRetentionDays: 7, AggregateRetentionDays: 365, RawContentRetentionDays: 30,
		AITimeoutMS: 1000, AIMaxConcurrency: 1, AIMinConfidence: 0.9,
		AIPerUserRPM: 1, AIPerUserDailyLimit: 1, AIGlobalDailyLimit: 1,
		TranslationTimeoutMS: 1000, TranslationMaxConcurrency: 1,
		TranslationChunkBytes: 1024, TranslationMaxBytes: 1024, TranslationResultTTLSeconds: 60,
	}
	updated := instructionRuntimeConfigFromRequest(UpdateInstructionRuntimeConfigRequest{}, current)
	require.Equal(t, 365, updated.AggregateRetentionDays)

	invalid := current
	invalid.AggregateRetentionDays = 29
	require.Equal(t, "instruction_audit_invalid_retention", infraerrors.Reason(validateInstructionRuntimeConfig(invalid)))
}

func pointerToTime(value time.Time) *time.Time {
	return &value
}
