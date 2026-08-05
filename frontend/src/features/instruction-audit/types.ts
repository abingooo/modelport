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
  active_binding_count: number
  pending_email_count: number
  queued_event_count: number
  dropped_event_count: number
  persist_failure_count: number
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

export interface InstructionBinding {
  id: number
  user_id: number
  user_email: string
  username: string
  model: string
  rule_set_id: number
  rule_set_name: string
  enabled: boolean
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
  notification_status: string
  created_at: string
}

export interface InstructionEventPage {
  items: InstructionEvent[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface InstructionUserOption {
  id: number
  email: string
  username: string
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

export interface SaveInstructionBindingRequest {
  user_id: number
  model: string
  rule_set_id: number
  enabled: boolean
}
