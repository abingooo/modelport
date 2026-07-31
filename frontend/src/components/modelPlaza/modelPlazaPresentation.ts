import type { BillingMode } from '@/constants/channel'
import { BILLING_MODE_TOKEN } from '@/constants/channel'
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

export function buildPlazaProviderSections(groups: ModelPlazaGroup[]): PlazaProviderSectionData[] {
  const providers = new Map<string, Map<string, PlazaModelCardData>>()

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
    .map(([platform, models]) => ({
      platform,
      label: plazaProviderLabel(platform),
      cards: [...models.values()]
        .map((card) => ({
          ...card,
          offers: [...card.offers].sort(
            (left, right) =>
              left.model.effective_multiplier - right.model.effective_multiplier ||
              left.group.name.localeCompare(right.group.name)
          )
        }))
        .sort((left, right) => left.name.localeCompare(right.name, undefined, { numeric: true }))
    }))
}
