export type InstructionAuditMode = 'off' | 'observe' | 'enforce'
export type InstructionHashStatus = 'candidate' | 'active' | 'disabled' | 'revoked'
export type InstructionEventOutcome =
  | 'hash_pass'
  | 'ai_pass'
  | 'blocked'
  | 'empty_pass'
  | 'user_allowlist_pass'
  | 'observe_allow'

export interface InstructionV2Config {
  mode: InstructionAuditMode
  effective_mode: InstructionAuditMode
  risk_control_enabled: boolean
  review_criteria: string
  confidence_threshold: number
  ai_input_max_chars: number
  ai_global_concurrency: number
  ai_queue_wait_ms: number
  ai_total_timeout_ms: number
  ai_cache_ttl_seconds: number
  event_retention_days: number
  evidence_retention_days: number
  candidate_retention_days: number
  raw_full_max_bytes: number
  config_version: number
  updated_by?: number | null
  updated_at: string
  gateway_http_max_body_bytes: number
  gateway_ws_max_body_bytes: number
  evidence_encryption_ready: boolean
  active_scope_count: number
  active_hash_count: number
  enabled_ai_node_count: number
  async_queue_depth: number
  async_queue_capacity: number
  last_config_load_error: string
  last_config_loaded_at?: string | null
}

export type UpdateInstructionV2Config = Pick<
  InstructionV2Config,
  | 'mode'
  | 'review_criteria'
  | 'confidence_threshold'
  | 'ai_input_max_chars'
  | 'ai_global_concurrency'
  | 'ai_queue_wait_ms'
  | 'ai_total_timeout_ms'
  | 'ai_cache_ttl_seconds'
  | 'event_retention_days'
  | 'evidence_retention_days'
  | 'candidate_retention_days'
  | 'raw_full_max_bytes'
> & { expected_config_version: number }

export interface InstructionClientMatcher {
  type: 'prefix' | 'regex'
  value: string
  case_sensitive: boolean
}

export interface InstructionClientProfile {
  id: number
  profile_key: string
  name: string
  description: string
  matchers: InstructionClientMatcher[]
  priority: number
  enabled: boolean
  built_in: boolean
  immutable_internal: boolean
  created_at: string
  updated_at: string
}

export interface SaveInstructionClientProfile {
  profile_key: string
  name: string
  description: string
  matchers: InstructionClientMatcher[]
  priority: number
  enabled: boolean
}

export interface InstructionGroupOption {
  id: number
  name: string
  platform: string
  status: string
}

export interface InstructionScope {
  id: number
  group_id: number
  group_name: string
  group_platform: string
  group_status: string
  client_profile_id?: number | null
  client_profile_key: string
  client_profile_name: string
  enabled: boolean
  effective: boolean
  created_at: string
  updated_at: string
}

export interface SaveInstructionScope {
  group_id: number
  client_profile_id: number | null
  enabled: boolean
}

