package securityaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	DefaultWorkerCount              = 4
	MaxWorkerCount                  = 32
	DefaultQueueCapacity            = 32768
	MaxQueueCapacity                = 100000
	DefaultTimeoutMS                = 3000
	MinTimeoutMS                    = 100
	MaxTimeoutMS                    = 30000
	DefaultInputLimit               = 4000
	MinInputLimit                   = 128
	MaxInputLimit                   = 100000
	PromptAuditModelContractVersion = 2
	DefaultMaxOutputTokens          = 256
	MinMaxOutputTokens              = 64
	MaxMaxOutputTokens              = 4096
	DefaultPayloadTTL               = 30 * time.Minute
)

const (
	ResponseModeAuto       = "auto"
	ResponseModeJSONSchema = "json_schema"
	ResponseModeJSONObject = "json_object"
	ResponseModeTextJSON   = "text_json"
)

type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// ConfigStore is the injectable boundary between hot-path prompt auditing and
// the concrete settings/PostgreSQL/Redis-backed configuration manager.
type ConfigStore interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	Active() (ActiveConfig, bool)
	EffectiveMode() Mode
	// BlockingActivationDegraded is true when storage intent requires blocking
	// but no usable blocking snapshot is active (cold start or failed reload).
	// It must stay false when blocking is not intended, even if config is
	// untrusted—otherwise default-off deployments fail closed for all traffic.
	BlockingActivationDegraded() bool
	Public() (PublicConfig, error)
	Save(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error)
	RuntimeState() (expected int64, active int64, loadedAt *time.Time, loadError string)
	Encrypt(value string) (string, error)
	Decrypt(value string) (string, error)
}

type StorageEndpoint struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Protocol              string `json:"protocol"`
	BaseURL               string `json:"base_url"`
	Model                 string `json:"model"`
	TokenCiphertext       string `json:"token_ciphertext,omitempty"`
	TimeoutMS             int    `json:"timeout_ms"`
	InputLimit            int    `json:"input_limit"`
	ResponseMode          string `json:"response_mode"`
	MaxOutputTokens       int    `json:"max_output_tokens"`
	EffectiveResponseMode string `json:"effective_response_mode,omitempty"`
	RequiresReconfigure   bool   `json:"requires_reconfigure,omitempty"`
	Enabled               bool   `json:"enabled"`
}

type storageConfig struct {
	ModelContractVersion   int               `json:"model_contract_version"`
	Enabled                bool              `json:"enabled"`
	BlockingEnabled        bool              `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool              `json:"blocking_latest_turn_only"`
	StorePassEvents        bool              `json:"store_pass_events"`
	Strategy               string            `json:"strategy"`
	WorkerCount            int               `json:"worker_count"`
	QueueCapacity          int               `json:"queue_capacity"`
	Scanners               []string          `json:"scanners"`
	AllGroups              bool              `json:"all_groups"`
	GroupIDs               []int64           `json:"group_ids"`
	Endpoints              []StorageEndpoint `json:"endpoints"`
	ConfigVersion          int64             `json:"config_version"`
	UpdatedAt              time.Time         `json:"updated_at"`
	UpdatedBy              int64             `json:"updated_by"`
	ChangeSummary          string            `json:"change_summary"`
}

type ActiveEndpoint struct {
	ID                    string
	Name                  string
	Protocol              string
	BaseURL               string
	Model                 string
	Token                 string
	TimeoutMS             int
	InputLimit            int
	ResponseMode          string
	MaxOutputTokens       int
	EffectiveResponseMode string
	RequiresReconfigure   bool
	Enabled               bool
	// TokenInvalid marks an endpoint whose persisted token ciphertext cannot be
	// decrypted with the current encryption key (key changed or auto-generated
	// on restart). The endpoint is kept visible for admins but excluded from
	// runtime use until the token is re-entered or cleared (issue #4887).
	TokenInvalid bool
}

type ActiveConfig struct {
	ModelContractVersion   int
	RiskControlEnabled     bool
	Enabled                bool
	BlockingEnabled        bool
	BlockingLatestTurnOnly bool
	StorePassEvents        bool
	Strategy               string
	WorkerCount            int
	QueueCapacity          int
	Scanners               []string
	AllGroups              bool
	GroupIDs               []int64
	Endpoints              []ActiveEndpoint
	ConfigVersion          int64
	UpdatedAt              time.Time
	UpdatedBy              int64
	ChangeSummary          string
}

type PublicEndpoint struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Protocol              string `json:"protocol"`
	BaseURL               string `json:"base_url"`
	Model                 string `json:"model"`
	TimeoutMS             int    `json:"timeout_ms"`
	InputLimit            int    `json:"input_limit"`
	ResponseMode          string `json:"response_mode"`
	MaxOutputTokens       int    `json:"max_output_tokens"`
	EffectiveResponseMode string `json:"effective_response_mode"`
	RequiresReconfigure   bool   `json:"requires_reconfigure"`
	Enabled               bool   `json:"enabled"`
	HasToken              bool   `json:"has_token"`
	TokenStatus           string `json:"token_status"`
}

type PublicConfig struct {
	ModelContractVersion   int              `json:"model_contract_version"`
	Enabled                bool             `json:"enabled"`
	BlockingEnabled        bool             `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool             `json:"blocking_latest_turn_only"`
	StorePassEvents        bool             `json:"store_pass_events"`
	EffectiveMode          Mode             `json:"effective_mode"`
	Strategy               string           `json:"strategy"`
	WorkerCount            int              `json:"worker_count"`
	QueueCapacity          int              `json:"queue_capacity"`
	Scanners               []string         `json:"scanners"`
	AllGroups              bool             `json:"all_groups"`
	GroupIDs               []int64          `json:"group_ids"`
	Endpoints              []PublicEndpoint `json:"endpoints"`
	ConfigVersion          int64            `json:"config_version"`
	UpdatedAt              time.Time        `json:"updated_at"`
	UpdatedBy              int64            `json:"updated_by"`
	ChangeSummary          string           `json:"change_summary"`
}

