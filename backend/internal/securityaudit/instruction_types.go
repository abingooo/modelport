package securityaudit

import (
	"context"
	"errors"
	"time"
)

const (
	SettingKeyInstructionAuditEnabled          = "instruction_audit_enabled"
	SettingKeyInstructionEvidenceRetentionDays = "instruction_audit_evidence_retention_days"
	InstructionConfigInvalidationChannel       = "modelport:instruction_audit:config:invalidate"
	InstructionErrorCodeRejected               = "request_rejected"
	InstructionClientMessage                   = "Request rejected by security policy."
	InstructionOutcomeBlocked                  = "blocked"
	InstructionOutcomePolicyAllow              = "policy_allow"
	InstructionOutcomeAIPass                   = "ai_pass"
	InstructionOutcomeHashPass                 = "hash_pass"
	InstructionOutcomeExceptionPass            = "exception_pass"
	InstructionPolicyActionBlock               = "block"
	InstructionPolicyActionAllowAndRecord      = "allow_and_record"
)

var ErrInstructionAuditNoEffectiveGroupRules = errors.New("instruction audit requires effective group rules")
var errInvalidInstructionClientType = errors.New("invalid instruction audit client type")

type InstructionEngine interface {
	EvaluateInstruction(context.Context, Request) *InstructionDecision
}

type InstructionFieldResult struct {
	Present   bool   `json:"present"`
	SHA256    string `json:"sha256"`
	Result    string `json:"result"`
	Plaintext string `json:"-"`
}

type InstructionDecision struct {
	EventID       int64                  `json:"-"`
	Applicable    bool                   `json:"applicable"`
	Allow         bool                   `json:"allow"`
	Unavailable   bool                   `json:"unavailable"`
	Reason        string                 `json:"reason"`
	InitialReason string                 `json:"initial_reason"`
	FinalReason   string                 `json:"final_reason"`
	FinalOutcome  string                 `json:"final_outcome"`
	PolicyAction  string                 `json:"policy_action"`
	Instructions  InstructionFieldResult `json:"instructions"`
	Input1        InstructionFieldResult `json:"input1"`
	RuleSetIDs    []int64                `json:"rule_set_ids"`
	ConfigVersion int64                  `json:"config_version"`
	BodyBytes     int64                  `json:"body_bytes"`
	AIReviewID    *int64                 `json:"ai_review_id,omitempty"`
	AlertEnabled  bool                   `json:"-"`
	Latency       time.Duration          `json:"-"`
	AILatency     time.Duration          `json:"-"`
}

