import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import PlatformIcon from '../PlatformIcon.vue'

const providers = [
  'deepseek',
  'qwen',
  'glm',
  'kimi',
  'doubao',
  'minimax',
  'mimo'
] as const

describe('PlatformIcon provider branding', () => {
  it.each(providers)('renders the %s brand mark instead of a letter placeholder', (platform) => {
    const wrapper = mount(PlatformIcon, { props: { platform, size: 'lg' } })

    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.find('.model-icon-fallback').exists()).toBe(false)
  })
})
