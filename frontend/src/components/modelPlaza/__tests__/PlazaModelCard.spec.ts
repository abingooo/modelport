import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { UserSupportedModelPricing } from '@/api/channels'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import Select from '@/components/common/Select.vue'
import PlazaModelCard from '../PlazaModelCard.vue'
import type { PlazaModelCardData } from '../modelPlazaPresentation'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.count == null ? key : `${key}:${params.count}`
    })
  }
})

function pricing(overrides: Partial<UserSupportedModelPricing> = {}): UserSupportedModelPricing {
  return {
    billing_mode: 'token',
    input_price: 1.25e-8,
    output_price: 2.5e-8,
    cache_write_price: 0,
    cache_read_price: 2e-9,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    ...overrides
  }
}

function model(displayPricing: UserSupportedModelPricing): PlazaModel {
  return {
    name: 'grok-4.3',
    platform: 'grok',
    pricing: pricing({
      input_price: 2.5e-8,
      output_price: 5e-8,
      cache_read_price: 4e-9
    }),
    display_pricing: displayPricing,
    effective_multiplier: 0.5,
    official_pricing: {
      input_price: 2.5e-8,
      output_price: 5e-8,
      cache_write_price: null,
      cache_read_price: 4e-9
    }
  }
}

function group(id: number, name: string, isFree = false): ModelPlazaGroup {
  return {
    id,
    name,
    description: '',
    platform: 'grok',
    subscription_type: 'standard',
    rate_multiplier: isFree ? 0 : 0.5,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    applied_peak_multiplier: 1,
    effective_rate_multiplier: isFree ? 0 : 0.5,
    effective_image_rate_multiplier: isFree ? 0 : 0.5,
    is_free: isFree,
    is_exclusive: false,
    models: []
  }
}

function mountCard(card: PlazaModelCardData) {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(PlazaModelCard, {
    props: { card },
    global: {
      plugins: [pinia],
      stubs: {
        Icon: { template: '<i />' },
        PlatformIcon: { template: '<i />' }
      }
    }
  })
}

describe('PlazaModelCard', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('renders the four authoritative display prices without multiplying again', () => {
    const paidGroup = group(1, 'grok(测试)')
    const paidModel = model(pricing())
    const wrapper = mountCard({
      key: 'grok:grok-4.3',
      name: 'grok-4.3',
      platform: 'grok',
      offers: [{ group: paidGroup, model: paidModel }]
    })

    const text = wrapper.text()
    expect(text).toContain('$0.0125')
    expect(text).toContain('$0.025')
    expect(text).toContain('modelPlaza.badges.free')
    expect(text).toContain('$0.002')
    expect(wrapper.findAll('.price-grid > div')[2].text()).not.toContain('$0.00')
    expect(text).not.toContain('$0.00625')
  })

  it('switches group-specific pricing and displays free instead of 1x', async () => {
    const paidGroup = group(1, 'Paid')
    const freeGroup = group(2, 'Free', true)
    const wrapper = mountCard({
      key: 'grok:grok-4.3',
      name: 'grok-4.3',
      platform: 'grok',
      offers: [
        { group: paidGroup, model: model(pricing()) },
        { group: freeGroup, model: model(pricing({ input_price: 0, output_price: 0, cache_read_price: 0 })) }
      ]
    })

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 2)
    await nextTick()

    expect(wrapper.text()).toContain('modelPlaza.badges.free')
    expect(wrapper.text()).not.toContain('0x')
  })

  it('renders image specifications as per-image price rows', () => {
    const imagePricing = pricing({
      billing_mode: 'image',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      intervals: [
        { min_tokens: 0, max_tokens: null, tier_label: '1K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.04 },
        { min_tokens: 0, max_tokens: null, tier_label: '4K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.12 }
      ]
    })
    const imageModel = model(imagePricing)
    imageModel.pricing = imagePricing
    const wrapper = mountCard({
      key: 'grok:image',
      name: 'grok-image',
      platform: 'grok',
      offers: [{ group: group(1, 'Image'), model: imageModel }]
    })

    expect(wrapper.text()).toContain('1K')
    expect(wrapper.text()).toContain('$0.04')
    expect(wrapper.text()).toContain('4K')
    expect(wrapper.text()).toContain('$0.12')
    expect(wrapper.text()).toContain('modelPlaza.table.perUnitImage')
  })
})
