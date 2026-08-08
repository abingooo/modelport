<template>
  <BaseDialog :show="show" :title="t('admin.instructionAudit.hashDetail.title')" width="wide" @close="close">
    <div v-if="loading" class="flex min-h-56 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      <Icon name="refresh" size="md" class="mr-2 animate-spin" />
      {{ t('common.loading') }}
    </div>
    <div v-else-if="detail" class="min-w-0 space-y-5">
      <section class="grid min-w-0 gap-3 rounded-md border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800 sm:grid-cols-2 lg:grid-cols-4">
        <MetaItem :label="t('common.name')" :value="detail.name" />
        <MetaItem :label="t('common.status')" :value="hashStatusLabel(detail.status)" />
        <MetaItem :label="t('admin.instructionAudit.hashDetail.field')" :value="sourceLabel(detail.field_name || detail.observed_source)" />
        <MetaItem :label="t('admin.instructionAudit.hashDetail.rawStatus')" :value="rawStatusLabel(detail.raw_content_status)" />
        <div class="min-w-0 sm:col-span-2 lg:col-span-4">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">SHA-256</p>
          <div class="mt-1 flex min-w-0 items-start gap-2">
            <p class="min-w-0 break-all font-mono text-xs text-gray-900 dark:text-white">{{ detail.digest }}</p>
            <button type="button" class="icon-btn h-7 w-7 shrink-0" :title="t('common.copy')" @click="copyToClipboard(detail.digest)"><Icon name="copy" size="xs" /></button>
          </div>
        </div>
      </section>

      <div class="flex flex-wrap items-center gap-2">
        <span class="mr-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.hashDetail.globalActions') }}</span>
        <button v-if="canPromote" type="button" class="btn btn-primary btn-sm" :disabled="saving" @click="changeStatus('active')">
          <Icon name="check" size="sm" />{{ t('admin.instructionAudit.hashDetail.promote') }}
        </button>
        <button v-if="detail.status === 'active' || detail.status === 'candidate'" type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="changeStatus('disabled')">
          <Icon name="ban" size="sm" />{{ t('admin.instructionAudit.hashDetail.disable') }}
        </button>
        <button v-if="detail.status !== 'revoked'" type="button" class="btn btn-danger btn-sm" :disabled="saving" @click="globalRevokeRequested = true">
          <Icon name="x" size="sm" />{{ t('admin.instructionAudit.hashDetail.revoke') }}
        </button>
        <button v-if="detail.raw_content_status === 'stored' && !raw" type="button" class="btn btn-secondary btn-sm" :disabled="revealing || !sensitiveAccess" :title="sensitiveActionTitle" @click="revealRaw">
          <Icon :name="sensitiveAccess ? 'eye' : 'lock'" size="sm" />{{ sensitiveAccess ? t('admin.instructionAudit.hashDetail.revealRaw') : t('admin.instructionAudit.sensitiveAccess.lockedAction') }}
        </button>
      </div>

      <section>
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.hashDetail.scopes') }}</h3>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.hashDetail.scopeCount', { count: detail.scopes?.length || 0 }) }}</span>
        </div>
        <div v-if="detail.scopes?.length" class="divide-y divide-gray-100 overflow-hidden rounded-md border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
          <article v-for="(scope, index) in detail.scopes" :key="`${scope.rule_set_id}:${scope.binding_id || index}`" class="grid min-w-0 gap-2 px-3 py-3 sm:grid-cols-[minmax(150px,0.8fr)_minmax(180px,1fr)_auto]">
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="scope.rule_set_name">{{ scope.rule_set_name }}</p>
              <div class="mt-1 flex flex-wrap gap-1">
                <span :class="scopeSourcePill(scope.source_type)">{{ sourceTypeLabel(scope.source_type) }}</span>
                <span :class="scopeStatePill(scope)">{{ scopeStateLabel(scope) }}</span>
              </div>
            </div>
            <div class="min-w-0 text-xs text-gray-600 dark:text-gray-300">
              <p>{{ scope.group_id ? `${scope.group_name || '-'} #${scope.group_id}` : t('admin.instructionAudit.hashDetail.scopeUnbound') }}</p>
              <p class="mt-1 break-words">{{ t('admin.instructionAudit.hashDetail.clients') }}: {{ scopeClientLabels(scope.client_types) }}</p>
            </div>
            <div class="flex min-w-0 flex-col gap-2 text-xs text-gray-400 sm:items-end sm:text-right">
              <div>
              <p v-if="scope.valid_until">{{ t('admin.instructionAudit.hashDetail.expiresAt') }}</p>
              <time v-if="scope.valid_until" class="block">{{ formatDate(scope.valid_until) }}</time>
              <p v-else>{{ t('admin.instructionAudit.hashDetail.permanentScope') }}</p>
              </div>
              <div v-if="scope.system_managed" class="flex flex-wrap gap-1 sm:justify-end">
                <button v-if="scope.source_type === 'ai_review' && scope.status !== 'revoked'" type="button" class="btn btn-primary btn-xs" :disabled="saving" @click="changeScope(scope, 'promote')">
                  {{ t('admin.instructionAudit.hashDetail.promoteScope') }}
                </button>
                <button v-if="scope.status === 'active'" type="button" class="btn btn-secondary btn-xs" :disabled="saving" @click="changeScope(scope, 'disable')">
                  {{ t('admin.instructionAudit.hashDetail.disableScope') }}
                </button>
                <button v-if="scope.status !== 'revoked'" type="button" class="btn btn-danger btn-xs" :disabled="saving" @click="scopeRevokeRequested = scope">
                  {{ t('admin.instructionAudit.hashDetail.revokeScope') }}
                </button>
              </div>
            </div>
          </article>
        </div>
        <p v-else class="rounded-md border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('admin.instructionAudit.hashDetail.noScopes') }}</p>
      </section>

      <section>
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.hashDetail.provenance') }}</h3>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.hashDetail.sourceCount', { count: detail.sources?.length || 0 }) }}</span>
        </div>
        <div v-if="detail.sources?.length" class="divide-y divide-gray-100 overflow-hidden rounded-md border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
          <article v-for="source in detail.sources" :key="source.id" class="grid min-w-0 gap-2 px-3 py-3 sm:grid-cols-[minmax(120px,0.6fr)_minmax(160px,1fr)_auto]">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ sourceTypeLabel(source.source_type) }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ sourceLabel(source.field_name) }}</p>
            </div>
            <div class="min-w-0 text-xs text-gray-600 dark:text-gray-300">
              <p v-if="source.event_id">{{ t('admin.instructionAudit.eventNumber', { id: source.event_id }) }}</p>
              <p v-if="source.ai_review_id">AI #{{ source.ai_review_id }} · {{ source.reviewer_model || '-' }}</p>
              <p v-if="source.confidence != null">{{ t('admin.instructionAudit.hashDetail.confidence') }} {{ (source.confidence * 100).toFixed(1) }}%</p>
              <p v-if="source.review_reason" class="mt-1 break-words">{{ source.review_reason }}</p>
            </div>
            <time class="text-xs text-gray-400">{{ formatDate(source.created_at) }}</time>
          </article>
        </div>
        <p v-else class="rounded-md border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('admin.instructionAudit.hashDetail.noSources') }}</p>
      </section>

      <section v-if="raw" class="space-y-2">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.hashDetail.rawContent') }}</h3>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.byteCount', { count: raw.content_bytes }) }}</span>
        </div>
        <InstructionTranslationPanel
          v-if="translationField"
          resource-type="hash"
          :resource-id="detail.id"
          :field-name="translationField"
          :original="raw.raw_content || ''"
          :enabled="translationEnabled"
          :external-enabled="externalTranslationEnabled"
          :sensitive-access="sensitiveAccess"
          @copy-original="copyRaw"
          @access-denied="handleSensitiveAccessDenied"
        />
        <pre v-else class="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-md bg-gray-950 p-3 font-mono text-xs leading-5 text-gray-100">{{ raw.raw_content }}</pre>
        <p class="break-all text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.recomputedHash') }}: {{ raw.recomputed_sha256 || '-' }}</p>
      </section>

      <p v-if="detail.raw_expires_at" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.instructionAudit.hashDetail.rawExpiresAt') }}: {{ formatDate(detail.raw_expires_at) }}
      </p>
    </div>

    <ConfirmDialog
      :show="globalRevokeRequested"
      :title="t('admin.instructionAudit.hashDetail.revokeTitle')"
      :message="t('admin.instructionAudit.hashDetail.revokeConfirm')"
      :confirm-text="t('admin.instructionAudit.hashDetail.revoke')"
      danger
      @confirm="changeStatus('revoked')"
      @cancel="globalRevokeRequested = false"
    />
    <ConfirmDialog
      :show="Boolean(scopeRevokeRequested)"
      :title="t('admin.instructionAudit.hashDetail.revokeScopeTitle')"
      :message="t('admin.instructionAudit.hashDetail.revokeScopeConfirm')"
      :confirm-text="t('admin.instructionAudit.hashDetail.revokeScope')"
      danger
      @confirm="scopeRevokeRequested && changeScope(scopeRevokeRequested, 'revoke')"
      @cancel="scopeRevokeRequested = null"
    />
    <TotpStepUpDialog :controller="stepUp" />
    <template #footer><button type="button" class="btn btn-secondary" @click="close">{{ t('common.close') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import instructionAuditAPI from '../api'
