<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
        <div class="inline-flex w-full rounded-md bg-gray-100 p-1 dark:bg-dark-800 sm:w-auto" role="tablist">
          <button v-for="tab in tabs" :key="tab.value" type="button" role="tab" :aria-selected="activeTab === tab.value"
            :class="['min-h-9 flex-1 rounded px-4 text-sm font-medium transition sm:flex-none', activeTab === tab.value ? 'bg-white text-gray-950 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white']"
            @click="activeTab = tab.value">
            {{ tab.label }}
          </button>
        </div>
        <button type="button" class="icon-btn self-end" :title="t('lottery.refresh')" :disabled="activeLoading" @click="refreshActiveTab">
          <Icon name="refresh" size="md" :class="{ 'animate-spin': activeLoading }" />
        </button>
      </div>

      <template v-if="activeTab === 'activities'">
        <div v-if="campaignLoading" class="grid gap-4 lg:grid-cols-2">
          <div v-for="index in 4" :key="index" class="h-80 animate-pulse rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900" />
        </div>
        <div v-else-if="campaignError" class="py-16 text-center">
          <Icon name="exclamationCircle" size="xl" class="mx-auto text-red-500" />
          <p class="mt-4 text-sm text-gray-600 dark:text-gray-300">{{ campaignError }}</p>
          <button type="button" class="btn btn-secondary mt-5" @click="loadCampaigns">{{ t('lottery.retry') }}</button>
        </div>
        <EmptyState v-else-if="campaigns.length === 0" :title="t('lottery.emptyTitle')" :description="t('lottery.emptyDescription')" />
        <div v-else class="grid items-stretch gap-4 lg:grid-cols-2">
          <article v-for="campaign in campaigns" :key="campaign.id" class="relative flex min-h-[22rem] flex-col overflow-hidden rounded-md border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <span class="absolute inset-x-0 top-0 h-1" :class="campaign.mode === 'instant' ? 'bg-teal-500' : 'bg-sky-500'" />
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0">
                <h2 class="break-words text-base font-semibold text-gray-950 dark:text-white">{{ campaign.name }}</h2>
                <div class="mt-2 flex flex-wrap gap-2">
                  <span :class="['badge', campaign.mode === 'instant' ? 'border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-900 dark:bg-teal-950/40 dark:text-teal-300' : 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900 dark:bg-sky-950/40 dark:text-sky-300']">{{ t(`lottery.mode.${campaign.mode}`) }}</span>
                  <span :class="['badge', stateClass(campaign.state)]">{{ t(`lottery.state.${campaign.state}`) }}</span>
                </div>
              </div>
              <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('lottery.entryCount', { count: campaign.entry_count }) }}</span>
            </div>

            <p v-if="campaign.description" class="mt-4 whitespace-pre-line text-sm leading-6 text-gray-600 dark:text-gray-300">{{ campaign.description }}</p>

            <dl class="mt-4 grid grid-cols-1 gap-2 text-xs text-gray-500 dark:text-gray-400 sm:grid-cols-2">
              <div class="flex items-center gap-2"><Icon name="calendar" size="sm" /><span>{{ t('lottery.startsAt') }}: {{ formatDateTimeToMinute(campaign.starts_at) }}</span></div>
              <div class="flex items-center gap-2"><Icon name="clock" size="sm" /><span>{{ t('lottery.endsAt') }}: {{ formatDateTimeToMinute(campaign.ends_at) }}</span></div>
              <div v-if="campaign.draw_at" class="flex items-center gap-2 sm:col-span-2"><Icon name="sparkles" size="sm" /><span>{{ t('lottery.drawAt') }}: {{ formatDateTimeToMinute(campaign.draw_at) }}</span></div>
            </dl>

            <div class="mt-5">
              <p class="text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">{{ t('lottery.prizes') }}</p>
              <div class="mt-2 space-y-2">
                <div v-for="prize in campaign.prizes" :key="prize.id" class="flex items-center justify-between gap-3 rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-800">
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ prize.name }}</p>
                    <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ prizeSummary(prize) }}</p>
                  </div>
                  <div class="shrink-0 text-right text-[11px] text-gray-500 dark:text-gray-400">
                    <p>{{ t('lottery.probability', { value: formatProbability(prize.probability_bps) }) }}</p>
                    <p>{{ t('lottery.inventory', { remaining: Math.max(0, prize.inventory - prize.awarded_count), total: prize.inventory }) }}</p>
                  </div>
                </div>
              </div>
            </div>

            <div class="mt-auto border-t border-gray-100 pt-4 dark:border-dark-700">
              <div class="mb-3 flex items-center justify-between gap-3 text-xs">
                <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.attempts', { remaining: remainingAttempts(campaign), total: campaign.per_user_limit }) }}</span>
                <span v-if="!campaign.eligible" class="text-right text-amber-700 dark:text-amber-300">{{ eligibilityText(campaign) }}</span>
              </div>
              <button type="button" class="btn btn-primary w-full" :disabled="!campaign.eligible || participatingCampaignID !== null" @click="participate(campaign)">
                <Icon v-if="participatingCampaignID !== campaign.id" name="gift" size="sm" />
                <span v-else class="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" />
                {{ participatingCampaignID === campaign.id ? t('lottery.participating') : t('lottery.participate') }}
              </button>
            </div>
          </article>
        </div>
        <Pagination v-if="campaignPagination.total > campaignPagination.page_size" :page="campaignPagination.page" :page-size="campaignPagination.page_size" :total="campaignPagination.total" :show-page-size-selector="false" @update:page="changeCampaignPage" />
      </template>

      <template v-else>
        <div v-if="historyLoading" class="flex justify-center py-16"><LoadingSpinner /></div>
        <div v-else-if="historyError" class="py-16 text-center">
          <p class="text-sm text-red-600 dark:text-red-400">{{ historyError }}</p>
          <button type="button" class="btn btn-secondary mt-4" @click="loadHistory">{{ t('lottery.retry') }}</button>
        </div>
        <EmptyState v-else-if="history.length === 0" :title="t('lottery.historyEmptyTitle')" :description="t('lottery.historyEmptyDescription')" />
        <div v-else class="overflow-hidden rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
          <div v-for="entry in history" :key="entry.id" class="flex flex-col gap-3 border-b border-gray-100 p-4 last:border-b-0 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <p class="font-medium text-gray-950 dark:text-white">{{ entry.campaign_name }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('lottery.participatedAt') }}: {{ formatDateTime(entry.created_at) }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2 sm:justify-end">
              <span :class="['badge', entryStatusClass(entry.status)]">{{ t(`lottery.entryStatus.${entry.status}`) }}</span>
              <span v-if="entry.status === 'won'" class="text-sm font-semibold text-teal-700 dark:text-teal-300">{{ entryReward(entry) }}</span>
              <button v-if="entry.reward_code" type="button" class="icon-btn" :title="t('lottery.result.copyCode')" @click="copyToClipboard(entry.reward_code, t('common.copiedToClipboard'))"><Icon name="copy" size="sm" /></button>
            </div>
          </div>
        </div>
        <Pagination v-if="historyPagination.total > historyPagination.page_size" :page="historyPagination.page" :page-size="historyPagination.page_size" :total="historyPagination.total" :show-page-size-selector="false" @update:page="changeHistoryPage" />
      </template>
    </div>

    <LotteryResultDialog :show="resultEntry !== null" :entry="resultEntry" @close="resultEntry = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import LotteryResultDialog from '@/components/lottery/LotteryResultDialog.vue'
