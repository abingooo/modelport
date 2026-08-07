package securityaudit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *InstructionService) activateInstructionRuntimeSecrets(snapshot *instructionSnapshot) error {
	if snapshot == nil {
		return errors.New("instruction audit snapshot unavailable")
	}
	runtime := &snapshot.Runtime
	if (runtime.AIEnabled || runtime.TranslationEnabled) && strings.TrimSpace(runtime.AITokenCiphertext) != "" {
		if s.secretEncryptor == nil {
			return errors.New("instruction audit AI credential decryptor unavailable")
		}
		token, err := s.secretEncryptor.Decrypt(runtime.AITokenCiphertext)
		if err != nil {
			return fmt.Errorf("decrypt instruction audit AI credential: %w", err)
		}
		runtime.AIToken = token
	}
	if runtime.TranslationEnabled && runtime.ExternalTranslationEnabled && strings.TrimSpace(runtime.TranslationTokenCiphertext) != "" {
		if s.secretEncryptor == nil {
			return errors.New("instruction audit translation credential decryptor unavailable")
		}
		token, err := s.secretEncryptor.Decrypt(runtime.TranslationTokenCiphertext)
		if err != nil {
			return fmt.Errorf("decrypt instruction audit translation credential: %w", err)
		}
		runtime.TranslationToken = token
	}
	return nil
}

func (s *InstructionService) ListReasonPolicies(ctx context.Context) ([]InstructionReasonPolicy, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("instruction audit service unavailable")
	}
	return s.repository.ListReasonPolicies(ctx)
}

func (s *InstructionService) UpdateReasonPolicy(
	ctx context.Context,
	reason string,
	request UpdateInstructionReasonPolicyRequest,
	actorID int64,
) (*InstructionReasonPolicy, error) {
	reason = strings.TrimSpace(reason)
	if _, ok := validInstructionPolicyReasons[reason]; !ok {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_reason", "拒绝原因无效")
	}
	request.Action = strings.TrimSpace(request.Action)
	if request.Action != InstructionPolicyActionBlock && request.Action != InstructionPolicyActionAllowAndRecord {
		return nil, infraerrors.BadRequest("instruction_audit_invalid_policy_action", "原因处置动作无效")
	}
	if reason == "config_unavailable" || reason == "ai_error" {
		if request.Action != InstructionPolicyActionBlock {
			return nil, infraerrors.BadRequest("instruction_audit_reason_must_block", "该原因必须保持拦截")
		}
	}
	if strings.HasPrefix(reason, "ai_") && request.AIReviewEnabled {
		return nil, infraerrors.BadRequest("instruction_audit_ai_review_recursive", "AI 派生原因不能再次触发 AI 审核")
	}
	if request.AIReviewEnabled && reason != "hash_mismatch" && reason != "field_invalid" {
		return nil, infraerrors.BadRequest("instruction_audit_ai_review_unsupported_reason", "该原因无法安全提取字段进行 AI 审核")
	}
	if request.Action == InstructionPolicyActionAllowAndRecord {
		if !request.Confirmed {
			return nil, infraerrors.BadRequest("instruction_audit_high_risk_confirmation_required", "高风险放行需要明确二次确认")
		}
		if reason == "request_too_large" {
			if request.AllowUntil == nil || !request.AllowUntil.After(time.Now().UTC()) {
				return nil, infraerrors.BadRequest("instruction_audit_temporary_allow_required", "超大请求只能设置短期放行")
			}
			if request.AllowUntil.After(time.Now().UTC().Add(24 * time.Hour)) {
				return nil, infraerrors.BadRequest("instruction_audit_temporary_allow_too_long", "超大请求临时放行最长 24 小时")
			}
		}
	} else {
		request.AllowUntil = nil
	}
	item, currentVersion, err := s.repository.UpdateReasonPolicy(ctx, reason, request, actorID)
	if errors.Is(err, errInstructionAuditConfigConflict) {
		return nil, infraerrors.Conflict(
			"instruction_audit_config_conflict",
			fmt.Sprintf("配置已更新，请刷新后重试（当前版本 %d）", currentVersion),
		)
	}
	if err != nil {
		return nil, err
	}
	s.refreshAfterMutation(ctx, item.ConfigVersion)
	return item, nil
}

func (s *InstructionService) RuntimeConfig(ctx context.Context) (InstructionRuntimeConfig, error) {
	if s == nil || s.repository == nil {
		return InstructionRuntimeConfig{}, errors.New("instruction audit service unavailable")
	}
	config, err := s.repository.GetRuntimeConfig(ctx)
	if err != nil {
		return InstructionRuntimeConfig{}, err
	}
	config.AITokenCiphertext = ""
	config.TranslationTokenCiphertext = ""
	config.ConfigVersion = s.ConfigVersion()
	return config, nil
}

