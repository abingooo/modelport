import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'

import type { GroupPlatform } from '@/types'
import GroupOptionItem from '../GroupOptionItem.vue'

const providerCases = [
  ['anthropic', 'amber'],
  ['openai', 'emerald'],
  ['antigravity', 'purple'],
  ['gemini', 'blue'],
  ['grok', 'zinc'],
  ['deepseek', 'indigo'],
  ['qwen', 'violet'],
  ['glm', 'cyan'],
  ['kimi', 'teal'],
  ['doubao', 'sky'],
  ['siliconflow', 'fuchsia'],
  ['openrouter', 'slate'],
  ['minimax', 'rose'],
  ['mimo', 'orange'],
  ['composite', 'lime']
] as const satisfies ReadonlyArray<readonly [GroupPlatform, string]>

describe('GroupOptionItem platform colors', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it.each(providerCases)('uses the centralized %s rate palette', (platform, color) => {
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: platform,
        platform,
        rateMultiplier: 1
      },
      global: {
        plugins: [createI18n({
          legacy: false,
          locale: 'en',
          messages: { en: { admin: { groups: { rateLabel: 'rate' } } } }
        })]
      }
    })

    const classes = wrapper.findAll('*').flatMap((node) => node.classes())
    expect(classes.some((className) => className.includes(color))).toBe(true)
  })

  it('shows free instead of rate and peak multipliers', () => {
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Test group',
        platform: 'openai',
        rateMultiplier: 2,
        userRateMultiplier: 3,
        peakRateEnabled: true,
        peakStart: '09:00',
        peakEnd: '18:00',
        peakRateMultiplier: 4,
        isFree: true
      },
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
    expect(wrapper.text()).not.toContain('3x')
    expect(wrapper.text()).not.toContain('4x')
  })
})
