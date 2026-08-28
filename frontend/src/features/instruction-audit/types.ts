export type InstructionHashStatus = 'candidate' | 'active' | 'disabled' | 'expired' | 'revoked'
export type InstructionObservedSource = '' | 'instructions' | 'input1'
export type InstructionFinalOutcome = 'blocked' | 'policy_allow' | 'ai_pass' | 'hash_pass' | 'exception_pass'
export type InstructionPolicyAction = 'block' | 'allow_and_record'
export type InstructionEventPolicyAction = InstructionPolicyAction | 'hash_match' | 'exception' | 'ai_review'
export type InstructionTranslationStatus = 'pending' | 'retry' | 'processing' | 'succeeded' | 'partial' | 'failed' | 'expired'
export type InstructionDetectedClientType =
  | 'codex_vscode'
  | 'codex_cli'
  | 'codex_desktop'
  | 'opencode'
  | 'modelport_internal'
  | 'other'
  | 'unknown'
export type InstructionClientType = 'all' | InstructionDetectedClientType

export interface InstructionSensitiveCapability {
  user_id: number
  has_access: boolean
  can_manage: boolean
  grant_id?: number | null
  grant_source?: string
  granted_at?: string
}

export interface InstructionSensitiveGrant {
  id: number
  user_id: number
  email: string
  username: string
  user_status: string
  totp_enabled: boolean
  effective: boolean
  granted_by?: number | null
  grant_source: string
  grant_reason: string
  granted_at: string
}

export interface InstructionOverview {
  enabled: boolean
  config_version: number
  loaded_at?: string
  load_error: string
  hash_count: number
  active_hash_count: number
  rule_set_count: number
  audited_group_count: number
  effective_group_count: number
  pending_email_count: number
  queued_event_count: number
  dropped_event_count: number
  persist_failure_count: number
  evidence_encryption_available: boolean
  evidence_retention_days: number
  max_body_bytes: number
  http_gateway_max_body_bytes: number
  websocket_gateway_max_body_bytes: number
  effective_http_max_body_bytes: number
  effective_websocket_max_body_bytes: number
  parse_timeout_ms: number
  max_inflight_body_bytes: number
  ai_enabled: boolean
  translation_enabled: boolean
  translation_pending_count: number
  translation_processing_count: number
  translation_failed_count: number
  translation_active_workers: number
  translation_processed_total: number
  translation_worker_fail_total: number
  persisted_outcome_count: number
  aggregated_outcome_count: number
  expired_aggregate_event_count: number
  statistics_loss_count: number
  audit_latency_sample_count: number
  audit_latency_p95_ms: number
  audit_latency_p99_ms: number
  ai_latency_sample_count: number
  ai_latency_p95_ms: number
  ai_latency_p99_ms: number
}

export interface InstructionRuntimeConfig {
  config_version: number
  max_body_bytes: number
  parse_timeout_ms: number
  max_inflight_body_bytes: number
  pass_event_retention_days: number
  aggregate_retention_days: number
  raw_content_retention_days: number
  ai_enabled: boolean
  ai_base_url: string
  ai_model: string
  ai_has_token: boolean
  ai_timeout_ms: number
  ai_max_concurrency: number
  ai_min_confidence: number
  ai_per_user_rpm: number
  ai_per_user_daily_limit: number
  ai_global_daily_limit: number
  ai_prompt_version: string
  translation_enabled: boolean
  external_translation_enabled: boolean
  translation_base_url: string
  translation_model: string
  translation_has_token: boolean
  translation_timeout_ms: number
  translation_max_concurrency: number
  translation_chunk_bytes: number
  translation_max_bytes: number
  translation_result_ttl_seconds: number
  updated_by?: number | null
  updated_at: string
}

export interface UpdateInstructionRuntimeConfigRequest {
  max_body_bytes: number
  parse_timeout_ms: number
  max_inflight_body_bytes: number
  pass_event_retention_days: number
  aggregate_retention_days: number
  raw_content_retention_days: number
  ai_enabled: boolean
  ai_base_url: string
  ai_model: string
  ai_token: string
  clear_ai_token: boolean
  ai_timeout_ms: number
  ai_max_concurrency: number
  ai_min_confidence: number
  ai_per_user_rpm: number
  ai_per_user_daily_limit: number
  ai_global_daily_limit: number
  ai_prompt_version: string
  translation_enabled: boolean
  external_translation_enabled: boolean
  translation_base_url: string
  translation_model: string
  translation_token: string
  clear_translation_token: boolean
  translation_timeout_ms: number
  translation_max_concurrency: number
  translation_chunk_bytes: number
  translation_max_bytes: number
  translation_result_ttl_seconds: number
  expected_config_version: number
}