import lotteryAPI, { type LotteryCampaign, type LotteryEntry, type LotteryPrize } from '@/api/lottery'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatCurrency, formatDateTime, formatDateTimeToMinute } from '@/utils/format'
import { useClipboard } from '@/composables/useClipboard'

type Tab = 'activities' | 'history'
const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const { copyToClipboard } = useClipboard()
const activeTab = ref<Tab>('activities')
const campaigns = ref<LotteryCampaign[]>([])
const history = ref<LotteryEntry[]>([])
const campaignLoading = ref(false)
const historyLoading = ref(false)
const campaignError = ref('')
const historyError = ref('')
const participatingCampaignID = ref<number | null>(null)
const resultEntry = ref<LotteryEntry | null>(null)
const campaignPagination = ref({ page: 1, page_size: 20, total: 0 })
const historyPagination = ref({ page: 1, page_size: 20, total: 0 })
const pendingRequestKeys = new Map<number, string>()
let campaignController: AbortController | null = null
let historyController: AbortController | null = null

const tabs = computed(() => [
  { value: 'activities' as const, label: t('lottery.activities') },
  { value: 'history' as const, label: t('lottery.history') },
])
const activeLoading = computed(() => activeTab.value === 'activities' ? campaignLoading.value : historyLoading.value)

