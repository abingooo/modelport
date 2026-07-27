import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it } from 'vitest'

import type { GroupPlatform } from '@/types'
import GroupBadge from '../GroupBadge.vue'

const providerCases = [
  ['qwen', 'violet'],
  ['glm', 'cyan'],
  ['kimi', 'teal'],
  ['doubao', 'sky'],
  ['siliconflow', 'fuchsia'],
  ['openrouter', 'slate'],
  ['minimax', 'rose'],
  ['mimo', 'orange']
] as const satisfies ReadonlyArray<readonly [GroupPlatform, string]>

describe('GroupBadge platform colors', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it.each(providerCases)('uses the %s brand palette instead of the fallback', (platform, color) => {
    const wrapper = mount(GroupBadge, {
      props: { name: platform, platform, showRate: false },
      global: {
        plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: {} } })]
      }
    })

    expect(wrapper.classes().some((className) => className.includes(color))).toBe(true)
  })

  it('shows the free label instead of a rate', () => {
    const wrapper = mount(GroupBadge, {
      props: { name: 'Test group', platform: 'openai', rateMultiplier: 2, isFree: true },
      global: {
        plugins: [createI18n({
          legacy: false,
          locale: 'en',
          messages: { en: { admin: { groups: { freeBilling: { badge: () => 'Free' } } } } }
        })]
      }
    })

    expect(wrapper.text()).toContain('Free')
    expect(wrapper.text()).not.toContain('2x')
  })
})
