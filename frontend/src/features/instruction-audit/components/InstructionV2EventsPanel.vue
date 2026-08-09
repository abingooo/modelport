<template>
  <section class="min-w-0 space-y-4" data-test="instruction-v2-events">
    <div class="grid min-w-0 gap-3 rounded-md border-y border-gray-200 bg-white py-4 dark:border-dark-700 dark:bg-dark-800 md:grid-cols-2 xl:grid-cols-[minmax(240px,1.4fr)_repeat(4,minmax(130px,0.7fr))_auto]">
      <label class="min-w-0 px-1">
        <span class="input-label">{{ t('admin.instructionAudit.v2.searchEvents') }}</span>
        <div class="flex min-w-0">
          <input v-model.trim="filters.q" type="search" class="input min-w-0 rounded-r-none" :placeholder="t('admin.instructionAudit.v2.searchEventsHint')" @keyup.enter="applyFilters" />
          <button type="button" class="btn btn-primary rounded-l-none px-3" :title="t('common.search')" @click="applyFilters">
            <Icon name="search" size="sm" />
          </button>
        </div>
      </label>
      <label class="min-w-0 px-1">
        <span class="input-label">{{ t('admin.instructionAudit.v2.timeRange') }}</span>
        <select v-model="filters.range" class="input" @change="applyFilters">
          <option value="1h">{{ t('admin.instructionAudit.v2.lastHour') }}</option>
          <option value="24h">{{ t('admin.instructionAudit.v2.lastDay') }}</option>
          <option value="7d">{{ t('admin.instructionAudit.v2.lastWeek') }}</option>
          <option value="30d">{{ t('admin.instructionAudit.v2.lastMonth') }}</option>
          <option value="all">{{ t('common.all') }}</option>
        </select>
      </label>
      <label class="min-w-0 px-1">
        <span class="input-label">{{ t('admin.instructionAudit.v2.group') }}</span>
        <select v-model="filters.groupId" class="input" @change="applyFilters">
          <option :value="0">{{ t('common.all') }}</option>
          <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option>
        </select>
      </label>
      <label class="min-w-0 px-1">
        <span class="input-label">{{ t('admin.instructionAudit.v2.client') }}</span>
        <select v-model="filters.clientKey" class="input" @change="applyFilters">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="client in clients" :key="client.profile_key" :value="client.profile_key">{{ client.name }}</option>
        </select>
      </label>
      <label class="min-w-0 px-1">
        <span class="input-label">{{ t('admin.instructionAudit.v2.outcome') }}</span>
        <select v-model="filters.outcome" class="input" @change="applyFilters">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="outcome in outcomeOptions" :key="outcome" :value="outcome">{{ outcomeLabel(t, outcome) }}</option>
        </select>
      </label>
      <div class="flex items-end gap-2 px-1">
        <button type="button" class="btn btn-secondary flex-1" @click="advancedOpen = !advancedOpen">
          <Icon name="filter" size="sm" />
          {{ t('admin.instructionAudit.v2.moreFilters') }}
        </button>
        <button type="button" class="icon-btn h-10 w-10 shrink-0" :title="t('admin.instructionAudit.v2.resetFilters')" @click="resetFilters">
          <Icon name="x" size="sm" />
        </button>
      </div>
    </div>

    <div v-if="advancedOpen" class="grid min-w-0 gap-3 rounded-md border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-600 dark:bg-dark-800/50 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
      <label class="min-w-0">
        <span class="input-label">{{ t('admin.instructionAudit.v2.eventId') }}</span>
        <input v-model.number="filters.eventId" type="number" min="1" class="input" @keyup.enter="applyFilters" />
      </label>
      <label class="min-w-0">
        <span class="input-label">{{ t('admin.instructionAudit.v2.userId') }}</span>
        <input v-model.number="filters.userId" type="number" min="1" class="input" @keyup.enter="applyFilters" />
      </label>
      <label class="min-w-0">
        <span class="input-label">{{ t('admin.instructionAudit.v2.model') }}</span>
        <input v-model.trim="filters.model" class="input" @keyup.enter="applyFilters" />
      </label>
      <label class="min-w-0">
        <span class="input-label">{{ t('admin.instructionAudit.v2.reason') }}</span>
        <select v-model="filters.reason" class="input" @change="applyFilters">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="reason in reasonOptions" :key="reason" :value="reason">{{ reasonLabel(t, reason) }}</option>
        </select>
      </label>
      <label class="min-w-0">
        <span class="input-label">{{ t('admin.instructionAudit.v2.aiReview') }}</span>
        <select v-model="filters.aiResult" class="input" @change="applyFilters">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="result in aiResultOptions" :key="result" :value="result">{{ aiResultLabel(t, result) }}</option>
        </select>
      </label>
      <div class="flex items-end">
        <button type="button" class="btn btn-primary w-full" @click="applyFilters">
          <Icon name="check" size="sm" />
          {{ t('common.confirm') }}
        </button>
      </div>
    </div>

    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.auditLogs') }}</h2>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.eventCount', { count: page.total }) }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedIds.length || deleting" @click="confirmBatchDelete = true">
          <Icon name="trash" size="sm" />
          {{ t('admin.instructionAudit.v2.deleteSelected', { count: selectedIds.length }) }}
        </button>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadEvents">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <div v-if="error" role="alert" class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </div>

    <div v-if="loading && !page.items.length" class="grid min-w-0 gap-3 lg:grid-cols-2 2xl:grid-cols-3">
      <div v-for="index in 6" :key="index" class="h-72 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>

    <div v-else-if="page.items.length" class="audit-event-grid">
      <article v-for="event in page.items" :key="event.id" class="audit-event-card">
        <header class="flex min-w-0 items-start justify-between gap-3">
          <div class="flex min-w-0 items-start gap-3">
            <input type="checkbox" class="mt-1 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :checked="selectedIds.includes(event.id)" :aria-label="`#${event.id}`" @change="toggleSelection(event.id)" />
            <div class="min-w-0">
              <div class="flex min-w-0 flex-wrap items-center gap-2">
                <button type="button" class="font-semibold text-primary-700 hover:underline dark:text-primary-300" @click="filterEvent(event.id)">#{{ event.id }}</button>
                <span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="outcomePill(event.outcome)">{{ outcomeLabel(t, event.outcome) }}</span>
                <span class="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ modeLabel(t, event.mode) }}</span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ formatAuditDate(event.created_at) }}</p>
            </div>
          </div>
          <div class="flex shrink-0 items-center gap-1">
            <button type="button" class="icon-btn" :title="t('admin.instructionAudit.v2.reviewEvidence')" @click="openEvidence(event)">
              <Icon :name="sensitiveAccess ? 'eye' : 'lock'" size="sm" />
            </button>
            <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" @click="eventToDelete = event">
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </header>

        <div class="min-w-0 rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-800/70">
          <div class="flex min-w-0 items-center justify-between gap-2">
            <span class="audit-card-label">{{ t('admin.instructionAudit.v2.requestId') }}</span>
            <button type="button" class="icon-btn h-6 w-6" :disabled="!event.request_id" :title="t('common.copy')" @click="copyToClipboard(event.request_id)"><Icon name="copy" size="xs" /></button>
          </div>
          <p class="mt-1 min-w-0 break-all font-mono text-[11px] text-gray-600 dark:text-gray-300">{{ event.request_id || '-' }}</p>
        </div>

        <dl class="grid min-w-0 grid-cols-2 gap-x-4 gap-y-3">
          <div class="min-w-0">
            <dt class="audit-card-label">{{ t('admin.instructionAudit.v2.userAndKey') }}</dt>
            <dd class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white" :title="event.user_email">{{ event.user_email || '-' }}</dd>
            <dd class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400" :title="event.api_key_name">{{ event.api_key_name || '-' }}</dd>
          </div>
          <div class="min-w-0">
            <dt class="audit-card-label">{{ t('admin.instructionAudit.v2.groupAndClient') }}</dt>
            <dd class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white" :title="event.group_name">{{ event.group_name || '-' }}</dd>
            <dd class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400" :title="event.client_user_agent">{{ event.client_name || event.client_key }}</dd>
          </div>
          <div class="min-w-0">
            <dt class="audit-card-label">{{ t('admin.instructionAudit.v2.model') }}</dt>
            <dd class="mt-1 truncate font-mono text-xs text-gray-800 dark:text-gray-200" :title="event.model">{{ event.model || '-' }}</dd>
          </div>
          <div class="min-w-0">
            <dt class="audit-card-label">{{ t('admin.instructionAudit.v2.reason') }}</dt>
            <dd class="mt-1 break-words text-xs text-gray-800 dark:text-gray-200">{{ reasonLabel(t, event.reason) }}</dd>
          </div>
        </dl>

        <div class="grid min-w-0 gap-2 sm:grid-cols-2">
          <div v-for="field in eventFields(event)" :key="field.name" class="min-w-0 rounded-md border border-gray-200 px-3 py-2 dark:border-dark-600">
            <div class="flex min-w-0 items-center justify-between gap-2">
              <span class="font-mono text-xs font-semibold text-gray-800 dark:text-gray-200">{{ field.label }}</span>
              <span class="text-[11px] text-gray-500 dark:text-gray-400">{{ fieldStateLabel(t, field.value.state) }}</span>
            </div>
            <div class="mt-1 flex min-w-0 items-center justify-between gap-2">
              <span class="min-w-0 truncate font-mono text-[10px] text-gray-400" :title="field.value.sha256">{{ compactDigest(field.value.sha256) }}</span>
              <span class="shrink-0 text-[10px] tabular-nums text-gray-400">{{ formatAuditBytes(field.value.bytes) }}</span>
            </div>
          </div>
        </div>

        <footer class="mt-auto flex min-w-0 flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
          <div class="min-w-0 text-[11px] text-gray-500 dark:text-gray-400">
            <span>{{ t('admin.instructionAudit.v2.aiShort') }}: {{ aiResultLabel(t, event.ai_result) }}</span>
            <span class="mx-1.5">·</span>
            <span>{{ event.audit_latency_ms }} ms</span>
            <span class="mx-1.5">·</span>
            <span>{{ formatAuditBytes(event.body_bytes) }}</span>
          </div>
          <div class="flex items-center gap-1.5">
            <router-link :to="opsLogLink(event)" class="icon-btn h-8 w-8" :title="t('admin.instructionAudit.v2.relatedSystemLog')">
              <Icon name="link" size="sm" />
            </router-link>
            <button type="button" class="btn btn-primary btn-sm" :disabled="!sensitiveAccess || !trustableFields(event).length" @click="eventToTrust = event">
              <Icon name="plus" size="sm" />
              {{ t('admin.instructionAudit.v2.quickTrust') }}
            </button>
          </div>
        </footer>
      </article>
    </div>

    <div v-else class="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-gray-200 px-6 text-center dark:border-dark-600">
      <Icon name="inbox" size="xl" class="text-gray-300 dark:text-dark-500" />
      <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.v2.noEvents') }}</p>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.noEventsHint') }}</p>
    </div>

    <Pagination v-if="page.total > 0" :total="page.total" :page="page.page" :page-size="page.page_size" @update:page="changePage" @update:page-size="changePageSize" />

    <InstructionV2EvidenceDialog :show="Boolean(evidenceEvent)" :event="evidenceEvent" :sensitive-access="sensitiveAccess" @close="evidenceEvent = null" @trusted="handleTrusted" />

    <ConfirmDialog :show="Boolean(eventToDelete)" :title="t('admin.instructionAudit.v2.deleteEventTitle')" :message="t('admin.instructionAudit.v2.deleteEventConfirm', { id: eventToDelete?.id })" danger @confirm="deleteSingle" @cancel="eventToDelete = null" />
    <ConfirmDialog :show="confirmBatchDelete" :title="t('admin.instructionAudit.v2.deleteEventsTitle')" :message="t('admin.instructionAudit.v2.deleteEventsConfirm', { count: selectedIds.length })" danger @confirm="deleteBatch" @cancel="confirmBatchDelete = false" />
    <ConfirmDialog :show="Boolean(eventToTrust)" :title="t('admin.instructionAudit.v2.quickTrust')" :message="t('admin.instructionAudit.v2.quickTrustConfirm', { id: eventToTrust?.id, count: eventToTrust ? trustableFields(eventToTrust).length : 0 })" @confirm="quickTrust" @cancel="eventToTrust = null" />
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import instructionAuditV2API from '../v2Api'
import InstructionV2EvidenceDialog from './InstructionV2EvidenceDialog.vue'
import type {
  InstructionClientProfile,
  InstructionEvent,
  InstructionEventFilters,
  InstructionEventOutcome,
  InstructionEventPage,
  InstructionField,
  InstructionGroupOption,
} from '../v2Types'
import {
  aiResultLabel,
  compactDigest,
  fieldStateLabel,
  formatAuditBytes,
  formatAuditDate,
  modeLabel,
  outcomeLabel,
  outcomePill,
  reasonLabel,
  instructionEventReasonOptions,
} from '../v2Presentation'

