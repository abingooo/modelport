import { describe, expect, it } from 'vitest'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import {
  buildPlazaProviderSections,
  plazaHasOfficialPricing,
  plazaProviderLabel
} from '../modelPlazaPresentation'

function model(name: string, platform: string, multiplier: number): PlazaModel {
  return {
    name,
    platform,
    pricing: null,
    display_pricing: null,
    effective_multiplier: multiplier,
    official_pricing: null
  }
}

function group(id: number, name: string, platform: string, models: PlazaModel[]): ModelPlazaGroup {
  return {
    id,
    name,
    description: '',
    platform,
    subscription_type: 'standard',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    applied_peak_multiplier: 1,
    effective_rate_multiplier: 1,
    effective_image_rate_multiplier: 1,
    is_free: false,
    is_exclusive: false,
    models
  }
}

describe('model plaza presentation', () => {
  it('groups providers into ordered sections and merges the same model across groups', () => {
    const sections = buildPlazaProviderSections([
      group(1, 'Standard', 'grok', [model('grok-4.3', 'grok', 1)]),
      group(2, 'Discount', 'grok', [model('GROK-4.3', 'grok', 0.5)]),
      group(3, 'Claude', 'anthropic', [model('claude-sonnet', 'anthropic', 1)])
    ])

    expect(sections.map((section) => section.platform)).toEqual(['anthropic', 'grok'])
    expect(sections[1].cards).toHaveLength(1)
    expect(sections[1].cards[0].offers.map((offer) => offer.group.name)).toEqual(['Discount', 'Standard'])
  })

  it('uses product-facing provider labels', () => {
    expect(plazaProviderLabel('grok')).toBe('xAI · Grok')
    expect(plazaProviderLabel('glm')).toBe('智谱AI · GLM')
    expect(plazaProviderLabel('unknown')).toBe('unknown')
  })

  it('detects models with official token pricing', () => {
    const pricedModel = model('gpt-test', 'openai', 1)
    pricedModel.official_pricing = {
      input_price: 2.5e-6,
      output_price: 1.5e-5,
      cache_write_price: null,
      cache_read_price: 2.5e-7
    }

    expect(plazaHasOfficialPricing(pricedModel)).toBe(true)
    expect(plazaHasOfficialPricing(model('no-price', 'openai', 1))).toBe(false)
  })
})
