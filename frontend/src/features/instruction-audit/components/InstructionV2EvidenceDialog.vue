<template>
  <BaseDialog
    :show="show"
    :title="t('admin.instructionAudit.v2.evidenceTitle', { id: event?.id ?? '-' })"
    width="extra-wide"
    @close="close"
  >
    <div v-if="loading" class="flex min-h-64 items-center justify-center gap-2 text-sm text-gray-500 dark:text-gray-400">
      <Icon name="refresh" size="md" class="animate-spin" />
      {{ t('common.loading') }}
    </div>

    <div v-else-if="detail" class="min-w-0 space-y-5">
      <section class="grid min-w-0 gap-3 rounded-md border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-600 dark:bg-dark-800/60 sm:grid-cols-2 xl:grid-cols-4">
        <div class="min-w-0">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.eventId') }}</p>
          <div class="mt-1 flex min-w-0 items-center gap-1.5">
            <span class="font-semibold text-primary-700 dark:text-primary-300">#{{ detail.id }}</span>
            <button type="button" class="icon-btn h-7 w-7" :title="t('common.copy')" @click="copyToClipboard(String(detail.id))">
              <Icon name="copy" size="xs" />
            </button>
          </div>
        </div>
        <div class="min-w-0">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.requestId') }}</p>
          <div class="mt-1 flex min-w-0 items-center gap-1.5">
            <span class="min-w-0 break-all font-mono text-xs text-gray-800 dark:text-gray-200">{{ detail.request_id || '-' }}</span>
            <button type="button" class="icon-btn h-7 w-7 shrink-0" :disabled="!detail.request_id" :title="t('common.copy')" @click="copyToClipboard(detail.request_id)">
              <Icon name="copy" size="xs" />
            </button>
          </div>
        </div>
        <div class="min-w-0">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.subject') }}</p>
          <p class="mt-1 break-all text-sm font-medium text-gray-900 dark:text-white">{{ detail.user_email || '-' }}</p>
          <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">{{ detail.api_key_name || '-' }}</p>
        </div>
        <div class="min-w-0">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.scopeAndClient') }}</p>
          <p class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white">{{ detail.group_name || '-' }}</p>
          <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">{{ detail.client_name || detail.client_key }}</p>
        </div>
      </section>

      <section class="grid min-w-0 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        <div class="audit-metric-cell">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.finalDecision') }}</p>
          <span class="mt-1.5 inline-flex rounded-full px-2 py-1 text-xs font-semibold" :class="outcomePill(detail.outcome)">
            {{ outcomeLabel(t, detail.outcome) }}
          </span>
        </div>
        <div class="audit-metric-cell">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.reason') }}</p>
          <p class="mt-1.5 break-words text-sm text-gray-900 dark:text-white">{{ reasonLabel(t, detail.reason) }}</p>
        </div>
        <div class="audit-metric-cell">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.aiReview') }}</p>
          <p class="mt-1.5 text-sm text-gray-900 dark:text-white">{{ aiResultLabel(t, detail.ai_result) }}</p>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ detail.ai_latency_ms }} ms</p>
        </div>
        <div class="audit-metric-cell">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.requestMetrics') }}</p>
          <p class="mt-1.5 text-sm tabular-nums text-gray-900 dark:text-white">{{ formatAuditBytes(detail.body_bytes) }} · {{ detail.audit_latency_ms }} ms</p>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ formatAuditDate(detail.created_at) }}</p>
        </div>
        <div class="audit-metric-cell">
          <p class="audit-detail-label">{{ t('admin.instructionAudit.v2.notificationStatus') }}</p>
          <p class="mt-1.5 text-xs text-gray-900 dark:text-white">
            {{ t('admin.instructionAudit.v2.userNotification') }}: {{ notificationLabel(t, detail.user_notification_status) }}
          </p>
          <p class="mt-1 text-xs text-gray-900 dark:text-white">
            {{ t('admin.instructionAudit.v2.opsNotification') }}: {{ notificationLabel(t, detail.ops_notification_status) }}
          </p>
        </div>
      </section>

      <div v-if="!sensitiveAccess" class="flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
        <Icon name="lock" size="md" class="mt-0.5 shrink-0" />
        <span>{{ t('admin.instructionAudit.v2.sensitiveAccessRequired') }}</span>
      </div>
      <div v-else-if="evidenceError" class="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
        {{ evidenceError }}
      </div>

      <section class="grid min-w-0 gap-4 xl:grid-cols-2">
        <article v-for="field in displayFields" :key="field.field_name" class="min-w-0 overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
          <header class="flex flex-wrap items-start justify-between gap-2 border-b border-gray-100 bg-gray-50/70 px-4 py-3 dark:border-dark-700 dark:bg-dark-800/60">
            <div>
              <h3 class="font-mono text-sm font-semibold text-gray-950 dark:text-white">{{ field.field_name === 'input1' ? 'input[1]' : field.field_name }}</h3>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ formatAuditBytes(field.content_bytes) }} · {{ evidenceStorageLabel(field) }}
              </p>
            </div>
            <span class="rounded-full px-2 py-1 text-[11px] font-medium" :class="evidenceDigestClass(field)">
              {{ evidenceDigestLabel(field) }}
            </span>
          </header>
          <div class="min-w-0 space-y-3 p-4">
            <div class="min-w-0">
              <div class="mb-1 flex items-center justify-between gap-2">
                <span class="audit-detail-label">SHA-256</span>
                <button type="button" class="icon-btn h-7 w-7" :title="t('common.copy')" @click="copyToClipboard(field.sha256)">
                  <Icon name="copy" size="xs" />
                </button>
              </div>
              <p class="break-all rounded bg-gray-100 px-3 py-2 font-mono text-[11px] text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ field.sha256 }}</p>
            </div>
            <div class="min-w-0">
              <div class="mb-1 flex items-center justify-between gap-2">
                <span class="audit-detail-label">{{ t('admin.instructionAudit.v2.plaintextEvidence') }}</span>
                <button type="button" class="btn btn-ghost btn-sm" :disabled="!field.plaintext" @click="copyEvidence(field)">
                  <Icon name="copy" size="sm" />
                  {{ t('common.copy') }}
                </button>
              </div>
              <pre class="max-h-80 min-h-36 overflow-auto whitespace-pre-wrap break-words rounded bg-gray-950 p-3 text-xs leading-5 text-gray-100">{{ field.plaintext || t('admin.instructionAudit.v2.plaintextUnavailable') }}</pre>
            </div>
          </div>
        </article>
      </section>

      <section v-if="detail.ai_reviews?.length" class="space-y-2">
        <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.aiAttempts') }}</h3>
        <div class="grid min-w-0 gap-2 lg:grid-cols-2">
          <article v-for="review in detail.ai_reviews" :key="review.id" class="min-w-0 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ aiResultLabel(t, review.result) }} · {{ Math.round(review.confidence * 100) }}%</span>
              <span class="text-xs text-gray-400">{{ review.latency_ms }} ms</span>
            </div>
            <p class="mt-2 break-words text-sm text-gray-600 dark:text-gray-300">{{ review.reason || '-' }}</p>
            <p class="mt-1 break-all font-mono text-[11px] text-gray-400">{{ review.node_name }} · {{ review.reviewer_model }} · {{ review.prompt_version }}</p>
          </article>
        </div>
      </section>

      <section v-if="evidence?.fields.length" class="rounded-md border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-900/50 dark:bg-primary-950/20">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end">
          <div class="min-w-0 flex-1">
            <h3 class="text-sm font-semibold text-primary-900 dark:text-primary-100">{{ t('admin.instructionAudit.v2.trustFromEvent') }}</h3>
            <div class="mt-3 flex flex-wrap gap-4">
              <label v-for="field in evidence.fields" :key="`trust-${field.field_name}`" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                <input v-model="trustFields" type="checkbox" :value="field.field_name" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                {{ field.field_name === 'input1' ? 'input[1]' : field.field_name }}
              </label>
            </div>
          </div>
          <label class="min-w-0 lg:w-64">
            <span class="input-label">{{ t('common.name') }}</span>
            <input v-model="trustName" maxlength="160" class="input" :placeholder="t('admin.instructionAudit.v2.optionalName')" />
          </label>
          <button type="button" class="btn btn-primary shrink-0" :disabled="trusting || !trustFields.length" @click="trustSelected">
            <Icon name="plus" size="sm" />
            {{ t('admin.instructionAudit.v2.addToTrusted') }}
          </button>
        </div>
      </section>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="close">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import instructionAuditV2API from '../v2Api'
