<template>
  <section class="min-w-0 space-y-4" data-test="instruction-v2-risk-hashes">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div class="min-w-0">
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.riskLibrary') }}</h2>
        <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.riskLibraryHint') }}</p>
      </div>
      <button type="button" class="btn btn-primary shrink-0" @click="openCreate"><Icon name="plus" size="sm" />{{ t('admin.instructionAudit.v2.addRisk') }}</button>
    </div>

    <div class="grid min-w-0 gap-3 rounded-md border-y border-gray-200 py-4 dark:border-dark-700 sm:grid-cols-[minmax(220px,1fr)_180px_auto]">
      <label class="min-w-0">
        <span class="input-label">{{ t('common.search') }}</span>
        <div class="flex min-w-0">
          <input v-model.trim="query" type="search" class="input min-w-0 rounded-r-none" :placeholder="t('admin.instructionAudit.v2.searchRisk')" @keyup.enter="applyFilters" />
          <button type="button" class="btn btn-primary rounded-l-none px-3" :title="t('common.search')" @click="applyFilters"><Icon name="search" size="sm" /></button>
        </div>
      </label>
      <label>
        <span class="input-label">{{ t('common.status') }}</span>
        <select v-model="status" class="input" @change="applyFilters">
          <option value="">{{ t('common.all') }}</option>
          <option value="active">{{ t('admin.instructionAudit.v2.riskStatuses.active') }}</option>
          <option value="disabled">{{ t('admin.instructionAudit.v2.riskStatuses.disabled') }}</option>
        </select>
      </label>
      <div class="flex items-end"><button type="button" class="btn btn-secondary" :disabled="loading" @click="loadItems"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('common.refresh') }}</button></div>
    </div>

    <div v-if="error" role="alert" class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">{{ error }}</div>

    <div v-if="loading && !page.items.length" class="risk-grid">
      <div v-for="index in 6" :key="index" class="h-72 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>
    <div v-else-if="page.items.length" class="risk-grid">
      <article v-for="item in page.items" :key="item.id" class="risk-card">
        <header class="flex min-w-0 items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-semibold text-red-700 dark:text-red-300">#{{ item.id }}</span>
              <span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="riskStatusPill(item.status)">{{ t(`admin.instructionAudit.v2.riskStatuses.${item.status}`) }}</span>
              <span class="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ t(`admin.instructionAudit.v2.riskReviewStatuses.${item.human_review_status}`) }}</span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ riskSourceLabel(item.source) }} · {{ formatAuditDate(item.created_at) }}</p>
          </div>
          <button type="button" class="icon-btn shrink-0 text-red-600 dark:text-red-400" :title="t('common.delete')" @click="requestAction(item, 'delete')"><Icon name="trash" size="sm" /></button>
        </header>

        <div class="min-w-0 rounded-md bg-red-50/60 px-3 py-2 dark:bg-red-950/20">
          <div class="flex min-w-0 items-center justify-between gap-2"><span class="risk-label">SHA-256</span><button type="button" class="icon-btn h-6 w-6" :title="t('common.copy')" @click="copyToClipboard(item.sha256)"><Icon name="copy" size="xs" /></button></div>
          <p class="mt-1 break-all font-mono text-[11px] text-gray-700 dark:text-gray-200">{{ item.sha256 }}</p>
        </div>

        <dl class="grid min-w-0 grid-cols-2 gap-3">
          <div><dt class="risk-label">{{ t('admin.instructionAudit.v2.observedField') }}</dt><dd class="mt-1 font-mono text-xs text-gray-800 dark:text-gray-200">{{ fieldLabel(item.observed_field) }}</dd></div>
          <div><dt class="risk-label">{{ t('admin.instructionAudit.v2.confidence') }}</dt><dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ item.confidence == null ? '-' : `${Math.round(item.confidence * 100)}%` }}</dd></div>
          <div class="col-span-2 min-w-0"><dt class="risk-label">{{ t('admin.instructionAudit.v2.reviewReason') }}</dt><dd class="mt-1 break-words text-sm text-gray-800 dark:text-gray-200">{{ item.review_reason || '-' }}</dd></div>
        </dl>

        <div v-if="item.source_event_id" class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.instructionAudit.v2.sourceEvent') }}:
          <router-link :to="{ query: { tab: 'events', event_id: item.source_event_id } }" class="font-semibold text-primary-700 hover:underline dark:text-primary-300">#{{ item.source_event_id }}</router-link>
        </div>

        <footer class="mt-auto flex flex-wrap gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
          <button type="button" class="btn btn-secondary btn-sm" @click="revealRaw(item)"><Icon name="eye" size="sm" />{{ t('admin.instructionAudit.v2.viewRaw') }}</button>
          <button v-if="item.status === 'active'" type="button" class="btn btn-secondary btn-sm" @click="requestAction(item, 'disable')"><Icon name="ban" size="sm" />{{ t('admin.instructionAudit.v2.disableAction') }}</button>
          <button v-else type="button" class="btn btn-secondary btn-sm" @click="requestAction(item, 'enable')"><Icon name="play" size="sm" />{{ t('admin.instructionAudit.v2.enableAction') }}</button>
          <button type="button" class="btn btn-secondary btn-sm" @click="requestAction(item, 'confirm_risk')"><Icon name="shield" size="sm" />{{ t('admin.instructionAudit.v2.confirmRisk') }}</button>
          <button type="button" class="btn btn-primary btn-sm" @click="requestAction(item, 'confirm_safe')"><Icon name="check" size="sm" />{{ t('admin.instructionAudit.v2.confirmSafe') }}</button>
        </footer>
      </article>
    </div>
    <div v-else class="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-gray-200 px-6 text-center dark:border-dark-600">
      <Icon name="shield" size="xl" class="text-gray-300 dark:text-dark-500" />
      <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.v2.noRisk') }}</p>
      <p class="mt-1 max-w-lg text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.noRiskHint') }}</p>
    </div>

    <Pagination v-if="page.total > 0" :total="page.total" :page="page.page" :page-size="page.page_size" @update:page="changePage" @update:page-size="changePageSize" />

    <BaseDialog :show="createForm.show" :title="t('admin.instructionAudit.v2.addRisk')" width="wide" @close="closeCreate">
      <form class="min-w-0 space-y-4" @submit.prevent="createRisk">
        <label class="block min-w-0"><span class="input-label">{{ t('admin.instructionAudit.v2.riskRaw') }}</span><textarea v-model="createForm.rawContent" rows="10" class="input font-mono text-xs" spellcheck="false" required /></label>
        <div class="grid min-w-0 gap-4 sm:grid-cols-2">
          <label><span class="input-label">{{ t('admin.instructionAudit.v2.observedField') }}</span><select v-model="createForm.observedField" class="input"><option value="">{{ t('admin.instructionAudit.v2.fieldAgnostic') }}</option><option value="instructions">instructions</option><option value="input1">input[1]</option></select></label>
          <label class="min-w-0"><span class="input-label">SHA-256 ({{ t('admin.instructionAudit.v2.optional') }})</span><input v-model.trim="createForm.sha256" maxlength="64" class="input font-mono text-xs" /></label>
        </div>
        <label class="block min-w-0"><span class="input-label">{{ t('admin.instructionAudit.v2.note') }}</span><textarea v-model="createForm.note" rows="3" maxlength="1000" class="input" /></label>
      </form>
      <template #footer><button type="button" class="btn btn-secondary" :disabled="saving" @click="closeCreate">{{ t('common.cancel') }}</button><button type="button" class="btn btn-primary" :disabled="saving || !createForm.rawContent" @click="createRisk"><Icon name="check" size="sm" />{{ t('common.save') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="Boolean(rawItem)" :title="t('admin.instructionAudit.v2.riskRawTitle', { id: rawItem?.id ?? '-' })" width="wide" @close="closeRaw">
      <div v-if="rawLoading" class="flex min-h-48 items-center justify-center"><Icon name="refresh" size="md" class="animate-spin text-primary-500" /></div>
      <div v-else-if="rawReview?.fields[0]" class="min-w-0 space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-2"><span class="break-all font-mono text-[11px] text-gray-500 dark:text-gray-400">{{ rawReview.fields[0].sha256 }}</span><button type="button" class="btn btn-secondary btn-sm" @click="copyRaw"><Icon name="copy" size="sm" />{{ t('common.copy') }}</button></div>
        <pre class="max-h-[60vh] min-h-56 overflow-auto whitespace-pre-wrap break-words rounded-md bg-gray-950 p-4 text-xs leading-5 text-gray-100">{{ rawReview.fields[0].plaintext }}</pre>
      </div>
      <template #footer><button type="button" class="btn btn-secondary" @click="closeRaw">{{ t('common.close') }}</button></template>
    </BaseDialog>

    <ConfirmDialog :show="Boolean(pendingAction)" :title="actionTitle" :message="actionMessage" :danger="pendingAction?.action === 'delete' || pendingAction?.action === 'confirm_risk'" @confirm="executeAction" @cancel="pendingAction = null" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import instructionAuditV2API from '../v2Api'