import { isInstructionSensitiveAccessDenied } from '../sensitiveAccess'
import type { InstructionHashEntry, InstructionHashRawReview, InstructionHashScope } from '../types'
import InstructionTranslationPanel from './InstructionTranslationPanel.vue'

const props = withDefaults(defineProps<{
  show: boolean
  hashId: number | null
  translationEnabled: boolean
  externalTranslationEnabled: boolean
  sensitiveAccess?: boolean
}>(), { sensitiveAccess: true })
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'changed'): void
  (event: 'access-denied', error?: unknown): void
}>()
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const stepUp = useStepUp()
const loading = ref(false)
const saving = ref(false)
const revealing = ref(false)
const detail = ref<InstructionHashEntry | null>(null)
const raw = ref<InstructionHashRawReview | null>(null)
const globalRevokeRequested = ref(false)
const scopeRevokeRequested = ref<InstructionHashScope | null>(null)

const translationField = computed<'instructions' | 'input1' | null>(() => {
  const field = detail.value?.field_name || detail.value?.observed_source
  return field === 'instructions' || field === 'input1' ? field : null
})
const canPromote = computed(() => Boolean(
  detail.value
  && detail.value.status !== 'revoked'
  && (detail.value.status !== 'active' || detail.value.valid_until),
))
const sensitiveActionTitle = computed(() => props.sensitiveAccess
  ? ''
  : t('admin.instructionAudit.sensitiveAccess.lockedHint'))