const props = defineProps<{
  groups: InstructionGroupOption[]
  clients: InstructionClientProfile[]
  sensitiveAccess: boolean
  refreshKey: number
}>()

const emit = defineEmits<{
  (event: 'filters-change', filters: InstructionEventFilters): void
  (event: 'trusted'): void
}>()

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const stepUp = useStepUp()
const page = reactive<InstructionEventPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const loading = ref(false)
const deleting = ref(false)
const error = ref('')
const advancedOpen = ref(false)
const selectedIds = ref<number[]>([])
const evidenceEvent = ref<InstructionEvent | null>(null)
const eventToDelete = ref<InstructionEvent | null>(null)
const eventToTrust = ref<InstructionEvent | null>(null)
const confirmBatchDelete = ref(false)

const filters = reactive({
  q: '',
  eventId: Number(route.query.event_id) || 0,
  userId: 0,
  groupId: 0,
  clientKey: '',
  outcome: '',
  reason: '',
  aiResult: '',
  model: '',
  range: '24h',
})

const outcomeOptions: InstructionEventOutcome[] = ['blocked', 'hash_pass', 'ai_pass', 'observe_allow', 'empty_pass', 'user_allowlist_pass']
const reasonOptions = instructionEventReasonOptions
const aiResultOptions = ['not_run', 'pass', 'reject', 'uncertain', 'error', 'queue_full']

