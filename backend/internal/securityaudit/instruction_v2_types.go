package securityaudit

import (
	"context"
	"regexp"
	"time"
)

const (
	InstructionV2ModeOff     = "off"
	InstructionV2ModeObserve = "observe"
	InstructionV2ModeEnforce = "enforce"

	InstructionV2OutcomeHashPass      = "hash_pass"
	InstructionV2OutcomeAIPass        = "ai_pass"
	InstructionV2OutcomeBlocked       = "blocked"
	InstructionV2OutcomeEmptyPass     = "empty_pass"
	InstructionV2OutcomeAllowlistPass = "user_allowlist_pass"
	InstructionV2OutcomeObserveAllow  = "observe_allow"

	InstructionV2DefaultAIInputChars        = 64000
	InstructionV2DefaultGlobalConcurrency   = 64
	InstructionV2DefaultNodeConcurrency     = 16
	InstructionV2DefaultQueueWaitMS         = 2000
	InstructionV2DefaultReviewTimeoutMS     = 30000
	InstructionV2DefaultCacheTTLSeconds     = 600
	InstructionV2DefaultEventRetentionDays  = 30
	InstructionV2DefaultEvidenceDays        = 7
	InstructionV2DefaultCandidateDays       = 30
	InstructionV2DefaultRawFullMaxBytes     = 4 << 20
	InstructionV2ConfigInvalidationChannel  = "modelport:instruction_audit_v2:config:invalidate"
	InstructionV2AIReviewPurposeHeader      = "instruction-audit-v2-review"
	InstructionV2PromptWrapperVersion       = "instruction-audit-v2-wrapper-1"
	InstructionV2AsyncQueueCapacity         = 32768
	InstructionV2AsyncWorkerMaximum         = 512
	InstructionV2EventBatchSize             = 100
	InstructionV2EventBatchFlushInterval    = 100 * time.Millisecond
	InstructionV2ConfigurationRefreshPeriod = 10 * time.Second
)

type InstructionV2Engine interface {
	InstructionEngine
	Start(context.Context) error
	Shutdown(context.Context) error
}

type InstructionV2Config struct {
	Mode                    string     `json:"mode"`
	EffectiveMode           string     `json:"effective_mode"`
	RiskControlEnabled      bool       `json:"risk_control_enabled"`
	ReviewCriteria          string     `json:"review_criteria"`
	ConfidenceThreshold     float64    `json:"confidence_threshold"`
	AIInputMaxChars         int        `json:"ai_input_max_chars"`
	AIGlobalConcurrency     int        `json:"ai_global_concurrency"`
	AIQueueWaitMS           int        `json:"ai_queue_wait_ms"`
	AITotalTimeoutMS        int        `json:"ai_total_timeout_ms"`
	AICacheTTLSeconds       int        `json:"ai_cache_ttl_seconds"`
	EventRetentionDays      int        `json:"event_retention_days"`
	EvidenceRetentionDays   int        `json:"evidence_retention_days"`
	CandidateRetentionDays  int        `json:"candidate_retention_days"`
	RawFullMaxBytes         int        `json:"raw_full_max_bytes"`
	ConfigVersion           int64      `json:"config_version"`
	UpdatedBy               *int64     `json:"updated_by,omitempty"`
	UpdatedAt               time.Time  `json:"updated_at"`
	GatewayHTTPMaxBodyBytes int64      `json:"gateway_http_max_body_bytes"`
	GatewayWSMaxBodyBytes   int64      `json:"gateway_ws_max_body_bytes"`
	EvidenceEncryptionReady bool       `json:"evidence_encryption_ready"`
	ActiveScopeCount        int64      `json:"active_scope_count"`
	ActiveHashCount         int64      `json:"active_hash_count"`
	EnabledAINodeCount      int64      `json:"enabled_ai_node_count"`
	AsyncQueueDepth         int        `json:"async_queue_depth"`
	AsyncQueueCapacity      int        `json:"async_queue_capacity"`
	LastConfigLoadError     string     `json:"last_config_load_error"`
	LastConfigLoadedAt      *time.Time `json:"last_config_loaded_at,omitempty"`
}