type UpdateEndpoint struct {
	ID                    string  `json:"id" binding:"required"`
	Name                  string  `json:"name" binding:"required"`
	Protocol              string  `json:"protocol"`
	BaseURL               string  `json:"base_url" binding:"required"`
	Model                 string  `json:"model"`
	Token                 string  `json:"token,omitempty"`
	ClearToken            bool    `json:"clear_token"`
	TimeoutMS             int     `json:"timeout_ms"`
	InputLimit            int     `json:"input_limit"`
	ResponseMode          *string `json:"response_mode,omitempty"`
	MaxOutputTokens       *int    `json:"max_output_tokens,omitempty"`
	EffectiveResponseMode string  `json:"-"`
	RequiresReconfigure   bool    `json:"-"`
	ProbeVerified         bool    `json:"-"`
	Enabled               bool    `json:"enabled"`
}

type UpdateConfigRequest struct {
	ExpectedConfigVersion  int64            `json:"expected_config_version" binding:"required"`
	Enabled                bool             `json:"enabled"`
	BlockingEnabled        bool             `json:"blocking_enabled"`
	BlockingLatestTurnOnly bool             `json:"blocking_latest_turn_only"`
	StorePassEvents        bool             `json:"store_pass_events"`
	Strategy               string           `json:"strategy"`
	WorkerCount            int              `json:"worker_count"`
	QueueCapacity          int              `json:"queue_capacity"`
	Scanners               []string         `json:"scanners"`
	AllGroups              bool             `json:"all_groups"`
	GroupIDs               []int64          `json:"group_ids"`
	Endpoints              []UpdateEndpoint `json:"endpoints"`
}

func DefaultStorageConfig() storageConfig {
	return storageConfig{
		ModelContractVersion:   PromptAuditModelContractVersion,
		Enabled:                false,
		BlockingEnabled:        false,
		BlockingLatestTurnOnly: false,
		StorePassEvents:        false,
		Strategy:               "priority",
		WorkerCount:            DefaultWorkerCount,
		QueueCapacity:          DefaultQueueCapacity,
		Scanners:               append([]string(nil), AllScannerIDs...),
		AllGroups:              true,
		GroupIDs:               []int64{},
		Endpoints:              []StorageEndpoint{},
		ConfigVersion:          1,
	}
}

