import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.count == null ? key : `${key}:${params.count}`,
      locale: { value: 'zh-CN' }
    })
  }
})

function model(name: string, platform: string): PlazaModel {
  return {
    name,
    platform,
    pricing: null,
    display_pricing: null,
    effective_multiplier: 1,
    official_pricing: {
      input_price: 1e-6,
      output_price: 2e-6,
      cache_write_price: null,
      cache_read_price: null
    }
  }
}

function group(id: number, name: string, platform: string): ModelPlazaGroup {
  return {
    id,
    name,
    description: '',
    platform,
    subscription_type: 'standard',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    applied_peak_multiplier: 1,
    effective_rate_multiplier: 1,
    effective_image_rate_multiplier: 1,
    is_free: false,
    is_exclusive: false,
    models: [model(`${platform}-model`, platform)]
  }
}

const response: ModelPlazaResponse = {
  description: '',
  currency: 'CNY',
  official_pricing_source: 'LiteLLM',
  groups: [group(82, 'OpenAI Group', 'openai'), group(83, 'Grok Group', 'grok')]
}

describe('ModelPlazaContent', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('restores and persists pricing choices per platform', async () => {
    localStorage.setItem('modelport_model_plaza_platform', 'openai')
    localStorage.setItem('modelport_model_plaza_group_openai', '82')
    localStorage.setItem('modelport_model_plaza_sort', 'output')

    const pinia = createPinia()
    setActivePinia(pinia)
    const wrapper = mount(ModelPlazaContent, {
      props: { response, loading: false },
      global: {
        plugins: [pinia],
        stubs: {
          Icon: { template: '<i />' },
          PlatformIcon: { template: '<i />' },
          PlazaModelCard: { template: '<article />' }
        }
      }
    })
    const filterBar = wrapper.findComponent(PlazaFilterBar)

    expect(filterBar.props('platform')).toBe('openai')
    expect(filterBar.props('groupId')).toBe(82)
    expect(filterBar.props('sortMode')).toBe('output')

    filterBar.vm.$emit('update:platform', 'grok')
    await nextTick()
    expect(filterBar.props('groupId')).toBe('all')

    filterBar.vm.$emit('update:groupId', 83)
    filterBar.vm.$emit('update:sortMode', 'name')
    await nextTick()
    expect(localStorage.getItem('modelport_model_plaza_group_grok')).toBe('83')
    expect(localStorage.getItem('modelport_model_plaza_sort')).toBe('name')

    filterBar.vm.$emit('update:platform', 'openai')
    await nextTick()
    expect(filterBar.props('groupId')).toBe(82)
  })

  it('applies the wide public grid only outside the embedded dashboard', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const wrapper = mount(ModelPlazaContent, {
      props: { response, loading: false },
      global: {
        plugins: [pinia],
        stubs: {
          Icon: { template: '<i />' },
          PlatformIcon: { template: '<i />' },
          PlazaModelCard: { template: '<article />' }
        }
      }
    })

    expect(wrapper.classes()).toContain('model-plaza-content-public')
    expect(wrapper.findAll('.plaza-model-grid-public')).toHaveLength(2)
    expect(wrapper.findAll('.plaza-model-grid-public-sparse')).toHaveLength(2)

    await wrapper.setProps({ embedded: true })

    expect(wrapper.classes()).not.toContain('model-plaza-content-public')
    expect(wrapper.find('.plaza-model-grid-public').exists()).toBe(false)
    expect(wrapper.find('.plaza-model-grid-public-sparse').exists()).toBe(false)
  })
})
