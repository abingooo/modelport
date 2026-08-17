import { describe, expect, it } from 'vitest'
import type { PromptAuditConfig } from '../types'
import {
  buildUpdateRequest,
  configToDraft,
  createDefaultEndpoint,
  draftFingerprint,
  emptyEventFilters,
  eventFilterPayload,
  hasExplicitDeleteRange,
  inferPromptAuditProvider,
  SCANNER_CATALOG,
} from '../viewModel'

const config = (): PromptAuditConfig => ({
  enabled: true,
  blocking_enabled: false,
  blocking_latest_turn_only: false,
  store_pass_events: false,
  effective_mode: 'async_audit',
  strategy: 'priority',
  worker_count: 4,
  queue_capacity: 100,
  scanners: SCANNER_CATALOG.map((item) => item.id),
  all_groups: true,
  group_ids: [],
  endpoints: [{
    id: 'guard-1', name: 'Guard One', protocol: 'openai_compatible', provider: 'custom', base_url: 'http://127.0.0.1:8000',
    model: 'safety-model', timeout_ms: 3000, input_limit: 4000, response_mode: 'auto', max_output_tokens: 256,
    effective_response_mode: 'json_object', requires_reconfigure: false, enabled: true,
    has_token: true, token_status: 'configured',
  }],
  config_version: 7,
  updated_at: '2026-07-16T00:00:00Z',
  updated_by: 1,
  change_summary: '{}',
})

describe('Prompt Audit view model', () => {
  it('normalizes legacy null collections from the public config', () => {
    const legacy = { ...config(), group_ids: null, scanners: null, endpoints: null } as unknown as PromptAuditConfig
    expect(configToDraft(legacy)).toMatchObject({ group_ids: [], scanners: [], endpoints: [] })
  })

  it('normalizes missing legacy endpoint contract fields without restoring the retired Guard model', () => {
    const endpoint = { ...config().endpoints[0] } as Record<string, unknown>
    delete endpoint.provider
    delete endpoint.response_mode
    delete endpoint.max_output_tokens
    delete endpoint.effective_response_mode
    delete endpoint.requires_reconfigure
    const legacy = { ...config(), endpoints: [endpoint] } as unknown as PromptAuditConfig

    expect(configToDraft(legacy).endpoints[0]).toMatchObject({
      provider: 'custom',
      model: 'safety-model',
      response_mode: 'auto',
      max_output_tokens: 256,
      effective_response_mode: '',
      requires_reconfigure: false,
    })
  })

  it('uses Qwen defaults for new nodes and infers presets from normalized URLs', () => {
    expect(createDefaultEndpoint(1)).toMatchObject({
      provider: 'qwen',
      base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      model: 'qwen3.7-plus',
      response_mode: 'auto',
      max_output_tokens: 256,
    })
    expect(inferPromptAuditProvider('https://api.deepseek.com/')).toBe('deepseek')
  })

  it('models all nine official input scanners', () => {
    expect(SCANNER_CATALOG).toHaveLength(9)
    expect(SCANNER_CATALOG.map((item) => item.id)).toContain('suicide_and_self_harm')
  })

  it('keeps, replaces, or explicitly clears a saved token without copying plaintext from the server', () => {
    const draft = configToDraft(config())
    expect(draft.endpoints[0].token).toBe('')
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({ token: undefined, clear_token: false })

    draft.endpoints[0].token = 'temporary-canary-token'
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({ token: 'temporary-canary-token', clear_token: false })

    draft.endpoints[0].token = ''
    draft.endpoints[0].clear_token = true
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({ token: undefined, clear_token: true })
  })

  it('includes the optional narrow blocking scope in the update payload', () => {
    const draft = configToDraft(config())
    draft.blocking_latest_turn_only = true
    expect(buildUpdateRequest(draft)).toMatchObject({ blocking_latest_turn_only: true })
  })

  it('round-trips hidden legacy group fields and the generic model contract exactly', () => {
    const draft = configToDraft({ ...config(), all_groups: true, group_ids: [9, 3] })
    draft.endpoints[0].response_mode = 'text_json'
    draft.endpoints[0].max_output_tokens = 512

    expect(buildUpdateRequest(draft)).toMatchObject({
      all_groups: true,
      group_ids: [3, 9],
      endpoints: [expect.objectContaining({
        model: 'safety-model',
        response_mode: 'text_json',
        max_output_tokens: 512,
      })],
    })
  })

  it('tracks dirty state from the full normalized save payload', () => {
    const original = configToDraft(config())
    const changed = configToDraft(config())
    expect(draftFingerprint(changed)).toBe(draftFingerprint(original))
    changed.queue_capacity += 1
    expect(draftFingerprint(changed)).not.toBe(draftFingerprint(original))
  })

  it('requires a valid explicit range and sends canonical ISO timestamps for filter deletion', () => {
    const filters = emptyEventFilters()
    expect(hasExplicitDeleteRange(filters)).toBe(false)
    filters.start_at = '2026-07-15T10:00'
    filters.end_at = '2026-07-16T10:00'
    filters.group_id = '9'
    expect(hasExplicitDeleteRange(filters)).toBe(true)
    expect(eventFilterPayload(filters)).toMatchObject({
      group_id: 9,
      start_at: new Date(filters.start_at).toISOString(),
      end_at: new Date(filters.end_at).toISOString(),
    })
  })
})
