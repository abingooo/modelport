/**
 * Admin Affiliate API endpoints
 * Manage per-user affiliate (邀请返利) configurations:
 * exclusive invite codes (overrides aff_code) and exclusive rebate rates.
 */

import { apiClient } from '../client'
import type { AffiliateRewardProgramConfig, PaginatedResponse } from '@/types'

export interface AffiliateAdminEntry {
  user_id: number
  email: string
  username: string
  aff_code: string
  aff_code_custom: boolean
  aff_rebate_rate_percent?: number | null
  aff_count: number
}

export interface ListAffiliateUsersParams {
  page?: number
  page_size?: number
  search?: string
}

export interface ListAffiliateRecordsParams {
  page?: number
  page_size?: number
  search?: string
  start_at?: string
  end_at?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
  timezone?: string
}

export interface AffiliateInviteRecord {
  inviter_id: number
  inviter_email: string
  inviter_username: string
  invitee_id: number
  invitee_email: string
  invitee_username: string
  aff_code: string
  total_rebate: number
  created_at: string
}

export interface AffiliateRebateRecord {
  order_id: number
  out_trade_no: string
  inviter_id: number
  inviter_email: string
  inviter_username: string
  invitee_id: number
  invitee_email: string
  invitee_username: string
  order_amount: number
  pay_amount: number
  rebate_amount: number
  payment_type: string
  order_status: string
  created_at: string
}

export interface AffiliateTransferRecord {
  ledger_id: number
  user_id: number
  user_email: string
  username: string
  amount: number
  balance_after?: number | null
  available_quota_after?: number | null
  frozen_quota_after?: number | null
  history_quota_after?: number | null
  snapshot_available: boolean
  created_at: string
}

export interface AffiliateUserOverview {
  user_id: number
  email: string
  username: string
  aff_code: string
  rebate_rate_percent: number
  invited_count: number
  rebated_invitee_count: number
  available_quota: number
  history_quota: number
}

export interface UpdateAffiliateUserRequest {
  aff_code?: string
  aff_rebate_rate_percent?: number | null
  /** Set true to explicitly clear the per-user rate (sets it to NULL). */
  clear_rebate_rate?: boolean
}

export interface BatchSetRateRequest {
  user_ids: number[]
  aff_rebate_rate_percent?: number | null
  /** Set true to clear rates instead of setting. */
  clear?: boolean
}

export interface SimpleUser {
  id: number
  email: string
  username: string
}

export type AffiliateRewardStatus = 'pending' | 'approved' | 'rejected' | 'paid'

export interface AffiliateRewardReview {
  id: number
  inviter_user_id: number
  invitee_user_id: number
  reward_user_id: number
  reward_type: string
  reward_amount: number
  payment_order_id?: number | null
  status: AffiliateRewardStatus | string
  risk_flags: Record<string, unknown>
  risk_level: 'low' | 'medium' | 'high' | 'unknown' | string
  risk_score: number
  reviewed_by?: number | null
  reviewed_at?: string | null
  review_note: string
  paid_at?: string | null
  created_at: string
  updated_at: string
  inviter_email: string
  invitee_email: string
  reward_user_email: string
  reviewed_by_email: string
  order_amount?: number | null
  order_pay_amount?: number | null
  order_status: string
  registration_ip: string
  registration_ip_first_seen_at?: string | null
  approval_blocked_reason?: string
}

export interface AffiliateRewardReviewStats {
  pending_count: number
  pending_amount: number
  paid_count: number
  paid_amount: number
  rejected_count: number
  high_risk_pending_count: number
  today_paid_count: number
  by_type: Record<string, unknown>
}

export interface ListAffiliateRewardReviewsParams {
  page?: number
  page_size?: number
  search?: string
  status?: string
  reward_type?: string
  risk?: string
}

export interface AffiliateRewardReviewResult {
  review_ids: number[]
  status: string
  effects: Array<{ user_id: number; kind: string; group_id?: number }>
}

export async function listUsers(
  params: ListAffiliateUsersParams = {},
): Promise<PaginatedResponse<AffiliateAdminEntry>> {
  const { data } = await apiClient.get<PaginatedResponse<AffiliateAdminEntry>>(
    '/admin/affiliates/users',
    {
      params: {
        page: params.page ?? 1,
        page_size: params.page_size ?? 20,
        search: params.search ?? '',
      },
    },
  )
  return data
}

