import { describe, expect, it } from 'vitest'
import type { ModelCatalogItem } from '@/api/modelCatalog'
import { buildModelCatalogExample, normalizeCatalogGatewayBase } from '@/utils/modelCatalogExamples'

function makeItem(overrides: Partial<ModelCatalogItem> = {}): ModelCatalogItem {
  return {
    metadata_id: 0,
    platform: 'openai',
    name: 'gpt-5',
    display_name: 'GPT 5',
    description: '',
    capabilities: ['text'],
    context_window: 0,
    interface_formats: ['openai', 'anthropic'],
    scenarios: ['chat'],
    example_overrides: {},
    is_recommended: false,
    is_visible: true,
    sort_order: 0,
    available: true,
    offers: [],
    ...overrides,
  }
}

describe('model catalog examples', () => {
  it.each([
    ['https://test.modelport.link', 'https://test.modelport.link'],
    ['https://test.modelport.link/v1', 'https://test.modelport.link'],
    ['https://test.modelport.link/v1/chat/completions', 'https://test.modelport.link'],
  ])('normalizes %s without duplicating version paths', (input, expected) => {
    expect(normalizeCatalogGatewayBase(input)).toBe(expected)
  })

  it('builds supported protocol endpoints', () => {
    const item = makeItem({ name: 'gemini-3-pro' })
    expect(buildModelCatalogExample(item, 'openai', 'https://test.modelport.link/v1'))
      .toContain("https://test.modelport.link/v1/chat/completions")
    expect(buildModelCatalogExample(item, 'anthropic', 'https://test.modelport.link'))
      .toContain("https://test.modelport.link/v1/messages")
    expect(buildModelCatalogExample(item, 'google', 'https://test.modelport.link/v1beta'))
      .toContain("https://test.modelport.link/v1beta/models/gemini-3-pro:generateContent")
  })

  it.each([
    ['image', '/v1/images/generations'],
    ['video', '/v1/videos/generations'],
    ['embedding', '/v1/embeddings'],
  ])('uses the OpenAI endpoint for the %s scenario', (scenario, endpoint) => {
    const item = makeItem({ scenarios: [scenario] })
    expect(buildModelCatalogExample(item, 'openai', 'https://test.modelport.link')).toContain(endpoint)
  })

  it('prefers an administrator example override', () => {
    const item = makeItem({ example_overrides: { openai: 'custom example' } })
    expect(buildModelCatalogExample(item, 'openai', 'https://ignored.example')).toBe('custom example')
  })
})