const MetaItem = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(itemProps) {
    return () => h('div', { class: 'min-w-0' }, [
      h('p', { class: 'text-xs font-medium text-gray-500 dark:text-gray-400' }, itemProps.label),
      h('p', { class: 'mt-1 break-words text-sm font-medium text-gray-900 dark:text-white' }, itemProps.value || '-'),
    ])
  },
})

watch(() => [props.show, props.hashId] as const, async ([show, hashId]) => {
  detail.value = null
  raw.value = null
  globalRevokeRequested.value = false
  scopeRevokeRequested.value = null
  if (!show || !hashId) return
  loading.value = true
  try {
    detail.value = await instructionAuditAPI.getHash(hashId)
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
    emit('close')
  } finally {
    loading.value = false
  }
}, { immediate: true })
watch(() => props.sensitiveAccess, (allowed) => {
  if (!allowed) raw.value = null
})

async function revealRaw() {
  if (!detail.value || !props.sensitiveAccess || revealing.value) return
  revealing.value = true
  try {
    raw.value = await stepUp.run(() => instructionAuditAPI.revealHashRaw(detail.value!.id))
  } catch (error) {
    if (isInstructionSensitiveAccessDenied(error)) {
      handleSensitiveAccessDenied(error)
      return
    }
    if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    revealing.value = false
  }
}

