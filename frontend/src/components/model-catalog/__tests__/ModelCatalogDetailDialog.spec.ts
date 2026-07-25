import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelCatalogDetailDialog from '../ModelCatalogDetailDialog.vue'
import type { ModelCatalogItem } from '@/api/modelCatalog'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

function item(): ModelCatalogItem {
  return {
    metadata_id: 1,
    platform: 'openai',
    name: 'gpt-priced',
    display_name: 'GPT Priced',
    description: 'Pricing fixture',
    capabilities: ['text'],
    context_window: 128000,
    interface_formats: ['openai'],
    scenarios: ['chat'],
    example_overrides: {},
    is_recommended: false,
    is_visible: true,
    sort_order: 0,
    available: true,
    offers: [{
      channel_id: 1,
      channel_name: 'Primary pool',
      groups: [{
        id: 2,
        name: 'Standard',
        rate_multiplier: 0.8,
        peak_rate_enabled: true,
        peak_rate_multiplier: 1.5,
        subscription_type: 'standard',
        is_exclusive: false,
      }],
      pricing: {
        billing_mode: 'token',
        input_price: 0.000001,
        output_price: 0.000002,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [{
          min_tokens: 0,
          max_tokens: 10000,
          tier_label: 'Short context',
          input_price: 0.0000005,
          output_price: 0.000001,
          cache_write_price: null,
          cache_read_price: null,
          per_request_price: null,
        }],
      },
    }],
  }
}

describe('ModelCatalogDetailDialog', () => {
  it('shows group-adjusted flat and interval pricing', () => {
    const wrapper = mount(ModelCatalogDetailDialog, {
      props: { show: true, item: item(), baseUrl: 'https://test.modelport.link' },
      global: {
        stubs: {
          BaseDialog: { template: '<section><slot /><slot name="footer" /></section>' },
          PlatformIcon: true,
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('×0.8')
    expect(wrapper.text()).toContain('modelCatalog.pricing.peakRate ×1.2')
    expect(wrapper.text()).toContain('￥0.8')
    expect(wrapper.text()).toContain('￥1.6')
    expect(wrapper.text()).toContain('Short context')
    expect(wrapper.text()).toContain('￥0.4')
  })
})
