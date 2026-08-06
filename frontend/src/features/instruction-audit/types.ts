export type InstructionHashStatus = 'candidate' | 'active' | 'disabled' | 'expired'
export type InstructionObservedSource = '' | 'instructions' | 'input1'
export type InstructionDetectedClientType =
  | 'codex_vscode'
  | 'codex_cli'
  | 'codex_desktop'
  | 'opencode'
  | 'modelport_internal'
  | 'other'
  | 'unknown'
export type InstructionClientType = 'all' | InstructionDetectedClientType

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
  rule_set_ids: number[]
  config_version: number
  latency_ms: number
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
  from?: string
  to?: string
  group_ids?: string
  reasons?: string
  instructions_results?: string
  input1_results?: string
  user_notifications?: string
  ops_notifications?: string
  client_types?: string
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
  name: string
  note: string
  observed_source: InstructionObservedSource
  client_name: string
  client_version: string
  status: InstructionHashStatus
  valid_from?: string | null
  valid_until?: string | null
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