type InstructionHashEntry struct {
	ID              int64                   `json:"id"`
	Digest          string                  `json:"digest"`
	Name            string                  `json:"name"`
	Note            string                  `json:"note"`
	ObservedSource  string                  `json:"observed_source"`
	ClientName      string                  `json:"client_name"`
	ClientVersion   string                  `json:"client_version"`
	Status          string                  `json:"status"`
	HashAlgorithm   string                  `json:"hash_algorithm"`
	Normalization   string                  `json:"normalization_version"`
	FieldName       string                  `json:"field_name"`
	RawStatus       string                  `json:"raw_content_status"`
	ContentBytes    int                     `json:"content_bytes"`
	KeyVersion      string                  `json:"encryption_key_version,omitempty"`
	RawExpiresAt    *time.Time              `json:"raw_expires_at,omitempty"`
	Sources         []InstructionHashSource `json:"sources,omitempty"`
	Scopes          []InstructionHashScope  `json:"scopes,omitempty"`
	ScopeSource     string                  `json:"scope_source,omitempty"`
	ScopeStatus     string                  `json:"scope_status,omitempty"`
	ScopeValidUntil *time.Time              `json:"scope_valid_until,omitempty"`
	ValidFrom       *time.Time              `json:"valid_from,omitempty"`
	ValidUntil      *time.Time              `json:"valid_until,omitempty"`
	CreatedBy       *int64                  `json:"created_by,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

type InstructionHashScope struct {
	RuleSetID      int64      `json:"rule_set_id"`
	RuleSetName    string     `json:"rule_set_name"`
	RuleSetEnabled bool       `json:"rule_set_enabled"`
	SystemManaged  bool       `json:"system_managed"`
	SourceType     string     `json:"source_type"`
	Status         string     `json:"status"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
	BindingID      *int64     `json:"binding_id,omitempty"`
	GroupID        *int64     `json:"group_id,omitempty"`
	GroupName      string     `json:"group_name"`
	ClientTypes    []string   `json:"client_types"`
	BindingEnabled bool       `json:"binding_enabled"`
	UpdatedBy      *int64     `json:"updated_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateInstructionHashRequest struct {
	Digest         string     `json:"digest"`
	RawContent     string     `json:"raw_content"`
	SourceType     string     `json:"source_type"`
	Name           string     `json:"name"`
	Note           string     `json:"note"`
	ObservedSource string     `json:"observed_source"`
	ClientName     string     `json:"client_name"`
	ClientVersion  string     `json:"client_version"`
	Status         string     `json:"status"`
	ValidFrom      *time.Time `json:"valid_from"`
	ValidUntil     *time.Time `json:"valid_until"`
}

type UpdateInstructionHashRequest struct {
	Name            *string    `json:"name"`
	Note            *string    `json:"note"`
	ObservedSource  *string    `json:"observed_source"`
	ClientName      *string    `json:"client_name"`
	ClientVersion   *string    `json:"client_version"`
	Status          *string    `json:"status"`
	ValidFrom       *time.Time `json:"valid_from"`
	ValidUntil      *time.Time `json:"valid_until"`
	ClearValidFrom  bool       `json:"clear_valid_from"`
	ClearValidUntil bool       `json:"clear_valid_until"`
}

type InstructionRuleSet struct {
	ID               int64                    `json:"id"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description"`
	Enabled          bool                     `json:"enabled"`
	AllowEmptyFields bool                     `json:"allow_empty_fields"`
	SystemManaged    bool                     `json:"system_managed"`
	SystemKey        string                   `json:"system_key,omitempty"`
	Version          int64                    `json:"version"`
	Hashes           []InstructionHashEntry   `json:"hashes"`
	AllowedUsers     []InstructionRuleSetUser `json:"allowed_users"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

type InstructionRuleSetUser struct {
	ID      int64  `json:"id"`
	Email   string `json:"email"`
	Deleted bool   `json:"deleted"`
}

type SaveInstructionRuleSetRequest struct {
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Enabled          bool    `json:"enabled"`
	AllowEmptyFields bool    `json:"allow_empty_fields"`
	HashIDs          []int64 `json:"hash_ids"`
	AllowedUserIDs   []int64 `json:"allowed_user_ids"`
}

type InstructionGroupBinding struct {
	ID            int64     `json:"id"`
	GroupID       int64     `json:"group_id"`
	GroupName     string    `json:"group_name"`
	Platform      string    `json:"platform"`
	GroupStatus   string    `json:"group_status"`
	RuleSetID     int64     `json:"rule_set_id"`
	RuleSetName   string    `json:"rule_set_name"`
	SystemManaged bool      `json:"system_managed"`
	ClientTypes   []string  `json:"client_types"`
	Enabled       bool      `json:"enabled"`
	Effective     bool      `json:"effective"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SaveInstructionGroupBindingsRequest struct {
	GroupIDs    []int64  `json:"group_ids"`
	RuleSetID   int64    `json:"rule_set_id"`
	ClientTypes []string `json:"client_types"`
	Enabled     bool     `json:"enabled"`
}

type InstructionGroupOption struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
}

type InstructionEvent struct {
	ID                     int64                  `json:"id"`
	RequestID              string                 `json:"request_id"`
	UserID                 *int64                 `json:"user_id,omitempty"`
	UserEmailSnapshot      string                 `json:"user_email"`
	APIKeyID               *int64                 `json:"api_key_id,omitempty"`
	GroupID                *int64                 `json:"group_id,omitempty"`
	GroupNameSnapshot      string                 `json:"group_name"`
	ClientType             string                 `json:"client_type"`
	ClientUserAgent        string                 `json:"client_user_agent"`
	Model                  string                 `json:"model"`
	Endpoint               string                 `json:"endpoint"`
	Stage                  string                 `json:"stage"`
	Instructions           InstructionFieldResult `json:"instructions"`
	Input1                 InstructionFieldResult `json:"input1"`
	Decision               string                 `json:"decision"`
	Reason                 string                 `json:"reason"`
	InitialReason          string                 `json:"initial_reason"`
	FinalReason            string                 `json:"final_reason"`
	FinalOutcome           string                 `json:"final_outcome"`
	PolicyAction           string                 `json:"policy_action"`
	RuleSetIDs             []int64                `json:"rule_set_ids"`
	ConfigVersion          int64                  `json:"config_version"`
	BodyBytes              *int64                 `json:"body_bytes,omitempty"`
	LatencyMS              int                    `json:"latency_ms"`
	AuditLatencyMS         int                    `json:"audit_latency_ms"`
	AILatencyMS            *int                   `json:"ai_latency_ms,omitempty"`
	AIReviewID             *int64                 `json:"ai_review_id,omitempty"`
	EvidenceStatus         string                 `json:"evidence_status"`
	EvidenceExpiresAt      *time.Time             `json:"evidence_expires_at,omitempty"`
	UserNotificationStatus string                 `json:"user_notification_status"`
	OpsNotificationStatus  string                 `json:"ops_notification_status"`
	CreatedAt              time.Time              `json:"created_at"`
}

