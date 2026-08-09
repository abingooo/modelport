package securityaudit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	modelportservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type recordingInstructionNotificationEnqueuer struct {
	inputs []modelportservice.SecurityNotificationEnqueueInput
}

func (r *recordingInstructionNotificationEnqueuer) Enqueue(
	_ context.Context,
	input modelportservice.SecurityNotificationEnqueueInput,
) error {
	r.inputs = append(r.inputs, input)
	return nil
}

func (r *recordingInstructionNotificationEnqueuer) Prepare(
	_ context.Context,
	input modelportservice.SecurityNotificationEnqueueInput,
) ([]modelportservice.SecurityNotificationAudienceInput, error) {
	items := make([]modelportservice.SecurityNotificationAudienceInput, 0, 2)
	if !input.SkipUser {
		items = append(items, modelportservice.SecurityNotificationAudienceInput{
			SourceType: input.SourceType, Audience: "user", UserID: input.UserID,
			Recipients: []string{input.UserEmail}, TemplateEvent: input.UserTemplate,
			Variables: input.Variables,
		})
	}
	if !input.SkipOps {
		items = append(items, modelportservice.SecurityNotificationAudienceInput{
			SourceType: input.SourceType, Audience: "ops", Recipients: []string{"ops@example.test"},
			TemplateEvent: input.OpsTemplate, Variables: input.Variables,
		})
	}
	return items, nil
}

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

func TestInstructionServiceRuleSetExceptions(t *testing.T) {
	const groupID int64 = 42
	snapshot := instructionTestSnapshot(true, groupID)
	policy := snapshot.Policies[groupID]
	policy.AllowEmptyFields = true
	policy.AllowedUsers = map[int64]struct{}{77: {}}
	snapshot.Policies[groupID] = policy
	service := &InstructionService{}
	service.snapshot.Store(snapshot)

	whitelisted := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 77, GroupID: instructionTestGroupID(groupID),
		InstructionBody: []byte(`{"instructions":"unlisted"}`),
	})
	require.True(t, whitelisted.Applicable)
	require.True(t, whitelisted.Allow)
	require.Equal(t, "user_allowlist", whitelisted.Reason)
	require.Equal(t, "not_checked", whitelisted.Instructions.Result)

	malformedWhitelisted := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, UserID: 77, GroupID: instructionTestGroupID(groupID),
		InstructionBody: []byte(`{`),
	})
	require.True(t, malformedWhitelisted.Applicable)
	require.False(t, malformedWhitelisted.Allow)
	require.Equal(t, "invalid_json", malformedWhitelisted.Reason)

	for _, body := range []string{
		`{}`,
		`{"instructions":""}`,
		`{"input":[{}]}`,
		`{"instructions":"","input":[{}, {"content":[{"type":"input_text","text":""}]}]}`,
	} {
		decision := service.EvaluateInstruction(context.Background(), Request{
			Protocol: instructionAuditProtocol, UserID: 78, GroupID: instructionTestGroupID(groupID),
			InstructionBody: []byte(body),
		})
		require.True(t, decision.Allow, body)
		require.Equal(t, "empty_fields_allowed", decision.Reason)
	}

	for _, body := range []string{
		`{"instructions":null}`,
		`{"instructions":" "}`,
		`{"input":null}`,
		`{"input":[{}, {"content":[{"type":"input_image"}]}]}`,
	} {
		decision := service.EvaluateInstruction(context.Background(), Request{
			Protocol: instructionAuditProtocol, UserID: 78, GroupID: instructionTestGroupID(groupID),
			InstructionBody: []byte(body),
		})
		require.False(t, decision.Allow, body)
	}
}

