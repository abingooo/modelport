package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *InstructionService) GetHashDetail(ctx context.Context, hashID int64) (*InstructionHashEntry, error) {
	if hashID <= 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_hash_id", "哈希 ID 无效")
	}
	item, err := s.repository.GetHash(ctx, hashID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_hash_not_found", "哈希不存在")
	}
	if err != nil {
		return nil, err
	}
	item.Sources, err = s.repository.ListHashSources(ctx, hashID)
	if err != nil {
		return nil, err
	}
	item.Scopes, err = s.repository.ListHashScopes(ctx, hashID)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *InstructionService) ListAIReviewsForEvent(ctx context.Context, eventID int64) ([]InstructionAIReview, error) {
	if eventID <= 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_event_id", "审核事件 ID 无效")
	}
	if _, err := s.GetEvent(ctx, eventID); err != nil {
		return nil, err
	}
	return s.repository.ListAIReviewsForEvent(ctx, eventID)
}

func (s *InstructionService) RevealHashRaw(ctx context.Context, hashID int64, access InstructionSensitiveAccess) (*InstructionHashRawReview, error) {
	if _, _, err := s.requireInstructionSensitiveAuthorization(ctx, access.ActorID); err != nil {
		return nil, err
	}
	if hashID <= 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_hash_id", "哈希 ID 无效")
	}
	hash, err := s.repository.GetHash(ctx, hashID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_hash_not_found", "哈希不存在")
	}
	if err != nil {
		return nil, err
	}
	storage, err := s.repository.GetHashRaw(ctx, hashID)
	if err != nil {
		return nil, err
	}
	review := &InstructionHashRawReview{
		HashID: hashID, FieldName: hash.FieldName, RawStatus: storage.Status,
		ContentBytes: storage.ContentBytes, SHA256: hash.Digest, RawExpiresAt: storage.ExpiresAt,
	}
	fail := func(code string) (*InstructionHashRawReview, error) {
		access.ResourceType, access.ResourceID, access.Action = "hash_raw", hashID, "reveal"
		access.Succeeded, access.ErrorCode = false, code
		_ = s.repository.RecordSensitiveAccess(ctx, access)
		return review, infraerrors.BadRequest("instruction_audit_hash_raw_unavailable", "哈希原文当前不可查看")
	}
	if storage.Status != "stored" || len(storage.Ciphertext) == 0 {
		return fail(storage.Status)
	}
	if storage.ExpiresAt != nil && !time.Now().UTC().Before(*storage.ExpiresAt) {
		_, _ = s.repository.ExpireHashRawContents(ctx)
		return fail("expired")
	}
	if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
		return fail("encryption_unavailable")
	}
	plaintext, err := s.evidenceCipher.DecryptHashRaw(hash.Digest, storage.Ciphertext)
	if err != nil {
		return fail("decrypt_failed")
	}
	recomputed := sha256Hex(plaintext)
	review.RawContent = plaintext
	review.RecomputedSHA256 = recomputed
	review.DigestConsistent = recomputed == hash.Digest && len([]byte(plaintext)) == storage.ContentBytes
	if !review.DigestConsistent {
		return fail("digest_mismatch")
	}
	access.ResourceType, access.ResourceID, access.Action = "hash_raw", hashID, "reveal"
	access.Succeeded, access.ErrorCode = true, ""
	if err = s.repository.RecordSensitiveAccess(ctx, access); err != nil {
		return nil, err
	}
	return review, nil
}

func (s *InstructionService) RecordHashRawCopy(ctx context.Context, hashID int64, access InstructionSensitiveAccess) error {
	if _, _, err := s.requireInstructionSensitiveAuthorization(ctx, access.ActorID); err != nil {
		return err
	}
	if hashID <= 0 {
		return infraerrors.BadRequest("instruction_audit_invalid_hash_id", "哈希 ID 无效")
	}
	revealed, err := s.repository.HasSuccessfulHashRawReveal(ctx, hashID, access.ActorID)
	if err != nil {
		return err
	}
	access.ResourceType, access.ResourceID, access.Action = "hash_raw", hashID, "copy"
	if !revealed {
		access.Succeeded, access.ErrorCode = false, "reveal_required"
		_ = s.repository.RecordSensitiveAccess(ctx, access)
		return infraerrors.BadRequest("instruction_audit_hash_raw_review_required", "请先查看原文再复制")
	}
	access.Succeeded, access.ErrorCode = true, ""
	return s.repository.RecordSensitiveAccess(ctx, access)
}

func (s *InstructionService) ChangeHashStatus(ctx context.Context, hashID int64, status string, actorID int64, access InstructionSensitiveAccess) (*InstructionHashEntry, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "active" && status != "disabled" && status != "revoked" {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_hash_status", "哈希状态无效")
	}
	if hashID <= 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_hash_id", "哈希 ID 无效")
	}
	current, err := s.repository.GetHash(ctx, hashID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_hash_not_found", "哈希不存在")
	}
	if err != nil {
		return nil, err
	}
	if current.Status == "revoked" && status != "revoked" {
		return nil, infraerrors.Conflict("instruction_audit_hash_revoked", "已撤销哈希不能重新启用")
	}
	action := "disable"
	switch status {
	case "active":
		action = "promote"
	case "revoked":
		action = "revoke"
	}
	access.ResourceType, access.ResourceID, access.Action = "ai_hash", hashID, action
	access.ActorID = actorID
	version, err := s.repository.UpdateHashStatus(ctx, hashID, status, access)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_hash_not_found", "哈希不存在")
	}
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, version)
	return s.GetHashDetail(ctx, hashID)
}

func (s *InstructionService) ChangeHashScope(
	ctx context.Context,
	hashID int64,
	ruleSetID int64,
	action string,
	actorID int64,
	access InstructionSensitiveAccess,
) (*InstructionHashEntry, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	if action != "promote" && action != "disable" && action != "revoke" {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_scope_action", "规则作用域操作无效")
	}
	if hashID <= 0 || ruleSetID <= 0 {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_scope_id", "规则作用域 ID 无效")
	}
	access.ResourceType, access.ResourceID, access.Action = "ai_scope", hashID, action
	access.ActorID = actorID
	version, err := s.repository.UpdateHashScope(ctx, hashID, ruleSetID, action, access)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("instruction_audit_scope_not_found", "规则作用域不存在")
	}
	if errors.Is(err, errInstructionAIScopeNotManaged) {
		return nil, infraerrors.BadRequest("instruction_audit_scope_not_managed", "该作用域不支持独立 AI 生命周期操作")
	}
	if errors.Is(err, errInstructionAIScopeRevoked) {
		return nil, infraerrors.Conflict("instruction_audit_scope_revoked", "已撤销的规则作用域不能重新启用")
	}
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, version)
	return s.GetHashDetail(ctx, hashID)
}
