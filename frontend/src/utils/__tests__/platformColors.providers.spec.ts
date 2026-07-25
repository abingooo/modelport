import { describe, expect, it } from 'vitest'

import {
  platformAccentBarClass,
  platformAccentDotClass,
  platformBadgeClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformButtonClass,
  platformDiscountClass,
  platformGradientClass,
  platformGradientSubtextClass,
  platformGradientTextClass,
  platformIconClass,
  platformLabel,
  platformTextClass
} from '../platformColors'

const platforms = [
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
] as const

const colorResolvers = [
  platformAccentBarClass,
  platformAccentDotClass,
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

describe('platform colors', () => {
  it.each(platforms)('%s uses its dedicated %s palette across platform styles', (platform, color) => {
    for (const resolveColor of colorResolvers) {
      expect(resolveColor(platform)).toContain(color)
    }
  })

  it('keeps every group badge palette distinct', () => {
    const badgeClasses = platforms.map(([platform]) => platformBadgeLightClass(platform))
    expect(new Set(badgeClasses)).toHaveLength(platforms.length)
  })

  it('uses the approved Zhipu AI product name', () => {
    expect(platformLabel('glm')).toBe('智谱AI')
  })
})