export interface InstructionReasonPolicy {
  reason: string
  action: InstructionPolicyAction
  ai_review_enabled: boolean
  alert_enabled: boolean
  allow_until?: string | null
  config_version: number
  updated_by?: number | null
  updated_at: string
}

export interface UpdateInstructionReasonPolicyRequest {
  action: InstructionPolicyAction
  ai_review_enabled: boolean
  alert_enabled: boolean
  allow_until?: string | null
  expected_config_version: number
  confirmed: boolean
}

export interface InstructionStatistics {
  blocked: number
  policy_allow: number
  ai_pass: number
  hash_pass: number
  exception_pass: number
  total: number
  block_rate: number
}

export interface InstructionHashSource {
  id: number
  source_type: 'manual' | 'ai_review' | 'import' | string
  field_name: string
  event_id?: number | null
  ai_review_id?: number | null
  reviewer_model: string
  prompt_version: string
  confidence?: number | null
  review_reason: string
  created_by?: number | null
  created_at: string
}

export interface InstructionHashScope {
  rule_set_id: number
  rule_set_name: string
  rule_set_enabled: boolean
  system_managed: boolean
  source_type: 'manual' | 'ai_review' | string
  status: 'active' | 'disabled' | 'revoked' | string
  valid_until?: string | null
  binding_id?: number | null
  group_id?: number | null
  group_name: string
  client_types: string[]
  binding_enabled: boolean
  updated_by?: number | null
  created_at: string
  updated_at: string
}

export interface InstructionHashEntry {
  id: number
  digest: string
  name: string
  note: string
  observed_source: InstructionObservedSource
  client_name: string
  client_version: string
  status: InstructionHashStatus
  hash_algorithm: string
  normalization_version: string
  field_name: string
  raw_content_status: string
  content_bytes: number
  encryption_key_version?: string
  raw_expires_at?: string | null
  sources?: InstructionHashSource[]
  scopes?: InstructionHashScope[]
  scope_source?: 'manual' | 'ai_review' | string
  scope_status?: 'active' | 'disabled' | 'revoked' | string
  scope_valid_until?: string | null
  valid_from?: string | null
  valid_until?: string | null
  created_by?: number | null
  created_at: string
  updated_at: string
}

export interface InstructionRuleSet {
  id: number
  name: string
  description: string
  enabled: boolean
  allow_empty_fields: boolean
  system_managed: boolean
  system_key?: string
  version: number
  hashes: InstructionHashEntry[]
  allowed_users: InstructionRuleSetUser[]
  created_at: string
  updated_at: string
}

export interface InstructionRuleSetUser {
  id: number
  email: string
  deleted: boolean
}

export interface InstructionGroupBinding {
  id: number
  group_id: number
  group_name: string
  platform: string
  group_status: string
  rule_set_id: number
  rule_set_name: string
  system_managed: boolean
  client_types: InstructionClientType[]
  enabled: boolean
  effective: boolean
  created_at: string
  updated_at: string
}

export interface InstructionFieldResult {
  present: boolean
  sha256: string
  result: string
}

export interface InstructionEvent {
  id: number
  request_id: string
  user_id?: number | null
  user_email: string
  api_key_id?: number | null
  group_id?: number | null
  group_name: string
  client_type: InstructionDetectedClientType
  client_user_agent: string
  model: string
  endpoint: string
  stage: string
  instructions: InstructionFieldResult
  input1: InstructionFieldResult
  decision: string
  reason: string
  initial_reason: string
  final_reason: string
  final_outcome: InstructionFinalOutcome
  policy_action: InstructionEventPolicyAction
  rule_set_ids: number[]
  config_version: number
  body_bytes?: number | null
  latency_ms: number
  audit_latency_ms?: number | null
  ai_latency_ms?: number | null
  ai_review_id?: number | null
  evidence_status: InstructionEvidenceStatus
  evidence_expires_at?: string | null
  user_notification_status: string
  ops_notification_status: string
  created_at: string
}

export type InstructionEvidenceStatus =
  | 'stored'
  | 'not_available'
  | 'encryption_unavailable'
  | 'expired'
  | 'legacy_unavailable'

