/**
 * Model Plaza API（公开端点，可匿名访问）
 * 以分组为中心的模型价目：渠道官方基础价 + 应用分组和用户倍率后的展示价。
 * 带 token 请求时后端会额外返回专属分组与用户专属倍率。
 */

import { apiClient } from './client'
import type { UserSupportedModelPricing } from './channels'

/** 兼容旧客户端的渠道 token 官方价投影；完整官方价使用 PlazaModel.pricing。 */
export interface PlazaOfficialPricing {
  input_price: number | null
  output_price: number | null
  /** 缓存写入价格。 */
  cache_write_price: number | null
  /** 旧版兼容字段；渠道定价不单独提供 1h 缓存写入价格。 */
  cache_write_1h_price?: number | null
  cache_read_price: number | null
}

export interface PlazaModel {
  name: string
  platform: string
  pricing: UserSupportedModelPricing | null
  /** 后端按当前用户、分组、峰时及图片独立倍率计算后的展示价格。 */
  display_pricing: UserSupportedModelPricing | null
  effective_multiplier: number
  /** @deprecated 使用 pricing；该字段仅保留旧客户端兼容。 */
  official_pricing: PlazaOfficialPricing | null
}

export interface ModelPlazaGroup {
  id: number
  name: string
  description: string
  platform: string
  /** 'standard' | 'subscription' */
  subscription_type: string
  rate_multiplier: number
  /** 登录且管理员为该用户配了专属倍率时返回；生效倍率 = user_rate ?? rate_multiplier。 */
  user_rate_multiplier?: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  applied_peak_multiplier: number
  effective_rate_multiplier: number
  effective_image_rate_multiplier: number
  effective_video_rate_multiplier: number
  is_free: boolean
  is_exclusive: boolean
  /** 生图独立倍率：true 时图片计费模型的实付倍率取 image_rate_multiplier，不取分组/专属倍率。 */
  image_rate_independent: boolean
  image_rate_multiplier: number
  /** 视频独立倍率：true 时视频计费模型的实付倍率取 video_rate_multiplier。 */
  video_rate_independent: boolean
  video_rate_multiplier: number
  models: PlazaModel[]
}

export interface ModelPlazaResponse {
  /** 管理员配置的全局价格说明（Markdown）。 */
  description: string
  currency: string
  official_pricing_source: string
  official_pricing_updated_at?: string
  groups: ModelPlazaGroup[]
}

/** 获取模型广场数据。开关未启用时后端返回 404。 */
export async function getModelPlaza(options?: { signal?: AbortSignal }): Promise<ModelPlazaResponse> {
  const { data } = await apiClient.get<ModelPlazaResponse>('/model-plaza', {
    signal: options?.signal
  })
  return data
}

export const modelPlazaAPI = { getModelPlaza }

export default modelPlazaAPI
