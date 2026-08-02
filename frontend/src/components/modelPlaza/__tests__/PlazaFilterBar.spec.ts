import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import Select from '@/components/common/Select.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

function mountFilterBar(showOfficialPricing = true) {
  return mount(PlazaFilterBar, {
    props: {
      platforms: [{ id: 'openai', count: 4 }],
      groups: [
        {
          id: 82,
          name: 'GPT（测试）',
          platforms: ['openai'],
          subscriptionType: 'standard',
          effectiveMultiplier: 1,
          isFree: false,
          isExclusive: false
        }
      ],
      billingModes: ['token'],
      platform: 'all',
      groupId: 'all',
      showOfficialPricing,
      billingMode: 'all',
      sortMode: 'name',
      search: '',
      resultCount: 4
    },
    global: {
      stubs: {
        Icon: { template: '<i />' },
        PlatformIcon: { template: '<i />' }
      }
    }
  })
}

describe('PlazaFilterBar', () => {
  it('shows official pricing in the global group filter and emits its value', async () => {
    const wrapper = mountFilterBar()
    const groupSelect = wrapper.findAllComponents(Select)[0]

    expect(groupSelect.props('options')).toContainEqual(
      expect.objectContaining({ value: 'all', label: 'modelPlaza.filters.allGroups' })
    )
    expect(groupSelect.props('options')).toContainEqual(
      expect.objectContaining({ value: '__official__', label: 'modelPlaza.card.officialGroup' })
    )
    expect(groupSelect.props('options')).toContainEqual(
      expect.objectContaining({ value: 82, label: 'GPT（测试）', badge: '1x' })
    )

    groupSelect.vm.$emit('update:modelValue', '__official__')
    await nextTick()
    expect(wrapper.emitted('update:groupId')).toEqual([['__official__']])
  })

  it('hides the virtual group when no official pricing is available', () => {
    const wrapper = mountFilterBar(false)
    const groupSelect = wrapper.findAllComponents(Select)[0]

    expect(groupSelect.props('options')).not.toContainEqual(
      expect.objectContaining({ value: '__official__' })
    )
  })

  it('uses compact labels for the default mobile selections', () => {
    const wrapper = mountFilterBar()
    const selects = wrapper.findAllComponents(Select)

    expect(selects[0].text()).toContain('modelPlaza.filters.allShort')
    expect(selects[1].text()).toContain('modelPlaza.filters.allShort')
  })

  it('renders platform counts and emits platform and sort changes', async () => {
    const wrapper = mountFilterBar()

    expect(wrapper.get('[data-testid="platform-tab-all"]').text()).toContain('4')
    expect(wrapper.get('[data-testid="platform-tab-openai"]').text()).toContain('4')

    await wrapper.get('[data-testid="platform-tab-openai"]').trigger('click')
    await wrapper.get('[data-testid="plaza-sort-toggle"]').trigger('click')

    expect(wrapper.emitted('update:platform')).toEqual([['openai']])
    expect(wrapper.emitted('update:sortMode')).toEqual([['output']])
  })
})
