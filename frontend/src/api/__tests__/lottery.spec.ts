import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, remove } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), remove: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post, put, delete: remove } }))

import lotteryAPI from '@/api/lottery'

describe('lottery API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('sends the participation idempotency key in the standard header', async () => {
    post.mockResolvedValue({ data: { id: 9, status: 'pending' } })
    await lotteryAPI.participate(4, 'lottery-request-123')
    expect(post).toHaveBeenCalledWith('/lottery/4/participate', undefined, {
      headers: { 'Idempotency-Key': 'lottery-request-123' },
    })
  })

  it('uses native user and admin lottery endpoints', async () => {
    get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20, pages: 0 } })
    post.mockResolvedValue({ data: { id: 1 } })
    put.mockResolvedValue({ data: { id: 1 } })
    remove.mockResolvedValue({})

    await lotteryAPI.list({ page: 1 })
    await lotteryAPI.history({ page: 2 })
    await lotteryAPI.admin.list({ status: 'active' })
    await lotteryAPI.admin.setStatus(1, 'paused')
    await lotteryAPI.admin.draw(1)
    await lotteryAPI.admin.delete(1)

    expect(get).toHaveBeenCalledWith('/lottery', expect.objectContaining({ params: { page: 1 } }))
    expect(get).toHaveBeenCalledWith('/lottery/history', expect.objectContaining({ params: { page: 2 } }))
    expect(get).toHaveBeenCalledWith('/admin/lottery', expect.objectContaining({ params: { status: 'active' } }))
    expect(put).toHaveBeenCalledWith('/admin/lottery/1/status', { status: 'paused' })
    expect(post).toHaveBeenCalledWith('/admin/lottery/1/draw')
    expect(remove).toHaveBeenCalledWith('/admin/lottery/1')
  })
})