onMounted(loadEvents)
watch(() => props.refreshKey, loadEvents)

function requestFilters(): InstructionEventFilters {
  const result: InstructionEventFilters = {}
  if (filters.q) result.q = filters.q
  if (filters.eventId > 0) result.event_id = filters.eventId
  if (filters.userId > 0) result.user_id = filters.userId
  if (filters.groupId > 0) result.group_ids = String(filters.groupId)
  if (filters.clientKey) result.client_keys = filters.clientKey
  if (filters.outcome) result.outcomes = filters.outcome
  if (filters.reason) result.reasons = filters.reason
  if (filters.aiResult) result.ai_results = filters.aiResult
  if (filters.model) result.model = filters.model
  if (filters.range !== 'all') {
    const hours = filters.range === '1h' ? 1 : filters.range === '24h' ? 24 : filters.range === '7d' ? 168 : 720
    result.from = new Date(Date.now() - hours * 60 * 60 * 1000).toISOString()
  }
  return result
}

async function loadEvents() {
  loading.value = true
  error.value = ''
  const activeFilters = requestFilters()
  emit('filters-change', activeFilters)
  try {
    const next = await instructionAuditV2API.listEvents({ ...activeFilters, page: page.page, page_size: page.page_size })
    Object.assign(page, next)
    selectedIds.value = selectedIds.value.filter((id) => next.items.some((item) => item.id === id))
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, t('common.error'))
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.page = 1
  loadEvents()
}