func (s *InstructionService) UpdateRuntimeConfig(
	ctx context.Context,
	request UpdateInstructionRuntimeConfigRequest,
	actorID int64,
) (InstructionRuntimeConfig, error) {
	current, err := s.repository.GetRuntimeConfig(ctx)
	if err != nil {
		return InstructionRuntimeConfig{}, err
	}
	updated := instructionRuntimeConfigFromRequest(request, current)
	if err := validateInstructionRuntimeConfig(updated); err != nil {
		return InstructionRuntimeConfig{}, err
	}
	if updated.TranslationEnabled && (s.evidenceCipher == nil || !s.evidenceCipher.Available() || s.secretEncryptor == nil || s.redis == nil) {
		return InstructionRuntimeConfig{}, infraerrors.BadRequest("instruction_audit_translation_unavailable", "启用翻译前必须配置固定加密密钥和 Redis")
	}
	if strings.TrimSpace(request.AIToken) != "" {
		if s.evidenceCipher == nil || !s.evidenceCipher.Available() || s.secretEncryptor == nil {
			return InstructionRuntimeConfig{}, infraerrors.BadRequest("instruction_audit_encryption_required", "保存审计服务凭据前必须配置固定加密密钥")
		}
		updated.AITokenCiphertext, err = s.secretEncryptor.Encrypt(strings.TrimSpace(request.AIToken))
		if err != nil {
			return InstructionRuntimeConfig{}, err
		}
	} else if request.ClearAIToken {
		updated.AITokenCiphertext = ""
	}
	if strings.TrimSpace(request.TranslationToken) != "" {
		if s.evidenceCipher == nil || !s.evidenceCipher.Available() || s.secretEncryptor == nil {
			return InstructionRuntimeConfig{}, infraerrors.BadRequest("instruction_audit_encryption_required", "保存翻译服务凭据前必须配置固定加密密钥")
		}
		updated.TranslationTokenCiphertext, err = s.secretEncryptor.Encrypt(strings.TrimSpace(request.TranslationToken))
		if err != nil {
			return InstructionRuntimeConfig{}, err
		}
	} else if request.ClearTranslationToken {
		updated.TranslationTokenCiphertext = ""
	}
	if err := validateInstructionRuntimeCredentials(updated); err != nil {
		return InstructionRuntimeConfig{}, err
	}
	result, currentVersion, err := s.repository.UpdateRuntimeConfig(ctx, updated, request.ExpectedVersion, actorID)
	if errors.Is(err, errInstructionAuditConfigConflict) {
		return InstructionRuntimeConfig{}, infraerrors.Conflict(
			"instruction_audit_config_conflict",
			fmt.Sprintf("配置已更新，请刷新后重试（当前版本 %d）", currentVersion),
		)
	}
	if err != nil {
		return InstructionRuntimeConfig{}, err
	}
	s.refreshAfterMutation(ctx, currentVersion)
	result.ConfigVersion = currentVersion
	result.AITokenCiphertext = ""
	result.TranslationTokenCiphertext = ""
	return result, nil
}

func instructionRuntimeConfigFromRequest(request UpdateInstructionRuntimeConfigRequest, current InstructionRuntimeConfig) InstructionRuntimeConfig {
	aggregateRetentionDays := request.AggregateRetentionDays
	if aggregateRetentionDays == 0 {
		aggregateRetentionDays = current.AggregateRetentionDays
	}
	return InstructionRuntimeConfig{
		MaxBodyBytes: request.MaxBodyBytes, ParseTimeoutMS: request.ParseTimeoutMS,
		MaxInflightBodyBytes:    request.MaxInflightBodyBytes,
		PassEventRetentionDays:  request.PassEventRetentionDays,
		AggregateRetentionDays:  aggregateRetentionDays,
		RawContentRetentionDays: request.RawContentRetentionDays,
		AIEnabled:               request.AIEnabled, AIBaseURL: strings.TrimSpace(request.AIBaseURL),
		AIModel: strings.TrimSpace(request.AIModel), AITokenCiphertext: current.AITokenCiphertext,
		AITimeoutMS: request.AITimeoutMS, AIMaxConcurrency: request.AIMaxConcurrency,
		AIMinConfidence: request.AIMinConfidence,
		AIPerUserRPM:    request.AIPerUserRPM, AIPerUserDailyLimit: request.AIPerUserDailyLimit,
		AIGlobalDailyLimit:          request.AIGlobalDailyLimit,
		AIPromptVersion:             strings.TrimSpace(request.AIPromptVersion),
		TranslationEnabled:          request.TranslationEnabled,
		ExternalTranslationEnabled:  request.ExternalTranslationEnabled,
		TranslationBaseURL:          strings.TrimSpace(request.TranslationBaseURL),
		TranslationModel:            strings.TrimSpace(request.TranslationModel),
		TranslationTokenCiphertext:  current.TranslationTokenCiphertext,
		TranslationTimeoutMS:        request.TranslationTimeoutMS,
		TranslationMaxConcurrency:   request.TranslationMaxConcurrency,
		TranslationChunkBytes:       request.TranslationChunkBytes,
		TranslationMaxBytes:         request.TranslationMaxBytes,
		TranslationResultTTLSeconds: request.TranslationResultTTLSeconds,
	}
}