func TestInstructionServiceClientScopeRequiresGroupAndDetectedClient(t *testing.T) {
	const groupID int64 = 42
	scope := instructionPolicyScope{GroupID: groupID, ClientType: InstructionClientCodexCLI}
	snapshot := &instructionSnapshot{
		Enabled: true, ConfigVersion: 8, LoadedAt: time.Now().UTC(),
		AuditedGroups: map[int64]struct{}{}, Policies: map[int64]instructionPolicy{},
		AuditedClientScopes: map[instructionPolicyScope]struct{}{scope: {}},
		ClientPolicies: map[instructionPolicyScope]instructionPolicy{
			scope: {RuleSetIDs: []int64{21}, Hashes: []instructionPolicyHash{allowedDigest("trusted")}},
		},
	}
	service := &InstructionService{}
	service.snapshot.Store(snapshot)

	matching := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), UserAgent: "codex_cli_rs/0.145.0",
		InstructionBody: []byte(`{"instructions":"trusted"}`),
	})
	require.True(t, matching.Applicable)
	require.True(t, matching.Allow)
	require.Equal(t, []int64{21}, matching.RuleSetIDs)

	nonMatching := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), UserAgent: "opencode/1.0",
		InstructionBody: []byte(`{`),
	})
	require.False(t, nonMatching.Applicable)
	require.True(t, nonMatching.Allow)

	wrongGroup := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(99), UserAgent: "codex_cli_rs/0.145.0",
		InstructionBody: []byte(`{`),
	})
	require.False(t, wrongGroup.Applicable)
	require.True(t, wrongGroup.Allow)
}

func TestInstructionServiceSupportsEveryDetectedClientScope(t *testing.T) {
	const groupID int64 = 42
	tests := []struct {
		name            string
		clientType      string
		userAgent       string
		trustedInternal bool
	}{
		{name: "codex vscode", clientType: InstructionClientCodexVSCode, userAgent: "codex_vscode/1.0"},
		{name: "codex cli", clientType: InstructionClientCodexCLI, userAgent: "codex_cli_rs/1.0"},
		{name: "codex desktop", clientType: InstructionClientCodexDesktop, userAgent: "Codex Desktop/1.0"},
		{name: "opencode", clientType: InstructionClientOpenCode, userAgent: "opencode/1.0"},
		{name: "modelport internal", clientType: InstructionClientModelPortInternal, userAgent: "forged/1.0", trustedInternal: true},
		{name: "other", clientType: InstructionClientOther, userAgent: "curl/8.0"},
		{name: "unknown", clientType: InstructionClientUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope := instructionPolicyScope{GroupID: groupID, ClientType: test.clientType}
			snapshot := &instructionSnapshot{
				Enabled: true, ConfigVersion: 8, LoadedAt: time.Now().UTC(),
				AuditedGroups: map[int64]struct{}{}, Policies: map[int64]instructionPolicy{},
				AuditedClientScopes: map[instructionPolicyScope]struct{}{scope: {}},
				ClientPolicies: map[instructionPolicyScope]instructionPolicy{
					scope: {RuleSetIDs: []int64{21}, Hashes: []instructionPolicyHash{allowedDigest("trusted")}},
				},
			}
			service := &InstructionService{}
			service.snapshot.Store(snapshot)
			decision := service.EvaluateInstruction(context.Background(), Request{
				Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID),
				UserAgent: test.userAgent, TrustedInternalClient: test.trustedInternal,
				InstructionBody: []byte(`{"instructions":"trusted"}`),
			})
			require.True(t, decision.Applicable)
			require.True(t, decision.Allow)
			require.Equal(t, []int64{21}, decision.RuleSetIDs)
		})
	}
}

