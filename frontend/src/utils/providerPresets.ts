import type { AccountPlatform, GroupPlatform, OpenAICompatibleProviderPlatform } from '@/types'

export interface OpenAICompatibleProviderPreset {
  id: OpenAICompatibleProviderPlatform
  name: string
  shortName: string
  defaultBaseUrl: string
  defaultModel: string
  modelSuggestions: string[]
  apiKeyPlaceholder: string
  supportsModelList: boolean
  modelReferenceMode: 'model_id' | 'endpoint_or_model'
}

export const OPENAI_COMPATIBLE_PROVIDER_PRESETS: readonly OpenAICompatibleProviderPreset[] = [
  {
    id: 'deepseek', name: 'DeepSeek', shortName: 'DS', defaultBaseUrl: 'https://api.deepseek.com',
    defaultModel: 'deepseek-chat', modelSuggestions: ['deepseek-chat', 'deepseek-reasoner'],
    apiKeyPlaceholder: 'sk-...',
    supportsModelList: true, modelReferenceMode: 'model_id'
  },
  {
    id: 'qwen', name: '通义千问', shortName: 'QW', defaultBaseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    defaultModel: 'qwen3.7-plus', modelSuggestions: ['qwen3.8-max-preview', 'qwen3.7-max', 'qwen3.7-plus', 'qwen3.7-flash', 'qwen3-coder-plus', 'qwen-vl-plus'],
    apiKeyPlaceholder: 'Enter API key',
    supportsModelList: true, modelReferenceMode: 'model_id'
  },
  {
    id: 'glm', name: '智谱AI', shortName: 'GLM', defaultBaseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    defaultModel: 'glm-5.2', modelSuggestions: ['glm-5.2', 'glm-5.1', 'glm-5-turbo', 'glm-4.7', 'glm-4.6v'],
    apiKeyPlaceholder: 'Enter API key',
    supportsModelList: true, modelReferenceMode: 'model_id'
  },
  {
    id: 'kimi', name: 'Kimi', shortName: 'KM', defaultBaseUrl: 'https://api.moonshot.cn/v1',
    defaultModel: 'kimi-k3', modelSuggestions: ['kimi-k3', 'kimi-k2.7-code-highspeed', 'kimi-k2.6', 'kimi-k2.5', 'moonshot-v1-128k'],
    apiKeyPlaceholder: 'Enter API key',
    supportsModelList: true, modelReferenceMode: 'model_id'
  },
  {
    id: 'doubao', name: 'ByteDance', shortName: 'BD', defaultBaseUrl: 'https://ark.cn-beijing.volces.com/api/v3',
    defaultModel: '', modelSuggestions: ['doubao-seed-1.8', 'doubao-seed-code', 'doubao-seed-1.6-vision'],
    apiKeyPlaceholder: 'Enter API key',
    supportsModelList: false, modelReferenceMode: 'endpoint_or_model'
  },
  {
    id: 'minimax', name: 'MiniMax', shortName: 'MM', defaultBaseUrl: 'https://api.minimaxi.com/v1',
    defaultModel: 'MiniMax-M3', modelSuggestions: ['MiniMax-M3', 'MiniMax-M2.7', 'MiniMax-M2.7-highspeed', 'MiniMax-M2.5'],
    apiKeyPlaceholder: 'Enter API key',
    supportsModelList: false, modelReferenceMode: 'model_id'
  },
  {
    id: 'mimo', name: '小米 MiMo', shortName: 'MI', defaultBaseUrl: 'https://api.xiaomimimo.com/v1',
    defaultModel: 'mimo-v2.5', modelSuggestions: ['mimo-v2.5-pro', 'mimo-v2.5', 'mimo-v2-pro', 'mimo-v2-omni', 'mimo-v2-flash'],
    apiKeyPlaceholder: 'Enter API key',
    supportsModelList: false, modelReferenceMode: 'model_id'
  }
] as const

export const OPENAI_COMPATIBLE_PROVIDER_IDS = OPENAI_COMPATIBLE_PROVIDER_PRESETS.map(preset => preset.id)

export const CONCRETE_PLATFORM_ORDER: readonly AccountPlatform[] = [
  'anthropic', 'openai', 'gemini', 'antigravity', 'grok', ...OPENAI_COMPATIBLE_PROVIDER_IDS
]

export const GROUP_PLATFORM_ORDER: readonly GroupPlatform[] = [...CONCRETE_PLATFORM_ORDER, 'composite']

const providerPresetByID = new Map(OPENAI_COMPATIBLE_PROVIDER_PRESETS.map(preset => [preset.id, preset]))

export function getOpenAICompatibleProviderPreset(platform?: string | null): OpenAICompatibleProviderPreset | null {
  if (!platform) return null
  return providerPresetByID.get(platform as OpenAICompatibleProviderPlatform) ?? null
}

export function isDedicatedOpenAICompatibleProvider(platform?: string | null): platform is OpenAICompatibleProviderPlatform {
  return getOpenAICompatibleProviderPreset(platform) !== null
}

export function platformDisplayName(platform?: string | null): string {
  const preset = getOpenAICompatibleProviderPreset(platform)
  if (preset) return preset.name
  switch (platform) {
    case 'anthropic': return 'Anthropic'
    case 'openai': return 'OpenAI'
    case 'gemini': return 'Gemini'
    case 'antigravity': return 'Antigravity'
    case 'grok': return 'Grok'
    case 'composite': return 'Composite'
    default: return platform || 'API'
  }
}

export function defaultBaseUrlForPlatform(platform?: string | null): string {
  const preset = getOpenAICompatibleProviderPreset(platform)
  if (preset) return preset.defaultBaseUrl
  switch (platform) {
    case 'openai': return 'https://api.openai.com'
    case 'gemini': return 'https://generativelanguage.googleapis.com'
    case 'antigravity': return 'https://cloudcode-pa.googleapis.com'
    case 'grok': return 'https://api.x.ai/v1'
    default: return 'https://api.anthropic.com'
  }
}

export function apiKeyPlaceholderForPlatform(platform?: string | null): string {
  const preset = getOpenAICompatibleProviderPreset(platform)
  if (preset) return preset.apiKeyPlaceholder
  switch (platform) {
    case 'openai': return 'sk-proj-...'
    case 'gemini': return 'AIza...'
    case 'grok': return 'xai-...'
    case 'antigravity': return 'sk-...'
    default: return 'sk-ant-...'
  }
}
