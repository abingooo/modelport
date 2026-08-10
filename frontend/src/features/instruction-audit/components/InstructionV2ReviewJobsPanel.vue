<template>
  <section class="min-w-0 space-y-4" data-test="instruction-v2-review-jobs">
    <div>
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.reviewJobs') }}</h2>
      <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.reviewJobsHint') }}</p>
    </div>

    <div class="grid min-w-0 gap-3 rounded-md border-y border-gray-200 py-4 dark:border-dark-700 sm:grid-cols-[minmax(220px,1fr)_180px_auto]">
      <label class="min-w-0">
        <span class="input-label">{{ t('common.search') }}</span>
        <div class="flex min-w-0">
          <input v-model.trim="query" type="search" class="input min-w-0 rounded-r-none" :placeholder="t('admin.instructionAudit.v2.searchReviewJobs')" @keyup.enter="applyFilters" />
          <button type="button" class="btn btn-primary rounded-l-none px-3" :title="t('common.search')" @click="applyFilters"><Icon name="search" size="sm" /></button>
        </div>
      </label>
      <label>
        <span class="input-label">{{ t('common.status') }}</span>
        <select v-model="status" class="input" @change="applyFilters"><option value="">{{ t('common.all') }}</option><option v-for="value in jobStatuses" :key="value" :value="value">{{ jobStatusLabel(value) }}</option></select>
      </label>
      <div class="flex items-end"><button type="button" class="btn btn-secondary" :disabled="loading" @click="loadJobs"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('common.refresh') }}</button></div>
    </div>

    <div v-if="error" role="alert" class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">{{ error }}</div>

    <div v-if="loading && !page.items.length" class="job-grid"><div v-for="index in 6" :key="index" class="h-72 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" /></div>
    <div v-else-if="page.items.length" class="job-grid">
      <article v-for="job in page.items" :key="job.id" class="job-card">
        <header class="flex min-w-0 items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <button type="button" class="font-semibold text-primary-700 hover:underline dark:text-primary-300" @click="openDetail(job)">#{{ job.id }}</button>
              <span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="jobStatusPill(job.status)">{{ jobStatusLabel(job.status) }}</span>
              <span v-if="job.observe_only" class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] text-amber-800 dark:bg-amber-950/50 dark:text-amber-200">{{ t('admin.instructionAudit.v2.observeTask') }}</span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ formatAuditDate(job.created_at) }}</p>
          </div>
          <button type="button" class="icon-btn shrink-0" :title="t('admin.instructionAudit.v2.reviewJobDetail')" @click="openDetail(job)"><Icon name="eye" size="sm" /></button>
        </header>

        <div class="min-w-0 rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-800/70">
          <div class="flex min-w-0 items-center justify-between gap-2"><span class="job-label">SHA-256</span><button type="button" class="icon-btn h-6 w-6" :title="t('common.copy')" @click="copyToClipboard(job.sha256)"><Icon name="copy" size="xs" /></button></div>
          <p class="mt-1 break-all font-mono text-[11px] text-gray-700 dark:text-gray-200">{{ job.sha256 }}</p>
        </div>

        <dl class="grid min-w-0 grid-cols-2 gap-3">
          <div><dt class="job-label">{{ t('admin.instructionAudit.v2.selectedField') }}</dt><dd class="mt-1 font-mono text-xs text-gray-800 dark:text-gray-200">{{ fieldLabel(job.selected_field) }}</dd></div>
          <div><dt class="job-label">{{ t('admin.instructionAudit.v2.finalResult') }}</dt><dd class="mt-1 text-sm font-semibold" :class="job.final_result === 'reject' ? 'text-red-600 dark:text-red-300' : job.final_result === 'pass' ? 'text-primary-700 dark:text-primary-300' : 'text-gray-500'">{{ finalResultLabel(job.final_result) }}</dd></div>
          <div><dt class="job-label">{{ t('admin.instructionAudit.v2.passVotes') }}</dt><dd class="mt-1 text-sm tabular-nums text-primary-700 dark:text-primary-300">{{ job.pass_votes }} / 3</dd></div>
          <div><dt class="job-label">{{ t('admin.instructionAudit.v2.rejectVotes') }}</dt><dd class="mt-1 text-sm tabular-nums text-red-600 dark:text-red-300">{{ job.reject_votes }} / 3</dd></div>
          <div><dt class="job-label">{{ t('admin.instructionAudit.v2.retryRound') }}</dt><dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ job.retry_round }}</dd></div>
          <div><dt class="job-label">{{ t('admin.instructionAudit.v2.contentSize') }}</dt><dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ formatAuditBytes(job.content_bytes) }}</dd></div>
        </dl>

        <p v-if="job.last_error" class="break-words rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-200">{{ job.last_error }}</p>

        <footer class="mt-auto flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
          <router-link v-if="job.source_event_id" :to="{ query: { tab: 'events', event_id: job.source_event_id } }" class="btn btn-ghost btn-sm"><Icon name="link" size="sm" />{{ t('admin.instructionAudit.v2.eventId') }} #{{ job.source_event_id }}</router-link>
          <span v-else />
          <button v-if="job.status === 'failed'" type="button" class="btn btn-primary btn-sm" :disabled="retryingId === job.id" @click="retryJob(job)"><Icon name="refresh" size="sm" :class="{ 'animate-spin': retryingId === job.id }" />{{ t('admin.instructionAudit.v2.retryNow') }}</button>
        </footer>
      </article>
    </div>
    <div v-else class="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-gray-200 px-6 text-center dark:border-dark-600">
      <Icon name="clock" size="xl" class="text-gray-300 dark:text-dark-500" />
      <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.v2.noReviewJobs') }}</p>
      <p class="mt-1 max-w-lg text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.noReviewJobsHint') }}</p>
    </div>

    <Pagination v-if="page.total > 0" :total="page.total" :page="page.page" :page-size="page.page_size" @update:page="changePage" @update:page-size="changePageSize" />

    <BaseDialog :show="Boolean(detailJob)" :title="t('admin.instructionAudit.v2.reviewJobTitle', { id: detailJob?.id ?? '-' })" width="extra-wide" @close="closeDetail">
      <div v-if="detailLoading" class="flex min-h-56 items-center justify-center"><Icon name="refresh" size="md" class="animate-spin text-primary-500" /></div>
      <div v-else-if="detailJob" class="min-w-0 space-y-5">
        <section class="grid min-w-0 gap-3 rounded-md border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-600 dark:bg-dark-800/60 sm:grid-cols-2 lg:grid-cols-4">
          <div><p class="job-label">{{ t('common.status') }}</p><p class="mt-1 text-sm font-semibold">{{ jobStatusLabel(detailJob.status) }}</p></div>
          <div><p class="job-label">{{ t('admin.instructionAudit.v2.selectedField') }}</p><p class="mt-1 font-mono text-sm">{{ fieldLabel(detailJob.selected_field) }}</p></div>
          <div><p class="job-label">{{ t('admin.instructionAudit.v2.voteSummary') }}</p><p class="mt-1 text-sm"><span class="text-primary-700 dark:text-primary-300">{{ detailJob.pass_votes }} {{ t('admin.instructionAudit.v2.pass') }}</span> · <span class="text-red-600 dark:text-red-300">{{ detailJob.reject_votes }} {{ t('admin.instructionAudit.v2.reject') }}</span></p></div>
          <div><p class="job-label">{{ t('admin.instructionAudit.v2.configVersion') }}</p><p class="mt-1 text-sm">v{{ detailJob.config_version }}</p></div>
        </section>

        <section class="min-w-0">
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2"><h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.encryptedEvidence') }}</h3><button v-if="!rawReview" type="button" class="btn btn-secondary btn-sm" :disabled="rawLoading" @click="revealRaw"><Icon name="eye" size="sm" />{{ t('admin.instructionAudit.v2.viewRaw') }}</button><button v-else type="button" class="btn btn-secondary btn-sm" @click="copyRaw"><Icon name="copy" size="sm" />{{ t('common.copy') }}</button></div>
          <div v-if="rawLoading" class="flex min-h-32 items-center justify-center"><Icon name="refresh" size="md" class="animate-spin text-primary-500" /></div>
          <pre v-else-if="rawReview?.fields[0]" class="max-h-72 min-h-40 overflow-auto whitespace-pre-wrap break-words rounded-md bg-gray-950 p-4 text-xs leading-5 text-gray-100">{{ rawReview.fields[0].plaintext }}</pre>
          <p v-else class="rounded-md border border-dashed border-gray-200 px-4 py-8 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('admin.instructionAudit.v2.rawNotLoaded') }}</p>
        </section>

        <section class="space-y-2">
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.asyncAttempts') }}</h3>
          <div v-if="detailJob.attempts?.length" class="attempt-grid">
            <article v-for="attempt in detailJob.attempts" :key="attempt.id" class="min-w-0 rounded-md border border-gray-200 p-3 dark:border-dark-600">
              <div class="flex flex-wrap items-center justify-between gap-2"><span class="font-mono text-xs font-semibold text-gray-800 dark:text-gray-200">{{ attempt.node_slot }} · #{{ attempt.attempt_no }}</span><span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="attempt.result === 'pass' ? 'bg-primary-100 text-primary-700 dark:bg-primary-950/50 dark:text-primary-200' : attempt.result === 'reject' ? 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300' : 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'">{{ aiResultLabel(t, attempt.result) }}</span></div>
              <p class="mt-2 break-words text-sm text-gray-700 dark:text-gray-200">{{ attempt.reason || '-' }}</p>
              <p class="mt-2 break-all text-[11px] text-gray-400">{{ attempt.node_name }} · {{ attempt.reviewer_model }} · {{ Math.round(attempt.confidence * 100) }}% · {{ attempt.latency_ms }} ms</p>
            </article>
          </div>
          <p v-else class="rounded-md border border-dashed border-gray-200 px-4 py-8 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('admin.instructionAudit.v2.noAttempts') }}</p>
        </section>
      </div>
      <template #footer><button type="button" class="btn btn-secondary" @click="closeDetail">{{ t('common.close') }}</button></template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import instructionAuditV2API from '../v2Api'
