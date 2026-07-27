import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export type ModelInterfaceFormat = 'openai' | 'anthropic' | 'google'

export interface ModelCatalogGroup {
  id: number
  name: string
  rate_multiplier: number
  is_free: boolean
  peak_rate_enabled: boolean
  peak_rate_multiplier: number
  subscription_type: string
  is_exclusive: boolean
}

export interface ModelCatalogPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface ModelCatalogPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_input_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: ModelCatalogPricingInterval[]
}

export interface ModelCatalogOffer {
  channel_id: number
  channel_name: string
  groups: ModelCatalogGroup[]
  pricing: ModelCatalogPricing | null
}

export interface ModelCatalogMetadata {
  id: number
  platform: string
  model_name: string
  display_name: string
  description: string
  capabilities: string[]
  context_window: number
  interface_formats: ModelInterfaceFormat[]
  scenarios: string[]
  example_overrides: Partial<Record<ModelInterfaceFormat, string>>
  is_recommended: boolean
  is_visible: boolean
  sort_order: number
  created_at?: string
  updated_at?: string
}

export interface ModelCatalogItem {
  metadata_id: number
  platform: string
  name: string
  display_name: string
  description: string
  capabilities: string[]
  context_window: number
  interface_formats: ModelInterfaceFormat[]
  scenarios: string[]
  example_overrides: Partial<Record<ModelInterfaceFormat, string>>
  is_recommended: boolean
  is_visible: boolean
  sort_order: number
  available: boolean
  offers: ModelCatalogOffer[]
}

export type ModelCatalogMetadataInput = Omit<ModelCatalogMetadata, 'id' | 'created_at' | 'updated_at'> & {
  id?: number
}

export async function listModelCatalog(options?: { signal?: AbortSignal }): Promise<ModelCatalogItem[]> {
  const { data } = await apiClient.get<ModelCatalogItem[]>('/model-catalog', { signal: options?.signal })
  return data
}

export async function listAdminModelCatalog(options?: { signal?: AbortSignal }): Promise<ModelCatalogItem[]> {
  const { data } = await apiClient.get<ModelCatalogItem[]>('/admin/model-catalog', { signal: options?.signal })
  return data
}

export async function saveModelCatalogMetadata(input: ModelCatalogMetadataInput): Promise<ModelCatalogMetadata> {
  const { data } = await apiClient.put<ModelCatalogMetadata>('/admin/model-catalog', input)
  return data
}

export async function deleteModelCatalogMetadata(id: number): Promise<void> {
  await apiClient.delete(`/admin/model-catalog/${id}`)
}

export const modelCatalogAPI = {
  list: listModelCatalog,
  listAdmin: listAdminModelCatalog,
  saveMetadata: saveModelCatalogMetadata,
  deleteMetadata: deleteModelCatalogMetadata,
}

export default modelCatalogAPI
