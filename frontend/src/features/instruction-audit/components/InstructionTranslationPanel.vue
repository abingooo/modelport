<template>
  <div class="grid min-w-0 gap-3 lg:grid-cols-2" data-test="instruction-translation-panel">
    <section class="min-w-0 overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
        <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.translation.original') }}</span>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="!sensitiveAccess" :title="sensitiveActionTitle" @click="copyOriginal">
          <Icon :name="sensitiveAccess ? 'copy' : 'lock'" size="sm" />
          {{ t('common.copy') }}
        </button>
      </div>
      <pre v-if="sensitiveAccess" class="max-h-80 min-h-40 overflow-auto whitespace-pre-wrap break-words p-3 font-mono text-xs leading-5 text-gray-800 dark:text-gray-100">{{ original }}</pre>
      <div v-else class="flex min-h-40 items-center justify-center gap-2 p-4 text-center text-sm text-amber-700 dark:text-amber-300">
        <Icon name="lock" size="sm" />
        {{ t('admin.instructionAudit.sensitiveAccess.lockedHint') }}
      </div>
    </section>

    <section class="min-w-0 overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
        <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.translation.translated') }}</span>
        <button v-if="job?.translated_text && sensitiveAccess" type="button" class="btn btn-ghost btn-sm" @click="copyTranslation">
          <Icon name="copy" size="sm" />
          {{ t('common.copy') }}
        </button>
      </div>
      <div v-if="!job" class="flex min-h-40 flex-col justify-center gap-3 p-3">
        <div class="grid gap-2 sm:grid-cols-2">
          <label>
            <span class="input-label">{{ t('admin.instructionAudit.translation.targetLanguage') }}</span>
            <select v-model="targetLanguage" class="input h-9 py-1.5 text-sm">
              <option value="zh-CN">{{ t('admin.instructionAudit.translation.languages.zhCN') }}</option>
              <option value="en">{{ t('admin.instructionAudit.translation.languages.en') }}</option>
              <option value="ja">{{ t('admin.instructionAudit.translation.languages.ja') }}</option>
              <option value="ko">{{ t('admin.instructionAudit.translation.languages.ko') }}</option>
            </select>
          </label>
          <label>
            <span class="input-label">{{ t('admin.instructionAudit.translation.provider') }}</span>
            <select v-model="provider" class="input h-9 py-1.5 text-sm">
              <option value="internal">{{ t('admin.instructionAudit.translation.internal') }}</option>
              <option value="external" :disabled="!externalEnabled">{{ t('admin.instructionAudit.translation.external') }}</option>
            </select>
          </label>
        </div>
        <button type="button" class="btn btn-primary self-start" :disabled="!enabled || !sensitiveAccess || starting || !original" :title="sensitiveActionTitle" @click="start">
          <Icon :name="sensitiveAccess ? 'globe' : 'lock'" size="sm" />
          {{ starting ? t('common.loading') : t('admin.instructionAudit.translation.translate') }}
        </button>
        <p v-if="startError" role="alert" class="text-xs text-red-700 dark:text-red-300">{{ startError }}</p>
        <p v-if="!sensitiveAccess" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.instructionAudit.sensitiveAccess.lockedHint') }}</p>
        <p v-if="!enabled" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.instructionAudit.translation.disabledHint') }}</p>
      </div>
      <div v-else-if="pollError" class="flex min-h-40 flex-col justify-center gap-3 p-4" data-test="translation-poll-error" role="alert">
        <div class="flex items-start gap-2">
          <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0 text-amber-600 dark:text-amber-300" />
          <div class="min-w-0">
            <p class="text-sm font-medium text-gray-900 dark:text-white">{{ pollStatusLabel }}</p>
            <p class="mt-1 break-words text-xs text-red-700 dark:text-red-300">{{ pollError }}</p>
            <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">
              {{ t('admin.instructionAudit.translation.jobReference', { id: job.id }) }}
            </p>
          </div>
        </div>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-primary btn-sm" data-test="translation-continue" :disabled="polling" @click="continuePolling">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': polling }" />
            {{ t('admin.instructionAudit.translation.continueQuery') }}
          </button>
          <button type="button" class="btn btn-secondary btn-sm" data-test="translation-restart" :disabled="polling" @click="reset">
            {{ t('admin.instructionAudit.translation.restart') }}
          </button>
        </div>
      </div>
      <div v-else-if="isRunning" class="flex min-h-40 flex-col justify-center p-4" role="status">
        <div class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <Icon name="refresh" size="sm" class="animate-spin" />
          {{ t('admin.instructionAudit.translation.processing') }}
        </div>
        <div class="mt-3 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div class="h-full rounded-full bg-primary-500 transition-all" :style="{ width: `${progress}%` }" />
        </div>
        <p class="mt-2 text-xs tabular-nums text-gray-500 dark:text-gray-400">{{ job.completed_chunks }} / {{ job.chunk_count || '?' }}</p>
      </div>
      <div v-else-if="job.translated_text" class="min-h-40">
        <pre class="max-h-80 overflow-auto whitespace-pre-wrap break-words p-3 font-mono text-xs leading-5 text-gray-800 dark:text-gray-100">{{ job.translated_text }}</pre>
        <div v-if="job.status === 'partial'" class="border-t border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300">
          {{ t('admin.instructionAudit.translation.partial') }}
        </div>
      </div>
      <div v-else class="flex min-h-40 flex-col justify-center gap-3 p-4">
        <p class="text-sm text-red-700 dark:text-red-300">{{ translationError }}</p>
        <button type="button" class="btn btn-secondary self-start" @click="reset">{{ t('admin.instructionAudit.translation.retry') }}</button>
      </div>
    </section>
    <TotpStepUpDialog :controller="stepUp" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import instructionAuditAPI from '../api'