type InstructionEventFilter struct {
	EventID            int64      `json:"event_id,omitempty"`
	Query              string     `json:"q,omitempty"`
	UserID             int64      `json:"user_id,omitempty"`
	Model              string     `json:"model,omitempty"`
	From               *time.Time `json:"from,omitempty"`
	To                 *time.Time `json:"to,omitempty"`
	GroupIDs           []int64    `json:"group_ids,omitempty"`
	ClientTypes        []string   `json:"client_types,omitempty"`
	Reasons            []string   `json:"reasons,omitempty"`
	InitialReasons     []string   `json:"initial_reasons,omitempty"`
	FinalReasons       []string   `json:"final_reasons,omitempty"`
	Outcomes           []string   `json:"outcomes,omitempty"`
	InstructionResults []string   `json:"instructions_results,omitempty"`
	Input1Results      []string   `json:"input1_results,omitempty"`
	UserNotifications  []string   `json:"user_notifications,omitempty"`
	OpsNotifications   []string   `json:"ops_notifications,omitempty"`
}

type InstructionEvidence struct {
	Source         string
	Digest         string
	Ciphertext     []byte
	KeyVersion     string
	PlaintextBytes int
	ExpiresAt      time.Time
}

type InstructionEvidenceField struct {
	Source           string `json:"source"`
	Available        bool   `json:"available"`
	Plaintext        string `json:"plaintext,omitempty"`
	SHA256           string `json:"sha256"`
	PlaintextBytes   int    `json:"plaintext_bytes"`
	RecomputedSHA256 string `json:"recomputed_sha256,omitempty"`
	DigestConsistent bool   `json:"digest_consistent"`
}

type InstructionEvidenceReview struct {
	EventID     int64                      `json:"event_id"`
	RequestID   string                     `json:"request_id"`
	Status      string                     `json:"status"`
	ExpiresAt   *time.Time                 `json:"expires_at,omitempty"`
	Fields      []InstructionEvidenceField `json:"fields"`
	AccessCount int64                      `json:"access_count"`
}

type InstructionEvidenceAccess struct {
	ActorID   int64
	Action    string
	Source    string
	RequestID string
	ClientIP  string
	UserAgent string
	Succeeded bool
	ErrorCode string
}

type RecordInstructionEvidenceAccessRequest struct {
	Source string `json:"source"`
}

