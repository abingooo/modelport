<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div class="card border-l-4 border-l-amber-400 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.stats.pending') }}</p>
          <div class="mt-2 flex items-end justify-between gap-3">
            <p class="text-2xl font-semibold text-gray-950 dark:text-white">{{ stats.pending_count }}</p>
            <p class="text-sm font-medium text-amber-700 dark:text-amber-300">{{ formatAmount(stats.pending_amount) }}</p>
          </div>
        </div>
        <div class="card border-l-4 border-l-red-500 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.stats.attention') }}</p>
          <p class="mt-2 text-2xl font-semibold text-red-700 dark:text-red-300">{{ stats.high_risk_pending_count }}</p>
        </div>
        <div class="card border-l-4 border-l-teal-500 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.stats.paid') }}</p>
          <div class="mt-2 flex items-end justify-between gap-3">
            <p class="text-2xl font-semibold text-gray-950 dark:text-white">{{ stats.paid_count }}</p>
            <p class="text-sm font-medium text-teal-700 dark:text-teal-300">{{ formatAmount(stats.paid_amount) }}</p>
          </div>
        </div>
        <div class="card border-l-4 border-l-primary-500 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.stats.today') }}</p>
          <p class="mt-2 text-2xl font-semibold text-primary-700 dark:text-primary-300">{{ stats.today_paid_count }}</p>
        </div>
      </div>

      <section class="card">
        <div class="border-b border-gray-200 p-4 dark:border-dark-700 sm:p-5">
          <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div class="grid flex-1 gap-3 sm:grid-cols-2 xl:grid-cols-[minmax(220px,1fr)_170px_220px_160px]">
              <label class="relative block">
                <span class="sr-only">{{ t('admin.affiliates.reviews.filters.search') }}</span>
                <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input
                  v-model="filters.search"
                  class="input pl-9"
                  :placeholder="t('admin.affiliates.reviews.filters.searchPlaceholder')"
                  @keyup.enter="reloadFromFirstPage"
                />
              </label>
              <select v-model="filters.status" class="input" @change="reloadFromFirstPage">
                <option value="pending_review">{{ t('admin.affiliates.reviews.filters.pendingReview') }}</option>
                <option value="final">{{ t('admin.affiliates.reviews.filters.final') }}</option>
                <option value="pending">{{ statusText('pending') }}</option>
                <option value="paid">{{ statusText('paid') }}</option>
                <option value="rejected">{{ statusText('rejected') }}</option>
              </select>
              <select v-model="filters.reward_type" class="input" @change="reloadFromFirstPage">
                <option value="all">{{ t('admin.affiliates.reviews.filters.allTypes') }}</option>
                <option v-for="type in knownRewardTypes" :key="type" :value="type">{{ rewardTypeText(type) }}</option>
              </select>
              <select v-model="filters.risk" class="input" @change="reloadFromFirstPage">
                <option value="all">{{ t('admin.affiliates.reviews.filters.allRisk') }}</option>
                <option value="attention">{{ t('admin.affiliates.reviews.filters.attention') }}</option>
                <option value="low">{{ riskText('low') }}</option>
                <option value="medium">{{ riskText('medium') }}</option>
                <option value="high">{{ riskText('high') }}</option>
                <option value="unknown">{{ riskText('unknown') }}</option>
              </select>
            </div>
            <div class="flex justify-end gap-2">
              <button type="button" class="icon-btn" :title="t('common.refresh')" :disabled="loading" @click="loadAll">
                <Icon name="refresh" size="md" :class="{ 'animate-spin': loading }" />
              </button>
              <button type="button" class="btn btn-secondary" @click="openProgramDialog">
                <Icon name="cog" size="sm" />
                {{ t('admin.affiliates.reviews.program.action') }}
              </button>
            </div>
          </div>
        </div>

        <div v-if="loading" class="flex justify-center py-20"><LoadingSpinner /></div>
        <div v-else-if="items.length === 0" class="py-16 text-center">
          <div class="mx-auto flex h-11 w-11 items-center justify-center rounded-lg bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-dark-400">
            <Icon name="inbox" size="lg" />
          </div>
          <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.affiliates.reviews.empty') }}</p>
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[1120px] text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">
              <tr>
                <th class="px-5 py-3 font-medium">{{ t('admin.affiliates.reviews.columns.review') }}</th>
                <th class="px-5 py-3 font-medium">{{ t('admin.affiliates.reviews.columns.event') }}</th>
                <th class="px-5 py-3 font-medium">{{ t('admin.affiliates.reviews.columns.relationship') }}</th>
                <th class="px-5 py-3 font-medium">{{ t('admin.affiliates.reviews.columns.reward') }}</th>
                <th class="px-5 py-3 font-medium">{{ t('admin.affiliates.reviews.columns.risk') }}</th>
                <th class="px-5 py-3 font-medium">{{ t('admin.affiliates.reviews.columns.status') }}</th>
                <th class="px-5 py-3 text-right font-medium">{{ t('admin.affiliates.reviews.columns.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in items" :key="item.id" class="align-top hover:bg-gray-50/60 dark:hover:bg-dark-700/30">
                <td class="px-5 py-4">
                  <button type="button" class="font-semibold text-primary-700 hover:underline dark:text-primary-300" @click="selectedDetail = item">#{{ item.id }}</button>
                  <p class="mt-1 whitespace-nowrap text-xs text-gray-400">{{ formatDateTime(item.created_at) }}</p>
                </td>
                <td class="max-w-64 px-5 py-4">
                  <p class="font-medium text-gray-900 dark:text-white">{{ rewardTypeText(item.reward_type) }}</p>
                  <p v-if="item.payment_order_id" class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                    {{ t('admin.affiliates.reviews.order', { id: item.payment_order_id }) }}
                  </p>
                  <span v-if="!isSupportedReward(item.reward_type)" class="mt-2 inline-flex badge badge-warning">
                    {{ t('admin.affiliates.reviews.unsupported') }}
                  </span>
                  <span v-if="item.approval_blocked_reason === 'legacy_cutoff'" class="mt-2 inline-flex badge badge-warning">
                    {{ t('admin.affiliates.reviews.legacyProtected') }}
                  </span>
                </td>
                <td class="px-5 py-4">
                  <p class="max-w-52 truncate text-gray-900 dark:text-white" :title="item.inviter_email">
                    {{ item.inviter_email || '#' + item.inviter_user_id }}
                  </p>
                  <p class="mt-1 max-w-52 truncate text-xs text-gray-500 dark:text-dark-400" :title="item.invitee_email">
                    <Icon name="arrowRight" size="xs" class="mr-1 inline" />{{ item.invitee_email || '#' + item.invitee_user_id }}
                  </p>
                </td>
                <td class="px-5 py-4">
                  <p class="max-w-52 truncate text-gray-700 dark:text-gray-300" :title="item.reward_user_email">
                    {{ item.reward_user_email || '#' + item.reward_user_id }}
                  </p>
                  <p class="mt-1 font-semibold text-gray-950 dark:text-white">{{ formatAmount(item.reward_amount) }}</p>
                </td>
                <td class="px-5 py-4">
                  <button type="button" :class="riskBadge(item.risk_level)" @click="selectedRisk = item">
                    {{ riskText(item.risk_level) }} · {{ item.risk_score }}
                  </button>
                </td>
                <td class="px-5 py-4">
                  <span :class="statusBadge(item.status)">{{ statusText(item.status) }}</span>
                  <p v-if="item.reviewed_by_email" class="mt-1 max-w-40 truncate text-xs text-gray-400" :title="item.reviewed_by_email">{{ item.reviewed_by_email }}</p>
                </td>
                <td class="px-5 py-4">
                  <div v-if="canReview(item)" class="flex justify-end gap-2">
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm text-red-700 dark:text-red-300"
                      @click="openReview(item, 'reject')"
                    >
                      <Icon name="x" size="xs" />{{ t('admin.affiliates.reviews.reject') }}
                    </button>
                    <button
                      type="button"
                      class="btn btn-primary btn-sm"
                      :disabled="!canApproveReward(item)"
                      :title="approvalDisabledReason(item)"
                      @click="openReview(item, 'approve')"
                    >
                      <Icon name="check" size="xs" />{{ t('admin.affiliates.reviews.approve') }}
                    </button>
                  </div>
                  <span v-else class="block text-right text-xs text-gray-400">-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :page-size="pagination.page_size"
          :total="pagination.total"
          @update:page="changePage"
          @update:page-size="changePageSize"
        />
      </section>
    </div>

    <BaseDialog
      :show="selectedRisk !== null"
      :title="t('admin.affiliates.reviews.riskDialog.title', { id: selectedRisk?.id || '' })"
      width="wide"
      @close="selectedRisk = null"
    >
      <div v-if="selectedRisk" class="space-y-5">
        <div class="flex flex-wrap items-center gap-2">
          <span :class="riskBadge(selectedRisk.risk_level)">{{ riskText(selectedRisk.risk_level) }}</span>
          <span class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.riskDialog.score', { score: selectedRisk.risk_score }) }}</span>
        </div>
        <dl class="grid gap-x-6 gap-y-4 sm:grid-cols-2">
          <div v-for="[key, value] in riskEntries(selectedRisk)" :key="key" class="border-b border-gray-100 pb-3 dark:border-dark-700">
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ riskKeyText(key) }}</dt>
            <dd class="mt-1 break-words text-sm text-gray-900 dark:text-white">{{ formatRiskValue(value) }}</dd>
          </div>
        </dl>
      </div>
    </BaseDialog>

    <BaseDialog
      :show="selectedDetail !== null"
      :title="t('admin.affiliates.reviews.detailDialog.title', { id: selectedDetail?.id || '' })"
      width="wide"
      @close="selectedDetail = null"
    >
      <dl v-if="selectedDetail" class="grid gap-x-6 gap-y-5 sm:grid-cols-2">
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.columns.event') }}</dt><dd class="detail-value">{{ rewardTypeText(selectedDetail.reward_type) }}</dd></div>
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.columns.status') }}</dt><dd class="mt-1"><span :class="statusBadge(selectedDetail.status)">{{ statusText(selectedDetail.status) }}</span></dd></div>
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.detailDialog.inviter') }}</dt><dd class="detail-value">{{ selectedDetail.inviter_email || selectedDetail.inviter_user_id }}</dd></div>
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.detailDialog.invitee') }}</dt><dd class="detail-value">{{ selectedDetail.invitee_email || selectedDetail.invitee_user_id }}</dd></div>
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.detailDialog.beneficiary') }}</dt><dd class="detail-value">{{ selectedDetail.reward_user_email || selectedDetail.reward_user_id }}</dd></div>
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.detailDialog.amount') }}</dt><dd class="detail-value">{{ formatAmount(selectedDetail.reward_amount) }}</dd></div>
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.detailDialog.order') }}</dt><dd class="detail-value">{{ selectedDetail.payment_order_id || '-' }}</dd></div>
        <div><dt class="detail-label">{{ t('admin.affiliates.reviews.detailDialog.registrationIp') }}</dt><dd class="detail-value">{{ selectedDetail.registration_ip || '-' }}</dd></div>
        <div class="sm:col-span-2"><dt class="detail-label">{{ t('admin.affiliates.reviews.detailDialog.note') }}</dt><dd class="detail-value whitespace-pre-wrap">{{ selectedDetail.review_note || '-' }}</dd></div>
      </dl>
    </BaseDialog>

    <BaseDialog
      :show="reviewTarget !== null"
      :title="reviewAction === 'approve' ? t('admin.affiliates.reviews.confirm.approveTitle') : t('admin.affiliates.reviews.confirm.rejectTitle')"
      width="narrow"
      :close-on-escape="!reviewing"
      :show-close-button="!reviewing"
      @close="closeReviewDialog"
    >
      <div v-if="reviewTarget">
        <div class="rounded-lg bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
          {{ t('admin.affiliates.reviews.confirm.groupNotice') }}
        </div>
        <div class="mt-4 flex items-center justify-between border-y border-gray-100 py-3 text-sm dark:border-dark-700">
          <span class="text-gray-500 dark:text-dark-400">{{ rewardTypeText(reviewTarget.reward_type) }}</span>
          <span class="font-semibold text-gray-950 dark:text-white">{{ formatAmount(reviewTarget.reward_amount) }}</span>
        </div>
        <label class="mt-4 block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.affiliates.reviews.confirm.note') }}</span>
          <textarea v-model="reviewNote" rows="3" class="input mt-2 resize-none" :placeholder="t('admin.affiliates.reviews.confirm.notePlaceholder')" />
        </label>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="reviewing" @click="closeReviewDialog">{{ t('common.cancel') }}</button>
        <button
          type="button"
          :class="reviewAction === 'approve' ? 'btn btn-primary' : 'btn btn-danger'"
          :disabled="reviewing || (reviewAction === 'reject' && !reviewNote.trim())"
          @click="submitReview"
        >
          <Icon v-if="reviewing" name="refresh" size="sm" class="animate-spin" />
          {{ reviewAction === 'approve' ? t('admin.affiliates.reviews.approve') : t('admin.affiliates.reviews.reject') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="programDialog"
      :title="t('admin.affiliates.reviews.program.title')"
      width="wide"
      :close-on-escape="!savingProgram"
      :show-close-button="!savingProgram"
      @close="closeProgramDialog"
    >
      <div v-if="programDraft" class="space-y-6">
        <div class="flex items-center justify-between border-b border-gray-100 pb-5 dark:border-dark-700">
          <div>
            <p class="font-medium text-gray-950 dark:text-white">{{ t('admin.affiliates.reviews.program.master') }}</p>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.program.masterHint') }}</p>
          </div>
          <Toggle v-model="programDraft.enabled" />
        </div>

        <section>
          <div class="flex items-center justify-between">
            <div>
              <h4 class="font-medium text-gray-950 dark:text-white">{{ t('admin.affiliates.reviews.program.registration') }}</h4>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.program.registrationHint') }}</p>
            </div>
            <Toggle v-model="programDraft.registration.enabled" />
          </div>
          <div class="mt-4 grid gap-4 sm:grid-cols-2">
            <label class="form-field"><span>{{ t('admin.affiliates.reviews.program.inviterBonus') }}</span><input v-model.number="programDraft.registration.inviter_bonus" type="number" min="0" step="0.01" class="input" /></label>
            <label class="form-field"><span>{{ t('admin.affiliates.reviews.program.trialAmount') }}</span><input v-model.number="programDraft.registration.invitee_trial_amount" type="number" min="0" step="0.01" class="input" /></label>
            <label class="form-field">
              <span>{{ t('admin.affiliates.reviews.program.trialGroup') }}</span>
              <select v-model.number="programDraft.registration.invitee_trial_group_id" class="input">
                <option v-if="!groupExists(programDraft.registration.invitee_trial_group_id)" :value="programDraft.registration.invitee_trial_group_id">
                  #{{ programDraft.registration.invitee_trial_group_id }}
                </option>
                <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }} (#{{ group.id }})</option>
              </select>
            </label>
            <label class="form-field"><span>{{ t('admin.affiliates.reviews.program.trialDays') }}</span><input v-model.number="programDraft.registration.invitee_trial_days" type="number" min="1" max="3650" step="1" class="input" /></label>
          </div>
        </section>

        <section class="border-t border-gray-100 pt-5 dark:border-dark-700">
          <div class="flex items-center justify-between">
            <div>
              <h4 class="font-medium text-gray-950 dark:text-white">{{ t('admin.affiliates.reviews.program.firstRecharge') }}</h4>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.reviews.program.firstRechargeHint') }}</p>
            </div>
            <Toggle v-model="programDraft.first_recharge.enabled" />
          </div>
          <div class="mt-4 grid gap-4 sm:grid-cols-2">
            <label class="form-field"><span>{{ t('admin.affiliates.reviews.program.inviterBonus') }}</span><input v-model.number="programDraft.first_recharge.inviter_bonus" type="number" min="0" step="0.01" class="input" /></label>
            <label class="form-field"><span>{{ t('admin.affiliates.reviews.program.inviteePercent') }}</span><input v-model.number="programDraft.first_recharge.invitee_bonus_percent" type="number" min="0" max="100" step="0.01" class="input" /></label>
          </div>
        </section>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="savingProgram" @click="closeProgramDialog">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="savingProgram || !programDraft" @click="saveProgram">
          <Icon v-if="savingProgram" name="refresh" size="sm" class="animate-spin" />
          {{ t('common.save') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI, affiliatesAPI } from '@/api/admin'
import type {
  AffiliateRewardReview,
  AffiliateRewardReviewStats,
} from '@/api/admin/affiliates'
import type { AdminGroup, AffiliateRewardProgramConfig } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatCurrency, formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const knownRewardTypes = [
  'invite_register_invitee_pro_trial_card',
  'invite_register_invitee_bonus',
  'invite_register_inviter_bonus',
  'first_recharge_invitee_bonus',
  'first_recharge_inviter_bonus',
  'limited_recharge_bonus',
] as const
const supportedRewardTypes = new Set(knownRewardTypes)
const loading = ref(false)
const items = ref<AffiliateRewardReview[]>([])
const groups = ref<AdminGroup[]>([])
const stats = reactive<AffiliateRewardReviewStats>({
  pending_count: 0,
  pending_amount: 0,
  paid_count: 0,
  paid_amount: 0,
  rejected_count: 0,
  high_risk_pending_count: 0,
  today_paid_count: 0,
  by_type: {},
})
const filters = reactive({ search: '', status: 'pending_review', reward_type: 'all', risk: 'all' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const selectedRisk = ref<AffiliateRewardReview | null>(null)
const selectedDetail = ref<AffiliateRewardReview | null>(null)
const reviewTarget = ref<AffiliateRewardReview | null>(null)
const reviewAction = ref<'approve' | 'reject'>('approve')
const reviewNote = ref('')
const reviewing = ref(false)
const program = ref<AffiliateRewardProgramConfig | null>(null)
const programDraft = ref<AffiliateRewardProgramConfig | null>(null)
const programDialog = ref(false)
const savingProgram = ref(false)

async function loadReviews(): Promise<void> {
  const response = await affiliatesAPI.listRewardReviews({
    page: pagination.page,
    page_size: pagination.page_size,
    ...filters,
  })
  items.value = response.items
  pagination.page = response.page
  pagination.page_size = response.page_size
  pagination.total = response.total
}
async function loadStats(): Promise<void> {
  Object.assign(stats, await affiliatesAPI.getRewardReviewStats())
}
async function loadProgram(): Promise<void> {
  program.value = await affiliatesAPI.getRewardProgram()
}
async function loadGroups(): Promise<void> {
  groups.value = (await adminAPI.groups.getAll()).filter((group) => group.status === 'active')
}
async function loadAll(): Promise<void> {
  loading.value = true
  try {
    await Promise.all([loadReviews(), loadStats(), loadProgram(), groups.value.length ? Promise.resolve() : loadGroups()])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.affiliates.errors.loadFailed')))
  } finally {
    loading.value = false
  }
}
function reloadFromFirstPage(): void {
  pagination.page = 1
  void loadAll()
}
function changePage(page: number): void {
  pagination.page = page
  void loadReviewsSafely()
}
function changePageSize(size: number): void {
  pagination.page = 1
  pagination.page_size = size
  void loadReviewsSafely()
}
async function loadReviewsSafely(): Promise<void> {
  loading.value = true
  try {
    await loadReviews()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.affiliates.errors.loadFailed')))
  } finally {
    loading.value = false
  }
}
function rewardTypeText(type: string): string {
  return supportedRewardTypes.has(type as typeof knownRewardTypes[number])
    ? t(`admin.affiliates.reviews.types.${type}`)
    : type
}
function statusText(status: string): string {
  return t(`admin.affiliates.reviews.status.${status}`, status)
}
function riskText(risk: string): string {
  return t(`admin.affiliates.reviews.risk.${risk}`, risk)
}
function statusBadge(status: string): string {
  if (status === 'paid') return 'badge border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-900 dark:bg-teal-950/40 dark:text-teal-300'
  if (status === 'pending' || status === 'approved') return 'badge border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300'
  if (status === 'rejected') return 'badge badge-danger'
  return 'badge badge-gray'
}
function riskBadge(risk: string): string {
  if (risk === 'high') return 'badge cursor-pointer border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300'
  if (risk === 'medium') return 'badge cursor-pointer border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300'
  if (risk === 'low') return 'badge cursor-pointer border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-900 dark:bg-teal-950/40 dark:text-teal-300'
  return 'badge badge-gray cursor-pointer'
}
function formatAmount(value: number): string {
  return formatCurrency(Number(value || 0))
}
function isSupportedReward(type: string): boolean {
  return supportedRewardTypes.has(type as typeof knownRewardTypes[number])
}
function canReview(item: AffiliateRewardReview): boolean {
  return item.status === 'pending' || item.status === 'approved'
}
function canApproveReward(item: AffiliateRewardReview): boolean {
  return isSupportedReward(item.reward_type) && !item.approval_blocked_reason
}
function approvalDisabledReason(item: AffiliateRewardReview): string | undefined {
  if (!isSupportedReward(item.reward_type)) return t('admin.affiliates.reviews.unsupportedApprove')
  if (item.approval_blocked_reason === 'legacy_cutoff') return t('admin.affiliates.reviews.legacyProtectedApprove')
  return undefined
}
function openReview(item: AffiliateRewardReview, action: 'approve' | 'reject'): void {
  reviewTarget.value = item
  reviewAction.value = action
  reviewNote.value = ''
}
function closeReviewDialog(): void {
  if (!reviewing.value) reviewTarget.value = null
}
async function submitReview(): Promise<void> {
  if (!reviewTarget.value || reviewing.value) return
  reviewing.value = true
  try {
    await affiliatesAPI.reviewReward(reviewTarget.value.id, reviewAction.value, reviewNote.value.trim())
    appStore.showSuccess(t(`admin.affiliates.reviews.confirm.${reviewAction.value}Success`))
    reviewTarget.value = null
    await Promise.all([loadReviews(), loadStats()])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.affiliates.errors.loadFailed')))
  } finally {
    reviewing.value = false
  }
}
function riskEntries(item: AffiliateRewardReview): Array<[string, unknown]> {
  return Object.entries(item.risk_flags || {}).sort(([left], [right]) => left.localeCompare(right))
}
function riskKeyText(key: string): string {
  return t(`admin.affiliates.reviews.riskKeys.${key}`, key.replace(/_/g, ' '))
}
function formatRiskValue(value: unknown): string {
  if (Array.isArray(value)) return value.length ? value.map(String).join(', ') : '-'
  if (typeof value === 'boolean') return value ? t('common.yes') : t('common.no')
  if (value === null || value === undefined || value === '') return '-'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}
function openProgramDialog(): void {
  if (!program.value) return
  programDraft.value = JSON.parse(JSON.stringify(program.value)) as AffiliateRewardProgramConfig
  programDialog.value = true
}
function closeProgramDialog(): void {
  if (!savingProgram.value) programDialog.value = false
}
function groupExists(groupID: number): boolean {
  return groups.value.some((group) => group.id === groupID)
}
async function saveProgram(): Promise<void> {
  if (!programDraft.value || savingProgram.value) return
  savingProgram.value = true
  try {
    program.value = await affiliatesAPI.updateRewardProgram(programDraft.value)
    programDialog.value = false
    appStore.showSuccess(t('admin.affiliates.reviews.program.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.affiliates.reviews.program.saveFailed')))
  } finally {
    savingProgram.value = false
  }
}

onMounted(() => void loadAll())
</script>

<style scoped>
.detail-label {
  @apply text-xs font-medium text-gray-500 dark:text-dark-400;
}
.detail-value {
  @apply mt-1 break-words text-sm text-gray-900 dark:text-white;
}
.form-field {
  @apply space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300;
}
</style>