import { isInstructionSensitiveAccessDenied } from '../sensitiveAccess'
import type { InstructionTranslationJob } from '../types'

const props = withDefaults(defineProps<{
  resourceType: 'event' | 'hash'
  resourceId: number
  fieldName: 'instructions' | 'input1'
  original: string
  enabled: boolean
  externalEnabled: boolean
  sensitiveAccess?: boolean
}>(), { sensitiveAccess: true })
const emit = defineEmits<{
  (event: 'copy-original'): void
  (event: 'access-denied', error?: unknown): void
}>()
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const stepUp = useStepUp()
const targetLanguage = ref('zh-CN')
const provider = ref<'internal' | 'external'>('internal')
const starting = ref(false)
const job = ref<InstructionTranslationJob | null>(null)
const polling = ref(false)
const pollFailureCount = ref(0)
const pollPhase = ref<'idle' | 'scheduled' | 'retrying' | 'paused'>('idle')
const pollError = ref('')
const startError = ref('')
let pollTimer: ReturnType<typeof setTimeout> | null = null

const INITIAL_POLL_DELAY_MS = 800
const POLL_RETRY_DELAYS_MS = [1000, 2000] as const
const MAX_CONSECUTIVE_POLL_FAILURES = POLL_RETRY_DELAYS_MS.length + 1
const knownTranslationErrorCodes = new Set([
  'provider_timeout', 'provider_unavailable', 'provider_status', 'invalid_response',
  'invalid_input', 'internal_provider_unavailable', 'external_provider_disabled',
  'invalid_provider', 'encryption_unavailable', 'result_store_unavailable',
  'source_too_large', 'source_expired', 'source_integrity_failed', 'source_unavailable',
  'result_expired', 'result_too_large', 'worker_panic', 'translation_failed',
])