export interface InstructionEvidenceField {
  source: 'instructions' | 'input1'
  available: boolean
  plaintext?: string
  sha256: string
  plaintext_bytes: number
  recomputed_sha256?: string
  digest_consistent: boolean
}

export interface InstructionEvidenceReview {
  event_id: number
  request_id: string
  status: InstructionEvidenceStatus
  expires_at?: string | null
  fields: InstructionEvidenceField[]
  access_count: number
}

export interface InstructionEventFilters {
  event_id?: number
  q?: string
  user_id?: number
  model?: string
  from?: string
  to?: string
  group_ids?: string
  reasons?: string
  initial_reasons?: string
  final_reasons?: string
  final_outcomes?: string
  instructions_results?: string
  input1_results?: string
  user_notifications?: string
  ops_notifications?: string
  client_types?: string
}

export interface InstructionStatisticsFilters {
  user_id?: number
  model?: string
  from?: string
  to?: string
  group_ids?: string
  client_types?: string
  final_reasons?: string
  final_outcomes?: string
}

export interface InstructionEventDeleteFilter {
  event_id?: number
  q?: string
  user_id?: number
  model?: string
  from: string
  to: string
  group_ids: number[]
  client_types: InstructionDetectedClientType[]
  reasons: string[]
  initial_reasons?: string[]
  final_reasons?: string[]
  outcomes?: InstructionFinalOutcome[]
  instructions_results: string[]
  input1_results: string[]
  user_notifications: string[]
  ops_notifications: string[]
}

export interface InstructionDeletePreview {
  matched_count: number
  filter_summary: InstructionEventDeleteFilter
  snapshot_max_id: number
  filter_hash: string
}

export interface InstructionDeleteResult {
  deleted_events: number
}

export interface AddInstructionEventToRuleSetResult {
  rule_set_id: number
  hash_ids: number[]
  created_hashes: number
  activated_hashes: number
  attached_hashes: number
  config_version: number
}

export interface InstructionEventPage {
  items: InstructionEvent[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface InstructionGroupOption {
  id: number
  name: string
  platform: string
  status: string
}

export interface SaveInstructionHashRequest {
  digest: string
  raw_content?: string
  source_type: 'manual' | 'import'
  name: string
  note: string
  observed_source: InstructionObservedSource
  client_name: string
  client_version: string
  status: InstructionHashStatus
  valid_from?: string | null
  valid_until?: string | null
}

export interface InstructionHashRawReview {
  hash_id: number
  field_name: string
  raw_content_status: string
  raw_content?: string
  content_bytes: number
  sha256: string
  recomputed_sha256?: string
  digest_consistent: boolean
  raw_expires_at?: string | null
}

export interface InstructionAIReview {
  id: number
  event_id?: number | null
  request_id: string
  user_id?: number | null
  group_id?: number | null
  client_type: string
  model: string
  reviewed_source: string
  reviewed_sha256: string
  result: 'pass' | 'reject' | 'uncertain' | 'error' | string
  approved_source?: string
  confidence: number
  reason: string
  reviewer_model: string
  prompt_version: string
  latency_ms: number
  automatic_hash_id?: number | null
  created_at: string
}

export interface InstructionTranslationRequest {
  resource_type: 'event' | 'hash'
  resource_id: number
  field_name: 'instructions' | 'input1'
  target_language: string
  provider: 'internal' | 'external'
}

export interface InstructionTranslationJob {
  id: number
  resource_type: 'event' | 'hash'
  resource_id: number
  field_name: 'instructions' | 'input1'
  target_language: string
  provider: 'internal' | 'external'
  status: InstructionTranslationStatus
  error_code: string
  chunk_count: number
  completed_chunks: number
  attempts: number
  max_attempts: number
  result_bytes: number
  redaction_count: number
  provider_latency_ms: number
  requested_by?: number | null
  processing_started_at?: string | null
  translated_text?: string
  expires_at: string
  created_at: string
  updated_at: string
}

export interface SaveInstructionRuleSetRequest {
  name: string
  description: string
  enabled: boolean
  allow_empty_fields: boolean
  hash_ids: number[]
  allowed_user_ids: number[]
}

export interface SaveInstructionGroupBindingsRequest {
  group_ids: number[]
  rule_set_id: number
  client_types?: InstructionClientType[]
  enabled: boolean
}
