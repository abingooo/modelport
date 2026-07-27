import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { GroupPlatform } from '@/types'
import GroupOptionItem from '../GroupOptionItem.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key === 'admin.groups.freeBilling.badge' ? 'Free' : key,
    }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

describe('GroupOptionItem description layout', () => {
  it('applies multiline and overflow-safe text styles', () => {
    const description = 'First section\nvery-long-unbroken-description-value-that-must-not-overflow'
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Example group',
        platform: 'openai',
        description,
      },
      global: {
        stubs: {
          GroupBadge: true,
        },
      },
    })

    const descriptionElement = wrapper
      .findAll('span')
      .find((element) => element.text() === description)

    expect(descriptionElement).toBeDefined()
    expect(descriptionElement?.classes()).toContain('whitespace-pre-line')
    expect(descriptionElement?.classes()).toContain('[overflow-wrap:anywhere]')
    expect(descriptionElement?.classes()).toContain('line-clamp-3')
    expect(wrapper.find('[title]').attributes('title')).toBe(description)
  })
})

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
  ['minimax', 'rose'],
  ['mimo', 'orange'],
  ['composite', 'lime'],
] as const satisfies ReadonlyArray<readonly [GroupPlatform, string]>

describe('GroupOptionItem billing presentation', () => {
  it.each(providerCases)('uses the centralized %s rate palette', (platform, color) => {
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: platform,
        platform,
        rateMultiplier: 1,
      },
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
        isFree: true,
      },
    })

    expect(wrapper.text()).toContain('Free')
    expect(wrapper.text()).not.toContain('2x')
    expect(wrapper.text()).not.toContain('3x')
    expect(wrapper.text()).not.toContain('4x')
  })
})