type UpdateInstructionV2ConfigRequest struct {
	ExpectedConfigVersion  int64   `json:"expected_config_version" binding:"required"`
	Mode                   string  `json:"mode"`
	ReviewCriteria         string  `json:"review_criteria"`
	ConfidenceThreshold    float64 `json:"confidence_threshold"`
	AIInputMaxChars        int     `json:"ai_input_max_chars"`
	AIGlobalConcurrency    int     `json:"ai_global_concurrency"`
	AIQueueWaitMS          int     `json:"ai_queue_wait_ms"`
	AITotalTimeoutMS       int     `json:"ai_total_timeout_ms"`
	AICacheTTLSeconds      int     `json:"ai_cache_ttl_seconds"`
	EventRetentionDays     int     `json:"event_retention_days"`
	EvidenceRetentionDays  int     `json:"evidence_retention_days"`
	CandidateRetentionDays int     `json:"candidate_retention_days"`
	RawFullMaxBytes        int     `json:"raw_full_max_bytes"`
}

type InstructionV2AINode struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	BaseURL        string    `json:"base_url"`
	Model          string    `json:"model"`
	Priority       int       `json:"priority"`
	Enabled        bool      `json:"enabled"`
	TimeoutMS      int       `json:"timeout_ms"`
	MaxConcurrency int       `json:"max_concurrency"`
	HasAPIKey      bool      `json:"has_api_key"`
	APIKeyStatus   string    `json:"api_key_status"`
	CreatedBy      *int64    `json:"created_by,omitempty"`
	UpdatedBy      *int64    `json:"updated_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type InstructionV2AINodeTestResult struct {
	Result     string  `json:"result"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Category   string  `json:"category"`
	LatencyMS  int     `json:"latency_ms"`
}

type SaveInstructionV2AINodeRequest struct {
	Name           string `json:"name"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	APIKey         string `json:"api_key"`
	ClearAPIKey    bool   `json:"clear_api_key"`
	Priority       int    `json:"priority"`
	Enabled        bool   `json:"enabled"`
	TimeoutMS      int    `json:"timeout_ms"`
	MaxConcurrency int    `json:"max_concurrency"`
}

type InstructionV2ClientMatcher struct {
	Type          string `json:"type"`
	Value         string `json:"value"`
	CaseSensitive bool   `json:"case_sensitive"`
}

type InstructionV2ClientProfile struct {
	ID                int64                        `json:"id"`
	ProfileKey        string                       `json:"profile_key"`
	Name              string                       `json:"name"`
	Description       string                       `json:"description"`
	Matchers          []InstructionV2ClientMatcher `json:"matchers"`
	Priority          int                          `json:"priority"`
	Enabled           bool                         `json:"enabled"`
	BuiltIn           bool                         `json:"built_in"`
	ImmutableInternal bool                         `json:"immutable_internal"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
}

type SaveInstructionV2ClientProfileRequest struct {
	ProfileKey  string                       `json:"profile_key"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Matchers    []InstructionV2ClientMatcher `json:"matchers"`
	Priority    int                          `json:"priority"`
	Enabled     bool                         `json:"enabled"`
}

type InstructionV2Scope struct {
	ID                int64     `json:"id"`
	GroupID           int64     `json:"group_id"`
	GroupName         string    `json:"group_name"`
	GroupPlatform     string    `json:"group_platform"`
	GroupStatus       string    `json:"group_status"`
	ClientProfileID   *int64    `json:"client_profile_id,omitempty"`
	ClientProfileKey  string    `json:"client_profile_key"`
	ClientProfileName string    `json:"client_profile_name"`
	Enabled           bool      `json:"enabled"`
	Effective         bool      `json:"effective"`
	CreatedBy         *int64    `json:"created_by,omitempty"`
	UpdatedBy         *int64    `json:"updated_by,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SaveInstructionV2ScopeRequest struct {
	GroupID         int64  `json:"group_id"`
	ClientProfileID *int64 `json:"client_profile_id"`
	Enabled         bool   `json:"enabled"`
}