func ParseStorageConfig(raw string) (storageConfig, error) {
	cfg := DefaultStorageConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	var marker struct {
		ModelContractVersion *int `json:"model_contract_version"`
	}
	if err := json.Unmarshal([]byte(raw), &marker); err != nil {
		return storageConfig{}, fmt.Errorf("decode prompt audit config: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return storageConfig{}, fmt.Errorf("decode prompt audit config: %w", err)
	}
	if marker.ModelContractVersion == nil || *marker.ModelContractVersion != PromptAuditModelContractVersion {
		retireLegacyStorageConfig(&cfg)
	}
	normalizeStorageConfig(&cfg)
	if cfg.Enabled && !hasUsableGenericEndpoint(cfg.Endpoints) {
		cfg.Enabled = false
		cfg.BlockingEnabled = false
	}
	if err := validateStorageConfig(cfg); err != nil {
		return storageConfig{}, err
	}
	return cfg, nil
}

func retireLegacyStorageConfig(cfg *storageConfig) {
	if cfg == nil {
		return
	}
	cfg.ModelContractVersion = PromptAuditModelContractVersion
	cfg.Enabled = false
	cfg.BlockingEnabled = false
	for i := range cfg.Endpoints {
		cfg.Endpoints[i].Enabled = false
		cfg.Endpoints[i].RequiresReconfigure = true
		cfg.Endpoints[i].EffectiveResponseMode = ""
	}
}

func hasUsableGenericEndpoint(endpoints []StorageEndpoint) bool {
	for _, endpoint := range endpoints {
		if endpoint.Enabled && !endpoint.RequiresReconfigure && strings.TrimSpace(endpoint.Model) != "" {
			return true
		}
	}
	return false
}

func normalizeStorageConfig(cfg *storageConfig) {
	if cfg == nil {
		return
	}
	if cfg.ConfigVersion < 1 {
		cfg.ConfigVersion = 1
	}
	cfg.ModelContractVersion = PromptAuditModelContractVersion
	if strings.TrimSpace(cfg.Strategy) == "" {
		cfg.Strategy = "priority"
	}
	if cfg.WorkerCount == 0 {
		cfg.WorkerCount = DefaultWorkerCount
	}
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = DefaultQueueCapacity
	}
	if len(cfg.Scanners) == 0 {
		cfg.Scanners = append([]string(nil), AllScannerIDs...)
	}
	cfg.Scanners = canonicalScannerIDs(cfg.Scanners)
	// Prompt Audit inherits Instruction Audit V2 scope. These legacy fields are
	// accepted for old clients but no longer carry runtime meaning.
	cfg.AllGroups = true
	cfg.GroupIDs = []int64{}
	// Preserve an invalid blocking-without-audit combination so validation can
	// reject it instead of silently changing administrator intent.
	for i := range cfg.Endpoints {
		ep := &cfg.Endpoints[i]
		ep.ID = strings.TrimSpace(ep.ID)
		ep.Name = strings.TrimSpace(ep.Name)
		ep.Protocol = strings.TrimSpace(ep.Protocol)
		if ep.Protocol == "" {
			ep.Protocol = "openai_compatible"
		}
		ep.BaseURL = strings.TrimSpace(ep.BaseURL)
		ep.Model = strings.TrimSpace(ep.Model)
		ep.ResponseMode = normalizedResponseMode(ep.ResponseMode)
		if ep.MaxOutputTokens == 0 {
			ep.MaxOutputTokens = DefaultMaxOutputTokens
		}
		if ep.ResponseMode != ResponseModeAuto {
			ep.EffectiveResponseMode = ep.ResponseMode
		} else {
			ep.EffectiveResponseMode = strings.ToLower(strings.TrimSpace(ep.EffectiveResponseMode))
		}
		if ep.RequiresReconfigure {
			ep.EffectiveResponseMode = ""
		}
		if ep.TimeoutMS == 0 {
			ep.TimeoutMS = DefaultTimeoutMS
		}
		if ep.InputLimit == 0 {
			ep.InputLimit = DefaultInputLimit
		}
	}
}

