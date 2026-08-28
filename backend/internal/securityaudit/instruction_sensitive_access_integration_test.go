package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func insertInstructionSensitiveTestGrant(
	t *testing.T,
	db *sql.DB,
	userID int64,
	source string,
) int64 {
	t.Helper()
	var grantID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_sensitive_access_grants
			(subject_user_id, subject_email_snapshot, grant_source, grant_reason)
		SELECT id, email, $2, 'test authorization' FROM users WHERE id = $1
		RETURNING id`, userID, source).Scan(&grantID))
	return grantID
}

func instructionSensitiveTestContext(t *testing.T, db *sql.DB, userID int64) context.Context {
	t.Helper()
	var grantID int64
	require.NoError(t, db.QueryRow(`
		SELECT id FROM instruction_audit_sensitive_access_grants
		WHERE subject_user_id = $1 AND revoked_at IS NULL
		ORDER BY id DESC LIMIT 1`, userID).Scan(&grantID))
	return servermiddleware.WithInstructionSensitiveAuthorization(
		context.Background(),
		servermiddleware.InstructionSensitiveAuthorization{
			GrantID: grantID, UserID: userID, AuthMethod: service.AuditAuthMethodJWT,
			AuthorizationResult: "granted",
		},
	)
}

func TestInstructionAuditPostgresSensitiveMigrationBootstrapsEarliestActiveAdmin(t *testing.T) {
	db := openInstructionAuditSchema(t)
	for _, name := range []string{
		"198_instruction_audit.sql",
		"199_instruction_audit_group_scope.sql",
		"200_instruction_audit_review_notifications.sql",
		"201_instruction_audit_client_scope.sql",
		"203_instruction_audit_rule_exceptions.sql",
		"204_instruction_audit_outcomes_and_policies.sql",
		"205_instruction_audit_raw_ai_translation.sql",
		"206_instruction_audit_outcome_aggregation.sql",
		"208_instruction_audit_translation_execution.sql",
		"209_instruction_audit_aggregate_retention.sql",
		"210_instruction_audit_aggregate_shards.sql",
		"211_instruction_audit_repartition_legacy_aggregates.sql",
		"212_instruction_audit_hash_scope_lifecycle.sql",
	} {
		applyInstructionAuditMigration(t, db, name)
	}
	now := time.Now().UTC()
	var earliestID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO users (email, username, role, status, created_at)
		VALUES ('earliest@example.test', 'earliest', 'admin', 'active', $1)
		RETURNING id`, now.Add(-2*time.Hour)).Scan(&earliestID))
	_, err := db.Exec(`
		INSERT INTO users (email, username, role, status, created_at)
		VALUES
			('later@example.test', 'later', 'admin', 'active', $1),
			('disabled@example.test', 'disabled', 'admin', 'disabled', $2),
			('user@example.test', 'user', 'user', 'active', $2)`,
		now.Add(-time.Hour), now.Add(-3*time.Hour))
	require.NoError(t, err)

	applyInstructionAuditMigration(t, db, "213_instruction_audit_sensitive_access_grants.sql")
	applyInstructionAuditMigration(t, db, "213_instruction_audit_sensitive_access_grants.sql")

	var count int64
	var subjectID int64
	var source string
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*), MIN(subject_user_id), MIN(grant_source)
		FROM instruction_audit_sensitive_access_grants`).Scan(&count, &subjectID, &source))
	require.EqualValues(t, 1, count)
	require.Equal(t, earliestID, subjectID)
	require.Equal(t, "migration_bootstrap", source)
}

func TestInstructionSensitiveGrantLifecycleAndLastHolderProtection(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	instructionService := NewInstructionService(repository, nil, nil)
	actorID := insertInstructionAuditUser(t, db, "holder@example.test", "admin")
	targetID := insertInstructionAuditUser(t, db, "target@example.test", "admin")
	actorGrantID := insertInstructionSensitiveTestGrant(t, db, actorID, "emergency_cli")
	actorContext := instructionSensitiveTestContext(t, db, actorID)

	capability, err := instructionService.GetInstructionSensitiveCapability(
		context.Background(), actorID, service.AuditAuthMethodJWT,
	)
	require.NoError(t, err)
	require.True(t, capability.HasAccess)
	require.Equal(t, actorGrantID, *capability.GrantID)

	targetGrant, err := instructionService.GrantInstructionSensitiveAccess(
		actorContext, actorID, targetID, "review duty",
	)
	require.NoError(t, err)
	require.True(t, targetGrant.Effective)
	require.Equal(t, "manual", targetGrant.GrantSource)
	_, err = db.Exec(`UPDATE users SET totp_enabled = FALSE WHERE id = $1`, targetID)
	require.NoError(t, err)
	targetCapability, err := instructionService.GetInstructionSensitiveCapability(
		context.Background(), targetID, service.AuditAuthMethodJWT,
	)
	require.NoError(t, err)
	require.False(t, targetCapability.HasAccess)
	grants, err := instructionService.ListInstructionSensitiveGrants(actorContext, actorID)
	require.NoError(t, err)
	for _, grant := range grants {
		if grant.UserID == targetID {
			require.False(t, grant.Effective)
		}
	}
	_, err = instructionService.RevokeInstructionSensitiveAccess(
		actorContext, actorID, actorID, "must keep a TOTP-capable holder",
	)
	require.Equal(t, "INSTRUCTION_SENSITIVE_LAST_HOLDER", infraerrors.Reason(err))
	_, err = db.Exec(`UPDATE users SET totp_enabled = TRUE WHERE id = $1`, targetID)
	require.NoError(t, err)

	_, err = instructionService.RevokeInstructionSensitiveAccess(
		actorContext, actorID, targetID, "rotation",
	)
	require.NoError(t, err)
	_, err = repository.GetActiveInstructionSensitiveGrantByID(
		context.Background(), targetID, targetGrant.ID,
	)
	require.ErrorIs(t, err, sql.ErrNoRows)

	replacement, err := instructionService.GrantInstructionSensitiveAccess(
		actorContext, actorID, targetID, "rotation complete",
	)
	require.NoError(t, err)
	require.Greater(t, replacement.ID, targetGrant.ID)
	targetContext := instructionSensitiveTestContext(t, db, targetID)

	_, err = instructionService.RevokeInstructionSensitiveAccess(
		actorContext, actorID, actorID, "handoff",
	)
	require.NoError(t, err)
	_, _, err = instructionService.requireInstructionSensitiveAuthorization(actorContext, actorID)
	require.Equal(t, "INSTRUCTION_SENSITIVE_ACCESS_REQUIRED", infraerrors.Reason(err))

	_, err = instructionService.RevokeInstructionSensitiveAccess(
		targetContext, targetID, targetID, "remove final holder",
	)
	require.Equal(t, "INSTRUCTION_SENSITIVE_LAST_HOLDER", infraerrors.Reason(err))

	_, err = instructionService.GetInstructionSensitiveCapability(
		context.Background(), targetID, service.AuditAuthMethodAdminAPIKey,
	)
	require.Equal(t, "STEP_UP_ADMIN_API_KEY_FORBIDDEN", infraerrors.Reason(err))
}

func TestInstructionSensitiveAccessLogsLinkExactAuthorization(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	actorID := insertInstructionAuditUser(t, db, "audit-holder@example.test", "admin")
	grantID := insertInstructionSensitiveTestGrant(t, db, actorID, "emergency_cli")
	ctx := instructionSensitiveTestContext(t, db, actorID)

	require.NoError(t, repository.RecordSensitiveAccess(ctx, InstructionSensitiveAccess{
		ResourceType: "hash_raw", ResourceID: 101, ActorID: actorID, Action: "reveal", Succeeded: true,
	}))
	var loggedGrantID int64
	var authMethod, result string
	require.NoError(t, db.QueryRow(`
		SELECT grant_id, auth_method, authorization_result
		FROM instruction_audit_sensitive_access_logs
		WHERE resource_type = 'hash_raw' AND resource_id = 101`).Scan(
		&loggedGrantID, &authMethod, &result,
	))
	require.Equal(t, grantID, loggedGrantID)
	require.Equal(t, service.AuditAuthMethodJWT, authMethod)
	require.Equal(t, "granted", result)

	var eventID int64
	require.NoError(t, db.QueryRow(`
		INSERT INTO instruction_audit_events (request_id)
		VALUES ('sensitive-log-link') RETURNING id`).Scan(&eventID))
	require.NoError(t, repository.RecordEvidenceAccess(ctx, eventID, InstructionEvidenceAccess{
		ActorID: actorID, Action: "copy", Source: "instructions_plaintext", Succeeded: true,
	}))
	require.NoError(t, db.QueryRow(`
		SELECT grant_id, auth_method, authorization_result
		FROM instruction_audit_evidence_access_logs WHERE event_id = $1`, eventID).Scan(
		&loggedGrantID, &authMethod, &result,
	))
	require.Equal(t, grantID, loggedGrantID)
	require.Equal(t, service.AuditAuthMethodJWT, authMethod)
	require.Equal(t, "granted", result)
}

func TestInstructionSensitiveServiceRejectsMissingExactGrantContext(t *testing.T) {
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	instructionService := NewInstructionService(repository, nil, nil)
	actorID := insertInstructionAuditUser(t, db, "direct-call@example.test", "admin")
	insertInstructionSensitiveTestGrant(t, db, actorID, "emergency_cli")

	_, err := instructionService.RevealHashRaw(
		context.Background(), 1, InstructionSensitiveAccess{ActorID: actorID},
	)
	require.Equal(t, "INSTRUCTION_SENSITIVE_ACCESS_REQUIRED", infraerrors.Reason(err))
	require.False(t, errors.Is(err, sql.ErrNoRows))

	_, err = instructionService.CreateCandidateFromEvent(
		context.Background(), 1, CreateInstructionCandidateRequest{ReviewConfirmed: true}, actorID,
	)
	require.Equal(t, "INSTRUCTION_SENSITIVE_ACCESS_REQUIRED", infraerrors.Reason(err))
	_, err = instructionService.AddEventToRuleSet(
		context.Background(), 1,
		AddInstructionEventToRuleSetRequest{RuleSetID: 1, Sources: []string{"instructions"}, ReviewConfirmed: true},
		actorID,
	)
	require.Equal(t, "INSTRUCTION_SENSITIVE_ACCESS_REQUIRED", infraerrors.Reason(err))
}