import type { InstructionEvent, InstructionEvidenceField, InstructionEvidenceReview } from '../v2Types'
import {
  aiResultLabel,
  formatAuditBytes,
  formatAuditDate,
  notificationLabel,
  outcomeLabel,
  outcomePill,
  reasonLabel,
} from '../v2Presentation'

const props = defineProps<{
  show: boolean
  event: InstructionEvent | null
  sensitiveAccess: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'trusted'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const stepUp = useStepUp()
const loading = ref(false)
const detail = ref<InstructionEvent | null>(null)
const evidence = ref<InstructionEvidenceReview | null>(null)
const evidenceError = ref('')
const trustFields = ref<string[]>([])
const trustName = ref('')
const trusting = ref(false)

const displayFields = computed<InstructionEvidenceField[]>(() => evidence.value?.fields ?? [])

function evidenceStorageLabel(field: InstructionEvidenceField): string {
  return field.storage_kind === 'sample'
    ? t('admin.instructionAudit.v2.sampleEvidence')
    : t('admin.instructionAudit.v2.fullEvidence')
}

function evidenceDigestLabel(field: InstructionEvidenceField): string {
  if (field.storage_kind === 'sample') return t('admin.instructionAudit.v2.sampleDigestNotApplicable')
  return field.digest_consistent
    ? t('admin.instructionAudit.v2.digestVerified')
    : t('admin.instructionAudit.v2.digestUnverified')
}

function evidenceDigestClass(field: InstructionEvidenceField): string {
  if (field.storage_kind === 'sample') return 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'
  return field.digest_consistent
    ? 'bg-primary-100 text-primary-700 dark:bg-primary-950/60 dark:text-primary-200'
    : 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300'
}

watch(
  () => [props.show, props.event?.id, props.sensitiveAccess] as const,
  async ([show, eventID]) => {
    detail.value = null
    evidence.value = null
    evidenceError.value = ''
    trustFields.value = []
    trustName.value = ''
    if (!show || !eventID) return
    loading.value = true
    try {
      detail.value = await instructionAuditV2API.getEvent(eventID)
      if (props.sensitiveAccess) {
        try {
          evidence.value = await stepUp.run(() => instructionAuditV2API.revealEventEvidence(eventID))
          trustFields.value = evidence.value.fields.map((field) => field.field_name)
        } catch (error) {
          if (!isStepUpCancelled(error)) evidenceError.value = extractApiErrorMessage(error, t('common.error'))
        }
      }
    } catch (error) {
      if (!isStepUpCancelled(error)) appStore.showError(extractApiErrorMessage(error, t('common.error')))
      close()
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

function close() {
  detail.value = null
  evidence.value = null
  emit('close')
}

async function copyEvidence(field: InstructionEvidenceField) {
  if (!detail.value || !field.plaintext) return
  try {
    await stepUp.run(() => instructionAuditV2API.recordEventEvidenceCopy(detail.value!.id, field.field_name))
    await copyToClipboard(field.plaintext)
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractApiErrorMessage(error, t('common.error')))
  }
}

async function trustSelected() {
  if (!detail.value || !trustFields.value.length) return
  trusting.value = true
  try {
    await stepUp.run(() => instructionAuditV2API.trustEvent(detail.value!.id, trustFields.value, trustName.value))
    appStore.showSuccess(t('admin.instructionAudit.v2.trustCreated'))
    emit('trusted')
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    trusting.value = false
  }
}
</script>

<style scoped>
.audit-detail-label {
  @apply text-[11px] font-semibold uppercase text-gray-400;
}

.audit-metric-cell {
  @apply min-w-0 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600;
}
</style>