func validateStorageConfig(cfg storageConfig) error {
	if cfg.BlockingEnabled && !cfg.Enabled {
		return infraerrors.BadRequest(ErrorCodeRequiresEnabled, "开启同步阻止前必须先启用提示词审计")
	}
	if cfg.Strategy != "priority" {
		return infraerrors.BadRequest("prompt_audit_invalid_strategy", "提示词审计策略仅支持 priority")
	}
	if cfg.WorkerCount < 1 || cfg.WorkerCount > MaxWorkerCount {
		return infraerrors.BadRequest("prompt_audit_invalid_worker_count", "Worker 数量超出允许范围")
	}
	if cfg.QueueCapacity < 1 || cfg.QueueCapacity > MaxQueueCapacity {
		return infraerrors.BadRequest("prompt_audit_invalid_queue_capacity", "队列容量超出允许范围")
	}
	if len(cfg.Scanners) == 0 {
		return infraerrors.BadRequest("prompt_audit_scanners_required", "至少需要启用一个风险分类")
	}
	seen := make(map[string]struct{}, len(cfg.Endpoints))
	enabled := 0
	for _, ep := range cfg.Endpoints {
		if ep.ID == "" || ep.Name == "" || len(ep.ID) > 128 || len(ep.Name) > 160 {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint", "审计节点 ID 和名称不能为空")
		}
		if _, ok := seen[ep.ID]; ok {
			return infraerrors.BadRequest("prompt_audit_duplicate_endpoint", "审计节点 ID 不能重复")
		}
		seen[ep.ID] = struct{}{}
		if ep.Protocol != "openai_compatible" {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint_protocol", "审计节点仅支持 OpenAI 兼容协议")
		}
		if ep.RequiresReconfigure {
			if ep.Enabled && cfg.Enabled {
				return infraerrors.BadRequest("prompt_audit_endpoint_requires_reconfigure", "旧版审计节点必须重新配置后才能启用")
			}
			continue
		}
		if _, err := NormalizeBaseURL(ep.BaseURL); err != nil {
			return err
		}
		if ep.Model == "" || len(ep.Model) > 255 {
			return infraerrors.BadRequest("prompt_audit_invalid_model", "审计节点模型不能为空且不能超过 255 个字符")
		}
		if isRetiredNativeGuardModel(ep.Model) {
			return infraerrors.BadRequest("prompt_audit_retired_native_model", "旧版专用审核模型不能用于通用审核协议")
		}
		if !validResponseMode(ep.ResponseMode) {
			return infraerrors.BadRequest("prompt_audit_invalid_response_mode", "审计节点响应模式无效")
		}
		if ep.MaxOutputTokens < MinMaxOutputTokens || ep.MaxOutputTokens > MaxMaxOutputTokens {
			return infraerrors.BadRequest("prompt_audit_invalid_max_output_tokens", "审计节点输出 Token 上限超出允许范围")
		}
		if ep.EffectiveResponseMode != "" && !isConcreteResponseMode(ep.EffectiveResponseMode) {
			return infraerrors.BadRequest("prompt_audit_invalid_effective_response_mode", "审计节点实际响应模式无效")
		}
		if ep.Enabled && ep.ResponseMode == ResponseModeAuto && !isConcreteResponseMode(ep.EffectiveResponseMode) {
			return infraerrors.BadRequest("prompt_audit_endpoint_probe_required", "自动响应模式必须通过实测后才能启用")
		}
		if ep.TimeoutMS < MinTimeoutMS || ep.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
		}
		if ep.InputLimit < MinInputLimit || ep.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
		}
		if ep.Enabled {
			enabled++
		}
	}
	if cfg.Enabled && enabled == 0 {
		return infraerrors.BadRequest("prompt_audit_endpoint_required", "启用提示词审计前至少需要启用一个审计节点")
	}
	return nil
}