type InstructionV2UserAllowlistEntry struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	Note      string    `json:"note"`
	Enabled   bool      `json:"enabled"`
	CreatedBy *int64    `json:"created_by,omitempty"`
	UpdatedBy *int64    `json:"updated_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type InstructionV2UserOption struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

type SaveInstructionV2UserAllowlistRequest struct {
	UserID  int64  `json:"user_id"`
	Note    string `json:"note"`
	Enabled bool   `json:"enabled"`
}

type InstructionV2Hash struct {
	ID                   int64                    `json:"id"`
	SHA256               string                   `json:"sha256"`
	Name                 string                   `json:"name"`
	Note                 string                   `json:"note"`
	Status               string                   `json:"status"`
	Source               string                   `json:"source"`
	ObservedField        string                   `json:"observed_field"`
	HashAlgorithm        string                   `json:"hash_algorithm"`
	NormalizationVersion string                   `json:"normalization_version"`
	ContentBytes         int64                    `json:"content_bytes"`
	RawStorage           string                   `json:"raw_storage"`
	StoredBytes          int                      `json:"stored_bytes"`
	AISampled            bool                     `json:"ai_sampled"`
	SourceEventID        *int64                   `json:"source_event_id,omitempty"`
	ReviewerNodeID       *int64                   `json:"reviewer_node_id,omitempty"`
	ReviewerModel        string                   `json:"reviewer_model"`
	PromptVersion        string                   `json:"prompt_version"`
	Confidence           *float64                 `json:"confidence,omitempty"`
	ReviewReason         string                   `json:"review_reason"`
	ReviewCategory       string                   `json:"review_category"`
	CandidateExpiresAt   *time.Time               `json:"candidate_expires_at,omitempty"`
	ScopeIDs             []int64                  `json:"scope_ids"`
	Scopes               []InstructionV2HashScope `json:"scopes"`
	CreatedBy            *int64                   `json:"created_by,omitempty"`
	UpdatedBy            *int64                   `json:"updated_by,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
}

type InstructionV2HashScope struct {
	ScopeID            int64      `json:"scope_id"`
	GroupID            int64      `json:"group_id"`
	GroupName          string     `json:"group_name"`
	ClientProfileID    *int64     `json:"client_profile_id,omitempty"`
	ClientProfileKey   string     `json:"client_profile_key"`
	ClientProfileName  string     `json:"client_profile_name"`
	Status             string     `json:"status"`
	Source             string     `json:"source"`
	CandidateExpiresAt *time.Time `json:"candidate_expires_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type SaveInstructionV2HashRequest struct {
	RawContent string  `json:"raw_content"`
	SHA256     string  `json:"sha256"`
	Source     string  `json:"source"`
	Name       string  `json:"name"`
	Note       string  `json:"note"`
	Status     string  `json:"status"`
	ScopeIDs   []int64 `json:"scope_ids"`
}

type UpdateInstructionV2HashRequest struct {
	Name      *string `json:"name"`
	Note      *string `json:"note"`
	Status    *string `json:"status"`
	ScopeIDs  []int64 `json:"scope_ids"`
	SetScopes bool    `json:"set_scopes"`
}

type InstructionV2HashPage struct {
	Items    []InstructionV2Hash `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
	Pages    int                 `json:"pages"`
}