export async function lookupUsers(q: string): Promise<SimpleUser[]> {
  const { data } = await apiClient.get<SimpleUser[]>(
    '/admin/affiliates/users/lookup',
    { params: { q } },
  )
  return data
}

export async function updateUserSettings(
  userId: number,
  payload: UpdateAffiliateUserRequest,
): Promise<{ user_id: number }> {
  const { data } = await apiClient.put<{ user_id: number }>(
    `/admin/affiliates/users/${userId}`,
    payload,
  )
  return data
}

export async function clearUserSettings(
  userId: number,
): Promise<{ user_id: number }> {
  const { data } = await apiClient.delete<{ user_id: number }>(
    `/admin/affiliates/users/${userId}`,
  )
  return data
}

export async function batchSetRate(
  payload: BatchSetRateRequest,
): Promise<{ affected: number }> {
  const { data } = await apiClient.post<{ affected: number }>(
    '/admin/affiliates/users/batch-rate',
    payload,
  )
  return data
}

function recordParams(params: ListAffiliateRecordsParams = {}) {
  return {
    page: params.page ?? 1,
    page_size: params.page_size ?? 20,
    search: params.search ?? '',
    start_at: params.start_at || undefined,
    end_at: params.end_at || undefined,
    sort_by: params.sort_by || undefined,
    sort_order: params.sort_order || undefined,
    timezone: params.timezone || undefined,
  }
}

export async function listInviteRecords(
  params: ListAffiliateRecordsParams = {},
): Promise<PaginatedResponse<AffiliateInviteRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<AffiliateInviteRecord>>(
    '/admin/affiliates/invites',
    { params: recordParams(params) },
  )
  return data
}

export async function listRebateRecords(
  params: ListAffiliateRecordsParams = {},
): Promise<PaginatedResponse<AffiliateRebateRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<AffiliateRebateRecord>>(
    '/admin/affiliates/rebates',
    { params: recordParams(params) },
  )
  return data
}

export async function listTransferRecords(
  params: ListAffiliateRecordsParams = {},
): Promise<PaginatedResponse<AffiliateTransferRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<AffiliateTransferRecord>>(
    '/admin/affiliates/transfers',
    { params: recordParams(params) },
  )
  return data
}

export async function getUserOverview(
  userId: number,
): Promise<AffiliateUserOverview> {
  const { data } = await apiClient.get<AffiliateUserOverview>(
    `/admin/affiliates/users/${userId}/overview`,
  )
  return data
}

export async function getRewardProgram(): Promise<AffiliateRewardProgramConfig> {
  const { data } = await apiClient.get<AffiliateRewardProgramConfig>('/admin/affiliates/reward-program')
  return data
}

export async function updateRewardProgram(config: AffiliateRewardProgramConfig): Promise<AffiliateRewardProgramConfig> {
  const { data } = await apiClient.put<AffiliateRewardProgramConfig>('/admin/affiliates/reward-program', config)
  return data
}

export async function listRewardReviews(
  params: ListAffiliateRewardReviewsParams = {},
): Promise<PaginatedResponse<AffiliateRewardReview>> {
  const { data } = await apiClient.get<PaginatedResponse<AffiliateRewardReview>>('/admin/affiliates/reviews', { params })
  return data
}

export async function getRewardReviewStats(): Promise<AffiliateRewardReviewStats> {
  const { data } = await apiClient.get<AffiliateRewardReviewStats>('/admin/affiliates/reviews/stats')
  return data
}

export async function reviewReward(
  reviewId: number,
  action: 'approve' | 'reject',
  note = '',
): Promise<AffiliateRewardReviewResult> {
  const { data } = await apiClient.post<AffiliateRewardReviewResult>(
    `/admin/affiliates/reviews/${reviewId}/${action}`,
    { note },
  )
  return data
}

export const affiliatesAPI = {
  listUsers,
  lookupUsers,
  updateUserSettings,
  clearUserSettings,
  batchSetRate,
  listInviteRecords,
  listRebateRecords,
  listTransferRecords,
  getUserOverview,
  getRewardProgram,
  updateRewardProgram,
  listRewardReviews,
  getRewardReviewStats,
  reviewReward,
}

export default affiliatesAPI
