<template>
  <section class="min-w-0 space-y-7" data-test="instruction-v2-ai-settings">
    <div class="space-y-4">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.runtimeMode') }}</h2>
        <p class="mt-1 max-w-4xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.runtimeModeHint') }}</p>
      </div>

      <div class="grid min-w-0 gap-3 lg:grid-cols-3">
        <button v-for="option in modeOptions" :key="option.value" type="button" class="min-w-0 rounded-md border px-4 py-4 text-left transition" :class="form.mode === option.value ? 'border-primary-500 bg-primary-50 ring-2 ring-primary-500/10 dark:border-primary-600 dark:bg-primary-950/30' : 'border-gray-200 bg-white hover:border-primary-300 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-800'" @click="form.mode = option.value">
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm font-semibold text-gray-950 dark:text-white">{{ option.label }}</span>
            <span class="h-3 w-3 rounded-full border-2" :class="form.mode === option.value ? 'border-primary-600 bg-primary-600 ring-2 ring-primary-200 dark:ring-primary-900' : 'border-gray-300 dark:border-dark-500'" />
          </div>
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
        </button>
      </div>

      <div class="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div class="settings-status">
          <p class="settings-label">{{ t('admin.instructionAudit.v2.riskControlGate') }}</p>
          <p class="mt-1.5 text-sm font-semibold" :class="config.risk_control_enabled ? 'text-primary-700 dark:text-primary-300' : 'text-amber-700 dark:text-amber-300'">{{ config.risk_control_enabled ? t('common.enabled') : t('common.disabled') }}</p>
        </div>
        <div class="settings-status">
          <p class="settings-label">{{ t('admin.instructionAudit.v2.effectiveMode') }}</p>
          <span class="mt-1.5 inline-flex rounded-full px-2 py-1 text-xs font-semibold" :class="modePill(config.effective_mode)">{{ modeLabel(t, config.effective_mode) }}</span>
        </div>
        <div class="settings-status">
          <p class="settings-label">{{ t('admin.instructionAudit.v2.encryption') }}</p>
          <p class="mt-1.5 text-sm font-semibold" :class="config.evidence_encryption_ready ? 'text-primary-700 dark:text-primary-300' : 'text-red-600 dark:text-red-300'">{{ config.evidence_encryption_ready ? t('admin.instructionAudit.v2.ready') : t('admin.instructionAudit.v2.notReady') }}</p>
        </div>
        <div class="settings-status">
          <p class="settings-label">{{ t('admin.instructionAudit.v2.runtimeResources') }}</p>
          <p class="mt-1.5 text-sm text-gray-800 dark:text-gray-200">{{ config.active_scope_count }} {{ t('admin.instructionAudit.v2.scopesShort') }} · {{ config.active_hash_count }} {{ t('admin.instructionAudit.v2.hashesShort') }} · {{ config.enabled_ai_node_count }} AI</p>
        </div>
      </div>

      <div v-if="config.last_config_load_error" class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
        {{ config.last_config_load_error }}
      </div>
    </div>

    <div class="space-y-4 border-t border-gray-200 pt-6 dark:border-dark-700">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.aiNodes') }}</h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.aiNodesHint') }}</p>
        </div>
        <span class="rounded-full bg-primary-50 px-3 py-1.5 text-xs font-semibold text-primary-700 dark:bg-primary-950/40 dark:text-primary-300">{{ t('admin.instructionAudit.v2.fixedNodeSlots') }}</span>
      </div>

      <div class="node-grid">
        <article v-for="slotCard in slotCards" :key="slotCard.slot" class="node-card">
          <header class="flex min-w-0 items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ slotCard.label }}</h3>
                <span class="rounded-full bg-gray-100 px-2 py-0.5 font-mono text-[10px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ slotCard.slot }}</span>
                <span v-if="slotCard.node" class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="slotCard.node.enabled ? 'bg-primary-100 text-primary-700 dark:bg-primary-950/60 dark:text-primary-200' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300'">{{ slotCard.node.enabled ? t('common.enabled') : t('common.disabled') }}</span>
              </div>
              <p v-if="slotCard.node" class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ slotCard.node.name }} · <span class="font-mono text-primary-600 dark:text-primary-400">{{ slotCard.node.model }}</span></p>
              <p v-else class="mt-1 text-xs text-amber-700 dark:text-amber-300">{{ t('admin.instructionAudit.v2.slotNotConfigured') }}</p>
            </div>
            <div v-if="slotCard.node" class="flex shrink-0 items-center gap-1">
              <button type="button" class="icon-btn" :title="t('common.edit')" @click="openNodeForm(slotCard.node, slotCard.slot)"><Icon name="edit" size="sm" /></button>
              <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('common.delete')" @click="nodeToDelete = slotCard.node"><Icon name="trash" size="sm" /></button>
            </div>
          </header>
          <template v-if="slotCard.node">
          <div class="min-w-0 rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-800/70">
            <p class="settings-label">Base URL</p>
            <p class="mt-1 break-all font-mono text-[11px] text-gray-700 dark:text-gray-200">{{ slotCard.node.base_url }}</p>
          </div>
          <dl class="grid grid-cols-2 gap-3">
            <div><dt class="settings-label">{{ t('admin.instructionAudit.v2.timeout') }}</dt><dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ slotCard.node.timeout_ms }} ms</dd></div>
            <div><dt class="settings-label">{{ t('admin.instructionAudit.v2.concurrency') }}</dt><dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ slotCard.node.max_concurrency }}</dd></div>
            <div><dt class="settings-label">{{ t('admin.instructionAudit.v2.responseMode') }}</dt><dd class="mt-1 font-mono text-xs text-gray-800 dark:text-gray-200">{{ slotCard.node.response_mode }}</dd></div>
            <div><dt class="settings-label">{{ t('admin.instructionAudit.v2.maxOutputTokens') }}</dt><dd class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ slotCard.node.max_output_tokens }}</dd></div>
            <div><dt class="settings-label">API Key</dt><dd class="mt-1 text-sm" :class="slotCard.node.has_api_key ? 'text-primary-700 dark:text-primary-300' : 'text-red-600 dark:text-red-300'">{{ slotCard.node.has_api_key ? slotCard.node.api_key_status : t('admin.instructionAudit.v2.notConfigured') }}</dd></div>
          </dl>
          <div v-if="testResults[slotCard.node.id]" class="rounded-md border px-3 py-2 text-xs" :class="testResults[slotCard.node.id].result === 'pass' ? 'border-primary-200 bg-primary-50 text-primary-800 dark:border-primary-900/50 dark:bg-primary-950/20 dark:text-primary-200' : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-200'">
            <p class="font-semibold">{{ aiResultLabel(t, testResults[slotCard.node.id].result) }} · {{ Math.round(testResults[slotCard.node.id].confidence * 100) }}% · {{ testResults[slotCard.node.id].latency_ms }} ms</p>
            <p class="mt-1 break-words">{{ testResults[slotCard.node.id].reason }}</p>
          </div>
          <footer class="mt-auto border-t border-gray-100 pt-3 dark:border-dark-700">
            <button type="button" class="btn btn-secondary btn-sm w-full" :disabled="testingNodeId === slotCard.node.id || !slotCard.node.has_api_key" @click="testNode(slotCard.node)">
              <Icon name="beaker" size="sm" :class="{ 'animate-pulse': testingNodeId === slotCard.node.id }" />
              {{ testingNodeId === slotCard.node.id ? t('admin.instructionAudit.v2.testing') : t('admin.instructionAudit.v2.testNode') }}
            </button>
          </footer>
          </template>
          <button v-else type="button" class="btn btn-primary mt-auto w-full" @click="openNodeForm(undefined, slotCard.slot)"><Icon name="plus" size="sm" />{{ t('admin.instructionAudit.v2.configureSlot') }}</button>
        </article>
      </div>
    </div>

    <div class="space-y-4 border-t border-gray-200 pt-6 dark:border-dark-700">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.v2.reviewPolicy') }}</h2>
        <p class="mt-1 max-w-4xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.reviewPolicyHint') }}</p>
      </div>
      <label class="block min-w-0">
        <span class="input-label">{{ t('admin.instructionAudit.v2.reviewCriteria') }}</span>
        <textarea v-model="form.reviewCriteria" rows="6" maxlength="10000" class="input" :placeholder="t('admin.instructionAudit.v2.reviewCriteriaHint')" />
      </label>
      <div class="grid min-w-0 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <NumberField v-model="form.confidenceThreshold" :label="t('admin.instructionAudit.v2.confidenceThreshold')" :min="0.5" :max="1" :step="0.01" />
        <NumberField v-model="form.aiInputMaxChars" :label="t('admin.instructionAudit.v2.aiInputMaxChars')" :min="1000" :max="1000000" />
        <NumberField v-model="form.aiGlobalConcurrency" :label="t('admin.instructionAudit.v2.globalConcurrency')" :min="1" :max="512" />
        <NumberField v-model="form.aiQueueWaitMs" :label="t('admin.instructionAudit.v2.queueWait')" :min="0" :max="30000" suffix="ms" />
        <NumberField v-model="form.aiTotalTimeoutMs" :label="t('admin.instructionAudit.v2.totalTimeout')" :min="1000" :max="30000" suffix="ms" />
        <NumberField v-model="form.eventRetentionDays" :label="t('admin.instructionAudit.v2.eventRetention')" :min="1" :max="3650" :suffix="t('admin.instructionAudit.v2.days')" />
        <NumberField v-model="form.evidenceRetentionDays" :label="t('admin.instructionAudit.v2.evidenceRetention')" :min="1" :max="365" :suffix="t('admin.instructionAudit.v2.days')" />
        <label class="min-w-0 sm:col-span-2">
          <span class="input-label">{{ t('admin.instructionAudit.v2.retrySchedule') }}</span>
          <input v-model.trim="form.retrySchedule" class="input font-mono text-xs" placeholder="30, 120, 600, 3600, 21600" />
        </label>
        <label class="flex items-start gap-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600 sm:col-span-2">
          <input v-model="form.allowEmptyFields" type="checkbox" class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span>
            <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.instructionAudit.v2.allowEmptyFields') }}</span>
            <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.v2.allowEmptyFieldsHint') }}</span>
          </span>
        </label>
      </div>
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div class="settings-status"><p class="settings-label">HTTP {{ t('admin.instructionAudit.v2.gatewayLimit') }}</p><p class="mt-1.5 text-sm text-gray-800 dark:text-gray-200">{{ formatAuditBytes(config.gateway_http_max_body_bytes) }}</p></div>
        <div class="settings-status"><p class="settings-label">WebSocket {{ t('admin.instructionAudit.v2.gatewayLimit') }}</p><p class="mt-1.5 text-sm text-gray-800 dark:text-gray-200">{{ formatAuditBytes(config.gateway_ws_max_body_bytes) }}</p></div>
        <div class="settings-status"><p class="settings-label">{{ t('admin.instructionAudit.v2.pendingReviews') }}</p><p class="mt-1.5 text-sm text-gray-800 dark:text-gray-200">{{ config.pending_review_job_count }}</p></div>
        <div class="settings-status"><p class="settings-label">{{ t('admin.instructionAudit.v2.configVersion') }}</p><p class="mt-1.5 text-sm text-gray-800 dark:text-gray-200">v{{ config.config_version }}</p></div>
      </div>
      <div class="flex justify-end">
        <button type="button" class="btn btn-primary" :disabled="savingConfig" @click="saveConfig"><Icon name="check" size="sm" />{{ t('admin.instructionAudit.v2.saveConfiguration') }}</button>
      </div>
    </div>

    <BaseDialog :show="nodeForm.show" :title="nodeForm.id ? t('admin.instructionAudit.v2.editAINode') : t('admin.instructionAudit.v2.addAINode')" width="wide" @close="closeNodeForm">
      <div class="min-w-0 space-y-4">
        <div class="grid min-w-0 gap-4 sm:grid-cols-2">
          <label class="min-w-0"><span class="input-label">{{ t('common.name') }}</span><input v-model="nodeForm.name" maxlength="120" class="input" /></label>
          <label class="min-w-0"><span class="input-label">{{ t('admin.instructionAudit.v2.model') }}</span><input v-model="nodeForm.model" maxlength="255" class="input font-mono" /></label>
        </div>
        <div class="grid min-w-0 gap-4 sm:grid-cols-2">
          <label><span class="input-label">{{ t('admin.instructionAudit.v2.nodeSlot') }}</span><input :value="nodeForm.slot" class="input bg-gray-50 font-mono dark:bg-dark-700" disabled /></label>
          <label><span class="input-label">{{ t('admin.instructionAudit.v2.responseMode') }}</span><select v-model="nodeForm.responseMode" class="input"><option value="auto">auto</option><option value="json_schema">json_schema</option><option value="json_object">json_object</option></select></label>
        </div>
        <label class="block min-w-0"><span class="input-label">Base URL</span><input v-model="nodeForm.baseUrl" class="input font-mono text-xs" placeholder="https://api.example.com/v1" /></label>
        <label class="block min-w-0"><span class="input-label">API Key</span><input v-model="nodeForm.apiKey" type="password" autocomplete="new-password" class="input font-mono" :placeholder="nodeForm.hasApiKey ? t('admin.instructionAudit.v2.keepExistingKey') : 'sk-...'" /></label>
        <label v-if="nodeForm.id && nodeForm.hasApiKey" class="flex items-center gap-2 text-sm text-red-600 dark:text-red-300"><input v-model="nodeForm.clearApiKey" type="checkbox" class="rounded border-gray-300 text-red-600" />{{ t('admin.instructionAudit.v2.clearExistingKey') }}</label>
        <div class="grid min-w-0 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <label><span class="input-label">{{ t('admin.instructionAudit.v2.priority') }}</span><input v-model.number="nodeForm.priority" type="number" min="0" max="100000" class="input" /></label>
          <label><span class="input-label">{{ t('admin.instructionAudit.v2.timeout') }} (ms)</span><input v-model.number="nodeForm.timeoutMs" type="number" min="100" max="30000" class="input" /></label>
          <label><span class="input-label">{{ t('admin.instructionAudit.v2.concurrency') }}</span><input v-model.number="nodeForm.maxConcurrency" type="number" min="1" max="256" class="input" /></label>
          <label><span class="input-label">{{ t('admin.instructionAudit.v2.maxOutputTokens') }}</span><input v-model.number="nodeForm.maxOutputTokens" type="number" min="128" max="8192" class="input" /></label>
        </div>
        <label class="flex items-center justify-between gap-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600"><span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('common.enabled') }}</span><input v-model="nodeForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" /></label>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="savingNode" @click="closeNodeForm">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="savingNode || !nodeForm.name || !nodeForm.model || !nodeForm.baseUrl || (nodeForm.enabled && !nodeForm.apiKey && (!nodeForm.hasApiKey || nodeForm.clearApiKey))" @click="saveNode"><Icon name="check" size="sm" />{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="Boolean(nodeToDelete)" :title="t('admin.instructionAudit.v2.deleteAINodeTitle')" :message="t('admin.instructionAudit.v2.deleteAINodeConfirm')" danger @confirm="deleteNode" @cancel="nodeToDelete = null" />
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import instructionAuditV2API from '../v2Api'
import type {
  InstructionAINode,
  InstructionAINodeSlot,
  InstructionAIResponseMode,
  InstructionAINodeTestResult,
  InstructionAuditMode,
  InstructionV2Config,
} from '../v2Types'
import { aiResultLabel, formatAuditBytes, modeLabel, modePill } from '../v2Presentation'

const props = defineProps<{
  config: InstructionV2Config
  nodes: InstructionAINode[]
}>()
const emit = defineEmits<{
  (event: 'config-updated', config: InstructionV2Config): void
  (event: 'changed'): void
}>()
const { t } = useI18n()
const appStore = useAppStore()
const savingConfig = ref(false)
const savingNode = ref(false)
const testingNodeId = ref(0)
const nodeToDelete = ref<InstructionAINode | null>(null)
const testResults = reactive<Record<number, InstructionAINodeTestResult>>({})
const form = reactive({
  mode: 'off' as InstructionAuditMode,
  reviewCriteria: '',
  confidenceThreshold: 0.8,
  aiInputMaxChars: 64000,
  aiGlobalConcurrency: 64,
  aiQueueWaitMs: 2000,
  aiTotalTimeoutMs: 30000,
  aiCacheTtlSeconds: 600,
  eventRetentionDays: 30,
  evidenceRetentionDays: 7,
  rawFullMaxMiB: 4,
  allowEmptyFields: true,
  retrySchedule: '30, 120, 600, 3600, 21600',
})
const nodeForm = reactive({
  show: false,
  id: 0,
  hasApiKey: false,
  name: '',
  baseUrl: '',
  model: '',
  apiKey: '',
  clearApiKey: false,
  priority: 100,
  slot: 'sync' as InstructionAINodeSlot,
  responseMode: 'auto' as InstructionAIResponseMode,
  maxOutputTokens: 1024,
  enabled: true,
  timeoutMs: 15000,
  maxConcurrency: 16,
})

const nodeSlots: InstructionAINodeSlot[] = ['sync', 'async_1', 'async_2', 'async_3']
const slotCards = computed(() => nodeSlots.map((slot) => ({
  slot,
  label: t(`admin.instructionAudit.v2.nodeSlots.${slot}`),
  node: props.nodes.find((node) => node.slot === slot),
})))

const modeOptions = computed(() => [
  { value: 'off' as const, label: modeLabel(t, 'off'), description: t('admin.instructionAudit.v2.modeOffHint') },
  { value: 'observe' as const, label: modeLabel(t, 'observe'), description: t('admin.instructionAudit.v2.modeObserveHint') },
  { value: 'enforce' as const, label: modeLabel(t, 'enforce'), description: t('admin.instructionAudit.v2.modeEnforceHint') },
])

const NumberField = defineComponent({
  props: { modelValue: { type: Number, required: true }, label: { type: String, required: true }, min: Number, max: Number, step: { type: Number, default: 1 }, suffix: { type: String, default: '' } },
  emits: ['update:modelValue'],
  setup(componentProps, { emit: componentEmit }) {
    return () => h('label', { class: 'min-w-0' }, [
      h('span', { class: 'input-label' }, componentProps.label),
      h('div', { class: 'relative mt-1' }, [
        h('input', {
          type: 'number',
          value: componentProps.modelValue,
          min: componentProps.min,
          max: componentProps.max,
          step: componentProps.step,
          class: `input ${componentProps.suffix ? 'pr-12' : ''}`,
          onInput: (event: Event) => componentEmit('update:modelValue', Number((event.target as HTMLInputElement).value)),
        }),
        componentProps.suffix ? h('span', { class: 'pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-400' }, componentProps.suffix) : null,
      ]),
    ])
  },
})

watch(() => props.config, syncForm, { immediate: true, deep: true })

function syncForm() {
  Object.assign(form, {
    mode: props.config.mode,
    reviewCriteria: props.config.review_criteria,
    confidenceThreshold: props.config.confidence_threshold,
    aiInputMaxChars: props.config.ai_input_max_chars,
    aiGlobalConcurrency: props.config.ai_global_concurrency,
    aiQueueWaitMs: props.config.ai_queue_wait_ms,
    aiTotalTimeoutMs: props.config.ai_total_timeout_ms,
    aiCacheTtlSeconds: props.config.ai_cache_ttl_seconds,
    eventRetentionDays: props.config.event_retention_days,
    evidenceRetentionDays: props.config.evidence_retention_days,
    rawFullMaxMiB: props.config.raw_full_max_bytes / (1024 * 1024),
    allowEmptyFields: props.config.allow_empty_fields,
    retrySchedule: props.config.async_retry_schedule_seconds.join(', '),
  })
}

async function saveConfig() {
  const retrySchedule = form.retrySchedule.split(',').map((value) => Number(value.trim())).filter((value) => Number.isFinite(value))
  if (!retrySchedule.length || retrySchedule.some((value) => !Number.isInteger(value) || value < 1 || value > 604800)) {
    appStore.showError(t('admin.instructionAudit.v2.invalidRetrySchedule'))
    return
  }
  savingConfig.value = true
  try {
    const updated = await instructionAuditV2API.updateConfig({
      expected_config_version: props.config.config_version,
      mode: form.mode,
      review_criteria: form.reviewCriteria,
      confidence_threshold: form.confidenceThreshold,
      ai_input_max_chars: form.aiInputMaxChars,
      ai_global_concurrency: form.aiGlobalConcurrency,
      ai_queue_wait_ms: form.aiQueueWaitMs,
      ai_total_timeout_ms: form.aiTotalTimeoutMs,
      ai_cache_ttl_seconds: form.aiCacheTtlSeconds,
      event_retention_days: form.eventRetentionDays,
      evidence_retention_days: form.evidenceRetentionDays,
      raw_full_max_bytes: Math.round(form.rawFullMaxMiB * 1024 * 1024),
      allow_empty_fields: form.allowEmptyFields,
      async_retry_schedule_seconds: retrySchedule,
    })
    appStore.showSuccess(t('common.saved'))
    emit('config-updated', updated)
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    savingConfig.value = false
  }
}

function openNodeForm(node?: InstructionAINode, slot: InstructionAINodeSlot = 'sync') {
  Object.assign(nodeForm, node
    ? { show: true, id: node.id, hasApiKey: node.has_api_key, name: node.name, baseUrl: node.base_url, model: node.model, apiKey: '', clearApiKey: false, priority: node.priority, slot: node.slot, responseMode: node.response_mode, maxOutputTokens: node.max_output_tokens, enabled: node.enabled, timeoutMs: node.timeout_ms, maxConcurrency: node.max_concurrency }
    : { show: true, id: 0, hasApiKey: false, name: '', baseUrl: '', model: '', apiKey: '', clearApiKey: false, priority: nodeSlots.indexOf(slot) * 10, slot, responseMode: 'auto', maxOutputTokens: 1024, enabled: true, timeoutMs: 15000, maxConcurrency: 16 })
}

function closeNodeForm() {
  if (!savingNode.value) nodeForm.show = false
}

async function saveNode() {
  savingNode.value = true
  try {
    await instructionAuditV2API.saveAINode(nodeForm.id || null, {
      name: nodeForm.name,
      base_url: nodeForm.baseUrl,
      model: nodeForm.model,
      api_key: nodeForm.apiKey,
      clear_api_key: nodeForm.clearApiKey,
      priority: nodeForm.priority,
      slot: nodeForm.slot,
      response_mode: nodeForm.responseMode,
      max_output_tokens: nodeForm.maxOutputTokens,
      enabled: nodeForm.enabled,
      timeout_ms: nodeForm.timeoutMs,
      max_concurrency: nodeForm.maxConcurrency,
    })
    appStore.showSuccess(t('common.saved'))
    nodeForm.show = false
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    savingNode.value = false
  }
}

async function deleteNode() {
  if (!nodeToDelete.value) return
  try {
    await instructionAuditV2API.deleteAINode(nodeToDelete.value!.id)
    appStore.showSuccess(t('admin.instructionAudit.v2.aiNodeDeleted'))
    nodeToDelete.value = null
    emit('changed')
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  }
}

async function testNode(node: InstructionAINode) {
  testingNodeId.value = node.id
  try {
    testResults[node.id] = await instructionAuditV2API.testAINode(node.id)
    appStore.showSuccess(t('admin.instructionAudit.v2.nodeTestCompleted'))
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, t('common.error')))
  } finally {
    testingNodeId.value = 0
  }
}
</script>

<style scoped>
.settings-status {
  @apply min-w-0 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600;
}

.settings-label {
  @apply text-[11px] font-semibold uppercase text-gray-400;
}

.node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
  gap: 0.875rem;
}

.node-card {
  @apply flex min-w-0 flex-col gap-3 rounded-md border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800;
}
</style>
