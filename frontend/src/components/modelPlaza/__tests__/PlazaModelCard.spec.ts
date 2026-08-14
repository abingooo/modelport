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

function mountCard(card: PlazaModelCardData, officialPricing = false) {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(PlazaModelCard, {
    props: { card, officialPricing },
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

  it('renders actual group prices and switches to the official virtual group', async () => {
    const paidGroup = group(1, 'grok(测试)')
    const paidModel = model(pricing())
    const wrapper = mountCard({
      key: 'grok:grok-4.3',
      name: 'grok-4.3',
      platform: 'grok',
      offers: [{ group: paidGroup, model: paidModel }]
    })

    const text = wrapper.text()
    expect(text).toContain('￥0.0125')
    expect(text).toContain('￥0.025')
    expect(text).toContain('modelPlaza.badges.free')
    expect(text).toContain('￥0.002')
    expect(wrapper.findAll('.price-grid > div')[2].text()).not.toContain('￥0.00')
    expect(text).not.toContain('￥0.00625')
    expect(text).not.toContain('￥0.05')
    expect(text).not.toContain('$')

    const groupSelect = wrapper.findComponent(Select)
    expect(groupSelect.props('options')).toContainEqual({
      value: '__official__',
      label: 'modelPlaza.card.officialGroup'
    })
    groupSelect.vm.$emit('update:modelValue', '__official__')
    await nextTick()

    const officialText = wrapper.text()
    expect(officialText).toContain('￥0.025')
    expect(officialText).toContain('￥0.05')
    expect(officialText).toContain('￥0.004')
    expect(officialText).toContain('modelPlaza.card.officialReferenceHint')
    expect(officialText).not.toContain('$')
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

  it('follows the globally selected official pricing group', async () => {
    const wrapper = mountCard(
      {
        key: 'grok:grok-4.3',
        name: 'grok-4.3',
        platform: 'grok',
        offers: [{ group: group(1, 'Paid'), model: model(pricing()) }]
      },
      true
    )
    await nextTick()

    const groupSelect = wrapper.findComponent(Select)
    expect(groupSelect.props('modelValue')).toBe('__official__')
    expect(groupSelect.props('disabled')).toBe(true)
    expect(wrapper.text()).toContain('￥0.05')
    expect(wrapper.text()).toContain('modelPlaza.card.officialReferenceHint')

    await wrapper.setProps({ officialPricing: false })
    await nextTick()
    expect(groupSelect.props('modelValue')).toBe(1)
    expect(wrapper.text()).not.toContain('￥0.05')
  })

  it('renders image specifications and switches to channel official prices', async () => {
    const imageOfficialPricing = pricing({
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
    const imageDisplayPricing = pricing({
      ...imageOfficialPricing,
      intervals: imageOfficialPricing.intervals.map((interval) => ({
        ...interval,
        per_request_price: (interval.per_request_price ?? 0) * 0.5
      }))
    })
    const imageModel = model(imageDisplayPricing)
    imageModel.pricing = imageOfficialPricing
    const wrapper = mountCard({
      key: 'grok:image',
      name: 'grok-image',
      platform: 'grok',
      offers: [{ group: group(1, 'Image'), model: imageModel }]
    })

    expect(wrapper.text()).toContain('1K')
    expect(wrapper.text()).toContain('￥0.02')
    expect(wrapper.text()).toContain('4K')
    expect(wrapper.text()).toContain('￥0.06')
    expect(wrapper.text()).not.toContain('$')
    expect(wrapper.text()).toContain('modelPlaza.table.perUnitImage')
    expect(wrapper.findComponent(Select).exists()).toBe(true)

    wrapper.findComponent(Select).vm.$emit('update:modelValue', '__official__')
    await nextTick()
    expect(wrapper.text()).toContain('￥0.04')
    expect(wrapper.text()).toContain('￥0.12')
  })

  it('renders per-request pricing and switches to the channel official price', async () => {
    const requestOfficialPricing = pricing({
      billing_mode: 'per_request',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      per_request_price: 0.05
    })
    const requestModel = model(pricing({ ...requestOfficialPricing, per_request_price: 0.025 }))
    requestModel.pricing = requestOfficialPricing
    const wrapper = mountCard({
      key: 'grok:request',
      name: 'grok-request',
      platform: 'grok',
      offers: [{ group: group(1, 'Request'), model: requestModel }]
    })

    expect(wrapper.text()).toContain('￥0.025')
    expect(wrapper.text()).toContain('modelPlaza.table.perUnitRequest')
    expect(wrapper.text()).not.toContain('$')

    wrapper.findComponent(Select).vm.$emit('update:modelValue', '__official__')
    await nextTick()
    expect(wrapper.text()).toContain('￥0.05')
  })

  it('uses the billing mode of the active group or official price', async () => {
    const tokenDisplayPricing = pricing({ input_price: 1e-6, output_price: 2e-6 })
    const mixedModeModel = model(tokenDisplayPricing)
    mixedModeModel.pricing = pricing({
      billing_mode: 'image',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      intervals: [
        {
          min_tokens: 0,
          max_tokens: null,
          tier_label: '1K',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          per_request_price: 0.1
        }
      ]
    })
    const wrapper = mountCard({
      key: 'grok:mixed-mode',
      name: 'mixed-mode',
      platform: 'grok',
      offers: [{ group: group(1, 'Mixed'), model: mixedModeModel }]
    })

    expect(wrapper.text()).toContain('modelPlaza.billingModes.token')
    expect(wrapper.find('.price-grid').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('1K')

    wrapper.findComponent(Select).vm.$emit('update:modelValue', '__official__')
    await nextTick()

    expect(wrapper.text()).toContain('modelPlaza.billingModes.image')
    expect(wrapper.find('.price-grid').exists()).toBe(false)
    expect(wrapper.text()).toContain('1K')
    expect(wrapper.text()).toContain('￥0.1')
    expect(wrapper.text()).toContain('modelPlaza.table.perUnitImage')
  })

  it('labels video tier prices as per-second prices', () => {
    const videoModel = model(pricing({
      billing_mode: 'video',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      intervals: [
        {
          min_tokens: 0,
          max_tokens: null,
          tier_label: '720p',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          per_request_price: 0.07
        }
      ]
    }))
    const wrapper = mountCard({
      key: 'grok:video',
      name: 'grok-video',
      platform: 'grok',
      offers: [{ group: group(1, 'Video'), model: videoModel }]
    })

    expect(wrapper.text()).toContain('modelPlaza.billingModes.video')
    expect(wrapper.text()).toContain('720p')
    expect(wrapper.text()).toContain('￥0.07')
    expect(wrapper.text()).toContain('modelPlaza.table.perUnitSecond')
    expect(wrapper.text()).not.toContain('modelPlaza.table.perUnitRequest')
  })
})