type InstructionV2Field struct {
	State     string `json:"state"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	Partial   bool   `json:"partial"`
	Plaintext string `json:"-"`
	AISample  string `json:"-"`
	AISampled bool   `json:"ai_sampled"`
}

type InstructionV2Event struct {
	ID                     int64                   `json:"id"`
	RequestID              string                  `json:"request_id"`
	UserID                 *int64                  `json:"user_id,omitempty"`
	UserEmail              string                  `json:"user_email"`
	APIKeyID               *int64                  `json:"api_key_id,omitempty"`
	APIKeyName             string                  `json:"api_key_name"`
	GroupID                *int64                  `json:"group_id,omitempty"`
	GroupName              string                  `json:"group_name"`
	ScopeID                *int64                  `json:"scope_id,omitempty"`
	ClientProfileID        *int64                  `json:"client_profile_id,omitempty"`
	ClientKey              string                  `json:"client_key"`
	ClientName             string                  `json:"client_name"`
	ClientUserAgent        string                  `json:"client_user_agent"`
	Model                  string                  `json:"model"`
	Endpoint               string                  `json:"endpoint"`
	Stage                  string                  `json:"stage"`
	Mode                   string                  `json:"mode"`
	Decision               string                  `json:"decision"`
	Outcome                string                  `json:"outcome"`
	Reason                 string                  `json:"reason"`
	Instructions           InstructionV2Field      `json:"instructions"`
	Input1                 InstructionV2Field      `json:"input1"`
	MatchedHashID          *int64                  `json:"matched_hash_id,omitempty"`
	AIResult               string                  `json:"ai_result"`
	AIReviewedField        string                  `json:"ai_reviewed_field"`
	AISampled              bool                    `json:"ai_sampled"`
	AuditLatencyMS         int                     `json:"audit_latency_ms"`
	AILatencyMS            int                     `json:"ai_latency_ms"`
	BodyBytes              int64                   `json:"body_bytes"`
	ConfigVersion          int64                   `json:"config_version"`
	EvidenceStatus         string                  `json:"evidence_status"`
	UserNotificationStatus string                  `json:"user_notification_status"`
	OpsNotificationStatus  string                  `json:"ops_notification_status"`
	CreatedAt              time.Time               `json:"created_at"`
	AIReviews              []InstructionV2AIReview `json:"ai_reviews,omitempty"`
}

type InstructionV2AIReview struct {
	ID            int64     `json:"id"`
	EventID       int64     `json:"event_id"`
	NodeID        *int64    `json:"node_id,omitempty"`
	NodeName      string    `json:"node_name"`
	ReviewerModel string    `json:"reviewer_model"`
	FieldName     string    `json:"field_name"`
	SHA256        string    `json:"sha256"`
	Result        string    `json:"result"`
	Confidence    float64   `json:"confidence"`
	Reason        string    `json:"reason"`
	Category      string    `json:"category"`
	PromptVersion string    `json:"prompt_version"`
	Sampled       bool      `json:"sampled"`
	Cached        bool      `json:"cached"`
	LatencyMS     int       `json:"latency_ms"`
	CreatedAt     time.Time `json:"created_at"`
}

type InstructionV2EventFilter struct {
	Query      string     `json:"q,omitempty"`
	EventID    int64      `json:"event_id,omitempty"`
	UserID     int64      `json:"user_id,omitempty"`
	GroupIDs   []int64    `json:"group_ids,omitempty"`
	ClientKeys []string   `json:"client_keys,omitempty"`
	Outcomes   []string   `json:"outcomes,omitempty"`
	Reasons    []string   `json:"reasons,omitempty"`
	AIResults  []string   `json:"ai_results,omitempty"`
	Model      string     `json:"model,omitempty"`
	From       *time.Time `json:"from,omitempty"`
	To         *time.Time `json:"to,omitempty"`
}

type InstructionV2EventPage struct {
	Items    []InstructionV2Event `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Pages    int                  `json:"pages"`
}

type InstructionV2Statistics struct {
	From             time.Time `json:"from"`
	To               time.Time `json:"to"`
	Total            int64     `json:"total"`
	HashPass         int64     `json:"hash_pass"`
	AIPass           int64     `json:"ai_pass"`
	Blocked          int64     `json:"blocked"`
	EmptyOrAllowlist int64     `json:"empty_or_allowlist_pass"`
	AIFailures       int64     `json:"ai_failures"`
	BlockRate        float64   `json:"block_rate"`
}

type InstructionV2EvidenceField struct {
	FieldName        string `json:"field_name"`
	SHA256           string `json:"sha256"`
	StorageKind      string `json:"storage_kind"`
	Plaintext        string `json:"plaintext"`
	ContentBytes     int64  `json:"content_bytes"`
	StoredBytes      int    `json:"stored_bytes"`
	DigestConsistent bool   `json:"digest_consistent"`
}

type InstructionV2EvidenceReview struct {
	ResourceType string                       `json:"resource_type"`
	ResourceID   int64                        `json:"resource_id"`
	Fields       []InstructionV2EvidenceField `json:"fields"`
}

type InstructionV2RawAccess struct {
	ActorID   int64
	Action    string
	FieldName string
	RequestID string
	ClientIP  string
	UserAgent string
}

type InstructionV2DeleteRequest struct {
	IDs []int64 `json:"ids"`
}

