import { beforeEach, describe, expect, it, vi } from 'vitest'

const client = vi.hoisted(() => ({
  get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: client }))

import instructionAuditV2API from '../v2Api'

describe('instruction audit V2 API', () => {
  beforeEach(() => {
    Object.values(client).forEach(mock => mock.mockReset())
    client.get.mockResolvedValue({ data: {} })
    client.put.mockResolvedValue({ data: {} })
    client.post.mockResolvedValue({ data: {} })
    client.delete.mockResolvedValue({ data: {} })
  })

  it('keeps statistics separate from paginated events', async () => {
    const filters = { group_ids: '1,2', client_keys: 'codex_cli', outcomes: 'blocked', model: 'gpt-5' }
    await instructionAuditV2API.getStatistics(filters)
    await instructionAuditV2API.listEvents({ ...filters, page: 2, page_size: 20 })

    expect(client.get).toHaveBeenNthCalledWith(1, '/admin/instruction-audit/statistics', { params: filters })
    expect(client.get).toHaveBeenNthCalledWith(2, '/admin/instruction-audit/events', { params: { ...filters, page: 2, page_size: 20 } })
  })

  it('exposes evidence, copy audit, quick trust, and deletion endpoints', async () => {
    client.post.mockResolvedValueOnce({ data: { deleted: 2 } })
    client.post.mockResolvedValueOnce({ data: { hashes: [] } })

    expect(await instructionAuditV2API.deleteEvents([3, 4])).toBe(2)
    await instructionAuditV2API.revealEventEvidence(3)
    await instructionAuditV2API.recordEventEvidenceCopy(3, 'instructions')
    await instructionAuditV2API.trustEvent(3, ['instructions', 'input1'], 'trusted', 'reviewed')
    await instructionAuditV2API.deleteEvent(3)

    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/events/3/evidence')
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/events/3/evidence-access', { field_name: 'instructions' })
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/events/3/trust', { fields: ['instructions', 'input1'], name: 'trusted', note: 'reviewed' })
    expect(client.delete).toHaveBeenCalledWith('/admin/instruction-audit/events/3')
  })

  it('uses create and update contracts for scopes, clients, hashes, and AI nodes', async () => {
    await instructionAuditV2API.saveScope(null, { group_id: 7, client_profile_id: null, enabled: true })
    await instructionAuditV2API.saveScope(9, { group_id: 7, client_profile_id: 2, enabled: false })
    await instructionAuditV2API.saveClientProfile(null, { profile_key: 'custom', name: 'Custom', description: '', priority: 10, enabled: true, matchers: [] })
    await instructionAuditV2API.updateHash(5, { status: 'active', scope_ids: [9], set_scopes: true })
    await instructionAuditV2API.testAINode(6)

    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/scopes', { group_id: 7, client_profile_id: null, enabled: true })
    expect(client.put).toHaveBeenCalledWith('/admin/instruction-audit/scopes/9', { group_id: 7, client_profile_id: 2, enabled: false })
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/client-profiles', expect.objectContaining({ profile_key: 'custom' }))
    expect(client.put).toHaveBeenCalledWith('/admin/instruction-audit/hashes/5', { status: 'active', scope_ids: [9], set_scopes: true })
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/ai-nodes/6/test', {})
  })

  it('manages the global user allowlist without model bindings', async () => {
    await instructionAuditV2API.searchUsers('user@example.com')
    await instructionAuditV2API.saveUserAllowlist(17, 'approved operator')
    await instructionAuditV2API.deleteUserAllowlist(8)

    expect(client.get).toHaveBeenCalledWith('/admin/instruction-audit/users', { params: { q: 'user@example.com' } })
    expect(client.post).toHaveBeenCalledWith('/admin/instruction-audit/user-allowlist', { user_id: 17, note: 'approved operator', enabled: true })
    expect(client.delete).toHaveBeenCalledWith('/admin/instruction-audit/user-allowlist/8')
  })
})
