<template>
  <section class="min-w-0 space-y-4" data-test="instruction-v2-hashes">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.trustedInstructions') }}</h2>
        <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.trustedInstructionsHint') }}</p>
      </div>
      <button type="button" class="btn btn-primary shrink-0" @click="openCreate">
        <Icon name="plus" size="sm" />
        {{ t('admin.instructionAudit.v2.addTrusted') }}
      </button>
    </div>

    <div class="grid min-w-0 gap-3 rounded-md border-y border-gray-200 py-4 dark:border-dark-700 sm:grid-cols-[minmax(220px,1fr)_180px_auto]">
      <label class="min-w-0">
        <span class="input-label">{{ t('common.search') }}</span>
        <div class="flex min-w-0">
          <input v-model.trim="query" type="search" class="input min-w-0 rounded-r-none" :placeholder="t('admin.instructionAudit.v2.searchTrusted')" @keyup.enter="applyFilters" />
          <button type="button" class="btn btn-primary rounded-l-none px-3" :title="t('common.search')" @click="applyFilters"><Icon name="search" size="sm" /></button>
        </div>
      </label>
      <label>
        <span class="input-label">{{ t('common.status') }}</span>
        <select v-model="status" class="input" @change="applyFilters">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="value in statusOptions" :key="value" :value="value">{{ hashStatusLabel(t, value) }}</option>
        </select>
      </label>
      <div class="flex items-end">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadHashes">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <div v-if="error" role="alert" class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">{{ error }}</div>

    <div v-if="loading && !page.items.length" class="hash-grid">
      <div v-for="index in 6" :key="index" class="h-64 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>

    <div v-else-if="page.items.length" class="hash-grid">
      <article v-for="hash in page.items" :key="hash.id" class="hash-card">
        <header class="flex min-w-0 items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <h3 class="min-w-0 break-words text-sm font-semibold text-gray-950 dark:text-white">{{ hash.name || t('admin.instructionAudit.v2.unnamedTrusted') }}</h3>
              <span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="hashStatusPill(hash.status)">{{ hashStatusLabel(t, hash.status) }}</span>
              <span v-if="hash.global_trust" class="rounded-full bg-cyan-100 px-2 py-0.5 text-[11px] font-semibold text-cyan-800 dark:bg-cyan-950/50 dark:text-cyan-200">{{ t('admin.instructionAudit.v2.globalTrust') }}</span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">#{{ hash.id }} · {{ sourceLabel(t, hash.source) }} · {{ formatAuditDate(hash.created_at) }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-1">
            <button type="button" class="icon-btn" :title="t('common.edit')" @click="openEdit(hash)"><Icon name="edit" size="sm" /></button>
            <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" @click="hashToDelete = hash"><Icon name="trash" size="sm" /></button>
          </div>
        </header>

        <div class="min-w-0 rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-800/70">
          <div class="flex min-w-0 items-center justify-between gap-2">
            <span class="text-[11px] font-semibold uppercase text-gray-400">SHA-256</span>
            <button type="button" class="icon-btn h-6 w-6" :title="t('common.copy')" @click="copyToClipboard(hash.sha256)"><Icon name="copy" size="xs" /></button>
          </div>
          <p class="mt-1 break-all font-mono text-[11px] text-gray-600 dark:text-gray-300">{{ hash.sha256 }}</p>
        </div>

        <dl class="grid grid-cols-2 gap-3">
          <div>
            <dt class="hash-label">{{ t('admin.instructionAudit.v2.rawStorage') }}</dt>
            <dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ hash.raw_storage }} · {{ formatAuditBytes(hash.stored_bytes) }}</dd>
          </div>
          <div>
            <dt class="hash-label">{{ t('admin.instructionAudit.v2.contentSize') }}</dt>
            <dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ formatAuditBytes(hash.content_bytes) }}</dd>
          </div>
          <div>
            <dt class="hash-label">{{ t('admin.instructionAudit.v2.observedField') }}</dt>
            <dd class="mt-1 font-mono text-xs text-gray-800 dark:text-gray-200">{{ hash.observed_field === 'input1' ? 'input[1]' : hash.observed_field || '-' }}</dd>
          </div>
          <div>
            <dt class="hash-label">{{ t('admin.instructionAudit.v2.scopeCount') }}</dt>
            <dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ hash.scopes.length }}</dd>
          </div>
        </dl>

        <div class="min-w-0">
          <p class="hash-label">{{ t('admin.instructionAudit.v2.effectiveScopes') }}</p>
          <div class="mt-2 flex min-w-0 flex-wrap gap-1.5">
            <span v-for="scope in hash.scopes" :key="scope.scope_id" class="max-w-full truncate rounded bg-primary-50 px-2 py-1 text-[11px] text-primary-700 dark:bg-primary-950/40 dark:text-primary-300" :title="`${scope.group_name} / ${scope.client_profile_name}`">
              {{ scope.group_name }} · {{ scope.client_profile_name || t('admin.instructionAudit.v2.allClients') }}
            </span>
            <span v-if="hash.global_trust" class="text-xs font-medium text-cyan-700 dark:text-cyan-300">{{ t('admin.instructionAudit.v2.globalTrustHint') }}</span>
            <span v-else-if="!hash.scopes.length" class="text-xs text-gray-400">{{ t('admin.instructionAudit.v2.noScopes') }}</span>
          </div>
        </div>

        <p v-if="hash.note" class="break-words text-xs text-gray-500 dark:text-gray-400">{{ hash.note }}</p>

        <footer class="mt-auto flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="hash.raw_storage === 'unavailable'" @click="revealRaw(hash)">
            <Icon name="eye" size="sm" />
            {{ t('admin.instructionAudit.v2.viewRaw') }}
          </button>
        </footer>
      </article>
    </div>

    <div v-else class="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-gray-200 px-6 text-center dark:border-dark-600">
      <Icon name="key" size="xl" class="text-gray-300 dark:text-dark-500" />
      <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.v2.noTrusted') }}</p>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.noTrustedHint') }}</p>
    </div>

    <Pagination v-if="page.total > 0" :total="page.total" :page="page.page" :page-size="page.page_size" @update:page="changePage" @update:page-size="changePageSize" />

    <BaseDialog :show="form.show" :title="form.id ? t('admin.instructionAudit.v2.editTrusted') : t('admin.instructionAudit.v2.addTrusted')" width="wide" @close="closeForm">
      <form class="min-w-0 space-y-5" @submit.prevent="saveHash">
        <div class="grid min-w-0 gap-4 sm:grid-cols-2">
          <label class="min-w-0">
            <span class="input-label">{{ t('common.name') }}</span>
            <input v-model="form.name" maxlength="160" class="input" :placeholder="t('admin.instructionAudit.v2.optionalName')" />
          </label>
          <label>
            <span class="input-label">{{ t('common.status') }}</span>
            <select v-model="form.status" class="input">
              <option v-for="value in statusOptions" :key="value" :value="value">{{ hashStatusLabel(t, value) }}</option>
            </select>
          </label>
        </div>

        <template v-if="!form.id">
          <div class="inline-flex rounded-md bg-gray-100 p-1 dark:bg-dark-700">
            <button type="button" class="rounded px-3 py-1.5 text-sm font-medium" :class="form.inputMode === 'raw' ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500 dark:text-gray-300'" @click="form.inputMode = 'raw'">{{ t('admin.instructionAudit.v2.enterRaw') }}</button>
            <button type="button" class="rounded px-3 py-1.5 text-sm font-medium" :class="form.inputMode === 'digest' ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-300' : 'text-gray-500 dark:text-gray-300'" @click="form.inputMode = 'digest'">{{ t('admin.instructionAudit.v2.importDigest') }}</button>
          </div>
          <label v-if="form.inputMode === 'raw'" class="block min-w-0">
            <span class="input-label">{{ t('admin.instructionAudit.v2.canonicalRaw') }}</span>
            <textarea v-model="form.rawContent" rows="10" class="input font-mono text-xs" spellcheck="false" required />
            <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.exactBytesHint') }}</span>
          </label>
          <label v-else class="block min-w-0">
            <span class="input-label">SHA-256</span>
            <input v-model.trim="form.sha256" minlength="64" maxlength="64" pattern="[0-9a-fA-F]{64}" class="input font-mono text-xs" required />
          </label>
        </template>
        <div v-else class="rounded-md bg-gray-50 px-3 py-3 dark:bg-dark-800/70">
          <span class="hash-label">SHA-256</span>
          <p class="mt-1 break-all font-mono text-xs text-gray-700 dark:text-gray-200">{{ form.sha256 }}</p>
        </div>

        <label class="flex items-start gap-3 rounded-md border border-cyan-200 bg-cyan-50/60 p-3 dark:border-cyan-900/50 dark:bg-cyan-950/20">
          <input v-model="form.globalTrust" type="checkbox" class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span>
            <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.instructionAudit.v2.globalTrust') }}</span>
            <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.globalTrustWarning') }}</span>
          </span>
        </label>

        <fieldset class="min-w-0" :disabled="form.globalTrust">
          <legend class="input-label">{{ t('admin.instructionAudit.v2.bindScopes') }}</legend>
          <div class="mt-2 grid max-h-64 min-w-0 gap-2 overflow-y-auto rounded-md border border-gray-200 p-3 transition dark:border-dark-600 sm:grid-cols-2" :class="{ 'opacity-45': form.globalTrust }">
            <label v-for="scope in scopes" :key="scope.id" class="flex min-w-0 cursor-pointer items-start gap-2 rounded px-2 py-2 hover:bg-gray-50 dark:hover:bg-dark-700">
              <input v-model="form.scopeIds" type="checkbox" :value="scope.id" class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span class="min-w-0">
                <span class="block truncate text-sm font-medium text-gray-800 dark:text-gray-200">{{ scope.group_name }}</span>
                <span class="block truncate text-xs text-gray-500 dark:text-gray-400">{{ scope.client_profile_name || t('admin.instructionAudit.v2.allClients') }}</span>
              </span>
            </label>
          </div>
        </fieldset>

        <label class="block min-w-0">
          <span class="input-label">{{ t('admin.instructionAudit.v2.note') }}</span>
          <textarea v-model="form.note" rows="3" maxlength="1000" class="input" />
        </label>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeForm">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving || (!form.globalTrust && !form.scopeIds.length) || (!form.id && form.inputMode === 'raw' && !form.rawContent) || (!form.id && form.inputMode === 'digest' && form.sha256.length !== 64)" @click="saveHash">
          <Icon name="check" size="sm" />{{ t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog :show="Boolean(rawHash)" :title="t('admin.instructionAudit.v2.rawTitle', { id: rawHash?.id ?? '-' })" width="wide" @close="closeRaw">
      <div v-if="rawLoading" class="flex min-h-48 items-center justify-center"><Icon name="refresh" size="md" class="animate-spin text-primary-500" /></div>
      <div v-else-if="rawReview?.fields[0]" class="min-w-0 space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ rawReview.fields[0].storage_kind }} · {{ formatAuditBytes(rawReview.fields[0].content_bytes) }}</span>
          <button type="button" class="btn btn-secondary btn-sm" @click="copyRaw"><Icon name="copy" size="sm" />{{ t('common.copy') }}</button>
        </div>
        <pre class="max-h-[60vh] min-h-56 overflow-auto whitespace-pre-wrap break-words rounded-md bg-gray-950 p-4 text-xs leading-5 text-gray-100">{{ rawReview.fields[0].plaintext }}</pre>
      </div>
      <template #footer><button type="button" class="btn btn-secondary" @click="closeRaw">{{ t('common.close') }}</button></template>
    </BaseDialog>

    <ConfirmDialog :show="Boolean(hashToDelete)" :title="t('admin.instructionAudit.v2.deleteTrustedTitle')" :message="t('admin.instructionAudit.v2.deleteTrustedConfirm')" danger @confirm="deleteHash" @cancel="hashToDelete = null" />
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import instructionAuditV2API from '../v2Api'
import type { InstructionEvidenceReview, InstructionHash, InstructionHashPage, InstructionHashStatus, InstructionScope } from '../v2Types'
import { formatAuditBytes, formatAuditDate, hashStatusLabel, hashStatusPill, sourceLabel } from '../v2Presentation'

const props = defineProps<{
  scopes: InstructionScope[]
  refreshKey: number
}>()

const emit = defineEmits<{ (event: 'changed'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const page = reactive<InstructionHashPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const query = ref('')
const status = ref('')
const hashToDelete = ref<InstructionHash | null>(null)
const rawHash = ref<InstructionHash | null>(null)
const rawReview = ref<InstructionEvidenceReview | null>(null)
const rawLoading = ref(false)
const statusOptions: InstructionHashStatus[] = ['active', 'disabled', 'revoked']
const form = reactive({
  show: false,
  id: 0,
  inputMode: 'raw' as 'raw' | 'digest',
  rawContent: '',
  sha256: '',
  name: '',
  note: '',
  status: 'active' as InstructionHashStatus,
  scopeIds: [] as number[],
  globalTrust: false,
})

onMounted(loadHashes)
watch(() => props.refreshKey, loadHashes)

async function loadHashes() {
  loading.value = true
  error.value = ''
  try {
    Object.assign(page, await instructionAuditV2API.listHashes({ page: page.page, page_size: page.page_size, status: status.value || undefined, q: query.value || undefined }))
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, t('common.error'))
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.page = 1
  loadHashes()
}

function changePage(value: number) {
  page.page = value
  loadHashes()
}

function changePageSize(value: number) {
  page.page_size = value
  page.page = 1
  loadHashes()
}

function resetForm() {
  Object.assign(form, { show: false, id: 0, inputMode: 'raw', rawContent: '', sha256: '', name: '', note: '', status: 'active', scopeIds: [], globalTrust: false })
}

function openCreate() {
  resetForm()
  form.show = true
}

function openEdit(hash: InstructionHash) {
  Object.assign(form, { show: true, id: hash.id, inputMode: 'digest', rawContent: '', sha256: hash.sha256, name: hash.name, note: hash.note, status: hash.status, scopeIds: [...hash.scope_ids], globalTrust: hash.global_trust })
}

function closeForm() {
  if (!saving.value) resetForm()
}

async function saveHash() {
  if (!form.globalTrust && !form.scopeIds.length) return
  saving.value = true
  try {
    if (form.id) {
      await instructionAuditV2API.updateHash(form.id, { name: form.name, note: form.note, status: form.status, scope_ids: form.globalTrust ? [] : form.scopeIds, set_scopes: true, global_trust: form.globalTrust })
    } else {
      await instructionAuditV2API.createHash({
        raw_content: form.inputMode === 'raw' ? form.rawContent : '',
        sha256: form.inputMode === 'digest' ? form.sha256.toLowerCase() : '',
        source: form.inputMode === 'raw' ? 'manual' : 'import',
        name: form.name,
        note: form.note,
        status: form.status,
        scope_ids: form.globalTrust ? [] : form.scopeIds,
        global_trust: form.globalTrust,
      })
    }
    appStore.showSuccess(t('common.saved'))
    resetForm()
    emit('changed')
    await loadHashes()
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    saving.value = false
  }
}

async function deleteHash() {
  if (!hashToDelete.value) return
  try {
    await instructionAuditV2API.deleteHash(hashToDelete.value!.id)
    appStore.showSuccess(t('admin.instructionAudit.v2.trustedDeleted'))
    hashToDelete.value = null
    emit('changed')
    await loadHashes()
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

async function revealRaw(hash: InstructionHash) {
  rawHash.value = hash
  rawReview.value = null
  rawLoading.value = true
  try {
    rawReview.value = await instructionAuditV2API.revealHashRaw(hash.id)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
    closeRaw()
  } finally {
    rawLoading.value = false
  }
}

async function copyRaw() {
  const field = rawReview.value?.fields[0]
  if (!rawHash.value || !field?.plaintext) return
  try {
    await instructionAuditV2API.recordHashRawCopy(rawHash.value!.id)
    await copyToClipboard(field.plaintext)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

function closeRaw() {
  rawHash.value = null
  rawReview.value = null
}
</script>

<style scoped>
.hash-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 350px), 1fr));
  gap: 0.875rem;
}

.hash-card {
  @apply flex min-w-0 flex-col gap-3 rounded-md border border-gray-200 bg-white p-4 shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-800;
}

.hash-label {
  @apply text-[11px] font-semibold uppercase text-gray-400;
}
</style>
