import { describe, expect, it } from 'vitest'
import {
  findPricingModelConflict,
  isMissingRequiredUnitPrice,
  validatePricingEntryPrices,
  validateIntervals,
  type IntervalFormEntry,
  type PricingFormEntry,
} from '../types'

function makeInterval(over: Partial<IntervalFormEntry>): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: '',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: 0,
    ...over,
  }
}

function t(key: string, params?: Record<string, unknown>): string {
  return `${key}${params ? ` ${JSON.stringify(params)}` : ''}`
}

function makePricingEntry(over: Partial<PricingFormEntry>): PricingFormEntry {
  return {
    models: ['gpt-5'],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    user_visible: true,
    intervals: [],
    ...over,
  }
}

describe('isMissingRequiredUnitPrice', () => {
  it.each(['per_request', 'image', 'video'] as const)(
    'requires a fallback price or tier for %s billing',
    (billingMode) => {
      expect(isMissingRequiredUnitPrice({
        billing_mode: billingMode,
        per_request_price: null,
        intervals: [],
      })).toBe(true)
    },
  )

  it('accepts an explicit zero fallback price', () => {
    expect(isMissingRequiredUnitPrice({
      billing_mode: 'video',
      per_request_price: 0,
      intervals: [],
    })).toBe(false)
  })

  it('accepts tier pricing without a fallback price', () => {
    expect(isMissingRequiredUnitPrice({
      billing_mode: 'video',
      per_request_price: null,
      intervals: [makeInterval({ tier_label: '1080p', per_request_price: 0.2 })],
    })).toBe(false)
  })

  it('does not require a unit price for token billing', () => {
    expect(isMissingRequiredUnitPrice({
      billing_mode: 'token',
      per_request_price: null,
      intervals: [],
    })).toBe(false)
  })
})

describe('findPricingModelConflict', () => {
  it('detects exact and wildcard overlap case-insensitively', () => {
    expect(findPricingModelConflict(['GPT-*', 'gpt-5'])).toEqual([
      'GPT-*',
      'gpt-5',
    ])
  })

  it('matches backend normalization for Claude dot and hyphen spellings', () => {
    expect(findPricingModelConflict([
      'claude-3.5-sonnet',
      'claude-3-5-sonnet',
    ])).toEqual(['claude-3.5-sonnet', 'claude-3-5-sonnet'])
  })
})

describe('validatePricingEntryPrices', () => {
  it('accepts empty and explicit zero prices', () => {
    expect(validatePricingEntryPrices(makePricingEntry({ input_price: 0 }), t)).toBeNull()
  })

  it('rejects negative top-level prices', () => {
    expect(validatePricingEntryPrices(
      makePricingEntry({ image_output_price: -0.01 }),
      t,
    )).toContain('priceValidation.negative')
  })

  it.each([Number.NaN, Number.POSITIVE_INFINITY, 'not-a-number'])(
    'rejects non-finite top-level price %s',
    (value) => {
      expect(validatePricingEntryPrices(
        makePricingEntry({ per_request_price: value }),
        t,
      )).toContain('priceValidation.finite')
    },
  )
})

describe('validateIntervals', () => {
  describe('token mode', () => {
    it('rejects unbounded interval that is not last', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    })

    it('accepts unbounded interval at the end', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 200000, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: null, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toBeNull()
    })

    it('rejects overlapping intervals', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 250000, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('overlap')
    })

    it('rejects unbounded interval in token mode', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 100, max_tokens: 200, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    })
  })

  describe('image / per_request / video mode', () => {
    it('allows multiple unbounded tiers identified by label', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: 0.04 }),
        makeInterval({ tier_label: '2K', per_request_price: 0.06 }),
        makeInterval({ tier_label: '4K', per_request_price: 0.08 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toBeNull()
      expect(validateIntervals(intervals, 'per_request', t)).toBeNull()
      expect(validateIntervals(intervals, 'video', t)).toBeNull()
    })

	it('rejects duplicate tier labels case-insensitively', () => {
	  const intervals: IntervalFormEntry[] = [
		makeInterval({ tier_label: '2K', per_request_price: 0.06 }),
		makeInterval({ tier_label: ' 2k ', per_request_price: 0.08 }),
	  ]
	  expect(validateIntervals(intervals, 'image', t)).toContain('duplicateTier')
	})

    it('still rejects negative prices', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: -1 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('negativePrice')
    })

    it('rejects non-finite prices', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: Number.POSITIVE_INFINITY }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('nonFinitePrice')
    })

    it('rejects a tier with no price fields', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1080p' }),
      ]
      expect(validateIntervals(intervals, 'video', t)).toContain('missingPrice')
    })

    it('still rejects max <= min on a single tier', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', min_tokens: 100, max_tokens: 50, per_request_price: 0.04 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('maxGreaterThanMin')
    })
  })
})
