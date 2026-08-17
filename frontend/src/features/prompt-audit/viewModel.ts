import type {
  PromptAuditConfig,
  PromptAuditDraft,
  PromptAuditEndpointDraft,
  PromptAuditProvider,
  PromptAuditResponseMode,
  PromptAuditUpdateRequest,
  PromptEventFilters,
} from './types'
import { OPENAI_COMPATIBLE_PROVIDER_PRESETS } from '@/utils/providerPresets'

export const DEFAULT_MAX_OUTPUT_TOKENS = 256
export const PROMPT_AUDIT_RESPONSE_MODES: readonly PromptAuditResponseMode[] = ['auto', 'json_schema', 'json_object', 'text_json']
export const PROMPT_AUDIT_PROVIDERS: readonly PromptAuditProvider[] = ['qwen', 'deepseek', 'doubao', 'custom']

export const PROMPT_AUDIT_PROVIDER_PRESETS = OPENAI_COMPATIBLE_PROVIDER_PRESETS.filter(
  (preset) => preset.id === 'qwen' || preset.id === 'deepseek' || preset.id === 'doubao',
)

export function inferPromptAuditProvider(baseURL?: string | null): PromptAuditProvider {
  const normalized = normalizeComparableURL(baseURL)
  const preset = PROMPT_AUDIT_PROVIDER_PRESETS.find((item) => normalizeComparableURL(item.defaultBaseUrl) === normalized)
  return (preset?.id as PromptAuditProvider | undefined) ?? 'custom'
}

export function normalizePromptAuditResponseMode(value?: string | null): PromptAuditResponseMode {
  return PROMPT_AUDIT_RESPONSE_MODES.includes(value as PromptAuditResponseMode)
    ? value as PromptAuditResponseMode
    : 'auto'
}

function normalizeComparableURL(value?: string | null): string {
  return (value ?? '').trim().replace(/\/+$/, '').toLowerCase()
}

export const SCANNER_CATALOG = [
  { id: 'violent', label: 'Violent' },
  { id: 'non_violent_illegal_acts', label: 'Non-violent Illegal Acts' },
  { id: 'sexual_content_or_sexual_acts', label: 'Sexual Content or Sexual Acts' },
  { id: 'pii', label: 'PII' },
  { id: 'suicide_and_self_harm', label: 'Suicide & Self-Harm' },
  { id: 'unethical_acts', label: 'Unethical Acts' },
  { id: 'politically_sensitive_topics', label: 'Politically Sensitive Topics' },
  { id: 'copyright_violation', label: 'Copyright Violation' },
  { id: 'jailbreak', label: 'Jailbreak' },
] as const

// Vue props/refs are proxies and cannot be passed to structuredClone in every
// browser. Prompt Audit state is JSON-only, so this produces a detached draft
// without retaining reactive proxies or browser storage references.
export function cloneData<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function configToDraft(config: PromptAuditConfig): PromptAuditDraft {
  return {
    ...cloneData(config),
    group_ids: [...(config.group_ids ?? [])],
    scanners: [...(config.scanners ?? [])],
    endpoints: (config.endpoints ?? []).map((endpoint) => ({
      ...endpoint,
      provider: endpoint.provider ?? inferPromptAuditProvider(endpoint.base_url),
      response_mode: normalizePromptAuditResponseMode(endpoint.response_mode),
      max_output_tokens: Number(endpoint.max_output_tokens) || DEFAULT_MAX_OUTPUT_TOKENS,
      effective_response_mode: endpoint.effective_response_mode
        ? normalizePromptAuditResponseMode(endpoint.effective_response_mode)
        : '',
      requires_reconfigure: endpoint.requires_reconfigure === true,
      enabled: endpoint.requires_reconfigure === true ? false : endpoint.enabled,
      token: '',
      clear_token: false,
    })),
  }
}

export function createDefaultEndpoint(index = 1): PromptAuditEndpointDraft {
  const preset = PROMPT_AUDIT_PROVIDER_PRESETS.find((item) => item.id === 'qwen')!
  return {
    id: `guard-${Date.now()}-${index}`,
    name: `Guard ${index}`,
    protocol: 'openai_compatible',
    provider: 'qwen',
    base_url: preset.defaultBaseUrl,
    model: preset.defaultModel,
    timeout_ms: 3000,
    input_limit: 4000,
    response_mode: 'auto',
    max_output_tokens: DEFAULT_MAX_OUTPUT_TOKENS,
    effective_response_mode: '',
    requires_reconfigure: false,
    enabled: true,
    has_token: false,
    token_status: 'missing',
    token: '',
    clear_token: false,
  }
}