function makeRequestKey(campaignID: number): string {
  const random = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `lottery-${campaignID}-${random}`
}

async function loadCampaigns() {
  campaignController?.abort()
  campaignController = new AbortController()
  campaignLoading.value = true
  campaignError.value = ''
  try {
    const response = await lotteryAPI.list({ page: campaignPagination.value.page, page_size: campaignPagination.value.page_size }, campaignController.signal)
    campaigns.value = response.items
    campaignPagination.value = { page: response.page, page_size: response.page_size, total: response.total }
  } catch (error: unknown) {
    if (!campaignController.signal.aborted) campaignError.value = extractI18nErrorMessage(error, t, 'lottery.errors', t('lottery.loadFailed'))
  } finally {
    if (!campaignController.signal.aborted) campaignLoading.value = false
  }
}

async function loadHistory() {
  historyController?.abort()
  historyController = new AbortController()
  historyLoading.value = true
  historyError.value = ''
  try {
    const response = await lotteryAPI.history({ page: historyPagination.value.page, page_size: historyPagination.value.page_size }, historyController.signal)
    history.value = response.items
    historyPagination.value = { page: response.page, page_size: response.page_size, total: response.total }
  } catch (error: unknown) {
    if (!historyController.signal.aborted) historyError.value = extractI18nErrorMessage(error, t, 'lottery.errors', t('lottery.loadFailed'))
  } finally {
    if (!historyController.signal.aborted) historyLoading.value = false
  }
}

async function participate(campaign: LotteryCampaign) {
  if (!campaign.eligible || participatingCampaignID.value !== null) return
  participatingCampaignID.value = campaign.id
  const requestKey = pendingRequestKeys.get(campaign.id) ?? makeRequestKey(campaign.id)
  pendingRequestKeys.set(campaign.id, requestKey)
  try {
    const entry = await lotteryAPI.participate(campaign.id, requestKey)
    pendingRequestKeys.delete(campaign.id)
    resultEntry.value = entry
    if (entry.status === 'won' && entry.prize_type === 'balance') await authStore.refreshUser()
    await Promise.all([loadCampaigns(), loadHistory()])
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'lottery.errors', t('common.error')))
  } finally {
    participatingCampaignID.value = null
  }
}

function refreshActiveTab() {
  return activeTab.value === 'activities' ? loadCampaigns() : loadHistory()
}
function changeCampaignPage(page: number) { campaignPagination.value.page = page; void loadCampaigns() }
function changeHistoryPage(page: number) { historyPagination.value.page = page; void loadHistory() }
function remainingAttempts(campaign: LotteryCampaign) { return Math.max(0, campaign.per_user_limit - campaign.user_entry_count) }
function eligibilityText(campaign: LotteryCampaign) { return t(`lottery.eligibility.${campaign.eligibility_reason || campaign.state}`) }
function formatProbability(bps: number) { return (bps / 100).toFixed(bps % 100 === 0 ? 0 : 2) }
function prizeSummary(prize: LotteryPrize) {
  return prize.prize_type === 'balance'
    ? t('lottery.balancePrize', { amount: formatCurrency(prize.balance_amount).replace('￥', '') })
    : t('lottery.subscriptionPrize', { days: prize.subscription_validity_days })
}
function entryReward(entry: LotteryEntry) {
  return entry.prize_type === 'balance' ? formatCurrency(entry.balance_amount) : entry.prize_name || '-'
}
function stateClass(state: LotteryCampaign['state']) {
  if (state === 'active') return 'badge-success'
  if (state === 'awaiting_draw' || state === 'not_started') return 'badge-info'
  if (state === 'paused') return 'badge-warning'
  return 'badge-gray'
}
function entryStatusClass(status: LotteryEntry['status']) {
  if (status === 'won') return 'badge-success'
  if (status === 'pending') return 'badge-info'
  return 'badge-gray'
}

watch(activeTab, (tab) => { if (tab === 'history' && history.value.length === 0) void loadHistory() })
onMounted(loadCampaigns)
onBeforeUnmount(() => { campaignController?.abort(); historyController?.abort() })
</script>
