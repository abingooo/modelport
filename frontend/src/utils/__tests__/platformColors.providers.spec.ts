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
  ['qwen', 'cyan'],
  ['glm', 'sky'],
  ['kimi', 'pink'],
  ['doubao', 'red'],
  ['siliconflow', 'teal'],
  ['openrouter', 'amber'],
  ['minimax', 'violet'],
  ['mimo', 'lime']
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