type InstructionEventPage struct {
	Items    []InstructionEvent `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Pages    int                `json:"pages"`
}

type InstructionDeletePreview struct {
	MatchedCount  int64                  `json:"matched_count"`
	FilterSummary InstructionEventFilter `json:"filter_summary"`
	SnapshotMaxID int64                  `json:"snapshot_max_id"`
	FilterHash    string                 `json:"filter_hash"`
}

type InstructionDeleteResult struct {
	DeletedEvents int64 `json:"deleted_events"`
}

type DeleteInstructionEventsRequest struct {
	IDs []int64 `json:"ids"`
}

type DeleteInstructionEventsByFilterRequest struct {
	Filter        InstructionEventFilter `json:"filter"`
	SnapshotMaxID int64                  `json:"snapshot_max_id"`
	FilterHash    string                 `json:"filter_hash"`
	Confirm       bool                   `json:"confirm"`
}

type AddInstructionEventToRuleSetRequest struct {
	RuleSetID       int64    `json:"rule_set_id"`
	Sources         []string `json:"sources"`
	ReviewConfirmed bool     `json:"review_confirmed"`
}

type AddInstructionEventToRuleSetResult struct {
	RuleSetID       int64   `json:"rule_set_id"`
	HashIDs         []int64 `json:"hash_ids"`
	CreatedHashes   int     `json:"created_hashes"`
	ActivatedHashes int     `json:"activated_hashes"`
	AttachedHashes  int     `json:"attached_hashes"`
	ConfigVersion   int64   `json:"config_version"`
}

type InstructionOverview struct {
	Enabled                        bool       `json:"enabled"`
	ConfigVersion                  int64      `json:"config_version"`
	LoadedAt                       *time.Time `json:"loaded_at,omitempty"`
	LoadError                      string     `json:"load_error"`
	HashCount                      int64      `json:"hash_count"`
	ActiveHashCount                int64      `json:"active_hash_count"`
	RuleSetCount                   int64      `json:"rule_set_count"`
	AuditedGroupCount              int64      `json:"audited_group_count"`
	EffectiveGroupCount            int64      `json:"effective_group_count"`
	PendingEmailCount              int64      `json:"pending_email_count"`
	QueuedEventCount               int64      `json:"queued_event_count"`
	DroppedEventCount              int64      `json:"dropped_event_count"`
	PersistFailureCount            int64      `json:"persist_failure_count"`
	EvidenceEncryptionAvailable    bool       `json:"evidence_encryption_available"`
	EvidenceRetentionDays          int        `json:"evidence_retention_days"`
	MaxBodyBytes                   int64      `json:"max_body_bytes"`
	HTTPGatewayMaxBodyBytes        int64      `json:"http_gateway_max_body_bytes"`
	WebSocketGatewayMaxBodyBytes   int64      `json:"websocket_gateway_max_body_bytes"`
	EffectiveHTTPMaxBodyBytes      int64      `json:"effective_http_max_body_bytes"`
	EffectiveWebSocketMaxBodyBytes int64      `json:"effective_websocket_max_body_bytes"`
	ParseTimeoutMS                 int        `json:"parse_timeout_ms"`
	MaxInflightBodyBytes           int64      `json:"max_inflight_body_bytes"`
	AIEnabled                      bool       `json:"ai_enabled"`
	TranslationEnabled             bool       `json:"translation_enabled"`
	TranslationPendingCount        int64      `json:"translation_pending_count"`
	TranslationProcessingCount     int64      `json:"translation_processing_count"`
	TranslationFailedCount         int64      `json:"translation_failed_count"`
	TranslationActiveWorkers       int64      `json:"translation_active_workers"`
	TranslationProcessedTotal      int64      `json:"translation_processed_total"`
	TranslationWorkerFailTotal     int64      `json:"translation_worker_fail_total"`
	PersistedOutcomeCount          int64      `json:"persisted_outcome_count"`
	AggregatedOutcomeCount         int64      `json:"aggregated_outcome_count"`
	ExpiredAggregateEventCount     int64      `json:"expired_aggregate_event_count"`
	StatisticsLossCount            int64      `json:"statistics_loss_count"`
	AuditLatencySampleCount        int64      `json:"audit_latency_sample_count"`
	AuditLatencyP95MS              int64      `json:"audit_latency_p95_ms"`
	AuditLatencyP99MS              int64      `json:"audit_latency_p99_ms"`
	AILatencySampleCount           int64      `json:"ai_latency_sample_count"`
	AILatencyP95MS                 int64      `json:"ai_latency_p95_ms"`
	AILatencyP99MS                 int64      `json:"ai_latency_p99_ms"`
}

type UpdateInstructionEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type UpdateInstructionEvidenceRetentionRequest struct {
	Days int `json:"days"`
}

type CreateInstructionCandidateRequest struct {
	Source          string `json:"source"`
	Name            string `json:"name"`
	Note            string `json:"note"`
	ClientName      string `json:"client_name"`
	ClientVersion   string `json:"client_version"`
	ReviewConfirmed bool   `json:"review_confirmed"`
}

type instructionPolicy struct {
	RuleSetIDs       []int64
	Hashes           []instructionPolicyHash
	AllowedUsers     map[int64]struct{}
	AllowEmptyFields bool
}

type instructionPolicyHash struct {
	Digest     [32]byte
	ValidFrom  time.Time
	ValidUntil time.Time
}

type instructionSnapshot struct {
	Enabled             bool
	ConfigVersion       int64
	AuditedGroups       map[int64]struct{}
	Policies            map[int64]instructionPolicy
	AuditedClientScopes map[instructionPolicyScope]struct{}
	ClientPolicies      map[instructionPolicyScope]instructionPolicy
	ReasonPolicies      map[string]InstructionReasonPolicy
	Runtime             InstructionRuntimeConfig
	LoadedAt            time.Time
}

type instructionPolicyScope struct {
	GroupID    int64
	ClientType string
}
