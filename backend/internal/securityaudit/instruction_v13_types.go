package securityaudit

import "time"

const (
	InstructionDefaultMaxBodyBytes         int64 = 64 << 20
	InstructionDefaultParseTimeoutMS             = 500
	InstructionDefaultMaxInflightBodyBytes int64 = 256 << 20
	InstructionBodyWorkingSetMultiplier    int64 = 3
	InstructionHashNormalizationIdentityV1       = "identity_utf8_v1"
	InstructionHashAlgorithmSHA256               = "sha256"
)

type InstructionReasonPolicy struct {
	Reason          string     `json:"reason"`
	Action          string     `json:"action"`
	AIReviewEnabled bool       `json:"ai_review_enabled"`
	AlertEnabled    bool       `json:"alert_enabled"`
	AllowUntil      *time.Time `json:"allow_until,omitempty"`
	ConfigVersion   int64      `json:"config_version"`
	UpdatedBy       *int64     `json:"updated_by,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type UpdateInstructionReasonPolicyRequest struct {
	Action          string     `json:"action"`
	AIReviewEnabled bool       `json:"ai_review_enabled"`
	AlertEnabled    bool       `json:"alert_enabled"`
	AllowUntil      *time.Time `json:"allow_until"`
	ExpectedVersion int64      `json:"expected_config_version"`
	Confirmed       bool       `json:"confirmed"`
}

type InstructionRuntimeConfig struct {
	ConfigVersion               int64     `json:"config_version"`
	MaxBodyBytes                int64     `json:"max_body_bytes"`
	ParseTimeoutMS              int       `json:"parse_timeout_ms"`
	MaxInflightBodyBytes        int64     `json:"max_inflight_body_bytes"`
	PassEventRetentionDays      int       `json:"pass_event_retention_days"`
	AggregateRetentionDays      int       `json:"aggregate_retention_days"`
	RawContentRetentionDays     int       `json:"raw_content_retention_days"`
	AIEnabled                   bool      `json:"ai_enabled"`
	AIBaseURL                   string    `json:"ai_base_url"`
	AIModel                     string    `json:"ai_model"`
	AITokenCiphertext           string    `json:"-"`
	AIHasToken                  bool      `json:"ai_has_token"`
	AIToken                     string    `json:"-"`
	AITimeoutMS                 int       `json:"ai_timeout_ms"`
	AIMaxConcurrency            int       `json:"ai_max_concurrency"`
	AIMinConfidence             float64   `json:"ai_min_confidence"`
	AIPerUserRPM                int       `json:"ai_per_user_rpm"`
	AIPerUserDailyLimit         int       `json:"ai_per_user_daily_limit"`
	AIGlobalDailyLimit          int       `json:"ai_global_daily_limit"`
	AIPromptVersion             string    `json:"ai_prompt_version"`
	TranslationEnabled          bool      `json:"translation_enabled"`
	ExternalTranslationEnabled  bool      `json:"external_translation_enabled"`
	TranslationBaseURL          string    `json:"translation_base_url"`
	TranslationModel            string    `json:"translation_model"`
	TranslationTokenCiphertext  string    `json:"-"`
	TranslationHasToken         bool      `json:"translation_has_token"`
	TranslationToken            string    `json:"-"`
	TranslationTimeoutMS        int       `json:"translation_timeout_ms"`
	TranslationMaxConcurrency   int       `json:"translation_max_concurrency"`
	TranslationChunkBytes       int       `json:"translation_chunk_bytes"`
	TranslationMaxBytes         int       `json:"translation_max_bytes"`
	TranslationResultTTLSeconds int       `json:"translation_result_ttl_seconds"`
	UpdatedBy                   *int64    `json:"updated_by,omitempty"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

type UpdateInstructionRuntimeConfigRequest struct {
	MaxBodyBytes                int64   `json:"max_body_bytes"`
	ParseTimeoutMS              int     `json:"parse_timeout_ms"`
	MaxInflightBodyBytes        int64   `json:"max_inflight_body_bytes"`
	PassEventRetentionDays      int     `json:"pass_event_retention_days"`
	AggregateRetentionDays      int     `json:"aggregate_retention_days"`
	RawContentRetentionDays     int     `json:"raw_content_retention_days"`
	AIEnabled                   bool    `json:"ai_enabled"`
	AIBaseURL                   string  `json:"ai_base_url"`
	AIModel                     string  `json:"ai_model"`
	AIToken                     string  `json:"ai_token"`
	ClearAIToken                bool    `json:"clear_ai_token"`
	AITimeoutMS                 int     `json:"ai_timeout_ms"`
	AIMaxConcurrency            int     `json:"ai_max_concurrency"`
	AIMinConfidence             float64 `json:"ai_min_confidence"`
	AIPerUserRPM                int     `json:"ai_per_user_rpm"`
	AIPerUserDailyLimit         int     `json:"ai_per_user_daily_limit"`
	AIGlobalDailyLimit          int     `json:"ai_global_daily_limit"`
	AIPromptVersion             string  `json:"ai_prompt_version"`
	TranslationEnabled          bool    `json:"translation_enabled"`
	ExternalTranslationEnabled  bool    `json:"external_translation_enabled"`
	TranslationBaseURL          string  `json:"translation_base_url"`
	TranslationModel            string  `json:"translation_model"`
	TranslationToken            string  `json:"translation_token"`
	ClearTranslationToken       bool    `json:"clear_translation_token"`
	TranslationTimeoutMS        int     `json:"translation_timeout_ms"`
	TranslationMaxConcurrency   int     `json:"translation_max_concurrency"`
	TranslationChunkBytes       int     `json:"translation_chunk_bytes"`
	TranslationMaxBytes         int     `json:"translation_max_bytes"`
	TranslationResultTTLSeconds int     `json:"translation_result_ttl_seconds"`
	ExpectedVersion             int64   `json:"expected_config_version"`
}

type InstructionHashSource struct {
	ID            int64     `json:"id"`
	SourceType    string    `json:"source_type"`
	FieldName     string    `json:"field_name"`
	EventID       *int64    `json:"event_id,omitempty"`
	AIReviewID    *int64    `json:"ai_review_id,omitempty"`
	ReviewerModel string    `json:"reviewer_model"`
	PromptVersion string    `json:"prompt_version"`
	Confidence    *float64  `json:"confidence,omitempty"`
	ReviewReason  string    `json:"review_reason"`
	CreatedBy     *int64    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type InstructionHashRawReview struct {
	HashID           int64      `json:"hash_id"`
	FieldName        string     `json:"field_name"`
	RawStatus        string     `json:"raw_content_status"`
	RawContent       string     `json:"raw_content,omitempty"`
	ContentBytes     int        `json:"content_bytes"`
	SHA256           string     `json:"sha256"`
	RecomputedSHA256 string     `json:"recomputed_sha256,omitempty"`
	DigestConsistent bool       `json:"digest_consistent"`
	RawExpiresAt     *time.Time `json:"raw_expires_at,omitempty"`
}

type ChangeInstructionHashStatusRequest struct {
	Status string `json:"status"`
}

type ChangeInstructionHashScopeRequest struct {
	Action string `json:"action"`
}

type instructionHashRawStorage struct {
	HashID        int64
	Ciphertext    []byte
	Status        string
	ContentBytes  int
	HashAlgorithm string
	Normalization string
	KeyVersion    string
	ExpiresAt     *time.Time
}

type instructionHashMaterial struct {
	Request CreateInstructionHashRequest
	Raw     *instructionHashRawStorage
	Source  InstructionHashSource
}

type InstructionSensitiveAccess struct {
	ResourceType string
	ResourceID   int64
	ActorID      int64
	Action       string
	RequestID    string
	ClientIP     string
	UserAgent    string
	Succeeded    bool
	ErrorCode    string
}

type InstructionAIReview struct {
	ID              int64     `json:"id"`
	EventID         *int64    `json:"event_id,omitempty"`
	RequestID       string    `json:"request_id"`
	UserID          *int64    `json:"user_id,omitempty"`
	GroupID         *int64    `json:"group_id,omitempty"`
	ClientType      string    `json:"client_type"`
	Model           string    `json:"model"`
	ReviewedSource  string    `json:"reviewed_source"`
	ReviewedSHA256  string    `json:"reviewed_sha256"`
	Result          string    `json:"result"`
	ApprovedSource  string    `json:"approved_source,omitempty"`
	Confidence      float64   `json:"confidence"`
	Reason          string    `json:"reason"`
	ReviewerModel   string    `json:"reviewer_model"`
	PromptVersion   string    `json:"prompt_version"`
	LatencyMS       int       `json:"latency_ms"`
	AutomaticHashID *int64    `json:"automatic_hash_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type InstructionStatistics struct {
	Blocked       int64   `json:"blocked"`
	PolicyAllow   int64   `json:"policy_allow"`
	AIPass        int64   `json:"ai_pass"`
	HashPass      int64   `json:"hash_pass"`
	ExceptionPass int64   `json:"exception_pass"`
	Total         int64   `json:"total"`
	BlockRate     float64 `json:"block_rate"`
}

type InstructionLatencyMetrics struct {
	AuditSampleCount int64
	AuditP95MS       int64
	AuditP99MS       int64
	AISampleCount    int64
	AIP95MS          int64
	AIP99MS          int64
}

type InstructionTranslationRequest struct {
	ResourceType   string `json:"resource_type"`
	ResourceID     int64  `json:"resource_id"`
	FieldName      string `json:"field_name"`
	TargetLanguage string `json:"target_language"`
	Provider       string `json:"provider"`
}

type InstructionTranslationJob struct {
	ID                  int64      `json:"id"`
	ResourceType        string     `json:"resource_type"`
	ResourceID          int64      `json:"resource_id"`
	FieldName           string     `json:"field_name"`
	TargetLanguage      string     `json:"target_language"`
	Provider            string     `json:"provider"`
	Status              string     `json:"status"`
	ErrorCode           string     `json:"error_code"`
	ChunkCount          int        `json:"chunk_count"`
	CompletedChunks     int        `json:"completed_chunks"`
	Attempts            int        `json:"attempts"`
	MaxAttempts         int        `json:"max_attempts"`
	ClaimVersion        int64      `json:"-"`
	ResultBytes         int        `json:"result_bytes"`
	RedactionCount      int        `json:"redaction_count"`
	ProviderLatencyMS   int        `json:"provider_latency_ms"`
	RequestedBy         *int64     `json:"requested_by,omitempty"`
	AuthorizedGrantID   *int64     `json:"-"`
	ProcessingStartedAt *time.Time `json:"processing_started_at,omitempty"`
	TranslatedText      string     `json:"translated_text,omitempty"`
	ExpiresAt           time.Time  `json:"expires_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type instructionTranslationSource struct {
	Plaintext string
	Digest    string
	Bytes     int
}
