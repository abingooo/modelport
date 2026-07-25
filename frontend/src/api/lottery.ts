import { apiClient } from './client'
import type { PaginatedResponse } from '@/types'

export type LotteryMode = 'instant' | 'scheduled'
export type LotteryCampaignStatus = 'draft' | 'active' | 'paused' | 'completed'
export type LotteryCampaignState =
  | 'draft'
  | 'not_started'
  | 'active'
  | 'paused'
  | 'awaiting_draw'
  | 'ended'
  | 'completed'
export type LotteryEntryStatus = 'pending' | 'won' | 'not_won'
export type LotteryPrizeType = 'balance' | 'subscription_code'

export interface LotteryPrize {
  id: number
  campaign_id: number
  name: string
  prize_type: LotteryPrizeType
  balance_amount: number
  subscription_group_id?: number
  subscription_validity_days: number
  probability_bps: number
  inventory: number
  awarded_count: number
  is_enabled: boolean
  sort_order: number
  created_at: string
  updated_at: string
}

export interface LotteryCampaign {
  id: number
  name: string
  description: string
  mode: LotteryMode
  status: LotteryCampaignStatus
  state: LotteryCampaignState
  starts_at: string
  ends_at: string
  draw_at?: string
  per_user_limit: number
  minimum_balance: number
  required_subscription_group_ids: number[]
  eligible: boolean
  eligibility_reason?: string
  user_entry_count: number
  entry_count: number
  created_by?: number
  updated_by?: number
  created_at: string
  updated_at: string
  prizes: LotteryPrize[]
}

export interface LotteryEntry {
  id: number
  campaign_id: number
  campaign_name?: string
  campaign_mode?: LotteryMode
  user_id: number
  user_email?: string
  status: LotteryEntryStatus
  prize_id?: number
  prize_name?: string
  prize_type?: LotteryPrizeType
  balance_amount: number
  subscription_group_id?: number
  subscription_validity_days: number
  reward_redeem_code_id?: number
  reward_code?: string
  created_at: string
  resolved_at?: string
  replayed?: boolean
}

export interface LotteryPrizeInput {
  name: string
  prize_type: LotteryPrizeType
  balance_amount: number
  subscription_group_id: number | null
  subscription_validity_days: number
  probability_bps: number
  inventory: number
  is_enabled: boolean
  sort_order: number
}

export interface LotteryCampaignInput {
  name: string
  description: string
  mode: LotteryMode
  status: LotteryCampaignStatus
  starts_at: string
  ends_at: string
  draw_at: string | null
  per_user_limit: number
  minimum_balance: number
  required_subscription_group_ids: number[]
  prizes: LotteryPrizeInput[]
}

export interface LotteryDrawResult {
  campaign_id: number
  participant_count: number
  winner_count: number
  already_completed: boolean
}

export interface LotteryListParams {
  page?: number
  page_size?: number
  status?: string
  mode?: string
  search?: string
}

export async function listLotteryCampaigns(params: LotteryListParams = {}, signal?: AbortSignal): Promise<PaginatedResponse<LotteryCampaign>> {
  const { data } = await apiClient.get<PaginatedResponse<LotteryCampaign>>('/lottery', { params, signal })
  return data
}

export async function getLotteryCampaign(id: number): Promise<LotteryCampaign> {
  const { data } = await apiClient.get<LotteryCampaign>(`/lottery/${id}`)
  return data
}

export async function participateInLottery(id: number, idempotencyKey: string): Promise<LotteryEntry> {
  const { data } = await apiClient.post<LotteryEntry>(`/lottery/${id}/participate`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
  return data
}

export async function listLotteryHistory(params: LotteryListParams = {}, signal?: AbortSignal): Promise<PaginatedResponse<LotteryEntry>> {
  const { data } = await apiClient.get<PaginatedResponse<LotteryEntry>>('/lottery/history', { params, signal })
  return data
}

export async function listAdminLotteryCampaigns(params: LotteryListParams = {}, signal?: AbortSignal): Promise<PaginatedResponse<LotteryCampaign>> {
  const { data } = await apiClient.get<PaginatedResponse<LotteryCampaign>>('/admin/lottery', { params, signal })
  return data
}

export async function createLotteryCampaign(input: LotteryCampaignInput): Promise<LotteryCampaign> {
  const { data } = await apiClient.post<LotteryCampaign>('/admin/lottery', input)
  return data
}

export async function updateLotteryCampaign(id: number, input: LotteryCampaignInput): Promise<LotteryCampaign> {
  const { data } = await apiClient.put<LotteryCampaign>(`/admin/lottery/${id}`, input)
  return data
}

export async function setLotteryCampaignStatus(id: number, status: LotteryCampaignStatus): Promise<LotteryCampaign> {
  const { data } = await apiClient.put<LotteryCampaign>(`/admin/lottery/${id}/status`, { status })
  return data
}

export async function deleteLotteryCampaign(id: number): Promise<void> {
  await apiClient.delete(`/admin/lottery/${id}`)
}

export async function listAdminLotteryEntries(id: number, params: LotteryListParams = {}, signal?: AbortSignal): Promise<PaginatedResponse<LotteryEntry>> {
  const { data } = await apiClient.get<PaginatedResponse<LotteryEntry>>(`/admin/lottery/${id}/entries`, { params, signal })
  return data
}

export async function drawLotteryCampaign(id: number): Promise<LotteryDrawResult> {
  const { data } = await apiClient.post<LotteryDrawResult>(`/admin/lottery/${id}/draw`)
  return data
}

export const lotteryAPI = {
  list: listLotteryCampaigns,
  get: getLotteryCampaign,
  participate: participateInLottery,
  history: listLotteryHistory,
  admin: {
    list: listAdminLotteryCampaigns,
    create: createLotteryCampaign,
    update: updateLotteryCampaign,
    setStatus: setLotteryCampaignStatus,
    delete: deleteLotteryCampaign,
    entries: listAdminLotteryEntries,
    draw: drawLotteryCampaign,
  },
}

export default lotteryAPI
