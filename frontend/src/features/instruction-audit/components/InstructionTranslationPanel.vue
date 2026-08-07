<template>
  <div class="grid min-w-0 gap-3 lg:grid-cols-2" data-test="instruction-translation-panel">
    <section class="min-w-0 overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
        <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.translation.original') }}</span>
        <button type="button" class="btn btn-ghost btn-sm" @click="$emit('copy-original')">
          <Icon name="copy" size="sm" />
          {{ t('common.copy') }}
        </button>
      </div>
      <pre class="max-h-80 min-h-40 overflow-auto whitespace-pre-wrap break-words p-3 font-mono text-xs leading-5 text-gray-800 dark:text-gray-100">{{ original }}</pre>
    </section>

    <section class="min-w-0 overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
        <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.translation.translated') }}</span>
        <button v-if="job?.translated_text" type="button" class="btn btn-ghost btn-sm" @click="copyTranslation">
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
        <button type="button" class="btn btn-primary self-start" :disabled="!enabled || starting || !original" @click="start">
          <Icon name="globe" size="sm" />
          {{ starting ? t('common.loading') : t('admin.instructionAudit.translation.translate') }}
        </button>
        <p v-if="!enabled" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.instructionAudit.translation.disabledHint') }}</p>
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
import type { InstructionTranslationJob } from '../types'

const props = defineProps<{
  resourceType: 'event' | 'hash'
  resourceId: number
  fieldName: 'instructions' | 'input1'
  original: string
  enabled: boolean
  externalEnabled: boolean
}>()
defineEmits<{ (event: 'copy-original'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const stepUp = useStepUp()
const targetLanguage = ref('zh-CN')
const provider = ref<'internal' | 'external'>('internal')
const starting = ref(false)
const job = ref<InstructionTranslationJob | null>(null)
let pollTimer: ReturnType<typeof setTimeout> | null = null

const isRunning = computed(() => job.value?.status === 'pending' || job.value?.status === 'retry' || job.value?.status === 'processing')
const progress = computed(() => {
  if (!job.value?.chunk_count) return 8
  return Math.max(8, Math.min(100, Math.round(job.value.completed_chunks / job.value.chunk_count * 100)))
})
const translationError = computed(() => {
  if (!job.value) return t('admin.instructionAudit.translation.failed')
  if (job.value.status === 'expired') return t('admin.instructionAudit.translation.expired')
  return job.value.error_code || t('admin.instructionAudit.translation.failed')
})

watch(() => [props.resourceType, props.resourceId, props.fieldName], reset)
watch(() => props.externalEnabled, (enabled) => {
  if (!enabled && provider.value === 'external') provider.value = 'internal'
})

async function start() {
  if (!props.enabled || !props.original || starting.value) return
  starting.value = true
  try {
    job.value = await stepUp.run(() => instructionAuditAPI.createTranslation({
      resource_type: props.resourceType,
      resource_id: props.resourceId,
      field_name: props.fieldName,
      target_language: targetLanguage.value,
      provider: provider.value,
    }))
    schedulePoll()
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  } finally {
    starting.value = false
  }
}

function schedulePoll() {
  clearPoll()
  if (!job.value || !isRunning.value) return
  pollTimer = setTimeout(poll, 800)
}

async function poll() {
  if (!job.value) return
  try {
    job.value = await stepUp.run(() => instructionAuditAPI.getTranslation(job.value!.id))
    schedulePoll()
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error')))
  }
}

async function copyTranslation() {
  if (job.value?.translated_text) await copyToClipboard(job.value.translated_text)
}

function clearPoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

function reset() {
  clearPoll()
  job.value = null
}

onBeforeUnmount(clearPoll)
</script>