function resetFilters() {
  Object.assign(filters, { q: '', eventId: 0, userId: 0, groupId: 0, clientKey: '', outcome: '', reason: '', aiResult: '', model: '', range: '24h' })
  advancedOpen.value = false
  applyFilters()
}

function filterEvent(id: number) {
  filters.eventId = id
  advancedOpen.value = true
  applyFilters()
}

function changePage(value: number) {
  page.page = value
  loadEvents()
}

function changePageSize(value: number) {
  page.page_size = value
  page.page = 1
  loadEvents()
}

function toggleSelection(id: number) {
  selectedIds.value = selectedIds.value.includes(id)
    ? selectedIds.value.filter((item) => item !== id)
    : [...selectedIds.value, id]
}

function openEvidence(event: InstructionEvent) {
  evidenceEvent.value = event
}

function eventFields(event: InstructionEvent): Array<{ name: string; label: string; value: InstructionField }> {
  return [
    { name: 'instructions', label: 'instructions', value: event.instructions },
    { name: 'input1', label: 'input[1]', value: event.input1 },
  ]
}

function trustableFields(event: InstructionEvent): string[] {
  return eventFields(event).filter((field) => Boolean(field.value.sha256)).map((field) => field.name)
}

async function deleteSingle() {
  if (!eventToDelete.value) return
  deleting.value = true
  try {
    await stepUp.run(() => instructionAuditV2API.deleteEvent(eventToDelete.value!.id))
    appStore.showSuccess(t('admin.instructionAudit.v2.eventDeleted'))
    eventToDelete.value = null
    await loadEvents()
  } catch (caught) {
    if (!isStepUpCancelled(caught)) appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    deleting.value = false
  }
}