func validateInstructionRuntimeConfig(config InstructionRuntimeConfig) error {
	if config.MaxBodyBytes < 1<<20 || config.MaxBodyBytes > 128<<20 {
		return infraerrors.BadRequest("instruction_audit_invalid_body_limit", "请求体上限必须在 1-128 MiB 之间")
	}
	if config.ParseTimeoutMS < 50 || config.ParseTimeoutMS > 5000 {
		return infraerrors.BadRequest("instruction_audit_invalid_parse_timeout", "解析超时必须在 50-5000 ms 之间")
	}
	if config.MaxInflightBodyBytes < config.MaxBodyBytes || config.MaxInflightBodyBytes > 2<<30 {
		return infraerrors.BadRequest("instruction_audit_invalid_inflight_limit", "并发解析内存上限无效")
	}
	if config.PassEventRetentionDays < 1 || config.PassEventRetentionDays > 90 ||
		config.AggregateRetentionDays < 30 || config.AggregateRetentionDays > 3650 ||
		config.RawContentRetentionDays < 1 || config.RawContentRetentionDays > 3650 {
		return infraerrors.BadRequest("instruction_audit_invalid_retention", "保留期限超出允许范围")
	}
	if config.AITimeoutMS < 100 || config.AITimeoutMS > 30000 || config.AIMaxConcurrency < 1 || config.AIMaxConcurrency > 64 || config.AIMinConfidence < 0.5 || config.AIMinConfidence > 1 {
		return infraerrors.BadRequest("instruction_audit_invalid_ai_limits", "AI 审核超时或置信度无效")
	}
	if config.AIPerUserRPM < 1 || config.AIPerUserRPM > 120 || config.AIPerUserDailyLimit < 1 || config.AIPerUserDailyLimit > 1000 || config.AIGlobalDailyLimit < 1 || config.AIGlobalDailyLimit > 100000 {
		return infraerrors.BadRequest("instruction_audit_invalid_ai_rate_limits", "AI 审核频率限制无效")
	}
	if config.TranslationTimeoutMS < 100 || config.TranslationTimeoutMS > 60000 || config.TranslationMaxConcurrency < 1 || config.TranslationMaxConcurrency > 16 || config.TranslationChunkBytes < 1024 || config.TranslationChunkBytes > 65536 || config.TranslationMaxBytes < config.TranslationChunkBytes || config.TranslationMaxBytes > 1<<20 || config.TranslationResultTTLSeconds < 60 || config.TranslationResultTTLSeconds > 86400 {
		return infraerrors.BadRequest("instruction_audit_invalid_translation_limits", "翻译任务限制无效")
	}
	return nil
}

func validateInstructionRuntimeCredentials(config InstructionRuntimeConfig) error {
	if config.AIEnabled {
		if _, err := NormalizeBaseURL(config.AIBaseURL); err != nil {
			return err
		}
		if config.AIModel == "" || config.AITokenCiphertext == "" {
			return infraerrors.BadRequest("instruction_audit_ai_config_required", "启用 AI 二审前必须配置模型和凭据")
		}
	}
	if config.TranslationEnabled && config.ExternalTranslationEnabled {
		if _, err := NormalizeBaseURL(config.TranslationBaseURL); err != nil {
			return err
		}
		if config.TranslationModel == "" || config.TranslationTokenCiphertext == "" {
			return infraerrors.BadRequest("instruction_audit_translation_config_required", "启用外部翻译前必须配置模型和凭据")
		}
	}
	if config.TranslationEnabled {
		internalConfigured := strings.TrimSpace(config.AIBaseURL) != "" &&
			strings.TrimSpace(config.AIModel) != "" && strings.TrimSpace(config.AITokenCiphertext) != ""
		externalConfigured := config.ExternalTranslationEnabled &&
			strings.TrimSpace(config.TranslationBaseURL) != "" &&
			strings.TrimSpace(config.TranslationModel) != "" &&
			strings.TrimSpace(config.TranslationTokenCiphertext) != ""
		if !internalConfigured && !externalConfigured {
			return infraerrors.BadRequest("instruction_audit_translation_config_required", "启用翻译前必须配置内部模型或外部翻译服务")
		}
		if internalConfigured {
			if _, err := NormalizeBaseURL(config.AIBaseURL); err != nil {
				return err
			}
		}
	}
	return nil
}
