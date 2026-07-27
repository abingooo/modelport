import type {
  UserAvailableChannel,
  UserAvailableGroup,
  UserSupportedModel,
} from '@/api/channels'

export interface ModelPricingRow {
  key: string
  channelName: string
  platform: string
  groups: UserAvailableGroup[]
  model: UserSupportedModel
}

export interface ModelPricingFilters {
  search: string
  platform: string | null
  groupId: number | null
  billingCategory: ModelBillingCategory | null
}

export type ModelBillingCategory = 'usage' | 'request' | 'unconfigured'

export function modelBillingCategory(model: UserSupportedModel): ModelBillingCategory {
  if (!model.pricing) return 'unconfigured'
  return model.pricing.billing_mode === 'token' ? 'usage' : 'request'
}

export function flattenModelPricingRows(channels: UserAvailableChannel[]): ModelPricingRow[] {
  const rows: ModelPricingRow[] = []

  for (const channel of channels) {
    for (const section of channel.platforms) {
      for (const model of section.supported_models) {
        rows.push({
          key: `${channel.name}:${section.platform}:${model.name}`,
          channelName: channel.name,
          platform: section.platform,
          groups: section.groups,
          model,
        })
      }
    }
  }

  return rows.sort((left, right) =>
    left.platform.localeCompare(right.platform) ||
    left.model.name.localeCompare(right.model.name) ||
    left.channelName.localeCompare(right.channelName),
  )
}

export function filterModelPricingRows(
  rows: ModelPricingRow[],
  filters: ModelPricingFilters,
): ModelPricingRow[] {
  const search = filters.search.trim().toLowerCase()

  return rows.filter((row) => {
    if (filters.platform && row.platform !== filters.platform) return false
    if (filters.groupId != null && !row.groups.some((group) => group.id === filters.groupId)) {
      return false
    }
    if (filters.billingCategory && modelBillingCategory(row.model) !== filters.billingCategory) {
      return false
    }
    if (!search) return true

    return row.model.name.toLowerCase().includes(search) ||
      row.channelName.toLowerCase().includes(search) ||
      row.platform.toLowerCase().includes(search) ||
      row.groups.some((group) => group.name.toLowerCase().includes(search))
  })
}

export function collectAccessibleGroups(channels: UserAvailableChannel[]): UserAvailableGroup[] {
  const groups = new Map<number, UserAvailableGroup>()
  for (const channel of channels) {
    for (const section of channel.platforms) {
      for (const group of section.groups) groups.set(group.id, group)
    }
  }
  return [...groups.values()].sort((left, right) => left.name.localeCompare(right.name))
}
