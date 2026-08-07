<template>
  <BaseDialog
    :show="show"
    :title="t('admin.instructionAudit.evidenceReview')"
    width="wide"
    @close="close"
  >
    <div v-if="loading" class="flex min-h-56 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      <Icon name="refresh" size="md" class="mr-2 animate-spin" />
      {{ t('common.loading') }}
    </div>

    <div v-else-if="review && event" class="space-y-5">
      <div class="grid gap-3 rounded-md border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800 sm:grid-cols-[auto_minmax(0,1fr)_minmax(0,1fr)] sm:items-center">
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.eventInfo') }}</p>
          <div class="mt-1 flex items-center gap-1.5">
            <span class="text-sm font-semibold text-primary-700 dark:text-primary-300">{{ t('admin.instructionAudit.eventNumber', { id: event.id }) }}</span>
            <button type="button" class="icon-btn h-7 w-7" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copyValue('event_id', String(event.id))">
              <Icon name="copy" size="xs" />
            </button>
          </div>
        </div>
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.requestId') }}</p>
          <div class="mt-1 flex min-w-0 items-center gap-1.5">
            <p class="min-w-0 break-all font-mono text-sm text-gray-900 dark:text-white">{{ review.request_id || '-' }}</p>
            <button type="button" class="icon-btn h-7 w-7 shrink-0" :disabled="!review.request_id" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copyValue('request_id', review.request_id)">
              <Icon name="copy" size="xs" />
            </button>
          </div>
        </div>
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.client') }}</p>
          <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ clientTypeLabel(event.client_type) }}</p>
          <p v-if="event.client_user_agent" class="mt-0.5 truncate font-mono text-[11px] text-gray-500 dark:text-gray-400" :title="event.client_user_agent">
            {{ event.client_user_agent }}
          </p>
        </div>
      </div>

      <dl class="grid gap-3 rounded-md border border-gray-200 px-4 py-3 dark:border-dark-600 sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.finalOutcome') }}</dt>
          <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ outcomeLabel(event.final_outcome) }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.initialReason') }}</dt>
          <dd class="mt-1 break-words text-sm text-gray-900 dark:text-white">{{ reasonLabel(event.initial_reason || event.reason) }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.finalReason') }}</dt>
          <dd class="mt-1 break-words text-sm text-gray-900 dark:text-white">{{ reasonLabel(event.final_reason || event.reason) }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.performance') }}</dt>
          <dd class="mt-1 text-sm tabular-nums text-gray-900 dark:text-white">{{ formatBytes(event.body_bytes) }} · {{ event.audit_latency_ms ?? event.latency_ms }} ms<span v-if="event.ai_latency_ms != null"> · AI {{ event.ai_latency_ms }} ms</span></dd>
        </div>
      </dl>

      <div
        v-if="review.status !== 'stored'"
        class="rounded-md border px-4 py-3 text-sm"
        :class="review.status === 'encryption_unavailable'
          ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300'
          : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300'"
      >
        {{ evidenceStatusLabel(review.status) }}
      </div>

      <section v-for="field in reviewFields" :key="field.source" class="overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center gap-2">
            <h3 class="font-mono text-sm font-semibold text-gray-950 dark:text-white">{{ sourceLabel(field.source) }}</h3>
            <span :class="consistencyPill(field)">{{ consistencyLabel(field) }}</span>
          </div>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.instructionAudit.byteCount', { count: field.plaintext_bytes || 0 }) }}
          </span>
        </div>

        <div class="space-y-4 p-4">
          <div>
            <div class="mb-1.5 flex items-center justify-between gap-3">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">SHA-256</span>
              <button type="button" class="btn btn-ghost btn-sm" :disabled="!field.sha256" @click="copyValue(`${field.source}_hash`, field.sha256)">
                <Icon name="copy" size="sm" />
                {{ t('common.copy') }}
              </button>
            </div>
            <p class="break-all rounded-md bg-gray-100 px-3 py-2 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ field.sha256 || '-' }}</p>
          </div>

          <div v-if="field.available">
            <InstructionTranslationPanel
              resource-type="event"
              :resource-id="event.id"
              :field-name="field.source"
              :original="field.plaintext || ''"
              :enabled="translationEnabled"
              :external-enabled="externalTranslationEnabled"
              @copy-original="copyValue(`${field.source}_plaintext`, field.plaintext || '')"
            />
            <p class="mt-2 break-all text-[11px] text-gray-500 dark:text-gray-400">
              {{ t('admin.instructionAudit.recomputedHash') }}: <span class="font-mono">{{ field.recomputed_sha256 || '-' }}</span>
            </p>
          </div>
          <p v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.plaintextUnavailable') }}</p>

          <div v-if="field.available" class="flex justify-end">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="!reviewConfirmed || !field.digest_consistent"
              @click="$emit('candidate', field.source)"
            >
              <Icon name="plus" size="sm" />
              {{ t('admin.instructionAudit.addCandidate') }}
            </button>
          </div>
        </div>
      </section>

      <section class="space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.aiReviews.title') }}</h3>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.aiReviews.count', { count: aiReviews.length }) }}</span>
        </div>
        <div v-if="aiReviews.length" class="divide-y divide-gray-100 overflow-hidden rounded-md border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
          <article v-for="item in aiReviews" :key="item.id" class="grid min-w-0 gap-3 px-3 py-3 sm:grid-cols-[minmax(120px,0.6fr)_minmax(0,1fr)_auto]">
            <div>
              <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ aiResultLabel(item.result) }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ sourceLabel(item.reviewed_source as 'instructions' | 'input1') }} · {{ (item.confidence * 100).toFixed(1) }}%</p>
            </div>
            <div class="min-w-0">
              <p class="break-words text-sm text-gray-700 dark:text-gray-200">{{ item.reason || '-' }}</p>
              <p class="mt-1 break-all font-mono text-[11px] text-gray-400">{{ item.reviewer_model }} · {{ item.prompt_version }} · {{ item.latency_ms }} ms</p>
              <p v-if="item.automatic_hash_id" class="mt-1 text-xs text-primary-600 dark:text-primary-400">{{ t('admin.instructionAudit.aiReviews.hashCreated', { id: item.automatic_hash_id }) }}</p>
            </div>
            <time class="text-xs text-gray-400">{{ formatDate(item.created_at) }}</time>
          </article>
        </div>
        <p v-else class="rounded-md border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('admin.instructionAudit.aiReviews.empty') }}</p>
      </section>

      <label v-if="review.status === 'stored'" class="flex cursor-pointer items-start gap-3 rounded-md border border-primary-200 bg-primary-50/60 px-4 py-3 dark:border-primary-900/60 dark:bg-primary-950/20">
        <input v-model="reviewConfirmed" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.reviewConfirmation') }}</span>
      </label>

      <div class="flex flex-wrap items-center justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.instructionAudit.evidenceAccessCount', { count: review.access_count }) }}</span>
        <span v-if="review.expires_at">{{ t('admin.instructionAudit.evidenceExpiresAt') }}: {{ formatDate(review.expires_at) }}</span>
      </div>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="close">{{ t('common.close') }}</button>
      <button v-if="review && event" type="button" class="btn btn-primary" @click="copyReviewBundle">
        <Icon name="copy" size="sm" />
        {{ t('admin.instructionAudit.copyReviewBundle') }}
      </button>
    </template>
    <TotpStepUpDialog :controller="stepUp" />
  </BaseDialog>
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
import { extractI18nErrorMessage } from '@/utils/apiError'
import instructionAuditAPI from './api'
import InstructionTranslationPanel from './components/InstructionTranslationPanel.vue'
import type {
  InstructionAIReview,
  InstructionEvidenceField,
  InstructionEvidenceReview,
  InstructionEvidenceStatus,
  InstructionEvent,
} from './types'