const isRunning = computed(() => job.value?.status === 'pending' || job.value?.status === 'retry' || job.value?.status === 'processing')
const progress = computed(() => {
  if (!job.value?.chunk_count) return 8
  return Math.max(8, Math.min(100, Math.round(job.value.completed_chunks / job.value.chunk_count * 100)))
})
const translationError = computed(() => {
  if (!job.value) return t('admin.instructionAudit.translation.failed')
  if (job.value.status === 'expired') return t('admin.instructionAudit.translation.expired')
  if (knownTranslationErrorCodes.has(job.value.error_code)) {
    return t(`admin.instructionAudit.translation.errors.${job.value.error_code}`)
  }
  return t('admin.instructionAudit.translation.failed')
})
const pollStatusLabel = computed(() => {
  if (pollPhase.value === 'retrying') {
    return t('admin.instructionAudit.translation.pollRetrying', {
      current: pollFailureCount.value,
      total: MAX_CONSECUTIVE_POLL_FAILURES,
    })
  }
  return t('admin.instructionAudit.translation.pollPaused')
})
const sensitiveActionTitle = computed(() => props.sensitiveAccess
  ? ''
  : t('admin.instructionAudit.sensitiveAccess.lockedHint'))

watch(() => [props.resourceType, props.resourceId, props.fieldName], reset)
watch(() => props.sensitiveAccess, (allowed) => {
  if (!allowed) reset()
})
watch(() => props.externalEnabled, (enabled) => {
  if (!enabled && provider.value === 'external') provider.value = 'internal'
})

async function start() {
  if (!props.enabled || !props.sensitiveAccess || !props.original || starting.value) return
  starting.value = true
  startError.value = ''
  try {
    job.value = await stepUp.run(() => instructionAuditAPI.createTranslation({
      resource_type: props.resourceType,
      resource_id: props.resourceId,
      field_name: props.fieldName,
      target_language: targetLanguage.value,
      provider: provider.value,
    }))
    pollFailureCount.value = 0
    pollError.value = ''
    schedulePoll(INITIAL_POLL_DELAY_MS)
  } catch (error) {
    if (isInstructionSensitiveAccessDenied(error)) {
      reset()
      emit('access-denied', error)
      return
    }
    if (!isStepUpCancelled(error)) {
      startError.value = extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error'))
      appStore.showError(startError.value)
    }
  } finally {
    starting.value = false
  }
}

function schedulePoll(delay = INITIAL_POLL_DELAY_MS, phase: 'scheduled' | 'retrying' = 'scheduled') {
  clearPoll()
  if (!job.value || !isRunning.value) return
  pollPhase.value = phase
  pollTimer = setTimeout(() => void poll(), delay)
}

async function poll() {
  if (!job.value || !isRunning.value || polling.value) return
  clearPoll()
  polling.value = true
  try {
    job.value = await stepUp.run(() => instructionAuditAPI.getTranslation(job.value!.id))
    pollFailureCount.value = 0
    pollError.value = ''
    pollPhase.value = 'idle'
    schedulePoll(INITIAL_POLL_DELAY_MS)
  } catch (error) {
    if (isInstructionSensitiveAccessDenied(error)) {
      reset()
      emit('access-denied', error)
      return
    }
    pollFailureCount.value += 1
    if (isStepUpCancelled(error)) {
      pollError.value = t('admin.instructionAudit.translation.pollVerificationCancelled')
      pollPhase.value = 'paused'
      return
    }
    pollError.value = extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('admin.instructionAudit.translation.pollFailed'))
    if (pollFailureCount.value >= MAX_CONSECUTIVE_POLL_FAILURES) {
      pollPhase.value = 'paused'
      return
    }
    const retryDelay = POLL_RETRY_DELAYS_MS[pollFailureCount.value - 1]
    schedulePoll(retryDelay, 'retrying')
  } finally {
    polling.value = false
  }
}

function continuePolling() {
  if (!job.value || !isRunning.value || polling.value) return
  clearPoll()
  pollFailureCount.value = 0
  pollError.value = ''
  pollPhase.value = 'idle'
  void poll()
}

async function copyTranslation() {
  if (props.sensitiveAccess && job.value?.translated_text) await copyToClipboard(job.value.translated_text)
}

function copyOriginal() {
  if (props.sensitiveAccess) emit('copy-original')
}

function clearPoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

function reset() {
  clearPoll()
  job.value = null
  polling.value = false
  pollFailureCount.value = 0
  pollPhase.value = 'idle'
  pollError.value = ''
  startError.value = ''
}

onBeforeUnmount(clearPoll)
</script>
