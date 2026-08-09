package securityaudit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type instructionAIReviewerFunc func(context.Context, InstructionRuntimeConfig, string, string) (InstructionAIResult, error)

func (f instructionAIReviewerFunc) Review(ctx context.Context, config InstructionRuntimeConfig, source, content string) (InstructionAIResult, error) {
	return f(ctx, config, source, content)
}

func newInstructionAIFlowService(t *testing.T) (*InstructionService, *InstructionRepository, int64, int64, func()) {
	t.Helper()
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	adminID := insertInstructionAuditUser(t, db, "ai-flow-admin@example.test", "admin")
	insertInstructionSensitiveTestGrant(t, db, adminID, "emergency_cli")
	userID := insertInstructionAuditUser(t, db, "ai-flow-user@example.test", "user")
	groupID := insertInstructionAuditGroup(t, db, "AI Flow Group")
	digest := sha256Hex("known standard")
	hash, err := repository.CreateHash(context.Background(), CreateInstructionHashRequest{
		Digest: digest, Name: "known standard", ObservedSource: "instructions", Status: "active",
	}, adminID)
	require.NoError(t, err)
	_, err = repository.SaveRuleSet(context.Background(), 0, SaveInstructionRuleSetRequest{
		Name: "AI flow rules", Enabled: true, HashIDs: []int64{hash.ID},
	}, adminID)
	require.NoError(t, err)
	ruleSets, err := repository.ListRuleSets(context.Background())
	require.NoError(t, err)
	require.Len(t, ruleSets, 1)
	_, err = repository.SaveGroupBindings(context.Background(), SaveInstructionGroupBindingsRequest{
		GroupIDs: []int64{groupID}, RuleSetID: ruleSets[0].ID,
		ClientTypes: []string{InstructionClientCodexCLI}, Enabled: true,
	}, adminID)
	require.NoError(t, err)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("44", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	_, err = repository.db.Exec(`
		UPDATE settings SET value = 'true' WHERE key = 'instruction_audit_enabled';
		UPDATE instruction_audit_runtime_config
		SET ai_enabled = TRUE, ai_base_url = 'http://review.invalid', ai_model = 'review-model',
			ai_timeout_ms = 1000, ai_max_concurrency = 2, ai_min_confidence = 0.9,
			ai_per_user_rpm = 10, ai_per_user_daily_limit = 10,
			ai_global_daily_limit = 20, ai_prompt_version = 'test-v1'
		WHERE id = 1;
		UPDATE instruction_audit_reason_policies
		SET ai_review_enabled = TRUE, action = 'block'
		WHERE reason = 'hash_mismatch'`)
	require.NoError(t, err)
	service := NewInstructionService(repository, redisClient, nil)
	service.evidenceCipher = cipher
	snapshot, err := repository.LoadSnapshot(context.Background())
	require.NoError(t, err)
	snapshot.Enabled = true
	snapshot.Runtime.AIEnabled = true
	snapshot.Runtime.AIModel = "review-model"
	snapshot.Runtime.AIBaseURL = "http://review.invalid"
	snapshot.Runtime.AIToken = "test-token"
	snapshot.Runtime.AITimeoutMS = 1000
	snapshot.Runtime.AIMaxConcurrency = 2
	snapshot.Runtime.AIMinConfidence = 0.9
	snapshot.Runtime.AIPerUserRPM = 10
	snapshot.Runtime.AIPerUserDailyLimit = 10
	snapshot.Runtime.AIGlobalDailyLimit = 20
	snapshot.Runtime.AIPromptVersion = "test-v1"
	policy := snapshot.ReasonPolicies["hash_mismatch"]
	policy.AIReviewEnabled = true
	policy.Action = InstructionPolicyActionBlock
	snapshot.ReasonPolicies["hash_mismatch"] = policy
	service.snapshot.Store(snapshot)
	service.configureInstructionRequestBodyBudget(snapshot.Runtime.MaxInflightBodyBytes)
	service.configureInstructionAIBudget(snapshot.Runtime.AIMaxConcurrency)
	cleanup := func() { require.NoError(t, redisClient.Close()) }
	return service, repository, userID, groupID, cleanup
}

func TestInstructionServiceAIPassPersistsAndScopesRequest(t *testing.T) {
	service, repository, userID, groupID, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	service.aiReviewer = instructionAIReviewerFunc(func(_ context.Context, _ InstructionRuntimeConfig, source, content string) (InstructionAIResult, error) {
		require.Equal(t, "instructions", source)
		require.Equal(t, "unrecognized but stable", content)
		return InstructionAIResult{Result: "pass", ApprovedSource: source, Confidence: 0.98, Reason: "stable"}, nil
	})

	decision := service.EvaluateInstruction(context.Background(), Request{
		RequestID: "ai-flow-pass", UserID: userID, UserEmail: "ai-flow-user@example.test",
		GroupID: &groupID, GroupName: "AI Flow Group", UserAgent: "codex_cli_rs/0.145.0",
		Model: "gpt-test", Endpoint: "/v1/responses", Stage: "http", Protocol: instructionAuditProtocol,
		InstructionBody: []byte(`{"instructions":"unrecognized but stable"}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
	require.Equal(t, InstructionOutcomeAIPass, decision.FinalOutcome)
	require.Positive(t, decision.EventID)
	require.NotNil(t, decision.AIReviewID)

	event, err := repository.GetEvent(context.Background(), decision.EventID)
	require.NoError(t, err)
	require.Equal(t, InstructionOutcomeAIPass, event.FinalOutcome)
	require.Equal(t, "instructions_match", event.FinalReason)
	reviews, err := repository.ListAIReviewsForEvent(context.Background(), decision.EventID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	require.Equal(t, "pass", reviews[0].Result)
	require.NotNil(t, reviews[0].AutomaticHashID)

	snapshot, err := repository.LoadSnapshot(context.Background())
	require.NoError(t, err)
	policy, applicable := instructionPolicyFor(snapshot, groupID, InstructionClientCodexCLI)
	require.True(t, applicable)
	require.Len(t, policy.Hashes, 2)
	_, applicable = instructionPolicyFor(snapshot, groupID, InstructionClientOpenCode)
	require.False(t, applicable)
}

func TestInstructionServiceAIConfidenceBelowThresholdBlocks(t *testing.T) {
	service, repository, userID, groupID, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	service.aiReviewer = instructionAIReviewerFunc(func(_ context.Context, _ InstructionRuntimeConfig, source, _ string) (InstructionAIResult, error) {
		return InstructionAIResult{Result: "pass", ApprovedSource: source, Confidence: 0.5, Reason: "uncertain"}, nil
	})

	decision := service.EvaluateInstruction(context.Background(), Request{
		RequestID: "ai-flow-uncertain", UserID: userID, UserEmail: "ai-flow-user@example.test",
		GroupID: &groupID, GroupName: "AI Flow Group", UserAgent: "codex_cli_rs/0.145.0",
		Model: "gpt-test", Endpoint: "/v1/responses", Stage: "http", Protocol: instructionAuditProtocol,
		InstructionBody: []byte(`{"instructions":"low confidence content"}`),
	})
	require.False(t, decision.Allow)
	require.Equal(t, InstructionOutcomeBlocked, decision.FinalOutcome)
	require.Equal(t, "ai_uncertain", decision.FinalReason)
	require.Positive(t, decision.EventID)
	statistics, err := repository.InstructionStatistics(context.Background(), InstructionEventFilter{
		From: timePtr(time.Now().UTC().Add(-time.Minute)), To: timePtr(time.Now().UTC().Add(time.Minute)),
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, statistics.Blocked)
	require.Zero(t, statistics.AIPass)
}

func TestInstructionServiceAIReviewsInput1AfterInstructionsReject(t *testing.T) {
	service, repository, userID, groupID, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	calls := make([]string, 0, 2)
	service.aiReviewer = instructionAIReviewerFunc(func(_ context.Context, _ InstructionRuntimeConfig, source, content string) (InstructionAIResult, error) {
		calls = append(calls, source+":"+content)
		if source == "instructions" {
			return InstructionAIResult{Result: "reject", Confidence: 0.99, Reason: "not a template"}, nil
		}
		return InstructionAIResult{Result: "pass", ApprovedSource: source, Confidence: 0.99, Reason: "stable fallback"}, nil
	})

	decision := service.EvaluateInstruction(context.Background(), Request{
		RequestID: "ai-flow-input1-pass", UserID: userID, UserEmail: "ai-flow-user@example.test",
		GroupID: &groupID, GroupName: "AI Flow Group", UserAgent: "codex_cli_rs/0.145.0",
		Model: "gpt-test", Endpoint: "/v1/responses", Stage: "http", Protocol: instructionAuditProtocol,
		InstructionBody: []byte(`{"instructions":"reject this","input":[{}, {"content":[{"type":"input_text","text":"stable fallback"}]}]}`),
	})
	require.True(t, decision.Allow)
	require.Equal(t, InstructionOutcomeAIPass, decision.FinalOutcome)
	require.Equal(t, "input1_match", decision.FinalReason)
	require.Equal(t, []string{"instructions:reject this", "input1:stable fallback"}, calls)
	reviews, err := repository.ListAIReviewsForEvent(context.Background(), decision.EventID)
	require.NoError(t, err)
	require.Len(t, reviews, 2)
	require.Equal(t, "reject", reviews[0].Result)
	require.Equal(t, "pass", reviews[1].Result)
	require.Equal(t, "input1", reviews[1].ApprovedSource)
}

func TestInstructionServiceAIErrorFailsClosedAndPersistsReason(t *testing.T) {
	service, repository, userID, groupID, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	service.aiReviewer = instructionAIReviewerFunc(func(context.Context, InstructionRuntimeConfig, string, string) (InstructionAIResult, error) {
		return InstructionAIResult{}, errInstructionAIInvalid
	})

	decision := service.EvaluateInstruction(context.Background(), Request{
		RequestID: "ai-flow-error", UserID: userID, UserEmail: "ai-flow-user@example.test",
		GroupID: &groupID, GroupName: "AI Flow Group", UserAgent: "codex_cli_rs/0.145.0",
		Model: "gpt-test", Endpoint: "/v1/responses", Stage: "http", Protocol: instructionAuditProtocol,
		InstructionBody: []byte(`{"instructions":"review service error"}`),
	})
	require.False(t, decision.Allow)
	require.Equal(t, InstructionOutcomeBlocked, decision.FinalOutcome)
	require.Equal(t, "ai_error", decision.FinalReason)
	reviews, err := repository.ListAIReviewsForEvent(context.Background(), decision.EventID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	require.Equal(t, "error", reviews[0].Result)
	require.Equal(t, "invalid_response", reviews[0].Reason)
}

func TestInstructionServiceAIRPMLimitFailsClosedBeforeSecondCall(t *testing.T) {
	service, repository, userID, groupID, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	snapshot := service.snapshot.Load()
	snapshot.Runtime.AIPerUserRPM = 1
	service.snapshot.Store(snapshot)
	callCount := 0
	service.aiReviewer = instructionAIReviewerFunc(func(context.Context, InstructionRuntimeConfig, string, string) (InstructionAIResult, error) {
		callCount++
		return InstructionAIResult{Result: "reject", Confidence: 0.99, Reason: "rejected"}, nil
	})

	request := Request{
		UserID: userID, UserEmail: "ai-flow-user@example.test", GroupID: &groupID,
		GroupName: "AI Flow Group", UserAgent: "codex_cli_rs/0.145.0", Model: "gpt-test",
		Endpoint: "/v1/responses", Stage: "http", Protocol: instructionAuditProtocol,
	}
	request.RequestID = "ai-flow-rpm-first"
	request.InstructionBody = []byte(`{"instructions":"first rejected value"}`)
	first := service.EvaluateInstruction(context.Background(), request)
	require.Equal(t, "ai_rejected", first.FinalReason)
	request.RequestID = "ai-flow-rpm-second"
	request.InstructionBody = []byte(`{"instructions":"second limited value"}`)
	second := service.EvaluateInstruction(context.Background(), request)
	require.False(t, second.Allow)
	require.Equal(t, "ai_error", second.FinalReason)
	require.Equal(t, 1, callCount)
	reviews, err := repository.ListAIReviewsForEvent(context.Background(), second.EventID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	require.Equal(t, "rate_limited", reviews[0].Reason)
}

func TestInstructionServiceAIDailyAutomaticLimitFailsClosed(t *testing.T) {
	service, repository, userID, groupID, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	_, err := repository.db.Exec(`
		UPDATE instruction_audit_runtime_config
		SET ai_per_user_daily_limit = 1, ai_global_daily_limit = 1
		WHERE id = 1`)
	require.NoError(t, err)
	snapshot := service.snapshot.Load()
	snapshot.Runtime.AIPerUserDailyLimit = 1
	snapshot.Runtime.AIGlobalDailyLimit = 1
	service.snapshot.Store(snapshot)
	callCount := 0
	service.aiReviewer = instructionAIReviewerFunc(func(_ context.Context, _ InstructionRuntimeConfig, source, _ string) (InstructionAIResult, error) {
		callCount++
		return InstructionAIResult{Result: "pass", ApprovedSource: source, Confidence: 0.99, Reason: "stable"}, nil
	})

	request := Request{
		UserID: userID, UserEmail: "ai-flow-user@example.test", GroupID: &groupID,
		GroupName: "AI Flow Group", UserAgent: "codex_cli_rs/0.145.0", Model: "gpt-test",
		Endpoint: "/v1/responses", Stage: "http", Protocol: instructionAuditProtocol,
	}
	request.RequestID = "ai-flow-daily-first"
	request.InstructionBody = []byte(`{"instructions":"daily stable one"}`)
	first := service.EvaluateInstruction(context.Background(), request)
	require.True(t, first.Allow)
	require.Equal(t, InstructionOutcomeAIPass, first.FinalOutcome)
	request.RequestID = "ai-flow-daily-second"
	request.InstructionBody = []byte(`{"instructions":"daily stable two"}`)
	second := service.EvaluateInstruction(context.Background(), request)
	require.False(t, second.Allow)
	require.Equal(t, "ai_error", second.FinalReason)
	require.Equal(t, 2, callCount)
	reviews, err := repository.ListAIReviewsForEvent(context.Background(), second.EventID)
	require.NoError(t, err)
	require.Len(t, reviews, 2)
	require.Equal(t, "pass", reviews[0].Result)
	require.Equal(t, "rate_limited", reviews[1].Reason)
}

func TestInstructionServiceRejectsRecursiveAIReasonPolicy(t *testing.T) {
	service, _, actorID, _, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	_, err := service.UpdateReasonPolicy(context.Background(), "ai_rejected", UpdateInstructionReasonPolicyRequest{
		Action: InstructionPolicyActionBlock, AIReviewEnabled: true, ExpectedVersion: service.ConfigVersion(),
	}, actorID)
	require.Equal(t, "instruction_audit_ai_review_recursive", infraerrors.Reason(err))
}

func TestInstructionServiceHashRawLifecycleIsEncryptedAndAudited(t *testing.T) {
	service, repository, _, _, cleanup := newInstructionAIFlowService(t)
	t.Cleanup(cleanup)
	var actorID int64
	require.NoError(t, repository.db.QueryRow(`
		SELECT id FROM users WHERE email = 'ai-flow-admin@example.test'`).Scan(&actorID))
	ctx := instructionSensitiveTestContext(t, repository.db, actorID)
	plaintext := " exact standard\nclient instruction "
	hash, err := service.CreateHash(ctx, CreateInstructionHashRequest{
		RawContent: plaintext, Name: "raw standard", ObservedSource: "input1", Status: "candidate",
	}, actorID)
	require.NoError(t, err)
	require.Equal(t, sha256Hex(plaintext), hash.Digest)
	require.Equal(t, "stored", hash.RawStatus)
	require.Equal(t, len([]byte(plaintext)), hash.ContentBytes)
	require.NotNil(t, hash.RawExpiresAt)

	storage, err := repository.GetHashRaw(ctx, hash.ID)
	require.NoError(t, err)
	require.NotContains(t, string(storage.Ciphertext), plaintext)

	access := InstructionSensitiveAccess{ActorID: actorID, RequestID: "raw-review"}
	err = service.RecordHashRawCopy(ctx, hash.ID, access)
	require.Equal(t, "instruction_audit_hash_raw_review_required", infraerrors.Reason(err))
	review, err := service.RevealHashRaw(ctx, hash.ID, access)
	require.NoError(t, err)
	require.Equal(t, plaintext, review.RawContent)
	require.True(t, review.DigestConsistent)
	require.Equal(t, hash.Digest, review.RecomputedSHA256)
	require.NoError(t, service.RecordHashRawCopy(ctx, hash.ID, access))

	promoted, err := service.ChangeHashStatus(ctx, hash.ID, "active", actorID, access)
	require.NoError(t, err)
	require.Equal(t, "active", promoted.Status)
	require.Nil(t, promoted.ValidUntil)
	revoked, err := service.ChangeHashStatus(ctx, hash.ID, "revoked", actorID, access)
	require.NoError(t, err)
	require.Equal(t, "revoked", revoked.Status)
	_, err = service.ChangeHashStatus(ctx, hash.ID, "active", actorID, access)
	require.Equal(t, "instruction_audit_hash_revoked", infraerrors.Reason(err))

	_, err = service.CreateHash(ctx, CreateInstructionHashRequest{
		Digest: sha256Hex("different"), RawContent: plaintext,
		Name: "mismatch", ObservedSource: "instructions", Status: "active",
	}, actorID)
	require.Equal(t, "instruction_audit_digest_content_mismatch", infraerrors.Reason(err))

	var revealCount, copyCount, promoteCount, revokeCount int64
	require.NoError(t, repository.db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE resource_type = 'hash_raw' AND action = 'reveal' AND succeeded),
			COUNT(*) FILTER (WHERE resource_type = 'hash_raw' AND action = 'copy' AND succeeded),
			COUNT(*) FILTER (WHERE resource_type = 'ai_hash' AND action = 'promote' AND succeeded),
			COUNT(*) FILTER (WHERE resource_type = 'ai_hash' AND action = 'revoke' AND succeeded)
		FROM instruction_audit_sensitive_access_logs WHERE resource_id = $1`, hash.ID).Scan(
		&revealCount, &copyCount, &promoteCount, &revokeCount,
	))
	require.EqualValues(t, 1, revealCount)
	require.EqualValues(t, 1, copyCount)
	require.EqualValues(t, 1, promoteCount)
	require.EqualValues(t, 1, revokeCount)
}

func timePtr(value time.Time) *time.Time { return &value }