func validateUpdateConfigRequest(req UpdateConfigRequest) error {
	if strings.TrimSpace(req.Strategy) != "priority" {
		return infraerrors.BadRequest("prompt_audit_invalid_strategy", "提示词审计策略仅支持 priority")
	}
	if req.WorkerCount < 1 || req.WorkerCount > MaxWorkerCount {
		return infraerrors.BadRequest("prompt_audit_invalid_worker_count", "Worker 数量超出允许范围")
	}
	if req.QueueCapacity < 1 || req.QueueCapacity > MaxQueueCapacity {
		return infraerrors.BadRequest("prompt_audit_invalid_queue_capacity", "队列容量超出允许范围")
	}
	if len(req.Scanners) == 0 {
		return infraerrors.BadRequest("prompt_audit_scanners_required", "至少需要启用一个风险分类")
	}
	for _, scanner := range req.Scanners {
		if _, ok := ScannerCatalog[NormalizeCategory(scanner)]; !ok {
			return infraerrors.BadRequest("prompt_audit_invalid_scanner", "提示词审计风险分类无效")
		}
	}
	for _, endpoint := range req.Endpoints {
		if len(strings.TrimSpace(endpoint.ID)) > 128 || len(strings.TrimSpace(endpoint.Name)) > 160 {
			return infraerrors.BadRequest("prompt_audit_invalid_endpoint", "审计节点 ID 或名称超出允许长度")
		}
		if model := strings.TrimSpace(endpoint.Model); model == "" || len(model) > 255 {
			return infraerrors.BadRequest("prompt_audit_invalid_model", "审计节点模型不能为空且不能超过 255 个字符")
		} else if isRetiredNativeGuardModel(model) {
			return infraerrors.BadRequest("prompt_audit_retired_native_model", "旧版专用审核模型不能用于通用审核协议")
		}
		if endpoint.ResponseMode != nil && !validResponseMode(*endpoint.ResponseMode) {
			return infraerrors.BadRequest("prompt_audit_invalid_response_mode", "审计节点响应模式无效")
		}
		if endpoint.MaxOutputTokens != nil && (*endpoint.MaxOutputTokens < MinMaxOutputTokens || *endpoint.MaxOutputTokens > MaxMaxOutputTokens) {
			return infraerrors.BadRequest("prompt_audit_invalid_max_output_tokens", "审计节点输出 Token 上限超出允许范围")
		}
		if endpoint.TimeoutMS < MinTimeoutMS || endpoint.TimeoutMS > MaxTimeoutMS {
			return infraerrors.BadRequest("prompt_audit_invalid_timeout", "审计节点超时超出允许范围")
		}
		if endpoint.InputLimit < MinInputLimit || endpoint.InputLimit > MaxInputLimit {
			return infraerrors.BadRequest("prompt_audit_invalid_input_limit", "审计节点输入上限超出允许范围")
		}
	}
	return nil
}

func validResponseMode(mode string) bool {
	switch normalizedResponseMode(mode) {
	case ResponseModeAuto, ResponseModeJSONSchema, ResponseModeJSONObject, ResponseModeTextJSON:
		return true
	default:
		return false
	}
}

func isRetiredNativeGuardModel(model string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", "/", "").Replace(strings.ToLower(strings.TrimSpace(model)))
	return strings.Contains(normalized, "qwen3guard")
}

func (cfg ActiveConfig) EffectiveMode() Mode {
	if !cfg.RiskControlEnabled || !cfg.Enabled {
		return ModeOff
	}
	if cfg.BlockingEnabled {
		return ModeBlocking
	}
	return ModeAsync
}

func (cfg ActiveConfig) IncludesGroup(groupID *int64) bool {
	return true
}

func (cfg ActiveConfig) EnabledEndpoints() []ActiveEndpoint {
	result := make([]ActiveEndpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		if ep.Enabled && !ep.RequiresReconfigure && !ep.TokenInvalid {
			result = append(result, ep)
		}
	}
	return result
}

// InvalidTokenEndpointIDs lists endpoints whose stored token could not be
// decrypted with the current encryption key.
func (cfg ActiveConfig) InvalidTokenEndpointIDs() []string {
	ids := make([]string, 0)
	for _, ep := range cfg.Endpoints {
		if ep.TokenInvalid {
			ids = append(ids, ep.ID)
		}
	}
	return ids
}

func PublicFromStorage(cfg storageConfig, riskControlEnabled bool, invalidTokenEndpointIDs []string) PublicConfig {
	invalid := make(map[string]struct{}, len(invalidTokenEndpointIDs))
	for _, id := range invalidTokenEndpointIDs {
		invalid[id] = struct{}{}
	}
	scanners := append([]string{}, cfg.Scanners...)
	groupIDs := []int64{}
	endpoints := make([]PublicEndpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		hasToken := strings.TrimSpace(ep.TokenCiphertext) != ""
		status := "missing"
		if hasToken {
			status = "configured"
			if _, ok := invalid[ep.ID]; ok {
				status = "invalid"
			}
		}
		endpoints = append(endpoints, PublicEndpoint{
			ID: ep.ID, Name: ep.Name, Protocol: ep.Protocol, BaseURL: ep.BaseURL,
			Model: ep.Model, TimeoutMS: ep.TimeoutMS, InputLimit: ep.InputLimit,
			ResponseMode: ep.ResponseMode, MaxOutputTokens: ep.MaxOutputTokens,
			EffectiveResponseMode: ep.EffectiveResponseMode, RequiresReconfigure: ep.RequiresReconfigure,
			Enabled: ep.Enabled, HasToken: hasToken, TokenStatus: status,
		})
	}
	active := ActiveConfig{RiskControlEnabled: riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled}
	return PublicConfig{
		ModelContractVersion: cfg.ModelContractVersion,
		Enabled:              cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled, BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly, StorePassEvents: cfg.StorePassEvents,
		EffectiveMode: active.EffectiveMode(), Strategy: cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: scanners, AllGroups: true,
		GroupIDs: groupIDs, Endpoints: endpoints, ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
	}
}

