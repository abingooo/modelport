import { beforeEach, describe, expect, it, vi } from 'vitest'

const client = vi.hoisted(() => ({
  get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: client }))

import instructionAuditAPI from '../api'
import { instructionStatisticsFilters } from '../filters'

describe('instruction audit v0.1.170.13 API', () => {
  beforeEach(() => {
    Object.values(client).forEach((mock) => mock.mockReset())
    client.get.mockResolvedValue({ data: {} })
    client.put.mockResolvedValue({ data: {} })
    client.post.mockResolvedValue({ data: {} })
    client.delete.mockResolvedValue({ data: {} })
  })

  it('separates statistics from paginated events and forwards all scope filters', async () => {
    const filters = {
      from: '2026-08-01T00:00:00.000Z', to: '2026-08-02T00:00:00.000Z',
      group_ids: '1,2', user_id: 8, model: 'gpt-5', client_types: 'codex_cli',
      final_outcomes: 'blocked,ai_pass', reasons: 'hash_mismatch',
    }
    await instructionAuditAPI.getStatistics(filters)
    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/statistics', { params: filters })

    client.get.mockResolvedValueOnce({ data: { items: [], total: 0, page: 1, page_size: 20, pages: 0 } })
    await instructionAuditAPI.listEvents({ ...filters, page: 1, page_size: 20 })
    expect(client.get).toHaveBeenLastCalledWith('/admin/instruction-audit/events', { params: { ...filters, page: 1, page_size: 20 } })
  })

  it('removes event-only filters before requesting aggregate statistics', () => {
    const result = instructionStatisticsFilters({
      event_id: 91,
      q: 'request-or-email',
      user_id: 8,
      model: 'gpt-5.6-sol',
      from: '2026-08-01T00:00:00.000Z',
      to: '2026-08-02T00:00:00.000Z',
      group_ids: '1,2',
      reasons: 'hash_mismatch',
      final_outcomes: 'blocked,ai_pass',
      instructions_results: 'mismatch',
      input1_results: 'missing',
      user_notifications: 'sent',
      ops_notifications: 'failed',
      client_types: 'codex_cli',
    })

    expect(result).toEqual({
      from: '2026-08-01T00:00:00.000Z',
      to: '2026-08-02T00:00:00.000Z',
      group_ids: '1,2',
      user_id: 8,
      model: 'gpt-5.6-sol',
      client_types: 'codex_cli',
      final_outcomes: 'blocked,ai_pass',
      final_reasons: 'hash_mismatch',
    })
    expect(result).not.toHaveProperty('event_id')
    expect(result).not.toHaveProperty('q')
    expect(result).not.toHaveProperty('instructions_results')
    expect(result).not.toHaveProperty('user_notifications')
  })

  it('uses dedicated runtime and reason-policy contracts with optimistic versions', async () => {
    const runtime = {
      max_body_bytes: 64 * 1024 * 1024, parse_timeout_ms: 500, max_inflight_body_bytes: 256 * 1024 * 1024,
      pass_event_retention_days: 7, aggregate_retention_days: 365, raw_content_retention_days: 30,
      ai_enabled: false, ai_base_url: '', ai_model: '', ai_token: '', clear_ai_token: false,
      ai_timeout_ms: 3000, ai_max_concurrency: 4, ai_min_confidence: 0.9, ai_per_user_rpm: 5,
      ai_per_user_daily_limit: 20, ai_global_daily_limit: 100, ai_prompt_version: 'v1',
      translation_enabled: false, external_translation_enabled: false, translation_base_url: '', translation_model: '',
      translation_token: '', clear_translation_token: false, translation_timeout_ms: 5000,
      translation_max_concurrency: 2, translation_chunk_bytes: 8192, translation_max_bytes: 1048576,
      translation_result_ttl_seconds: 900, expected_config_version: 11,
    }
    await instructionAuditAPI.updateRuntimeConfig(runtime)
    expect(client.put).toHaveBeenCalledWith('/admin/instruction-audit/config', runtime)

    const policy = { action: 'allow_and_record' as const, ai_review_enabled: true, alert_enabled: true, allow_until: null, expected_config_version: 12, confirmed: true }
    await instructionAuditAPI.updateReasonPolicy('hash_mismatch', policy)
    expect(client.put).toHaveBeenLastCalledWith('/admin/instruction-audit/reason-policies/hash_mismatch', policy)
  })

  it('exposes provenance, sensitive raw access, AI reviews, and translation jobs', async () => {
    await instructionAuditAPI.getHash(4)
    await instructionAuditAPI.revealHashRaw(4)
    await instructionAuditAPI.recordHashRawCopy(4)
    await instructionAuditAPI.changeHashStatus(4, 'revoked')
    await instructionAuditAPI.changeHashScope(4, 12, 'promote')
    await instructionAuditAPI.listEventAIReviews(9)
    await instructionAuditAPI.createTranslation({ resource_type: 'event', resource_id: 9, field_name: 'instructions', target_language: 'zh-CN', provider: 'internal' })
    await instructionAuditAPI.getTranslation(20)

    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/hashes/4')
    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/hashes/4/raw')
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/hashes/4/raw-access', {})
    expect(client.put).toHaveBeenCalledWith('/admin/instruction-audit/hashes/4/status', { status: 'revoked' })
    expect(client.put).toHaveBeenCalledWith('/admin/instruction-audit/hashes/4/scopes/12', { action: 'promote' })
    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/events/9/ai-reviews')
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/translations', expect.objectContaining({ resource_id: 9 }))
    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/translations/20')
  })

  it('uses dedicated sensitive capability and grant endpoints', async () => {
    await instructionAuditAPI.getSensitiveAccessCapability()
    await instructionAuditAPI.listSensitiveAccessGrants()
    await instructionAuditAPI.grantSensitiveAccess(18, 'security rotation')
    await instructionAuditAPI.revokeSensitiveAccess(18, 'role changed')

    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/sensitive-access/me')
    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/sensitive-access/grants')
    expect(client.put).toHaveBeenCalledWith('/admin/instruction-audit/sensitive-access/grants/18', { reason: 'security rotation' })
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/sensitive-access/grants/18/revoke', { reason: 'role changed' })
  })
})
