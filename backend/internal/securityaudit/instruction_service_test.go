package securityaudit

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func instructionTestSnapshot(enabled bool, groupID int64, values ...string) *instructionSnapshot {
	hashes := make([]instructionPolicyHash, 0, len(values))
	for _, value := range values {
		hashes = append(hashes, allowedDigest(value))
	}
	return &instructionSnapshot{
		Enabled:       enabled,
		ConfigVersion: 7,
		LoadedAt:      time.Now().UTC(),
		AuditedGroups: map[int64]struct{}{groupID: {}},
		Policies: map[int64]instructionPolicy{
			groupID: {RuleSetIDs: []int64{11}, Hashes: hashes},
		},
	}
}

func instructionTestGroupID(value int64) *int64 { return &value }

func TestInstructionServiceEvaluationScopeAndFallback(t *testing.T) {
	const groupID int64 = 42

	tests := []struct {
		name       string
		snapshot   *instructionSnapshot
		request    Request
		wantApply  bool
		wantAllow  bool
		wantReason string
	}{
		{
			name:      "disabled skips malformed body",
			snapshot:  instructionTestSnapshot(false, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), InstructionBody: []byte(`{`)},
			wantAllow: true,
		},
		{
			name:      "unbound group is unchanged",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(99), InstructionBody: []byte(`{`)},
			wantAllow: true,
		},
		{
			name:      "same group audits every user and model",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, UserID: 99, GroupID: instructionTestGroupID(groupID), Model: "unrelated-model", InstructionBody: []byte(`{"model":"unrelated-model","instructions":"trusted"}`)},
			wantApply: true, wantAllow: true, wantReason: "instructions_match",
		},
		{
			name:      "selected group blocks malformed request without model",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), InstructionBody: []byte(`{`)},
			wantApply: true, wantReason: "invalid_json",
		},
		{
			name:      "instructions match",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), InstructionBody: []byte(`{"instructions":"trusted"}`)},
			wantApply: true, wantAllow: true, wantReason: "instructions_match",
		},
		{
			name:      "input1 fallback shares hash pool",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), InstructionBody: []byte(`{"instructions":"other","input":[{}, {"content":[{"type":"input_text","text":"trust"},{"type":"input_text","text":"ed"}]}]}`)},
			wantApply: true, wantAllow: true, wantReason: "input1_match",
		},
		{
			name:      "both mismatch",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), InstructionBody: []byte(`{"instructions":"other","input":[{}, {"content":[{"type":"input_text","text":"also-other"}]}]}`)},
			wantApply: true, wantReason: "hash_mismatch",
		},
		{
			name:      "strict json violation blocks audited group",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), InstructionBody: []byte(`{"instructions":"one","instructions":"trusted"}`)},
			wantApply: true, wantReason: "invalid_json",
		},
		{
			name:      "bound group with no effective hashes fails closed",
			snapshot:  instructionTestSnapshot(true, groupID),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), Model: "any-model", InstructionBody: []byte(`{"instructions":"trusted"}`)},
			wantApply: true, wantReason: "hash_mismatch",
		},
		{
			name:      "compact is explicitly excluded",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), InstructionAuditExcluded: true, InstructionBody: []byte(`{`)},
			wantAllow: true,
		},
		{
			name:      "other protocols are excluded",
			snapshot:  instructionTestSnapshot(true, groupID, "trusted"),
			request:   Request{Protocol: "openai_chat_completions", GroupID: instructionTestGroupID(groupID), InstructionBody: []byte(`{`)},
			wantAllow: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &InstructionService{}
			service.snapshot.Store(test.snapshot)
			decision := service.EvaluateInstruction(context.Background(), test.request)
			require.NotNil(t, decision)
			require.Equal(t, test.wantApply, decision.Applicable)
			require.Equal(t, test.wantAllow, decision.Allow)
			require.Equal(t, test.wantReason, decision.Reason)
		})
	}
}

func TestInstructionServiceFailsClosedBeforeFirstValidSnapshot(t *testing.T) {
	service := &InstructionService{}
	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol:        instructionAuditProtocol,
		UserID:          1,
		Model:           "gpt-5.6-sol",
		InstructionBody: []byte(`{"model":"gpt-5.6-sol","instructions":"trusted"}`),
	})
	require.True(t, decision.Applicable)
	require.False(t, decision.Allow)
	require.True(t, decision.Unavailable)
	require.Equal(t, "config_unavailable", decision.Reason)
}