type InstructionV2TrustEventRequest struct {
	Fields []string `json:"fields"`
	Name   string   `json:"name"`
	Note   string   `json:"note"`
}

type InstructionV2TrustEventResult struct {
	Hashes []InstructionV2Hash `json:"hashes"`
}

type instructionV2ParsedFields struct {
	Instructions InstructionV2Field
	Input1       InstructionV2Field
}

type instructionV2CompiledMatcher struct {
	matcherType   string
	value         string
	caseSensitive bool
	regex         *regexp.Regexp
}

type instructionV2ClientRuntime struct {
	profile  InstructionV2ClientProfile
	matchers []instructionV2CompiledMatcher
}

type instructionV2ScopeRuntime struct {
	ID               int64
	GroupID          int64
	ClientProfileID  *int64
	ClientProfileKey string
}

type instructionV2HashRuntime struct {
	ID       int64
	SHA256   string
	ScopeIDs map[int64]struct{}
}

type instructionV2AINodeRuntime struct {
	InstructionV2AINode
	APIKey    string
	semaphore chan struct{}
}

type instructionV2Snapshot struct {
	Config          InstructionV2Config
	PromptVersion   string
	Profiles        []instructionV2ClientRuntime
	ProfilesByKey   map[string]instructionV2ClientRuntime
	ScopesByGroup   map[int64][]instructionV2ScopeRuntime
	Hashes          map[string]instructionV2HashRuntime
	AllowedUsers    map[int64]struct{}
	AINodes         []*instructionV2AINodeRuntime
	GlobalSemaphore chan struct{}
	LoadedAt        time.Time
}

type instructionV2EvaluationContext struct {
	request   Request
	snapshot  *instructionV2Snapshot
	profile   instructionV2ClientRuntime
	scopes    []instructionV2ScopeRuntime
	scope     instructionV2ScopeRuntime
	fields    instructionV2ParsedFields
	bodyBytes int64
	startedAt time.Time
}

type instructionV2AIResult struct {
	Result     string  `json:"result"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Category   string  `json:"category"`
}

type instructionV2AIAttempt struct {
	NodeID        *int64
	NodeName      string
	ReviewerModel string
	FieldName     string
	SHA256        string
	Result        string
	Confidence    float64
	Reason        string
	Category      string
	PromptVersion string
	Sampled       bool
	Cached        bool
	LatencyMS     int
}

type instructionV2AIOutcome struct {
	Result        string
	ReviewedField string
	ApprovedField InstructionV2Field
	Attempts      []instructionV2AIAttempt
	Latency       time.Duration
}

type instructionV2PersistEvent struct {
	Event     InstructionV2Event
	Evidence  []instructionV2EvidenceWrite
	Reviews   []instructionV2AIAttempt
	Candidate *instructionV2CandidateWrite
}

type instructionV2EvidenceWrite struct {
	FieldName    string
	SHA256       string
	StorageKind  string
	Ciphertext   []byte
	ContentBytes int64
	StoredBytes  int
	ExpiresAt    time.Time
}

type instructionV2ManualHashWrite struct {
	SHA256             string
	Name               string
	Note               string
	Status             string
	Source             string
	ContentBytes       int64
	RawStorage         string
	RawCiphertext      []byte
	StoredBytes        int
	ScopeIDs           []int64
	CandidateExpiresAt *time.Time
}

type instructionV2CandidateWrite struct {
	SHA256             string
	Name               string
	Note               string
	ObservedField      string
	ContentBytes       int64
	RawStorage         string
	RawCiphertext      []byte
	StoredBytes        int
	AISampled          bool
	ScopeID            int64
	ReviewerNodeID     *int64
	ReviewerModel      string
	PromptVersion      string
	Confidence         float64
	ReviewReason       string
	ReviewCategory     string
	CandidateExpiresAt time.Time
}

type instructionV2PersistResult struct {
	EventID int64
	HashID  *int64
}

type instructionV2AsyncJob struct {
	request   Request
	snapshot  *instructionV2Snapshot
	profile   instructionV2ClientRuntime
	scopes    []instructionV2ScopeRuntime
	fields    instructionV2ParsedFields
	bodyBytes int64
	startedAt time.Time
	weight    int64
}
