import { describe, expect, it } from 'vitest'

import {
  platformAccentBarClass,
  platformBadgeClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformButtonClass,
  platformDiscountClass,
  platformGradientClass,
  platformGradientSubtextClass,
  platformGradientTextClass,
  platformIconClass,
  platformTextClass
} from '../platformColors'

const providers = [
  ['qwen', 'violet'],
  ['glm', 'blue'],
  ['kimi', 'zinc'],
  ['doubao', 'blue'],
  ['siliconflow', 'purple'],
  ['openrouter', 'indigo'],
  ['minimax', 'rose'],
  ['mimo', 'orange']
] as const

const colorResolvers = [
  platformAccentBarClass,
  platformBadgeClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformButtonClass,
  platformDiscountClass,
  platformGradientClass,
  platformGradientSubtextClass,
  platformGradientTextClass,
  platformIconClass,
  platformTextClass
]

describe('OpenAI-compatible provider colors', () => {
  it.each(providers)('%s uses its dedicated %s palette across platform styles', (platform, color) => {
    for (const resolveColor of colorResolvers) {
      expect(resolveColor(platform)).toContain(color)
    }
  })

  it('keeps all eight provider badge palettes distinct', () => {
    const badgeClasses = providers.map(([platform]) => platformBadgeClass(platform))
    expect(new Set(badgeClasses)).toHaveLength(providers.length)
  })
})