async function deleteBatch() {
  if (!selectedIds.value.length) return
  deleting.value = true
  try {
    const deleted = await stepUp.run(() => instructionAuditV2API.deleteEvents([...selectedIds.value]))
    appStore.showSuccess(t('admin.instructionAudit.v2.eventsDeleted', { count: deleted }))
    selectedIds.value = []
    confirmBatchDelete.value = false
    await loadEvents()
  } catch (caught) {
    if (!isStepUpCancelled(caught)) appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    deleting.value = false
  }
}

async function quickTrust() {
  if (!eventToTrust.value) return
  const event = eventToTrust.value
  try {
    await stepUp.run(() => instructionAuditV2API.trustEvent(event.id, trustableFields(event)))
    appStore.showSuccess(t('admin.instructionAudit.v2.trustCreated'))
    eventToTrust.value = null
    emit('trusted')
    await loadEvents()
  } catch (caught) {
    if (!isStepUpCancelled(caught)) appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

function handleTrusted() {
  evidenceEvent.value = null
  emit('trusted')
  loadEvents()
}

function opsLogLink(event: InstructionEvent) {
  return {
    path: '/admin/ops',
    query: { system_log_q: `\"event_id\": ${event.id}`, system_log_range: '30d' },
    hash: '#ops-system-logs',
  }
}
</script>

<style scoped>
.audit-event-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 390px), 1fr));
  gap: 0.875rem;
}

.audit-event-card {
  @apply flex min-w-0 flex-col gap-3 rounded-md border border-gray-200 bg-white p-4 shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-800;
}

.audit-card-label {
  @apply text-[11px] font-semibold uppercase text-gray-400;
}
</style>
