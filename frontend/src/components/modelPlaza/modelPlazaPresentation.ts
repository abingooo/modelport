import type { BillingMode } from '@/constants/channel'
import { BILLING_MODE_TOKEN } from '@/constants/channel'
import type { UserSupportedModelPricing } from '@/api/channels'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import { GROUP_PLATFORM_ORDER } from '@/utils/providerPresets'

export interface PlazaModelOffer {
  group: ModelPlazaGroup
  model: PlazaModel
}

export interface PlazaModelCardData {
  key: string
  name: string
  platform: string
  offers: PlazaModelOffer[]
}

export interface PlazaProviderSectionData {
  platform: string
  label: string
  cards: PlazaModelCardData[]
}

export interface PlazaFilterGroupData {
  id: number
  name: string
  platforms: string[]
  subscriptionType: string
  effectiveMultiplier: number
  isFree: boolean
  isExclusive: boolean
}

export type PlazaSortMode = 'name' | 'output'

export const PLAZA_OFFICIAL_GROUP_ID = '__official__' as const
export type PlazaGroupFilterValue = number | 'all' | typeof PLAZA_OFFICIAL_GROUP_ID

const PROVIDER_LABELS: Record<string, string> = {
  anthropic: 'Anthropic · Claude',
  openai: 'OpenAI',
  antigravity: 'Google · Antigravity',
  gemini: 'Google · Gemini',
  grok: 'xAI · Grok',
  deepseek: 'DeepSeek',
  qwen: 'Alibaba Cloud · Qwen',
  glm: '智谱AI · GLM',
  kimi: 'Moonshot · Kimi',
  doubao: 'ByteDance · Doubao',
  minimax: 'MiniMax',
  mimo: 'Xiaomi · MiMo',
  composite: 'ModelPort · Composite'
}

const platformOrder = new Map<string, number>(GROUP_PLATFORM_ORDER.map((platform, index) => [platform, index]))

export function plazaProviderLabel(platform: string): string {
  return PROVIDER_LABELS[platform] ?? (platform || 'API')
}

export function plazaBillingMode(model: PlazaModel): BillingMode {
  return (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

export function plazaHasOfficialPricing(model: PlazaModel): boolean {
  const pricing = model.pricing
  if (!pricing) return false
  const flatPrices = [
    pricing.input_price,
    pricing.output_price,
    pricing.cache_write_price,
    pricing.cache_read_price,
    pricing.image_input_price,
    pricing.image_output_price,
    pricing.per_request_price
  ]
  const intervalPrices = pricing.intervals.flatMap((interval) => [
    interval.input_price,
    interval.output_price,
    interval.cache_write_price,
    interval.cache_read_price,
    interval.per_request_price
  ])
  return Boolean(
    [...flatPrices, ...intervalPrices].some((value) => value != null && Number.isFinite(value))
  )
}

function firstFinitePrice(values: Array<number | null | undefined>): number | null {
  const value = values.find((candidate) => candidate != null && Number.isFinite(candidate))
  return value == null ? null : value
}

export function plazaModelSortPrice(model: PlazaModel | undefined, officialPricing = false): number | null {
  if (!model) return null
  const pricing: UserSupportedModelPricing | null = officialPricing ? model.pricing : model.display_pricing
  if (!pricing) return null
  if (plazaBillingMode(model) === BILLING_MODE_TOKEN) {
    const intervalPrices = pricing.intervals.flatMap((interval) => [
      interval.output_price,
      interval.input_price,
      interval.cache_write_price,
      interval.cache_read_price
    ])
    return firstFinitePrice([
      pricing.output_price,
      pricing.input_price,
      pricing.cache_write_price,
      pricing.cache_read_price,
      ...intervalPrices
    ])
  }

  const intervalPrices = pricing.intervals
    .map((interval) => interval.per_request_price)
    .filter((value): value is number => value != null && Number.isFinite(value))
  return firstFinitePrice([
    pricing.per_request_price,
    pricing.image_output_price,
    pricing.image_input_price,
    ...(intervalPrices.length > 0 ? [Math.min(...intervalPrices)] : [])
  ])
}

function compareCardNames(left: PlazaModelCardData, right: PlazaModelCardData): number {
  return left.name.localeCompare(right.name, undefined, { numeric: true })
}

export function sortPlazaCards(
  cards: PlazaModelCardData[],
  sortMode: PlazaSortMode,
  officialPricing = false
): PlazaModelCardData[] {
  return [...cards].sort((left, right) => {
    if (sortMode === 'name') return compareCardNames(left, right)

    const leftPrice = plazaModelSortPrice(left.offers[0]?.model, officialPricing)
    const rightPrice = plazaModelSortPrice(right.offers[0]?.model, officialPricing)
    if (leftPrice == null && rightPrice == null) return compareCardNames(left, right)
    if (leftPrice == null) return 1
    if (rightPrice == null) return -1
    return rightPrice - leftPrice || compareCardNames(left, right)
  })
}

export function buildPlazaProviderSections(
  groups: ModelPlazaGroup[],
  options: { sortMode?: PlazaSortMode; officialPricing?: boolean } = {}
): PlazaProviderSectionData[] {
  const providers = new Map<string, Map<string, PlazaModelCardData>>()
  const sortMode = options.sortMode ?? 'name'

  for (const group of groups) {
    for (const model of group.models) {
      const platform = model.platform || group.platform
      let models = providers.get(platform)
      if (!models) {
        models = new Map<string, PlazaModelCardData>()
        providers.set(platform, models)
      }
      const modelKey = model.name.toLowerCase()
      let card = models.get(modelKey)
      if (!card) {
        card = {
          key: `${platform}:${modelKey}`,
          name: model.name,
          platform,
          offers: []
        }
        models.set(modelKey, card)
      }
      card.offers.push({ group, model })
    }
  }

  return [...providers.entries()]
    .sort(([left], [right]) => {
      const leftOrder = platformOrder.get(left) ?? Number.MAX_SAFE_INTEGER
      const rightOrder = platformOrder.get(right) ?? Number.MAX_SAFE_INTEGER
      return leftOrder - rightOrder || plazaProviderLabel(left).localeCompare(plazaProviderLabel(right))
    })
    .map(([platform, models]) => {
      const cards = [...models.values()].map((card) => ({
        ...card,
        offers: [...card.offers].sort(
          (left, right) =>
            left.model.effective_multiplier - right.model.effective_multiplier ||
            left.group.name.localeCompare(right.group.name)
        )
      }))
      return {
        platform,
        label: plazaProviderLabel(platform),
        cards: sortPlazaCards(cards, sortMode, options.officialPricing)
      }
    })
}
