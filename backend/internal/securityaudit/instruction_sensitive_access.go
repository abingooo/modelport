package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type InstructionSensitiveGrant struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Email       string    `json:"email"`
	Username    string    `json:"username"`
	UserStatus  string    `json:"user_status"`
	TotpEnabled bool      `json:"totp_enabled"`
	Effective   bool      `json:"effective"`
	GrantedBy   *int64    `json:"granted_by,omitempty"`
	GrantSource string    `json:"grant_source"`
	GrantReason string    `json:"grant_reason"`
	GrantedAt   time.Time `json:"granted_at"`
}

type InstructionSensitiveCapability struct {
	UserID      int64     `json:"user_id"`
	HasAccess   bool      `json:"has_access"`
	CanManage   bool      `json:"can_manage"`
	GrantID     *int64    `json:"grant_id,omitempty"`
	GrantSource string    `json:"grant_source,omitempty"`
	GrantedAt   time.Time `json:"granted_at,omitempty"`
}

func (h *InstructionAdminHandler) AuthorizeInstructionSensitiveAccess(
	ctx context.Context,
	userID int64,
	authMethod string,
) (int64, error) {
	if h == nil || h.service == nil {
		return 0, instructionSensitiveUnavailable()
	}
	return h.service.AuthorizeInstructionSensitiveAccess(ctx, userID, authMethod)
}

func (s *InstructionService) AuthorizeInstructionSensitiveAccess(
	ctx context.Context,
	userID int64,
	authMethod string,
) (int64, error) {
	if authMethod == service.AuditAuthMethodAdminAPIKey {
		return 0, infraerrors.Forbidden(
			"STEP_UP_ADMIN_API_KEY_FORBIDDEN",
			"Admin API key cannot access sensitive instruction content",
		)
	}
	if authMethod != service.AuditAuthMethodJWT || userID <= 0 {
		return 0, infraerrors.Forbidden(
			"INSTRUCTION_SENSITIVE_HUMAN_SESSION_REQUIRED",
			"A signed-in administrator session is required for sensitive instruction content",
		)
	}
	if s == nil || s.repository == nil {
		return 0, instructionSensitiveUnavailable()
	}
	grant, err := s.repository.GetActiveInstructionSensitiveGrant(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, instructionSensitiveRequired()
	}
	if err != nil {
		return 0, instructionSensitiveUnavailable()
	}
	if !instructionSensitiveGrantValidAt(grant, time.Now().UTC()) {
		return 0, instructionSensitiveRequired()
	}
	return grant.ID, nil
}

func (s *InstructionService) GetInstructionSensitiveCapability(
	ctx context.Context,
	userID int64,
	authMethod string,
) (*InstructionSensitiveCapability, error) {
	if authMethod == service.AuditAuthMethodAdminAPIKey {
		return nil, infraerrors.Forbidden(
			"STEP_UP_ADMIN_API_KEY_FORBIDDEN",
			"Admin API key cannot inspect sensitive instruction content authorization",
		)
	}
	if authMethod != service.AuditAuthMethodJWT || userID <= 0 {
		return nil, infraerrors.Forbidden(
			"INSTRUCTION_SENSITIVE_HUMAN_SESSION_REQUIRED",
			"A signed-in administrator session is required",
		)
	}
	if s == nil || s.repository == nil {
		return nil, instructionSensitiveUnavailable()
	}
	result := &InstructionSensitiveCapability{UserID: userID}
	grant, err := s.repository.GetActiveInstructionSensitiveGrant(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return nil, instructionSensitiveUnavailable()
	}
	if instructionSensitiveGrantValidAt(grant, time.Now().UTC()) {
		result.HasAccess = true
		result.CanManage = true
		result.GrantID = &grant.ID
		result.GrantSource = grant.GrantSource
		result.GrantedAt = grant.GrantedAt
	}
	return result, nil
}

func (s *InstructionService) ListInstructionSensitiveGrants(
	ctx context.Context,
	actorID int64,
) ([]InstructionSensitiveGrant, error) {
	if _, _, err := s.requireInstructionSensitiveAuthorization(ctx, actorID); err != nil {
		return nil, err
	}
	items, err := s.repository.ListActiveInstructionSensitiveGrants(ctx)
	if err != nil {
		return nil, instructionSensitiveUnavailable()
	}
	return items, nil
}