async function copyRaw() {
  if (!detail.value || !props.sensitiveAccess || !raw.value?.raw_content) return
  try {
    await stepUp.run(() => instructionAuditAPI.recordHashRawCopy(detail.value!.id))
    await copyToClipboard(raw.value.raw_content)
  } catch (error) {
    if (isInstructionSensitiveAccessDenied(error)) {
      handleSensitiveAccessDenied(error)
      return
    }
    if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  }
}

async function changeStatus(status: 'active' | 'disabled' | 'revoked') {
  if (!detail.value || saving.value) return
  saving.value = true
  globalRevokeRequested.value = false
  try {
    detail.value = await stepUp.run(() => instructionAuditAPI.changeHashStatus(detail.value!.id, status))
    appStore.showSuccess(t('common.saved'))
    emit('changed')
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

async function changeScope(scope: InstructionHashScope, action: 'promote' | 'disable' | 'revoke') {
  if (!detail.value || saving.value) return
  saving.value = true
  scopeRevokeRequested.value = null
  try {
    detail.value = await stepUp.run(() => instructionAuditAPI.changeHashScope(detail.value!.id, scope.rule_set_id, action))
    appStore.showSuccess(t('common.saved'))
    emit('changed')
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

function close() {
  detail.value = null
  raw.value = null
  emit('close')
}

function handleSensitiveAccessDenied(error?: unknown) {
  raw.value = null
  emit('access-denied', error)
}

function sourceLabel(value: string): string {
  if (value === 'instructions') return t('admin.instructionAudit.fieldOne')
  if (value === 'input1') return t('admin.instructionAudit.fieldTwo')
  return value || '-'
}

function sourceTypeLabel(value: string): string {
  return t(`admin.instructionAudit.hashDetail.sourceTypes.${value}`, value)
}

function scopeClientLabels(values: string[]): string {
  if (!values?.length) return '-'
  return values.map((value) => value === 'all'
    ? t('admin.instructionAudit.allClients')
    : t(`admin.instructionAudit.clients.${value}`, value)).join(' / ')
}

function scopeExpired(scope: InstructionHashScope): boolean {
  return Boolean(scope.valid_until && new Date(scope.valid_until).getTime() <= Date.now())
}

function scopeStateLabel(scope: InstructionHashScope): string {
	if (detail.value?.status !== 'active') return t('admin.instructionAudit.hashDetail.globalHashInactive')
	if (scope.status === 'revoked') return t('admin.instructionAudit.hashDetail.scopeRevoked')
	if (scope.status === 'disabled') return t('admin.instructionAudit.hashDetail.scopeDisabled')
  if (scopeExpired(scope)) return t('admin.instructionAudit.hashDetail.scopeExpired')
  if (!scope.rule_set_enabled || (scope.binding_id && !scope.binding_enabled)) return t('common.disabled')
  if (!scope.binding_id) return t('admin.instructionAudit.hashDetail.scopeUnbound')
  return scope.valid_until
    ? t('admin.instructionAudit.hashDetail.temporaryScope')
    : t('admin.instructionAudit.hashDetail.permanentScope')
}

function scopeSourcePill(sourceType: string): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium'
  return sourceType === 'ai_review'
    ? `${base} bg-cyan-50 text-cyan-700 dark:bg-cyan-950/40 dark:text-cyan-300`
    : `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
}

function scopeStatePill(scope: InstructionHashScope): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium'
	if (detail.value?.status !== 'active' || scope.status === 'revoked') return `${base} bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300`
	if (scope.status === 'disabled') return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  if (scopeExpired(scope)) return `${base} bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300`
  if (!scope.rule_set_enabled || (scope.binding_id && !scope.binding_enabled) || !scope.binding_id) return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  return `${base} bg-primary-50 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300`
}

function hashStatusLabel(value: string): string {
  return t(`admin.instructionAudit.hashStatuses.${value}`, value)
}

function rawStatusLabel(value: string): string {
  return t(`admin.instructionAudit.rawStatuses.${value}`, value)
}

function formatDate(value?: string | null): string {
  return value ? new Date(value).toLocaleString() : '-'
}
</script>
