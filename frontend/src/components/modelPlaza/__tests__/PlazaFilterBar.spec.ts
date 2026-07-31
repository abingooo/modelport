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
      platforms: ['openai'],
      groups: [{ id: 82, name: 'GPT（测试）', platforms: ['openai'] }],
      billingModes: ['token'],
      platform: 'all',
      groupId: 'all',
      showOfficialPricing,
      billingMode: 'all',
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
    const groupSelect = wrapper.findAllComponents(Select)[1]

    expect(groupSelect.props('options')).toEqual([
      { value: 'all', label: 'modelPlaza.filters.allGroups' },
      { value: '__official__', label: 'modelPlaza.card.officialGroup' },
      { value: 82, label: 'GPT（测试）' }
    ])

    groupSelect.vm.$emit('update:modelValue', '__official__')
    await nextTick()
    expect(wrapper.emitted('update:groupId')).toEqual([['__official__']])
  })

  it('hides the virtual group when no official pricing is available', () => {
    const wrapper = mountFilterBar(false)
    const groupSelect = wrapper.findAllComponents(Select)[1]

    expect(groupSelect.props('options')).not.toContainEqual({
      value: '__official__',
      label: 'modelPlaza.card.officialGroup'
    })
  })
})