func TestInstructionServiceUnionsRulesAndFollowsAPIKeyGroupChanges(t *testing.T) {
	snapshot := instructionTestSnapshot(true, 7, "first", "second")
	policy := snapshot.Policies[7]
	policy.RuleSetIDs = []int64{11, 12}
	snapshot.Policies[7] = policy
	service := &InstructionService{}
	service.snapshot.Store(snapshot)

	request := Request{
		Protocol: instructionAuditProtocol, APIKeyID: 91, GroupID: instructionTestGroupID(7), Model: "model-a",
		InstructionBody: []byte(`{"instructions":"second"}`),
	}
	decision := service.EvaluateInstruction(context.Background(), request)
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
	require.Equal(t, []int64{11, 12}, decision.RuleSetIDs)

	request.GroupID = instructionTestGroupID(8)
	request.Model = "model-b"
	request.InstructionBody = []byte(`{`)
	decision = service.EvaluateInstruction(context.Background(), request)
	require.False(t, decision.Applicable)
	require.True(t, decision.Allow)
}

func TestInstructionServicePersistsBlockedEventBeforeReturning(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	service := NewInstructionService(NewInstructionRepository(db), nil, nil)
	request := Request{
		RequestID: "req-persisted", UserID: 7, APIKeyID: 9, Model: "gpt-test",
		GroupID: instructionTestGroupID(3), GroupName: "OpenAI",
		Body:            []byte(`{"authorization":"Bearer secret"}`),
		InstructionBody: []byte(`{"instructions":"plaintext must not survive"}`),
	}
	decision := &InstructionDecision{Allow: false, Reason: "hash_mismatch", RuleSetIDs: []int64{11}}

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO instruction_audit_events").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(17)))
	mock.ExpectExec("INSERT INTO instruction_audit_notification_outbox").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	service.recordBlocked(canceled, request, decision)
	require.Zero(t, service.failedBlockedEventPersists.Load())
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionServiceUsesStrictOriginalModel(t *testing.T) {
	service := &InstructionService{}
	service.snapshot.Store(instructionTestSnapshot(true, 7, "trusted"))
	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(7),
		UserID:          7,
		Model:           "mapped-model",
		InstructionBody: []byte(`{"model":"original-model","instructions":"trusted"}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)

	decision = service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "original-model", InstructionModelOverride: true,
		InstructionBody: []byte(`{"model":"mapped-model","instructions":"trusted"}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
}

func TestInstructionServiceReloadKeepsLastKnownGoodSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	service := NewInstructionService(NewInstructionRepository(db), nil, nil)
	knownGood := instructionTestSnapshot(true, 7, "trusted")
	service.snapshot.Store(knownGood)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	mock.ExpectClose()

	err = service.Reload(context.Background())
	require.Error(t, err)
	require.Same(t, knownGood, service.snapshot.Load())
	overview := &InstructionOverview{}
	service.stateMu.RLock()
	overview.LoadError = service.lastLoadError
	service.stateMu.RUnlock()
	require.NotEmpty(t, overview.LoadError)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionServiceFreshSnapshotSurvivesTransientRefreshFailure(t *testing.T) {
	service := &InstructionService{}
	service.snapshot.Store(instructionTestSnapshot(true, 7, "trusted"))
	service.setLoadError("configuration refresh failed")

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"trusted"}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
}

func TestInstructionServiceFailsClosedAfterSnapshotStalenessLimit(t *testing.T) {
	service := &InstructionService{}
	snapshot := instructionTestSnapshot(true, 7, "trusted")
	snapshot.LoadedAt = time.Now().Add(-instructionSnapshotMaxStaleness - time.Second)
	service.snapshot.Store(snapshot)
	service.setLoadError("configuration refresh failed")

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"trusted"}`),
	})
	require.True(t, decision.Applicable)
	require.False(t, decision.Allow)
	require.True(t, decision.Unavailable)
	require.Equal(t, "config_unavailable", decision.Reason)
}

func TestInstructionServiceStaleSnapshotDoesNotEnableDisabledAudit(t *testing.T) {
	service := &InstructionService{}
	snapshot := instructionTestSnapshot(false, 7, "trusted")
	snapshot.LoadedAt = time.Now().Add(-instructionSnapshotMaxStaleness - time.Second)
	service.snapshot.Store(snapshot)
	service.setLoadError("configuration refresh failed")

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"untrusted"}`),
	})
	require.False(t, decision.Applicable)
	require.True(t, decision.Allow)
}