func (s *InstructionService) GrantInstructionSensitiveAccess(
	ctx context.Context,
	actorID int64,
	targetUserID int64,
	reason string,
) (*InstructionSensitiveGrant, error) {
	_, authorization, err := s.requireInstructionSensitiveAuthorization(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if targetUserID <= 0 {
		return nil, infraerrors.BadRequest("INSTRUCTION_SENSITIVE_INVALID_USER", "Target administrator is invalid")
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 255 {
		return nil, infraerrors.BadRequest("INSTRUCTION_SENSITIVE_REASON_TOO_LONG", "Grant reason is too long")
	}
	item, err := s.repository.GrantInstructionSensitiveAccess(
		ctx, actorID, authorization.GrantID, targetUserID, reason,
	)
	return item, mapInstructionSensitiveMutationError(err)
}

func (s *InstructionService) RevokeInstructionSensitiveAccess(
	ctx context.Context,
	actorID int64,
	targetUserID int64,
	reason string,
) (*InstructionSensitiveGrant, error) {
	_, authorization, err := s.requireInstructionSensitiveAuthorization(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if targetUserID <= 0 {
		return nil, infraerrors.BadRequest("INSTRUCTION_SENSITIVE_INVALID_USER", "Target administrator is invalid")
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 255 {
		return nil, infraerrors.BadRequest("INSTRUCTION_SENSITIVE_REASON_TOO_LONG", "Revoke reason is too long")
	}
	item, err := s.repository.RevokeInstructionSensitiveAccess(
		ctx, actorID, authorization.GrantID, targetUserID, reason,
	)
	return item, mapInstructionSensitiveMutationError(err)
}

func (s *InstructionService) requireInstructionSensitiveAuthorization(
	ctx context.Context,
	actorID int64,
) (context.Context, servermiddleware.InstructionSensitiveAuthorization, error) {
	if s == nil || s.repository == nil {
		return ctx, servermiddleware.InstructionSensitiveAuthorization{}, instructionSensitiveUnavailable()
	}
	authorization, ok := instructionSensitiveAuthorizationFromContext(ctx)
	if !ok || authorization.AuthMethod != service.AuditAuthMethodJWT ||
		authorization.UserID != actorID || authorization.GrantID <= 0 {
		return ctx, servermiddleware.InstructionSensitiveAuthorization{}, instructionSensitiveRequired()
	}
	grant, err := s.repository.GetActiveInstructionSensitiveGrantByID(
		ctx, actorID, authorization.GrantID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ctx, servermiddleware.InstructionSensitiveAuthorization{}, instructionSensitiveRequired()
	}
	if err != nil {
		return ctx, servermiddleware.InstructionSensitiveAuthorization{}, instructionSensitiveUnavailable()
	}
	if !instructionSensitiveGrantValidAt(grant, time.Now().UTC()) {
		return ctx, servermiddleware.InstructionSensitiveAuthorization{}, instructionSensitiveRequired()
	}
	return ctx, authorization, nil
}

func instructionSensitiveAuthorizationFromContext(
	ctx context.Context,
) (servermiddleware.InstructionSensitiveAuthorization, bool) {
	return servermiddleware.InstructionSensitiveAuthorizationFromContext(ctx)
}

func instructionSensitiveRequired() error {
	return infraerrors.Forbidden(
		"INSTRUCTION_SENSITIVE_ACCESS_REQUIRED",
		"Sensitive instruction content access is required",
	)
}

func instructionSensitiveUnavailable() error {
	return infraerrors.ServiceUnavailable(
		"INSTRUCTION_SENSITIVE_ACCESS_UNAVAILABLE",
		"Sensitive content authorization is unavailable",
	)
}

func mapInstructionSensitiveMutationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return instructionSensitiveRequired()
	case errors.Is(err, errInstructionSensitiveGrantNotFound):
		return infraerrors.NotFound("INSTRUCTION_SENSITIVE_GRANT_NOT_FOUND", "Sensitive access grant was not found")
	case errors.Is(err, errInstructionSensitiveTargetNotAdmin):
		return infraerrors.BadRequest("INSTRUCTION_SENSITIVE_TARGET_NOT_ADMIN", "Target user must be an administrator")
	case errors.Is(err, errInstructionSensitiveTargetInactive):
		return infraerrors.Conflict("INSTRUCTION_SENSITIVE_TARGET_INACTIVE", "Target administrator must be active")
	case errors.Is(err, errInstructionSensitiveTargetTotpNeeded):
		return infraerrors.Conflict("INSTRUCTION_SENSITIVE_TARGET_TOTP_REQUIRED", "Target administrator must enable TOTP first")
	case errors.Is(err, errInstructionSensitiveLastHolder):
		return infraerrors.Conflict("INSTRUCTION_SENSITIVE_LAST_HOLDER", "The final effective sensitive-access holder cannot be revoked")
	default:
		return instructionSensitiveUnavailable()
	}
}
