import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelCatalogView from '../ModelCatalogView.vue'
import type { ModelCatalogItem } from '@/api/modelCatalog'

const list = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())

vi.mock('@/api/modelCatalog', () => ({
  default: { list },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { api_base_url: 'https://test.modelport.link/v1' },
    showError,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

function model(overrides: Partial<ModelCatalogItem>): ModelCatalogItem {
  return {
    metadata_id: 0,
    platform: 'openai',
    name: 'gpt-5',
    display_name: 'GPT 5',
    description: 'Reasoning model',
    capabilities: ['text', 'reasoning'],
    context_window: 400000,
    interface_formats: ['openai'],
    scenarios: ['chat'],
    example_overrides: {},
    is_recommended: false,
    is_visible: true,
    sort_order: 0,
    available: true,
    offers: [{
      channel_id: 1,
      channel_name: 'OpenAI pool',
      groups: [{
        id: 1, name: 'Standard', rate_multiplier: 0.8, peak_rate_enabled: false,
        peak_rate_multiplier: 1, subscription_type: 'standard', is_exclusive: false,
      }],
      pricing: {
        billing_mode: 'token', input_price: 0.000001, output_price: 0.000002,
        cache_write_price: null, cache_read_price: null, image_input_price: null,
        image_output_price: null, per_request_price: null, intervals: [],
      },
    }],
    ...overrides,
  }
}

function mountView() {
  return mount(ModelCatalogView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        PlatformIcon: true,
        Icon: true,
        EmptyState: { template: '<div data-testid="empty" />' },
        ModelCatalogDetailDialog: {
          props: ['show', 'item', 'baseUrl'],
          template: '<div data-testid="detail" :data-show="show" :data-model="item?.name" :data-base="baseUrl" />',
        },
      },
    },
  })
}

describe('ModelCatalogView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    list.mockResolvedValue([
      model({}),
      model({ platform: 'gemini', name: 'gemini-3-pro', display_name: 'Gemini 3 Pro', capabilities: ['text', 'vision'] }),
    ])
  })

  it('filters by search, platform, and capability and opens model details', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('GPT 5')
    expect(wrapper.text()).toContain('Gemini 3 Pro')

    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('gemini')
    expect(wrapper.text()).not.toContain('GPT 5')
    expect(wrapper.text()).toContain('Gemini 3 Pro')

    await inputs[0].setValue('')
    const selects = wrapper.findAll('select')
    await selects[0].setValue('openai')
    expect(wrapper.text()).toContain('GPT 5')
    expect(wrapper.text()).not.toContain('Gemini 3 Pro')

    await selects[0].setValue('')
    await selects[1].setValue('vision')
    expect(wrapper.text()).toContain('Gemini 3 Pro')
    expect(wrapper.text()).not.toContain('GPT 5')

    await wrapper.get('button.group').trigger('click')
    expect(wrapper.get('[data-testid="detail"]').attributes('data-model')).toBe('gemini-3-pro')
    expect(wrapper.get('[data-testid="detail"]').attributes('data-base')).toBe('https://test.modelport.link/v1')
  })

  it('renders an error state and retries', async () => {
    list.mockRejectedValueOnce(new Error('network unavailable')).mockResolvedValueOnce([])
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('modelCatalog.loadFailed')

    const retry = wrapper.findAll('button').find((button) => button.text() === 'modelCatalog.retry')
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="empty"]').exists()).toBe(true)
  })
})