func TestInstructionServiceUnionsWildcardAndClientSpecificPolicies(t *testing.T) {
	const groupID int64 = 7
	snapshot := instructionTestSnapshot(true, groupID, "shared")
	scope := instructionPolicyScope{GroupID: groupID, ClientType: InstructionClientOpenCode}
	snapshot.AuditedClientScopes = map[instructionPolicyScope]struct{}{scope: {}}
	snapshot.ClientPolicies = map[instructionPolicyScope]instructionPolicy{
		scope: {RuleSetIDs: []int64{12}, Hashes: []instructionPolicyHash{allowedDigest("opencode-only")}},
	}
	service := &InstructionService{}
	service.snapshot.Store(snapshot)

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID), UserAgent: "opencode/1.0",
		InstructionBody: []byte(`{"instructions":"opencode-only"}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
	require.Equal(t, []int64{11, 12}, decision.RuleSetIDs)
}

func TestInstructionServiceExpiredAIScopeDoesNotOverridePermanentRule(t *testing.T) {
	const groupID int64 = 7
	digest := allowedDigest("shared")
	snapshot := instructionTestSnapshot(true, groupID)
	snapshot.Policies[groupID] = instructionPolicy{
		RuleSetIDs: []int64{11},
		Hashes:     []instructionPolicyHash{digest},
	}
	scope := instructionPolicyScope{GroupID: groupID, ClientType: InstructionClientCodexCLI}
	snapshot.AuditedClientScopes = map[instructionPolicyScope]struct{}{scope: {}}
	expired := digest
	expired.ValidUntil = time.Now().UTC().Add(-time.Hour)
	snapshot.ClientPolicies = map[instructionPolicyScope]instructionPolicy{
		scope: {RuleSetIDs: []int64{12}, Hashes: []instructionPolicyHash{expired}},
	}
	service := &InstructionService{}
	service.snapshot.Store(snapshot)

	decision := service.EvaluateInstruction(context.Background(), Request{
		Protocol: instructionAuditProtocol, GroupID: instructionTestGroupID(groupID),
		UserAgent: "codex_cli_rs/0.145.0", InstructionBody: []byte(`{"instructions":"shared"}`),
	})
	require.True(t, decision.Applicable)
	require.True(t, decision.Allow)
	require.Equal(t, "instructions_match", decision.Reason)
	require.Equal(t, []int64{11, 12}, decision.RuleSetIDs)
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
	mock.ExpectCommit()
	mock.ExpectClose()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	service.recordBlocked(canceled, request, decision)
	require.EqualValues(t, 17, decision.EventID)
	require.Zero(t, service.pendingPersistFailures.Load())
	require.Zero(t, service.pendingStatisticsLosses.Load())
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionServiceFailsClosedWhenAllowedOutcomeCannotPersist(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	service := NewInstructionService(NewInstructionRepository(db), nil, nil)
	decision := &InstructionDecision{
		Applicable:    true,
		Allow:         true,
		Reason:        "instructions_match",
		InitialReason: "instructions_match",
		FinalReason:   "instructions_match",
		FinalOutcome:  InstructionOutcomeHashPass,
		PolicyAction:  "hash_match",
		Instructions: InstructionFieldResult{
			Present: true,
			SHA256:  sha256Hex("trusted"),
			Result:  "match",
		},
		Input1: InstructionFieldResult{Result: "not_checked"},
	}

	mock.ExpectBegin().WillReturnError(errors.New("database unavailable"))
	service.recordOutcomeOrFailClosed(context.Background(), Request{RequestID: "req-fail-closed"}, decision)

	require.False(t, decision.Allow)
	require.True(t, decision.Unavailable)
	require.Equal(t, "config_unavailable", decision.Reason)
	require.Equal(t, "config_unavailable", decision.FinalReason)
	require.Equal(t, InstructionOutcomeBlocked, decision.FinalOutcome)
	require.Equal(t, InstructionPolicyActionBlock, decision.PolicyAction)
	require.EqualValues(t, 1, service.pendingPersistFailures.Load())
	require.EqualValues(t, 1, service.pendingStatisticsLosses.Load())
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNormalizeInstructionHashRequestSeparatesManualAndImportSources(t *testing.T) {
	manual, err := normalizeInstructionHashRequest(CreateInstructionHashRequest{
		RawContent: "exact plaintext", Name: "manual", Status: "active",
	})
	require.NoError(t, err)
	require.Equal(t, "manual", manual.SourceType)
	require.Equal(t, sha256Hex("exact plaintext"), manual.Digest)

	missingRaw, err := normalizeInstructionHashRequest(CreateInstructionHashRequest{
		Digest: sha256Hex("digest only"), Name: "missing raw", Status: "active",
	})
	require.NoError(t, err)
	require.Equal(t, "manual", missingRaw.SourceType)
	_, err = (&InstructionService{}).CreateHash(context.Background(), missingRaw, 1)
	require.Equal(t, "instruction_audit_manual_raw_required", infraerrors.Reason(err))

	imported, err := normalizeInstructionHashRequest(CreateInstructionHashRequest{
		Digest: sha256Hex("digest only"), SourceType: "import", Name: "imported", Status: "active",
	})
	require.NoError(t, err)
	require.Equal(t, "import", imported.SourceType)
	require.Empty(t, imported.RawContent)

	_, err = normalizeInstructionHashRequest(CreateInstructionHashRequest{
		Digest: sha256Hex("invalid source"), SourceType: "ai_review", Name: "invalid", Status: "active",
	})
	require.Equal(t, "instruction_audit_invalid_hash_source_type", infraerrors.Reason(err))
}

func TestInstructionOperationalCountersRetryWithoutDoubleCounting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	service := NewInstructionService(NewInstructionRepository(db), nil, nil)
	service.pendingPersistFailures.Add(2)
	service.pendingStatisticsLosses.Add(3)

	mock.ExpectExec("INSERT INTO instruction_audit_operational_counters").
		WithArgs(int64(2), int64(3)).
		WillReturnError(errors.New("database unavailable"))
	require.Error(t, service.flushOperationalCounters(context.Background()))
	require.EqualValues(t, 2, service.pendingPersistFailures.Load())
	require.EqualValues(t, 3, service.pendingStatisticsLosses.Load())

	mock.ExpectExec("INSERT INTO instruction_audit_operational_counters").
		WithArgs(int64(2), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, service.flushOperationalCounters(context.Background()))
	require.Zero(t, service.pendingPersistFailures.Load())
	require.Zero(t, service.pendingStatisticsLosses.Load())

	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionOutcomeNotificationsRespectAudienceAndAlertPolicy(t *testing.T) {
	request := Request{
		RequestID: "notification-request", UserID: 7, UserEmail: "user@example.test",
		APIKeyID: 9, GroupID: instructionTestGroupID(3), GroupName: "OpenAI",
		InstructionClientType: InstructionClientCodexCLI, Model: "gpt-test",
	}
	baseDecision := InstructionDecision{
		AlertEnabled: true, InitialReason: "hash_mismatch", FinalReason: "hash_mismatch",
		PolicyAction: InstructionPolicyActionBlock, ConfigVersion: 12,
		Instructions: InstructionFieldResult{Present: true, SHA256: sha256Hex("notification-canary"), Result: "mismatch", Plaintext: "notification-plaintext-canary"},
		Input1:       InstructionFieldResult{Result: "missing"},
	}

	tests := []struct {
		name         string
		outcome      string
		alert        bool
		wantCalls    int
		wantSkipUser bool
	}{
		{name: "blocked notifies user and ops", outcome: InstructionOutcomeBlocked, alert: true, wantCalls: 1},
		{name: "policy allow notifies only ops", outcome: InstructionOutcomePolicyAllow, alert: true, wantCalls: 1, wantSkipUser: true},
		{name: "disabled alert queues nothing", outcome: InstructionOutcomeBlocked, alert: false},
		{name: "normal pass queues nothing", outcome: InstructionOutcomeHashPass, alert: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &recordingInstructionNotificationEnqueuer{}
			service := &InstructionService{notifications: recorder}
			decision := baseDecision
			decision.FinalOutcome = test.outcome
			decision.AlertEnabled = test.alert
			service.enqueueInstructionOutcomeNotifications(context.Background(), request, &decision, 41)
			require.Len(t, recorder.inputs, test.wantCalls)
			if test.wantCalls == 0 {
				return
			}
			input := recorder.inputs[0]
			require.Equal(t, test.wantSkipUser, input.SkipUser)
			require.False(t, input.SkipOps)
			require.Equal(t, modelportservice.NotificationEmailEventInstructionAuditUserNotice, input.UserTemplate)
			require.Equal(t, modelportservice.NotificationEmailEventInstructionAuditOpsNotice, input.OpsTemplate)
			require.Equal(t, "41", input.Variables["event_id"])
			for _, value := range input.Variables {
				require.NotContains(t, value, "notification-plaintext-canary")
			}
		})
	}
}

func TestInstructionAuditOperationalLogsExcludeRequestPlaintext(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	service := NewInstructionService(NewInstructionRepository(db), nil, nil)
	const canary = "MODELPORT-INSTRUCTION-PLAINTEXT-CANARY-17013"
	decision := &InstructionDecision{
		Allow: false, InitialReason: "hash_mismatch", FinalReason: "hash_mismatch",
		FinalOutcome: InstructionOutcomeBlocked, PolicyAction: InstructionPolicyActionBlock,
		Instructions: InstructionFieldResult{Present: true, SHA256: sha256Hex(canary), Result: "mismatch", Plaintext: canary},
		Input1:       InstructionFieldResult{Result: "missing"},
	}
	request := Request{
		RequestID: "log-canary-request", UserID: 7, APIKeyID: 9,
		Body: []byte(canary), InstructionBody: []byte(canary),
	}

	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	mock.ExpectBegin().WillReturnError(errors.New("database unavailable"))
	require.Error(t, service.recordOutcome(context.Background(), request, decision))
	require.Contains(t, output.String(), "instruction_audit.record_event_failed")
	require.NotContains(t, output.String(), canary)
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInstructionServiceMapsReferencedResourcesToConflict(t *testing.T) {
	t.Run("hash", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM instruction_audit_hashes WHERE id = \$1 FOR UPDATE`).
			WithArgs(int64(9)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM instruction_audit_rule_set_hashes WHERE hash_id = \$1`).
			WithArgs(int64(9)).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectRollback()

		err = NewInstructionService(NewInstructionRepository(db), nil, nil).DeleteHash(context.Background(), 9)
		require.Equal(t, http.StatusConflict, infraerrors.Code(err))
		require.Equal(t, "instruction_audit_hash_referenced", infraerrors.Reason(err))
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rule set", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT system_managed FROM instruction_audit_rule_sets WHERE id = \$1 FOR UPDATE`).
			WithArgs(int64(11)).
			WillReturnRows(sqlmock.NewRows([]string{"system_managed"}).AddRow(false))
		mock.ExpectQuery(`(?s)SELECT.*instruction_audit_group_bindings.*instruction_audit_bindings`).
			WithArgs(int64(11)).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectRollback()

		err = NewInstructionService(NewInstructionRepository(db), nil, nil).DeleteRuleSet(context.Background(), 11)
		require.Equal(t, http.StatusConflict, infraerrors.Code(err))
		require.Equal(t, "instruction_audit_rule_set_referenced", infraerrors.Reason(err))
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestInstructionEvidenceCopySourcesIncludeEventID(t *testing.T) {
	require.True(t, validInstructionEvidenceCopySource("event_id"))
	require.False(t, validInstructionEvidenceCopySource("authorization"))
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

func TestEffectiveInstructionBodyLimitUsesApplicationMinimum(t *testing.T) {
	require.EqualValues(t, 64<<20, effectiveInstructionBodyLimit(64<<20, 256<<20))
	require.EqualValues(t, 32<<20, effectiveInstructionBodyLimit(64<<20, 32<<20))
	require.EqualValues(t, 64<<20, effectiveInstructionBodyLimit(64<<20, 0))
}

func TestMergeInstructionPolicyHashValidityUsesUnionWindow(t *testing.T) {
	now := time.Now().UTC()
	digest := allowedDigest("shared").Digest
	merged := mergeInstructionPolicyHashValidity(
		instructionPolicyHash{Digest: digest, ValidFrom: now.Add(time.Hour), ValidUntil: now.Add(2 * time.Hour)},
		instructionPolicyHash{Digest: digest, ValidFrom: now, ValidUntil: now.Add(3 * time.Hour)},
	)
	require.Equal(t, now, merged.ValidFrom)
	require.Equal(t, now.Add(3*time.Hour), merged.ValidUntil)

	merged = mergeInstructionPolicyHashValidity(merged, instructionPolicyHash{Digest: digest})
	require.True(t, merged.ValidFrom.IsZero())
	require.True(t, merged.ValidUntil.IsZero())
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

func TestInstructionServiceRequestBodyBudgetOnlyAppliesWhileEnabled(t *testing.T) {
	service := &InstructionService{}
	service.configureInstructionRequestBodyBudget(InstructionDefaultMaxInflightBodyBytes)
	service.snapshot.Store(instructionTestSnapshot(false, 7, "trusted"))
	require.Nil(t, service.RequestBodyMemoryBudget())

	service.snapshot.Store(instructionTestSnapshot(true, 7, "trusted"))
	budget := service.RequestBodyMemoryBudget()
	require.NotNil(t, budget)
	require.Equal(t, InstructionDefaultMaxInflightBodyBytes, budget.Capacity())
	service.configureInstructionRequestBodyBudget(InstructionDefaultMaxInflightBodyBytes * 2)
	require.Same(t, budget, service.RequestBodyMemoryBudget())
	require.Equal(t, InstructionDefaultMaxInflightBodyBytes*2, budget.Capacity())
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

func TestNormalizeInstructionHashDefaultsToCandidate(t *testing.T) {
	request, err := normalizeInstructionHashRequest(CreateInstructionHashRequest{
		Digest: strings.Repeat("A", 64), SourceType: "import", Name: "Codex stable",
	})
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("a", 64), request.Digest)
	require.Equal(t, "candidate", request.Status)
}
