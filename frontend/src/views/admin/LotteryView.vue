<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div class="flex flex-1 flex-col gap-3 sm:flex-row">
            <label class="relative block w-full sm:max-w-sm">
              <span class="sr-only">{{ t('lottery.admin.search') }}</span>
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input v-model="filters.search" class="input pl-10" :placeholder="t('lottery.admin.search')" @keyup.enter="applySearch" />
            </label>
            <select v-model="filters.mode" class="input sm:w-44" @change="reloadFromFirstPage">
              <option value="">{{ t('lottery.admin.allModes') }}</option>
              <option value="instant">{{ t('lottery.mode.instant') }}</option>
              <option value="scheduled">{{ t('lottery.mode.scheduled') }}</option>
            </select>
            <select v-model="filters.status" class="input sm:w-44" @change="reloadFromFirstPage">
              <option value="">{{ t('lottery.admin.allStatuses') }}</option>
              <option v-for="status in campaignStatuses" :key="status" :value="status">{{ t(`lottery.state.${status}`) }}</option>
            </select>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="icon-btn" :title="t('common.refresh')" :disabled="loading" @click="loadCampaigns"><Icon name="refresh" size="md" :class="{ 'animate-spin': loading }" /></button>
            <button type="button" class="btn btn-primary" @click="openCreate"><Icon name="plus" size="sm" />{{ t('lottery.admin.create') }}</button>
          </div>
        </div>
      </template>

      <template #table>
        <div v-if="loading" class="flex justify-center py-20"><LoadingSpinner /></div>
        <div v-else-if="errorMessage" class="py-16 text-center">
          <p class="text-sm text-red-600 dark:text-red-400">{{ errorMessage }}</p>
          <button type="button" class="btn btn-secondary mt-4" @click="loadCampaigns">{{ t('lottery.retry') }}</button>
        </div>
        <EmptyState v-else-if="campaigns.length === 0" :title="t('lottery.admin.emptyTitle')" :description="t('lottery.admin.emptyDescription')" :action-text="t('lottery.admin.create')" @action="openCreate" />
        <div v-else class="table-wrapper">
          <table>
            <thead><tr>
              <th>{{ t('lottery.admin.campaign') }}</th>
              <th>{{ t('lottery.admin.window') }}</th>
              <th>{{ t('lottery.admin.progress') }}</th>
              <th>{{ t('lottery.admin.status') }}</th>
              <th class="text-right">{{ t('lottery.admin.actions') }}</th>
            </tr></thead>
            <tbody>
              <tr v-for="campaign in campaigns" :key="campaign.id">
                <td class="min-w-64 align-top">
                  <p class="font-medium text-gray-950 dark:text-white">{{ campaign.name }}</p>
                  <div class="mt-2 flex flex-wrap gap-1.5">
                    <span :class="['badge', campaign.mode === 'instant' ? 'border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-900 dark:bg-teal-950/40 dark:text-teal-300' : 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900 dark:bg-sky-950/40 dark:text-sky-300']">{{ t(`lottery.mode.${campaign.mode}`) }}</span>
                    <span v-if="campaign.full_draw_participant_limit" class="badge border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-900 dark:bg-primary-950/40 dark:text-primary-300">{{ t('lottery.admin.fullDrawBadge') }}</span>
                  </div>
                  <p v-if="campaign.entry_count > 0" class="mt-2 text-xs text-amber-700 dark:text-amber-300">{{ t('lottery.admin.hasEntries') }}</p>
                </td>
                <td class="min-w-56 align-top text-xs text-gray-600 dark:text-gray-300">
                  <p>{{ formatDateTimeToMinute(campaign.starts_at) }}</p><p class="mt-1">{{ formatDateTimeToMinute(campaign.ends_at) }}</p>
                  <p v-if="campaign.draw_at" class="mt-1 text-sky-700 dark:text-sky-300">{{ t('lottery.drawAt') }}: {{ formatDateTimeToMinute(campaign.draw_at) }}</p>
                </td>
                <td class="min-w-44 align-top text-sm text-gray-600 dark:text-gray-300">
                  <p v-if="campaign.full_draw_participant_limit">{{ t('lottery.participantProgress', { count: campaign.participant_count, limit: campaign.full_draw_participant_limit }) }}</p>
                  <p :class="campaign.full_draw_participant_limit ? 'mt-1 text-xs text-gray-400' : ''">{{ t('lottery.entryCount', { count: campaign.entry_count }) }}</p>
                  <p class="mt-1">{{ t('lottery.prizes') }}: {{ campaign.prizes.length }}</p>
                  <p class="mt-1 text-xs text-gray-400">{{ totalProbability(campaign) }}/10000 bps</p>
                </td>
                <td class="align-top"><span :class="['badge', stateClass(campaign.state)]">{{ t(`lottery.state.${campaign.state}`) }}</span></td>
                <td class="min-w-72 align-top">
                  <div class="flex flex-wrap justify-end gap-1">
                    <button type="button" class="icon-btn" :title="t('lottery.admin.entries')" @click="openEntries(campaign)"><Icon name="users" size="sm" /></button>
                    <button type="button" class="icon-btn" :title="t('lottery.admin.edit')" :disabled="campaign.entry_count > 0" @click="openEdit(campaign)"><Icon name="edit" size="sm" /></button>
                    <button v-if="campaign.mode === 'scheduled' && campaign.status === 'active' && drawReady(campaign)" type="button" class="btn btn-secondary btn-sm" :disabled="drawingID !== null" @click="pendingDraw = campaign"><Icon name="sparkles" size="sm" />{{ t('lottery.admin.draw') }}</button>
                    <button v-if="campaign.status === 'active' && !campaign.full_draw_reached_at" type="button" class="btn btn-secondary btn-sm" :disabled="statusUpdatingID !== null" @click="updateStatus(campaign, 'paused')">{{ t('lottery.admin.pause') }}</button>
                    <button v-else-if="campaign.status !== 'active' && campaign.status !== 'completed'" type="button" class="btn btn-secondary btn-sm" :disabled="statusUpdatingID !== null" @click="updateStatus(campaign, 'active')">{{ t('lottery.admin.activate') }}</button>
                    <button v-if="campaign.mode === 'instant' && campaign.status !== 'completed'" type="button" class="icon-btn" :title="t('lottery.admin.complete')" :disabled="statusUpdatingID !== null" @click="updateStatus(campaign, 'completed')"><Icon name="checkCircle" size="sm" /></button>
                    <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('lottery.admin.delete')" :disabled="campaign.entry_count > 0" @click="pendingDelete = campaign"><Icon name="trash" size="sm" /></button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <template #pagination>
        <Pagination v-if="pagination.total > 0" :page="pagination.page" :page-size="pagination.page_size" :total="pagination.total" @update:page="changePage" @update:page-size="changePageSize" />
      </template>
    </TablePageLayout>

    <LotteryCampaignEditorDialog :show="editorOpen" :campaign="editingCampaign" :saving="saving" :subscription-groups="subscriptionGroups" @close="closeEditor" @save="saveCampaign" />
    <LotteryEntriesDialog :show="entryCampaign !== null" :campaign="entryCampaign" :entries="entries" :loading="entriesLoading" :error="entriesError" :page="entryPagination.page" :page-size="entryPagination.page_size" :total="entryPagination.total" :drawing="drawingID !== null" @close="closeEntries" @refresh="loadEntries" @page="changeEntryPage" @draw="entryCampaign && (pendingDraw = entryCampaign)" />
    <ConfirmDialog :show="pendingDelete !== null" :title="t('lottery.admin.deleteTitle')" :message="t('lottery.admin.deleteMessage', { name: pendingDelete?.name || '' })" danger @cancel="pendingDelete = null" @confirm="confirmDelete" />
    <ConfirmDialog :show="pendingDraw !== null" :title="t('lottery.admin.drawConfirmTitle')" :message="t('lottery.admin.drawConfirmMessage', { name: pendingDraw?.name || '' })" @cancel="pendingDraw = null" @confirm="confirmDraw" />
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import LotteryCampaignEditorDialog from '@/components/lottery/LotteryCampaignEditorDialog.vue'
import LotteryEntriesDialog from '@/components/lottery/LotteryEntriesDialog.vue'
import lotteryAPI, { type LotteryCampaign, type LotteryCampaignInput, type LotteryCampaignStatus, type LotteryEntry } from '@/api/lottery'
import { adminAPI } from '@/api/admin'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const campaigns = ref<LotteryCampaign[]>([])
const subscriptionGroups = ref<AdminGroup[]>([])
const loading = ref(false)
const errorMessage = ref('')
const filters = reactive({ search: '', appliedSearch: '', mode: '', status: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const editorOpen = ref(false)
const editingCampaign = ref<LotteryCampaign | null>(null)
const saving = ref(false)
const statusUpdatingID = ref<number | null>(null)
const pendingDelete = ref<LotteryCampaign | null>(null)
const pendingDraw = ref<LotteryCampaign | null>(null)
const drawingID = ref<number | null>(null)
const entryCampaign = ref<LotteryCampaign | null>(null)
const entries = ref<LotteryEntry[]>([])
const entriesLoading = ref(false)
const entriesError = ref('')
const entryPagination = reactive({ page: 1, page_size: 20, total: 0 })
const campaignStatuses: LotteryCampaignStatus[] = ['draft', 'active', 'paused', 'completed']
let listController: AbortController | null = null
let entryController: AbortController | null = null

async function loadCampaigns() {
  listController?.abort()
  const controller = new AbortController()
  listController = controller
  loading.value = true
  errorMessage.value = ''
  try {
    const response = await lotteryAPI.admin.list(
      { page: pagination.page, page_size: pagination.page_size, search: filters.appliedSearch, mode: filters.mode, status: filters.status },
      controller.signal,
    )
    if (controller.signal.aborted || listController !== controller) return
    campaigns.value = response.items
    pagination.page = response.page
    pagination.page_size = response.page_size
    pagination.total = response.total
  } catch (error: unknown) {
    if (!controller.signal.aborted && listController === controller) {
      errorMessage.value = extractI18nErrorMessage(error, t, 'lottery.admin.errors', t('lottery.loadFailed'))
    }
  } finally {
    if (listController === controller) loading.value = false
  }
}
async function loadGroups() {
  try { subscriptionGroups.value = (await adminAPI.groups.getAll()).filter((group) => group.subscription_type === 'subscription') }
  catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'lottery.admin.errors', t('common.error'))) }
}
function applySearch() { filters.appliedSearch = filters.search.trim(); reloadFromFirstPage() }
function reloadFromFirstPage() { pagination.page = 1; void loadCampaigns() }
function changePage(page: number) { pagination.page = page; void loadCampaigns() }
function changePageSize(pageSize: number) { pagination.page = 1; pagination.page_size = pageSize; void loadCampaigns() }
function openCreate() { editingCampaign.value = null; editorOpen.value = true }
function openEdit(campaign: LotteryCampaign) { if (campaign.entry_count === 0) { editingCampaign.value = campaign; editorOpen.value = true } }
function closeEditor() { if (!saving.value) editorOpen.value = false }