const props = defineProps<{
  show: boolean
  event: InstructionEvent | null
  translationEnabled: boolean
  externalTranslationEnabled: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'candidate', source: 'instructions' | 'input1'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const stepUp = useStepUp()
const loading = ref(false)
const review = ref<InstructionEvidenceReview | null>(null)
const reviewConfirmed = ref(false)
const aiReviews = ref<InstructionAIReview[]>([])

const reviewFields = computed<InstructionEvidenceField[]>(() => {
  if (review.value?.fields?.length) return review.value.fields
  if (!props.event) return []
  const fallback: InstructionEvidenceField[] = [
    {
      source: 'instructions', available: false, plaintext: '',
      sha256: props.event.instructions.sha256, plaintext_bytes: 0,
      recomputed_sha256: '', digest_consistent: false,
    },
    {
      source: 'input1', available: false, plaintext: '',
      sha256: props.event.input1.sha256, plaintext_bytes: 0,
      recomputed_sha256: '', digest_consistent: false,
    },
  ]
  return fallback.filter((field) => field.sha256)
})

watch(
  () => [props.show, props.event?.id] as const,
  async ([show, eventId]) => {
    review.value = null
    reviewConfirmed.value = false
    aiReviews.value = []
    if (!show || !eventId) return
    loading.value = true
    try {
      const [nextReview, nextAIReviews] = await Promise.all([
        stepUp.run(() => instructionAuditAPI.revealEvidence(eventId)),
        instructionAuditAPI.listEventAIReviews(eventId),
      ])
      review.value = nextReview
      aiReviews.value = nextAIReviews
    } catch (error) {
      if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
      emit('close')
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

function close() {
  review.value = null
  reviewConfirmed.value = false
  aiReviews.value = []
  emit('close')
}

async function copyValue(source: string, value: string) {
  if (!props.event || !value) return
  try {
    await stepUp.run(() => instructionAuditAPI.recordEvidenceCopy(props.event!.id, source))
    await copyToClipboard(value)
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  }
}

async function copyReviewBundle() {
  if (!props.event || !review.value) return
  const bundle = {
    event_id: props.event.id,
    request_id: props.event.request_id,
    created_at: props.event.created_at,
    user_id: props.event.user_id,
    user_email: props.event.user_email,
    api_key_id: props.event.api_key_id,
    group_id: props.event.group_id,
    group_name: props.event.group_name,
    client_type: props.event.client_type,
    client_user_agent: props.event.client_user_agent,
    model: props.event.model,
    reason: props.event.reason,
    evidence_status: review.value.status,
    evidence: reviewFields.value,
  }
  await copyValue('review_bundle', JSON.stringify(bundle, null, 2))
}

function sourceLabel(source: 'instructions' | 'input1'): string {
  return source === 'instructions' ? t('admin.instructionAudit.fieldOne') : t('admin.instructionAudit.fieldTwo')
}

function clientTypeLabel(clientType: string): string {
  return t(`admin.instructionAudit.clients.${clientType}`, clientType)
}

function evidenceStatusLabel(status: InstructionEvidenceStatus): string {
  return t(`admin.instructionAudit.evidenceStatuses.${status}`)
}

function consistencyLabel(field: InstructionEvidenceField): string {
  if (!field.available) return t('admin.instructionAudit.notVerifiable')
  return field.digest_consistent
    ? t('admin.instructionAudit.hashConsistent')
    : t('admin.instructionAudit.hashInconsistent')
}

function consistencyPill(field: InstructionEvidenceField): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium'
  if (!field.available) return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  if (field.digest_consistent) return `${base} bg-primary-50 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300`
  return `${base} bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300`
}

function formatDate(value: string): string {
  return new Date(value).toLocaleString()
}

function reasonLabel(reason: string): string {
  return reason ? t(`admin.instructionAudit.reasons.${reason}`, reason) : '-'
}

function outcomeLabel(outcome: string): string {
  return outcome ? t(`admin.instructionAudit.outcomes.${outcome}`, outcome) : '-'
}

function aiResultLabel(result: string): string {
  return t(`admin.instructionAudit.aiReviews.results.${result}`, result)
}

function formatBytes(value?: number | null): string {
  if (value == null) return '-'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(2)} MiB`
}
</script>
