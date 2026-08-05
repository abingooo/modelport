package securityaudit

import (
	"context"
	"errors"
	"time"
)

const (
	SettingKeyInstructionAuditEnabled    = "instruction_audit_enabled"
	InstructionConfigInvalidationChannel = "modelport:instruction_audit:config:invalidate"
	InstructionErrorCodeRejected         = "request_rejected"
	InstructionClientMessage             = "Request rejected by security policy."
)

var ErrInstructionAuditNoEffectiveGroupRules = errors.New("instruction audit requires effective group rules")

type InstructionEngine interface {
	EvaluateInstruction(context.Context, Request) *InstructionDecision
}

type InstructionFieldResult struct {
	Present bool   `json:"present"`
	SHA256  string `json:"sha256"`
	Result  string `json:"result"`
}

type InstructionDecision struct {
	Applicable    bool                   `json:"applicable"`
	Allow         bool                   `json:"allow"`
	Unavailable   bool                   `json:"unavailable"`
	Reason        string                 `json:"reason"`
	Instructions  InstructionFieldResult `json:"instructions"`
	Input1        InstructionFieldResult `json:"input1"`
	RuleSetIDs    []int64                `json:"rule_set_ids"`
	ConfigVersion int64                  `json:"config_version"`
	Latency       time.Duration          `json:"-"`
}

type InstructionHashEntry struct {
	ID             int64      `json:"id"`
	Digest         string     `json:"digest"`
	Name           string     `json:"name"`
	Note           string     `json:"note"`
	ObservedSource string     `json:"observed_source"`
	ClientName     string     `json:"client_name"`
	ClientVersion  string     `json:"client_version"`
	Status         string     `json:"status"`
	ValidFrom      *time.Time `json:"valid_from,omitempty"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
	CreatedBy      *int64     `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateInstructionHashRequest struct {
	Digest         string     `json:"digest"`
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
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Version     int64                  `json:"version"`
	Hashes      []InstructionHashEntry `json:"hashes"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type SaveInstructionRuleSetRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Enabled     bool    `json:"enabled"`
	HashIDs     []int64 `json:"hash_ids"`
}

type InstructionGroupBinding struct {
	ID          int64     `json:"id"`
	GroupID     int64     `json:"group_id"`
	GroupName   string    `json:"group_name"`
	Platform    string    `json:"platform"`
	GroupStatus string    `json:"group_status"`
	RuleSetID   int64     `json:"rule_set_id"`
	RuleSetName string    `json:"rule_set_name"`
	Enabled     bool      `json:"enabled"`
	Effective   bool      `json:"effective"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SaveInstructionGroupBindingsRequest struct {
	GroupIDs  []int64 `json:"group_ids"`
	RuleSetID int64   `json:"rule_set_id"`
	Enabled   bool    `json:"enabled"`
}

type InstructionGroupOption struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
}

type InstructionEvent struct {
	ID                 int64                  `json:"id"`
	RequestID          string                 `json:"request_id"`
	UserID             *int64                 `json:"user_id,omitempty"`
	UserEmailSnapshot  string                 `json:"user_email"`
	APIKeyID           *int64                 `json:"api_key_id,omitempty"`
	GroupID            *int64                 `json:"group_id,omitempty"`
	GroupNameSnapshot  string                 `json:"group_name"`
	Model              string                 `json:"model"`
	Endpoint           string                 `json:"endpoint"`
	Stage              string                 `json:"stage"`
	Instructions       InstructionFieldResult `json:"instructions"`
	Input1             InstructionFieldResult `json:"input1"`
	Decision           string                 `json:"decision"`
	Reason             string                 `json:"reason"`
	RuleSetIDs         []int64                `json:"rule_set_ids"`
	ConfigVersion      int64                  `json:"config_version"`
	LatencyMS          int                    `json:"latency_ms"`
	NotificationStatus string                 `json:"notification_status"`
	CreatedAt          time.Time              `json:"created_at"`
}

type InstructionEventPage struct {
	Items    []InstructionEvent `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Pages    int                `json:"pages"`
}

type InstructionOverview struct {
	Enabled             bool       `json:"enabled"`
	ConfigVersion       int64      `json:"config_version"`
	LoadedAt            *time.Time `json:"loaded_at,omitempty"`
	LoadError           string     `json:"load_error"`
	HashCount           int64      `json:"hash_count"`
	ActiveHashCount     int64      `json:"active_hash_count"`
	RuleSetCount        int64      `json:"rule_set_count"`
	AuditedGroupCount   int64      `json:"audited_group_count"`
	EffectiveGroupCount int64      `json:"effective_group_count"`
	PendingEmailCount   int64      `json:"pending_email_count"`
	QueuedEventCount    int64      `json:"queued_event_count"`
	DroppedEventCount   int64      `json:"dropped_event_count"`
	PersistFailureCount int64      `json:"persist_failure_count"`
}

type UpdateInstructionEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type CreateInstructionCandidateRequest struct {
	Source        string `json:"source"`
	Name          string `json:"name"`
	Note          string `json:"note"`
	ClientName    string `json:"client_name"`
	ClientVersion string `json:"client_version"`
}

type instructionPolicy struct {
	RuleSetIDs []int64
	Hashes     []instructionPolicyHash
}

type instructionPolicyHash struct {
	Digest     [32]byte
	ValidFrom  time.Time
	ValidUntil time.Time
}

type instructionSnapshot struct {
	Enabled       bool
	ConfigVersion int64
	AuditedGroups map[int64]struct{}
	Policies      map[int64]instructionPolicy
	LoadedAt      time.Time
}