export interface InstructionUserAllowlistEntry {
  id: number
  user_id: number
  email: string
  username: string
  note: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface InstructionUserOption {
  id: number
  email: string
  username: string
  status: string
}

export interface InstructionHashScope {
  scope_id: number
  group_id: number
  group_name: string
  client_profile_id?: number | null
  client_profile_key: string
  client_profile_name: string
  status: InstructionHashStatus
  source: string
  candidate_expires_at?: string | null
  created_at: string
  updated_at: string
}

export interface InstructionHash {
  id: number
  sha256: string
  name: string
  note: string
  status: InstructionHashStatus
  source: string
  observed_field: string
  hash_algorithm: string
  normalization_version: string
  content_bytes: number
  raw_storage: 'full' | 'sample' | 'unavailable'
  stored_bytes: number
  ai_sampled: boolean
  source_event_id?: number | null
  reviewer_node_id?: number | null
  reviewer_model: string
  prompt_version: string
  confidence?: number | null
  review_reason: string
  review_category: string
  candidate_expires_at?: string | null
  scope_ids: number[]
  scopes: InstructionHashScope[]
  created_at: string
  updated_at: string
}

export interface InstructionHashPage {
  items: InstructionHash[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface SaveInstructionHash {
  raw_content: string
  sha256: string
  source: 'manual' | 'import'
  name: string
  note: string
  status: InstructionHashStatus
  scope_ids: number[]
}

export interface UpdateInstructionHash {
  name?: string
  note?: string
  status?: InstructionHashStatus
  scope_ids?: number[]
  set_scopes?: boolean
}

export interface InstructionField {
  state: 'not_checked' | 'missing' | 'empty' | 'valid' | 'invalid'
  sha256: string
  bytes: number
  partial: boolean
  ai_sampled: boolean
}

export interface InstructionAIReview {
  id: number
  event_id: number
  node_id?: number | null
  node_name: string
  reviewer_model: string
  field_name: 'instructions' | 'input1'
  sha256: string
  result: 'pass' | 'reject' | 'uncertain' | 'error'
  confidence: number
  reason: string
  category: string
  prompt_version: string
  sampled: boolean
  cached: boolean
  latency_ms: number
  created_at: string
}

export interface InstructionEvent {
  id: number
  request_id: string
  user_id?: number | null
  user_email: string
  api_key_id?: number | null
  api_key_name: string
  group_id?: number | null
  group_name: string
  scope_id?: number | null
  client_profile_id?: number | null
  client_key: string
  client_name: string
  client_user_agent: string
  model: string
  endpoint: string
  stage: string
  mode: 'observe' | 'enforce'
  decision: 'allow' | 'block'
  outcome: InstructionEventOutcome
  reason: string
  instructions: InstructionField
  input1: InstructionField
  matched_hash_id?: number | null
  ai_result: 'not_run' | 'pass' | 'reject' | 'uncertain' | 'error' | 'queue_full'
  ai_reviewed_field: string
  ai_sampled: boolean
  audit_latency_ms: number
  ai_latency_ms: number
  body_bytes: number
  config_version: number
  evidence_status: string
  user_notification_status: string
  ops_notification_status: string
  created_at: string
  ai_reviews?: InstructionAIReview[]
}

export interface InstructionEventPage {
  items: InstructionEvent[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface InstructionEventFilters {
  q?: string
  event_id?: number
  user_id?: number
  group_ids?: string
  client_keys?: string
  outcomes?: string
  reasons?: string
  ai_results?: string
  model?: string
  from?: string
  to?: string
}

export interface InstructionStatistics {
  from: string
  to: string
  total: number
  hash_pass: number
  ai_pass: number
  blocked: number
  empty_or_allowlist_pass: number
  ai_failures: number
  block_rate: number
}

export interface InstructionEvidenceField {
  field_name: string
  sha256: string
  storage_kind: 'full' | 'sample'
  plaintext: string
  content_bytes: number
  stored_bytes: number
  digest_consistent: boolean
}

export interface InstructionEvidenceReview {
  resource_type: 'event' | 'hash'
  resource_id: number
  fields: InstructionEvidenceField[]
}

export interface InstructionAINode {
  id: number
  name: string
  base_url: string
  model: string
  priority: number
  enabled: boolean
  timeout_ms: number
  max_concurrency: number
  has_api_key: boolean
  api_key_status: string
  created_at: string
  updated_at: string
}

export interface SaveInstructionAINode {
  name: string
  base_url: string
  model: string
  api_key: string
  clear_api_key: boolean
  priority: number
  enabled: boolean
  timeout_ms: number
  max_concurrency: number
}

export interface InstructionAINodeTestResult {
  result: string
  confidence: number
  reason: string
  category: string
  latency_ms: number
}

export interface InstructionSensitiveCapability {
  user_id: number
  has_access: boolean
  can_manage: boolean
  grant_id?: number | null
  grant_source?: string
  granted_at?: string
}