func ActiveFromStorage(cfg storageConfig, riskControlEnabled bool, encryptor SecretEncryptor) (ActiveConfig, error) {
	active := ActiveConfig{
		ModelContractVersion: cfg.ModelContractVersion,
		RiskControlEnabled:   riskControlEnabled, Enabled: cfg.Enabled, BlockingEnabled: cfg.BlockingEnabled,
		BlockingLatestTurnOnly: cfg.BlockingLatestTurnOnly,
		StorePassEvents:        cfg.StorePassEvents, Strategy: cfg.Strategy, WorkerCount: cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity, Scanners: append([]string(nil), cfg.Scanners...), AllGroups: true,
		GroupIDs: []int64{}, ConfigVersion: cfg.ConfigVersion,
		UpdatedAt: cfg.UpdatedAt, UpdatedBy: cfg.UpdatedBy, ChangeSummary: cfg.ChangeSummary,
		Endpoints: make([]ActiveEndpoint, 0, len(cfg.Endpoints)),
	}
	for _, ep := range cfg.Endpoints {
		token := ""
		tokenInvalid := false
		if ep.TokenCiphertext != "" {
			if encryptor == nil {
				return ActiveConfig{}, fmt.Errorf("prompt audit secret encryptor unavailable")
			}
			plain, err := encryptor.Decrypt(ep.TokenCiphertext)
			if err != nil {
				// An undecryptable token (encryption key changed or regenerated)
				// must not take the whole config down: admins would otherwise be
				// locked out of the real config version and unable to recover
				// (issue #4887). Keep the ciphertext persisted, but exclude the
				// endpoint from runtime use until the token is re-entered.
				tokenInvalid = true
			} else {
				token = plain
			}
		}
		active.Endpoints = append(active.Endpoints, ActiveEndpoint{
			ID: ep.ID, Name: ep.Name, Protocol: ep.Protocol, BaseURL: ep.BaseURL, Model: ep.Model,
			Token: token, TimeoutMS: ep.TimeoutMS, InputLimit: ep.InputLimit,
			ResponseMode: ep.ResponseMode, MaxOutputTokens: ep.MaxOutputTokens,
			EffectiveResponseMode: ep.EffectiveResponseMode, RequiresReconfigure: ep.RequiresReconfigure,
			Enabled: ep.Enabled && !ep.RequiresReconfigure && !tokenInvalid, TokenInvalid: tokenInvalid,
		})
	}
	return active, nil
}

func changeSummary(cfg storageConfig) string {
	summary := struct {
		Enabled                bool `json:"enabled"`
		BlockingEnabled        bool `json:"blocking_enabled"`
		BlockingLatestTurnOnly bool `json:"blocking_latest_turn_only"`
		StorePassEvents        bool `json:"store_pass_events"`
		EndpointCount          int  `json:"endpoint_count"`
		ScannerCount           int  `json:"scanner_count"`
	}{cfg.Enabled, cfg.BlockingEnabled, cfg.BlockingLatestTurnOnly, cfg.StorePassEvents, len(cfg.Endpoints), len(cfg.Scanners)}
	raw, _ := json.Marshal(summary)
	return string(raw)
}

func canonicalInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func canonicalScannerIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := NormalizeCategory(value)
		if _, ok := ScannerCatalog[id]; ok {
			seen[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for _, id := range AllScannerIDs {
		if _, ok := seen[id]; ok {
			result = append(result, id)
		}
	}
	return result
}
