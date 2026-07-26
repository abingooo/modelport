import { beforeEach, describe, expect, it, vi } from 'vitest'

const { put } = vi.hoisted(() => ({ put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { put } }))

import { bulkUpdateGroup } from '@/api/keys'

describe('API key API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('sends a stable idempotency key for a bulk group update', async () => {
    put.mockResolvedValue({ data: { updated_count: 2 } })

    await bulkUpdateGroup([11, 12], 7, 'bulk-group-request-123')

    expect(put).toHaveBeenCalledWith(
      '/keys/bulk/group',
      { key_ids: [11, 12], group_id: 7 },
      { headers: { 'Idempotency-Key': 'bulk-group-request-123' } }
    )
  })
})
