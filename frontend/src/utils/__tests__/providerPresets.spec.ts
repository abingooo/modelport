import { describe, expect, it } from 'vitest'

import {
  apiKeyPlaceholderForPlatform,
  getOpenAICompatibleProviderPreset
} from '../providerPresets'

const providers = [
  ['qwen', 'qwen3.7-plus'],
  ['glm', 'glm-5.2'],
  ['kimi', 'kimi-k3'],
  ['doubao', 'doubao-seed-1.8'],
  ['minimax', 'MiniMax-M3'],
  ['mimo', 'mimo-v2.5']
] as const

describe('OpenAI-compatible provider presets', () => {
  it.each(providers)('%s exposes model suggestions and a neutral API key placeholder', (platform, suggestion) => {
    const preset = getOpenAICompatibleProviderPreset(platform)

    expect(preset?.modelSuggestions).toContain(suggestion)
    expect(apiKeyPlaceholderForPlatform(platform)).toBe('Enter API key')
    expect(apiKeyPlaceholderForPlatform(platform)).not.toContain('ant')
  })
})