func TestInstructionServiceKeepsLastScopedSnapshotWhileNewerVersionLoads(t *testing.T) {
	service := &InstructionService{}
	service.snapshot.Store(instructionTestSnapshot(true, 7, "trusted"))
	service.requireConfigVersion(8)
	service.setLoadError("required configuration version is not available")

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"trusted"}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)

	disabled := instructionTestSnapshot(false, 7, "trusted")
	service.snapshot.Store(disabled)
	decision = service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"untrusted"}`),
	})
	require.False(t, decision.Applicable)
	require.True(t, decision.Allow)

	enabled := instructionTestSnapshot(true, 7, "trusted")
	service.snapshot.Store(enabled)
	decision = service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 99, GroupID: instructionTestGroupID(99), Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"untrusted"}`),
	})
	require.False(t, decision.Applicable)
	require.True(t, decision.Allow)
}

func TestInstructionServiceRejectsSnapshotVersionRegression(t *testing.T) {
	service := &InstructionService{}
	current := instructionTestSnapshot(true, 7, "trusted")
	current.ConfigVersion = 9
	require.NoError(t, service.storeSnapshot(current))

	stale := instructionTestSnapshot(false, 7, "trusted")
	stale.ConfigVersion = 8
	require.Error(t, service.storeSnapshot(stale))
	require.Same(t, current, service.snapshot.Load())
}

func TestInstructionServiceRejectsExpiredAndNotYetValidHashes(t *testing.T) {
	now := time.Now().UTC()
	for _, hash := range []instructionPolicyHash{
		{Digest: allowedDigest("trusted").Digest, ValidUntil: now.Add(-time.Second)},
		{Digest: allowedDigest("trusted").Digest, ValidFrom: now.Add(time.Hour)},
	} {
		service := &InstructionService{}
		snapshot := instructionTestSnapshot(true, 7)
		policy := snapshot.Policies[7]
		policy.Hashes = []instructionPolicyHash{hash}
		snapshot.Policies[7] = policy
		service.snapshot.Store(snapshot)

		decision := service.EvaluateInstruction(context.Background(), Request{
			Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "gpt-test",
			InstructionBody: []byte(`{"model":"gpt-test","instructions":"trusted"}`),
		})
		require.True(t, decision.Applicable)
		require.False(t, decision.Allow)
		require.Equal(t, "hash_mismatch", decision.Reason)
	}
}

func TestInstructionService47KBLatencyBudget(t *testing.T) {
	service := &InstructionService{}
	service.snapshot.Store(instructionTestSnapshot(true, 7, "trusted"))
	filler := strings.Repeat("a", 47*1024)
	request := Request{
		Protocol: instructionAuditProtocol, UserID: 7, GroupID: instructionTestGroupID(7), Model: "gpt-test",
		InstructionBody: []byte(`{"model":"gpt-test","instructions":"trusted","metadata":"` + filler + `"}`),
	}
	for range 20 {
		require.True(t, service.EvaluateInstruction(context.Background(), request).Allow)
	}
	latencies := make([]time.Duration, 300)
	for index := range latencies {
		startedAt := time.Now()
		require.True(t, service.EvaluateInstruction(context.Background(), request).Allow)
		latencies[index] = time.Since(startedAt)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	require.Less(t, latencies[284], 10*time.Millisecond)
	require.Less(t, latencies[296], 25*time.Millisecond)
}

func TestInstructionAuditEmailContainsOnlyMetadataAndDigests(t *testing.T) {
	userID, apiKeyID := int64(3), int64(9)
	event := &InstructionEvent{
		ID: 17, RequestID: "req-17", UserID: &userID, UserEmailSnapshot: "user@example.test", APIKeyID: &apiKeyID,
		Model: "gpt-test", Reason: "hash_mismatch", ConfigVersion: 4, CreatedAt: time.Unix(1_800_000_000, 0).UTC(),
		Instructions: InstructionFieldResult{Present: true, SHA256: strings.Repeat("a", 64), Result: "mismatch"},
		Input1:       InstructionFieldResult{Present: true, SHA256: strings.Repeat("b", 64), Result: "mismatch"},
	}
	body := buildInstructionAuditEmail(event)
	require.Contains(t, body, strings.Repeat("a", 64))
	require.Contains(t, body, strings.Repeat("b", 64))
	for _, forbidden := range []string{"Bearer ", "sk-secret", "raw instruction text", "Authorization"} {
		require.NotContains(t, body, forbidden)
	}
}

func TestNormalizeInstructionHashDefaultsToCandidate(t *testing.T) {
	request, err := normalizeInstructionHashRequest(CreateInstructionHashRequest{
		Digest: strings.Repeat("A", 64), Name: "Codex stable",
	})
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("a", 64), request.Digest)
	require.Equal(t, "candidate", request.Status)
}

func TestInstructionAuditRecipientRetrySkipsCompletedDeliveries(t *testing.T) {
	require.True(t, instructionRecipientAlreadySent([]int64{3, 7}, 7))
	require.False(t, instructionRecipientAlreadySent([]int64{3, 7}, 9))
}