import type { InstructionEvidenceReview, InstructionRiskAction, InstructionRiskHash, InstructionRiskHashPage } from '../v2Types'
import { formatAuditDate } from '../v2Presentation'

const props = defineProps<{ refreshKey: number }>()
const emit = defineEmits<{ (event: 'changed'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const page = reactive<InstructionRiskHashPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const query = ref('')
const status = ref('')
const rawItem = ref<InstructionRiskHash | null>(null)
const rawReview = ref<InstructionEvidenceReview | null>(null)
const rawLoading = ref(false)
const pendingAction = ref<{ item: InstructionRiskHash; action: InstructionRiskAction | 'delete' } | null>(null)
const createForm = reactive({ show: false, rawContent: '', sha256: '', observedField: '' as '' | 'instructions' | 'input1', note: '' })

const actionTitle = computed(() => pendingAction.value ? t(`admin.instructionAudit.v2.riskActions.${pendingAction.value.action}.title`) : '')
const actionMessage = computed(() => pendingAction.value ? t(`admin.instructionAudit.v2.riskActions.${pendingAction.value.action}.message`, { id: pendingAction.value.item.id }) : '')

onMounted(loadItems)
watch(() => props.refreshKey, loadItems)

async function loadItems() {
  loading.value = true
  error.value = ''
  try {
    Object.assign(page, await instructionAuditV2API.listRiskHashes({ page: page.page, page_size: page.page_size, status: status.value || undefined, q: query.value || undefined }))
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, t('common.error'))
  } finally {
    loading.value = false
  }
}

function applyFilters() { page.page = 1; loadItems() }
function changePage(value: number) { page.page = value; loadItems() }
function changePageSize(value: number) { page.page_size = value; page.page = 1; loadItems() }
function fieldLabel(value: string) { return value === 'input1' ? 'input[1]' : value || t('admin.instructionAudit.v2.fieldAgnostic') }
function riskStatusPill(value: string) { return value === 'active' ? 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' }
function riskSourceLabel(value: string) { return t(`admin.instructionAudit.v2.riskSources.${value}`) }
function requestAction(item: InstructionRiskHash, action: InstructionRiskAction | 'delete') { pendingAction.value = { item, action } }
function openCreate() { Object.assign(createForm, { show: true, rawContent: '', sha256: '', observedField: '', note: '' }) }
function closeCreate() { if (!saving.value) createForm.show = false }

async function createRisk() {
  if (!createForm.rawContent) return
  saving.value = true
  try {
    await instructionAuditV2API.createRiskHash({ raw_content: createForm.rawContent, sha256: createForm.sha256.toLowerCase(), observed_field: createForm.observedField, note: createForm.note })
    appStore.showSuccess(t('admin.instructionAudit.v2.riskCreated'))
    createForm.show = false
    emit('changed')
    await loadItems()
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    saving.value = false
  }
}

async function executeAction() {
  if (!pendingAction.value) return
  const { item, action } = pendingAction.value
  try {
    if (action === 'delete') await instructionAuditV2API.deleteRiskHash(item.id)
    else await instructionAuditV2API.updateRiskHash(item.id, action)
    appStore.showSuccess(t('common.saved'))
    pendingAction.value = null
    emit('changed')
    await loadItems()
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

async function revealRaw(item: InstructionRiskHash) {
  rawItem.value = item
  rawReview.value = null
  rawLoading.value = true
  try {
    rawReview.value = await instructionAuditV2API.revealRiskHashRaw(item.id)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
    closeRaw()
  } finally {
    rawLoading.value = false
  }
}

async function copyRaw() {
  const field = rawReview.value?.fields[0]
  if (!rawItem.value || !field?.plaintext) return
  try {
    await instructionAuditV2API.recordRiskHashRawCopy(rawItem.value.id)
    await copyToClipboard(field.plaintext)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

function closeRaw() { rawItem.value = null; rawReview.value = null }
</script>

<style scoped>
.risk-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 360px), 1fr));
  gap: 0.875rem;
}

.risk-card {
  @apply flex min-w-0 flex-col gap-3 rounded-md border border-gray-200 bg-white p-4 shadow-sm transition hover:border-red-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-800 dark:hover:border-red-900;
}

.risk-label {
  @apply text-[11px] font-semibold uppercase text-gray-400;
}
</style>