export function buildUpdateRequest(draft: PromptAuditDraft): PromptAuditUpdateRequest {
  return {
    expected_config_version: draft.config_version,
    enabled: draft.enabled,
    blocking_enabled: draft.enabled && draft.blocking_enabled,
    blocking_latest_turn_only: draft.blocking_latest_turn_only,
    store_pass_events: draft.store_pass_events,
    strategy: 'priority',
    worker_count: Number(draft.worker_count),
    queue_capacity: Number(draft.queue_capacity),
    scanners: [...draft.scanners],
    all_groups: draft.all_groups,
    group_ids: [...draft.group_ids].sort((a, b) => a - b),
    endpoints: draft.endpoints.map((endpoint) => ({
      id: endpoint.id.trim(),
      name: endpoint.name.trim(),
      protocol: 'openai_compatible',
      base_url: endpoint.base_url.trim(),
      model: endpoint.model.trim(),
      token: endpoint.token.trim() || undefined,
      clear_token: endpoint.clear_token,
      timeout_ms: Number(endpoint.timeout_ms),
      input_limit: Number(endpoint.input_limit),
      response_mode: normalizePromptAuditResponseMode(endpoint.response_mode),
      max_output_tokens: Number(endpoint.max_output_tokens) || DEFAULT_MAX_OUTPUT_TOKENS,
      enabled: endpoint.enabled,
    })),
  }
}

export function draftFingerprint(draft: PromptAuditDraft | null): string {
  if (!draft) return ''
  return JSON.stringify(buildUpdateRequest(draft))
}

export function emptyEventFilters(): PromptEventFilters {
  return {
    decision: '',
    risk_level: '',
    endpoint: '',
    group_id: '',
    user_id: '',
    api_key_id: '',
    request_id: '',
    prompt_hash: '',
    keyword: '',
    start_at: '',
    end_at: '',
  }
}

function toISO(value: string): string | undefined {
  if (!value.trim()) return undefined
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString()
}

export function eventQueryParams(filters: PromptEventFilters): Record<string, string | number> {
  const result: Record<string, string | number> = {}
  for (const key of ['decision', 'risk_level', 'endpoint', 'request_id', 'prompt_hash', 'keyword'] as const) {
    const value = filters[key].trim()
    if (value) result[key] = value
  }
  for (const key of ['group_id', 'user_id', 'api_key_id'] as const) {
    const value = Number(filters[key])
    if (Number.isInteger(value) && value > 0) result[key] = value
  }
  const start = toISO(filters.start_at)
  const end = toISO(filters.end_at)
  if (start) result.start_at = start
  if (end) result.end_at = end
  return result
}

export function eventFilterPayload(filters: PromptEventFilters): Record<string, unknown> {
  return eventQueryParams(filters)
}

export function hasExplicitDeleteRange(filters: PromptEventFilters): boolean {
  const start = toISO(filters.start_at)
  const end = toISO(filters.end_at)
  return Boolean(start && end && new Date(start).getTime() < new Date(end).getTime())
}

export type DeleteRangePreset = '1d' | '7d' | '30d' | '90d' | 'all' | 'custom'

export const DELETE_RANGE_PRESETS: ReadonlyArray<{ id: DeleteRangePreset; days: number | null }> = [
  { id: '1d', days: 1 },
  { id: '7d', days: 7 },
  { id: '30d', days: 30 },
  { id: '90d', days: 90 },
  { id: 'all', days: null },
  { id: 'custom', days: null },
]

const DAY_MS = 24 * 60 * 60 * 1000

// Presets delete events older than the chosen cutoff: the range always starts
// at the epoch and ends at (now - days) so the backend's explicit-range
// requirement is satisfied without asking the user for a begin date.
export function resolveDeleteRangeFilters(
  filters: PromptEventFilters,
  preset: DeleteRangePreset,
  now: number = Date.now(),
): PromptEventFilters {
  const resolved = cloneData(filters)
  if (preset === 'custom') return resolved
  const days = DELETE_RANGE_PRESETS.find((item) => item.id === preset)?.days ?? null
  resolved.start_at = new Date(0).toISOString()
  resolved.end_at = new Date(days === null ? now : now - days * DAY_MS).toISOString()
  return resolved
}
