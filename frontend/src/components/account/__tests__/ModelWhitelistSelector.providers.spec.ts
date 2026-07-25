import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { nextTick } from 'vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const providers = [
  ['qwen', 'qwen3.7-plus'],
  ['glm', 'glm-5.2'],
  ['kimi', 'kimi-k3'],
  ['doubao', 'doubao-seed-1.8'],
  ['siliconflow', 'deepseek-ai/DeepSeek-V3.2'],
  ['openrouter', 'openai/gpt-4o-mini'],
  ['minimax', 'MiniMax-M3'],
  ['mimo', 'mimo-v2.5']
] as const

describe('ModelWhitelistSelector provider suggestions', () => {
  it.each(providers)('shows %s suggestions in the whitelist dropdown', async (platform, suggestion) => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: { modelValue: [], platform },
      global: {
        plugins: [createPinia()],
        stubs: {
          Icon: true,
          ModelIcon: true
        }
      }
    })

    await wrapper.get('.cursor-pointer').trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain(suggestion)
    expect(wrapper.text()).not.toContain('admin.accounts.noMatchingModels')
    wrapper.unmount()
  })
})