async function saveCampaign(input: LotteryCampaignInput) {
  saving.value = true
  try {
    if (editingCampaign.value) await lotteryAPI.admin.update(editingCampaign.value.id, input)
    else await lotteryAPI.admin.create(input)
    editorOpen.value = false; appStore.showSuccess(t('lottery.admin.saved')); await loadCampaigns()
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'lottery.admin.errors', t('common.error'))) }
  finally { saving.value = false }
}
async function updateStatus(campaign: LotteryCampaign, status: LotteryCampaignStatus) {
  statusUpdatingID.value = campaign.id
  try { await lotteryAPI.admin.setStatus(campaign.id, status); appStore.showSuccess(t('lottery.admin.statusUpdated')); await loadCampaigns() }
  catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'lottery.admin.errors', t('common.error'))) }
  finally { statusUpdatingID.value = null }
}
async function confirmDelete() {
  const campaign = pendingDelete.value; if (!campaign) return
  pendingDelete.value = null
  try { await lotteryAPI.admin.delete(campaign.id); appStore.showSuccess(t('lottery.admin.deleted')); await loadCampaigns() }
  catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'lottery.admin.errors', t('common.error'))) }
}
async function confirmDraw() {
  const campaign = pendingDraw.value; if (!campaign) return
  pendingDraw.value = null; drawingID.value = campaign.id
  try {
    const result = await lotteryAPI.admin.draw(campaign.id)
    appStore.showSuccess(t('lottery.admin.drawResult', { participants: result.participant_count, winners: result.winner_count }))
    await loadCampaigns(); if (entryCampaign.value?.id === campaign.id) await loadEntries()
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'lottery.admin.errors', t('common.error'))) }
  finally { drawingID.value = null }
}
function openEntries(campaign: LotteryCampaign) { entryCampaign.value = campaign; entryPagination.page = 1; void loadEntries() }
function closeEntries() { entryController?.abort(); entryCampaign.value = null; entries.value = []; entriesError.value = '' }
async function loadEntries() {
  const campaign = entryCampaign.value; if (!campaign) return
  entryController?.abort()
  const controller = new AbortController()
  entryController = controller
  entriesLoading.value = true
  entriesError.value = ''
  try {
    const response = await lotteryAPI.admin.entries(
      campaign.id,
      { page: entryPagination.page, page_size: entryPagination.page_size },
      controller.signal,
    )
    if (controller.signal.aborted || entryController !== controller) return
    entries.value = response.items
    entryPagination.page = response.page
    entryPagination.page_size = response.page_size
    entryPagination.total = response.total
  } catch (error: unknown) {
    if (!controller.signal.aborted && entryController === controller) {
      entriesError.value = extractI18nErrorMessage(error, t, 'lottery.admin.errors', t('common.error'))
    }
  } finally {
    if (entryController === controller) entriesLoading.value = false
  }
}
function changeEntryPage(page: number) { entryPagination.page = page; void loadEntries() }
function totalProbability(campaign: LotteryCampaign) { return campaign.prizes.reduce((sum, prize) => sum + (prize.is_enabled ? prize.probability_bps : 0), 0) }
function drawReady(campaign: LotteryCampaign) {
  return Boolean(campaign.full_draw_reached_at || (campaign.draw_at && Date.now() >= new Date(campaign.draw_at).getTime()))
}
function stateClass(state: LotteryCampaign['state']) {
  if (state === 'active') return 'badge-success'
  if (state === 'not_started' || state === 'awaiting_draw') return 'badge-info'
  if (state === 'paused') return 'badge-warning'
  return 'badge-gray'
}

onMounted(() => { void Promise.all([loadCampaigns(), loadGroups()]) })
onBeforeUnmount(() => { listController?.abort(); entryController?.abort() })
</script>
