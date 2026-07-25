import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put, remove } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, put, delete: remove },
}))

import {
  deleteModelCatalogMetadata,
  listAdminModelCatalog,
  listModelCatalog,
  saveModelCatalogMetadata,
} from '@/api/modelCatalog'

describe('modelCatalog API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('uses the authenticated user catalog endpoint', async () => {
    const data = [{ name: 'gpt-5' }]
    get.mockResolvedValue({ data })
    await expect(listModelCatalog()).resolves.toEqual(data)
    expect(get).toHaveBeenCalledWith('/model-catalog', { signal: undefined })
  })

  it('uses administrator metadata endpoints', async () => {
    get.mockResolvedValue({ data: [] })
    put.mockResolvedValue({ data: { id: 1 } })
    remove.mockResolvedValue({})
    await listAdminModelCatalog()
    await saveModelCatalogMetadata({
      platform: 'openai', model_name: 'gpt-5', display_name: '', description: '',
      capabilities: [], context_window: 0, interface_formats: ['openai'], scenarios: [],
      example_overrides: {}, is_recommended: false, is_visible: true, sort_order: 0,
    })
    await deleteModelCatalogMetadata(1)
    expect(get).toHaveBeenCalledWith('/admin/model-catalog', { signal: undefined })
    expect(put).toHaveBeenCalledWith('/admin/model-catalog', expect.objectContaining({ model_name: 'gpt-5' }))
    expect(remove).toHaveBeenCalledWith('/admin/model-catalog/1')
  })
})
