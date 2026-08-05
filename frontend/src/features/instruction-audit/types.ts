export type InstructionHashStatus = 'candidate' | 'active' | 'disabled' | 'expired'
export type InstructionObservedSource = '' | 'instructions' | 'input1'

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
  version: number
  hashes: InstructionHashEntry[]
  created_at: string
  updated_at: string
}

export interface InstructionGroupBinding {
  id: number
  group_id: number
  group_name: string
  platform: string
  group_status: string
  rule_set_id: number
  rule_set_name: string
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
  q?: string
  from?: string
  to?: string
  group_ids?: string
  reasons?: string
  instructions_results?: string
  input1_results?: string
  user_notifications?: string
  ops_notifications?: string
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
  hash_ids: number[]
}

export interface SaveInstructionGroupBindingsRequest {
  group_ids: number[]
  rule_set_id: number
  enabled: boolean
}
