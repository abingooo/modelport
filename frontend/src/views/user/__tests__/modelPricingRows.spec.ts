import { describe, expect, it } from 'vitest'

import type { UserAvailableChannel, UserAvailableGroup } from '@/api/channels'
import {
  collectAccessibleGroups,
  filterModelPricingRows,
  flattenModelPricingRows,
  modelBillingCategory,
} from '../modelPricingRows'

function group(id: number, name: string, platform: string): UserAvailableGroup {
  return {
    id,
    name,
    platform,
    subscription_type: 'standard',
    rate_multiplier: 1,
    is_free: false,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
  }
}

const channels: UserAvailableChannel[] = [
  {
    name: 'Domestic',
    description: '',
    platforms: [
      {
        platform: 'mimo',
        groups: [group(2, 'MiMo Pro', 'mimo')],
        supported_models: [
          {
            name: 'mimo-v2.5',
            platform: 'mimo',
            pricing: {
              billing_mode: 'token',
              input_price: 1e-6,
              output_price: 3e-6,
              cache_write_price: null,
              cache_read_price: null,
              image_input_price: null,
              image_output_price: null,
              per_request_price: null,
              intervals: [],
            },
          },
        ],
      },
      {
        platform: 'deepseek',
        groups: [group(1, 'DeepSeek', 'deepseek')],
        supported_models: [
          { name: 'deepseek-chat', platform: 'deepseek', pricing: null },
          {
            name: 'deepseek-flat-rate',
            platform: 'deepseek',
            pricing: {
              billing_mode: 'per_request',
              input_price: null,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              image_input_price: null,
              image_output_price: null,
              per_request_price: 0.02,
              intervals: [],
            },
          },
        ],
      },
    ],
  },
]

describe('model pricing rows', () => {
  it('flattens channel models while retaining only the endpoint-provided groups', () => {
    const rows = flattenModelPricingRows(channels)

    expect(rows).toHaveLength(3)
    expect(rows.find((row) => row.model.name === 'mimo-v2.5')?.groups.map((item) => item.id)).toEqual([2])
  })

  it('filters by accessible group and model search', () => {
    const rows = filterModelPricingRows(flattenModelPricingRows(channels), {
      search: 'mimo-v2.5',
      platform: null,
      groupId: 2,
      billingCategory: 'usage',
    })

    expect(rows.map((row) => row.model.name)).toEqual(['mimo-v2.5'])
  })

  it('collects unique accessible groups for the filter', () => {
    expect(collectAccessibleGroups(channels).map((item) => item.id)).toEqual([1, 2])
  })

  it('classifies and filters usage and per-request pricing', () => {
    const rows = flattenModelPricingRows(channels)
    const requestRows = filterModelPricingRows(rows, {
      search: '',
      platform: null,
      groupId: null,
      billingCategory: 'request',
    })

    expect(modelBillingCategory(rows.find((row) => row.model.name === 'mimo-v2.5')!.model)).toBe('usage')
    expect(modelBillingCategory(rows.find((row) => row.model.name === 'deepseek-chat')!.model)).toBe('unconfigured')
    expect(requestRows.map((row) => row.model.name)).toEqual(['deepseek-flat-rate'])
  })
})