import type { InstructionEvidenceReview, InstructionReviewJob, InstructionReviewJobPage, InstructionReviewJobStatus } from '../v2Types'
import { aiResultLabel, formatAuditBytes, formatAuditDate } from '../v2Presentation'

const props = defineProps<{ refreshKey: number }>()
const emit = defineEmits<{ (event: 'changed'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const page = reactive<InstructionReviewJobPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const jobStatuses: InstructionReviewJobStatus[] = ['pending', 'processing', 'retry', 'completed', 'failed']
const loading = ref(false)
const error = ref('')
const query = ref('')
const status = ref('')
const retryingId = ref(0)
const detailJob = ref<InstructionReviewJob | null>(null)
const detailLoading = ref(false)
const rawReview = ref<InstructionEvidenceReview | null>(null)
const rawLoading = ref(false)

onMounted(loadJobs)
watch(() => props.refreshKey, loadJobs)

async function loadJobs() {
  loading.value = true
  error.value = ''
  try {
    Object.assign(page, await instructionAuditV2API.listReviewJobs({ page: page.page, page_size: page.page_size, status: status.value || undefined, q: query.value || undefined }))
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, t('common.error'))
  } finally {
    loading.value = false
  }
}

function applyFilters() { page.page = 1; loadJobs() }
function changePage(value: number) { page.page = value; loadJobs() }
function changePageSize(value: number) { page.page_size = value; page.page = 1; loadJobs() }
function fieldLabel(value: string) { return value === 'input1' ? 'input[1]' : value }
function jobStatusLabel(value: string) { return t(`admin.instructionAudit.v2.reviewJobStatuses.${value}`) }
function finalResultLabel(value: string) { return value ? t(`admin.instructionAudit.v2.finalResults.${value}`) : t('admin.instructionAudit.v2.pendingResult') }
function jobStatusPill(value: string) {
  if (value === 'completed') return 'bg-primary-100 text-primary-700 dark:bg-primary-950/50 dark:text-primary-200'
  if (value === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300'
  return 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'
}

async function openDetail(job: InstructionReviewJob) {
  detailJob.value = job
  detailLoading.value = true
  rawReview.value = null
  try {
    detailJob.value = await instructionAuditV2API.getReviewJob(job.id)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
    closeDetail()
  } finally {
    detailLoading.value = false
  }
}

async function retryJob(job: InstructionReviewJob) {
  retryingId.value = job.id
  try {
    await instructionAuditV2API.retryReviewJob(job.id)
    appStore.showSuccess(t('admin.instructionAudit.v2.reviewQueued'))
    emit('changed')
    await loadJobs()
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    retryingId.value = 0
  }
}

async function revealRaw() {
  if (!detailJob.value) return
  rawLoading.value = true
  try {
    rawReview.value = await instructionAuditV2API.revealReviewJobRaw(detailJob.value.id)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    rawLoading.value = false
  }
}

async function copyRaw() {
  const field = rawReview.value?.fields[0]
  if (!detailJob.value || !field?.plaintext) return
  try {
    await instructionAuditV2API.recordReviewJobRawCopy(detailJob.value.id)
    await copyToClipboard(field.plaintext)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

function closeDetail() { detailJob.value = null; rawReview.value = null }
</script>

<style scoped>
.job-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 360px), 1fr));
  gap: 0.875rem;
}

.job-card {
  @apply flex min-w-0 flex-col gap-3 rounded-md border border-gray-200 bg-white p-4 shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-800;
}

.attempt-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 300px), 1fr));
  gap: 0.75rem;
}

.job-label {
  @apply text-[11px] font-semibold uppercase text-gray-400;
}
</style>
