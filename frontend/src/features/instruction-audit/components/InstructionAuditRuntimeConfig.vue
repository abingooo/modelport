<template>
  <section class="card overflow-hidden" data-test="instruction-audit-runtime-config">
    <div class="border-b border-gray-100 px-4 py-4 dark:border-dark-700 sm:px-6">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.runtime.title') }}</h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.description') }}</p>
        </div>
        <div v-if="config" class="text-right text-xs text-gray-500 dark:text-gray-400">
          <p>{{ t('admin.instructionAudit.configVersion') }} {{ config.config_version }}</p>
          <p class="mt-1">{{ formatDate(config.updated_at) }}</p>
        </div>
      </div>
    </div>

    <InstructionAuditResourceState
      :loading="loading"
      :loaded="Boolean(config)"
      :error="error"
      :has-data="Boolean(draft)"
      :empty-description="t('admin.instructionAudit.states.runtimeEmpty')"
      :disabled="overview?.enabled === false"
      :skeleton-rows="4"
    >
    <form v-if="draft" class="divide-y divide-gray-100 dark:divide-dark-700" @submit.prevent="submit">
      <section class="space-y-4 px-4 py-5 sm:px-6">
        <div>
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.runtime.parserTitle') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.parserHint') }}</p>
        </div>
        <div class="grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(min(100%, 12rem), 1fr));">
          <NumberField v-model="maxBodyMiB" :label="t('admin.instructionAudit.runtime.maxBodyMiB')" :min="1" :max="128" />
          <NumberField v-model="draft.parse_timeout_ms" :label="t('admin.instructionAudit.runtime.parseTimeout')" :min="50" :max="5000" suffix="ms" />
          <NumberField v-model="maxInflightMiB" :label="t('admin.instructionAudit.runtime.maxInflightMiB')" :min="minimumInflightMiB" :max="2048" />
          <NumberField v-model="draft.pass_event_retention_days" :label="t('admin.instructionAudit.runtime.passRetention')" :min="1" :max="90" :suffix="t('admin.instructionAudit.days')" />
          <NumberField v-model="draft.aggregate_retention_days" :label="t('admin.instructionAudit.runtime.aggregateRetention')" :min="30" :max="3650" :suffix="t('admin.instructionAudit.days')" />
          <NumberField v-model="draft.raw_content_retention_days" :label="t('admin.instructionAudit.runtime.rawRetention')" :min="1" :max="3650" :suffix="t('admin.instructionAudit.days')" />
        </div>
        <dl v-if="overview" class="grid gap-x-6 gap-y-2 border-t border-gray-100 pt-4 text-xs dark:border-dark-700 sm:grid-cols-2 xl:grid-cols-4">
          <div>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.configuredBodyLimit') }}</dt>
            <dd class="mt-1 font-medium tabular-nums text-gray-900 dark:text-white">{{ formatBytes(overview.max_body_bytes) }}</dd>
          </div>
          <div>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.httpEffectiveLimit') }}</dt>
            <dd class="mt-1 font-medium tabular-nums text-gray-900 dark:text-white">{{ formatBytes(overview.effective_http_max_body_bytes) }}</dd>
            <p class="mt-0.5 text-gray-400 dark:text-gray-500">{{ t('admin.instructionAudit.runtime.gatewayLimit', { value: formatBytes(overview.http_gateway_max_body_bytes) }) }}</p>
          </div>
          <div>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.websocketEffectiveLimit') }}</dt>
            <dd class="mt-1 font-medium tabular-nums text-gray-900 dark:text-white">{{ formatBytes(overview.effective_websocket_max_body_bytes) }}</dd>
            <p class="mt-0.5 text-gray-400 dark:text-gray-500">{{ t('admin.instructionAudit.runtime.gatewayLimit', { value: formatBytes(overview.websocket_gateway_max_body_bytes) }) }}</p>
          </div>
          <div>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.externalLimit') }}</dt>
            <dd class="mt-1 text-gray-600 dark:text-gray-300">{{ t('admin.instructionAudit.runtime.externalLimitHint') }}</dd>
          </div>
        </dl>
      </section>

      <section class="space-y-4 px-4 py-5 sm:px-6">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.runtime.aiTitle') }}</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.aiHint') }}</p>
          </div>
          <Toggle v-model="draft.ai_enabled" :aria-label="t('admin.instructionAudit.runtime.aiEnabled')" />
        </div>
        <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" :class="{ 'opacity-70': !draft.ai_enabled }">
          <label class="sm:col-span-2 xl:col-span-2">
            <span class="input-label">{{ t('admin.instructionAudit.runtime.baseURL') }}</span>
            <input v-model.trim="draft.ai_base_url" type="url" class="input" placeholder="https://example.com/v1" />
          </label>
          <label>
            <span class="input-label">{{ t('admin.instructionAudit.model') }}</span>
            <input v-model.trim="draft.ai_model" class="input" autocomplete="off" />
          </label>
          <SecretField v-model="draft.ai_token" v-model:clear="draft.clear_ai_token" :configured="config?.ai_has_token || false" :label="t('admin.instructionAudit.runtime.apiToken')" />
          <NumberField v-model="draft.ai_timeout_ms" :label="t('admin.instructionAudit.runtime.timeout')" :min="100" :max="30000" suffix="ms" />
          <NumberField v-model="draft.ai_max_concurrency" :label="t('admin.instructionAudit.runtime.concurrency')" :min="1" :max="64" />
          <NumberField v-model="draft.ai_min_confidence" :label="t('admin.instructionAudit.runtime.minConfidence')" :min="0.5" :max="1" :step="0.01" />
          <NumberField v-model="draft.ai_per_user_rpm" :label="t('admin.instructionAudit.runtime.perUserRPM')" :min="1" :max="120" />
          <NumberField v-model="draft.ai_per_user_daily_limit" :label="t('admin.instructionAudit.runtime.perUserDaily')" :min="1" :max="1000" />
          <NumberField v-model="draft.ai_global_daily_limit" :label="t('admin.instructionAudit.runtime.globalDaily')" :min="1" :max="100000" />
          <label class="sm:col-span-2">
            <span class="input-label">{{ t('admin.instructionAudit.runtime.promptVersion') }}</span>
            <input v-model.trim="draft.ai_prompt_version" class="input" maxlength="120" />
          </label>
        </div>
      </section>

      <section class="space-y-4 px-4 py-5 sm:px-6">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.runtime.translationTitle') }}</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.translationHint') }}</p>
          </div>
          <Toggle v-model="draft.translation_enabled" :aria-label="t('admin.instructionAudit.runtime.translationEnabled')" />
        </div>
        <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" :class="{ 'opacity-70': !draft.translation_enabled }">
          <label class="flex items-center justify-between gap-4 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600 sm:col-span-2 xl:col-span-4">
            <span>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.instructionAudit.runtime.externalTranslation') }}</span>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.runtime.externalTranslationHint') }}</span>
            </span>
            <Toggle v-model="draft.external_translation_enabled" :disabled="!draft.translation_enabled" />
          </label>
          <label class="sm:col-span-2">
            <span class="input-label">{{ t('admin.instructionAudit.runtime.baseURL') }}</span>
            <input v-model.trim="draft.translation_base_url" type="url" class="input" placeholder="https://example.com/v1" :disabled="!draft.external_translation_enabled" />
          </label>
          <label>
            <span class="input-label">{{ t('admin.instructionAudit.model') }}</span>
            <input v-model.trim="draft.translation_model" class="input" autocomplete="off" :disabled="!draft.external_translation_enabled" />
          </label>
          <SecretField v-model="draft.translation_token" v-model:clear="draft.clear_translation_token" :configured="config?.translation_has_token || false" :disabled="!draft.external_translation_enabled" :label="t('admin.instructionAudit.runtime.apiToken')" />
          <NumberField v-model="draft.translation_timeout_ms" :label="t('admin.instructionAudit.runtime.timeout')" :min="100" :max="60000" suffix="ms" />
          <NumberField v-model="draft.translation_max_concurrency" :label="t('admin.instructionAudit.runtime.concurrency')" :min="1" :max="16" />
          <NumberField v-model="draft.translation_chunk_bytes" :label="t('admin.instructionAudit.runtime.chunkBytes')" :min="1024" :max="65536" suffix="B" />
          <NumberField v-model="draft.translation_max_bytes" :label="t('admin.instructionAudit.runtime.translationMaxBytes')" :min="draft.translation_chunk_bytes" :max="1048576" suffix="B" />
          <NumberField v-model="draft.translation_result_ttl_seconds" :label="t('admin.instructionAudit.runtime.resultTTL')" :min="60" :max="86400" suffix="s" />
        </div>
      </section>

      <div class="flex flex-wrap items-center justify-between gap-3 bg-gray-50 px-4 py-4 dark:bg-dark-800/70 sm:px-6">
        <p class="text-sm" :class="dirty ? 'text-amber-700 dark:text-amber-300' : 'text-gray-500 dark:text-gray-400'">
          {{ dirty ? t('admin.instructionAudit.runtime.unsaved') : t('admin.instructionAudit.runtime.synced') }}
        </p>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary" :disabled="!dirty || saving" @click="reset">{{ t('common.reset') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="!dirty || !valid || saving">
            <Icon name="check" size="sm" />
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </div>
    </form>
    </InstructionAuditResourceState>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import InstructionAuditResourceState from './InstructionAuditResourceState.vue'
import type { InstructionOverview, InstructionRuntimeConfig, UpdateInstructionRuntimeConfigRequest } from '../types'

const MIB = 1024 * 1024
const props = defineProps<{ config: InstructionRuntimeConfig | null; overview: InstructionOverview | null; loading: boolean; saving: boolean; error: string }>()
const emit = defineEmits<{ (event: 'save', payload: UpdateInstructionRuntimeConfigRequest): void }>()
const { t } = useI18n()
const draft = ref<UpdateInstructionRuntimeConfigRequest | null>(null)
const baseline = ref('')

const NumberField = defineComponent({
  props: {
    modelValue: { type: Number, required: true }, label: { type: String, required: true },
    min: { type: Number, required: true }, max: { type: Number, required: true }, step: { type: Number, default: 1 }, suffix: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(fieldProps, { emit: fieldEmit }) {
    return () => h('label', { class: 'min-w-0' }, [
      h('span', { class: 'input-label' }, fieldProps.label),
      h('span', { class: 'relative block' }, [
        h('input', {
          type: 'number', class: ['input tabular-nums', fieldProps.suffix ? 'pr-12' : ''], value: fieldProps.modelValue,
          min: fieldProps.min, max: fieldProps.max, step: fieldProps.step,
          onInput: (event: Event) => fieldEmit('update:modelValue', Number((event.target as HTMLInputElement).value)),
        }),
        fieldProps.suffix ? h('span', { class: 'pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-400' }, fieldProps.suffix) : null,
      ]),
    ])
  },
})

const SecretField = defineComponent({
  props: {
    modelValue: { type: String, required: true }, clear: { type: Boolean, required: true }, configured: { type: Boolean, required: true },
    disabled: { type: Boolean, default: false }, label: { type: String, required: true },
  },
  emits: ['update:modelValue', 'update:clear'],
  setup(fieldProps, { emit: fieldEmit }) {
    return () => h('label', { class: 'min-w-0' }, [
      h('span', { class: 'input-label' }, [fieldProps.label, fieldProps.configured ? h('span', { class: 'ml-2 text-[11px] text-primary-600 dark:text-primary-400' }, t('admin.instructionAudit.runtime.configured')) : null]),
      h('input', {
        type: 'password', class: 'input', value: fieldProps.modelValue, disabled: fieldProps.disabled || fieldProps.clear,
        autocomplete: 'new-password', placeholder: fieldProps.configured ? t('admin.instructionAudit.runtime.keepSecret') : '',
        onInput: (event: Event) => fieldEmit('update:modelValue', (event.target as HTMLInputElement).value),
      }),
      fieldProps.configured ? h('span', { class: 'mt-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400' }, [
        h('input', {
          type: 'checkbox', checked: fieldProps.clear, disabled: fieldProps.disabled,
          onChange: (event: Event) => fieldEmit('update:clear', (event.target as HTMLInputElement).checked),
        }),
        t('admin.instructionAudit.runtime.clearSecret'),
      ]) : null,
    ])
  },
})

const maxBodyMiB = computed({
  get: () => Math.round((draft.value?.max_body_bytes || MIB) / MIB),
  set: (value: number) => { if (draft.value) draft.value.max_body_bytes = Math.round(value * MIB) },
})
const maxInflightMiB = computed({
  get: () => Math.round((draft.value?.max_inflight_body_bytes || MIB) / MIB),
  set: (value: number) => { if (draft.value) draft.value.max_inflight_body_bytes = Math.round(value * MIB) },
})
const minimumInflightMiB = computed(() => maxBodyMiB.value * 3)
const dirty = computed(() => Boolean(draft.value) && JSON.stringify(draft.value) !== baseline.value)
const valid = computed(() => {
  const value = draft.value
  if (!value) return false
  if (value.max_body_bytes < MIB || value.max_body_bytes > 128 * MIB || value.max_inflight_body_bytes < value.max_body_bytes * 3 || value.max_inflight_body_bytes > 2048 * MIB) return false
  if (value.parse_timeout_ms < 50 || value.parse_timeout_ms > 5000 || value.pass_event_retention_days < 1 || value.pass_event_retention_days > 90 || value.aggregate_retention_days < 30 || value.aggregate_retention_days > 3650 || value.raw_content_retention_days < 1 || value.raw_content_retention_days > 3650) return false
  if (value.ai_timeout_ms < 100 || value.ai_timeout_ms > 30000 || value.ai_max_concurrency < 1 || value.ai_max_concurrency > 64 || value.ai_min_confidence < 0.5 || value.ai_min_confidence > 1) return false
  if (value.ai_per_user_rpm < 1 || value.ai_per_user_rpm > 120 || value.ai_per_user_daily_limit < 1 || value.ai_per_user_daily_limit > 1000 || value.ai_global_daily_limit < 1 || value.ai_global_daily_limit > 100000) return false
  if (value.translation_timeout_ms < 100 || value.translation_timeout_ms > 60000 || value.translation_max_concurrency < 1 || value.translation_max_concurrency > 16) return false
  if (value.translation_chunk_bytes < 1024 || value.translation_chunk_bytes > 65536 || value.translation_max_bytes < value.translation_chunk_bytes || value.translation_max_bytes > MIB) return false
  if (value.translation_result_ttl_seconds < 60 || value.translation_result_ttl_seconds > 86400) return false
  if (value.ai_enabled && (!value.ai_base_url || !value.ai_model || (!props.config?.ai_has_token && !value.ai_token))) return false
  if (value.external_translation_enabled && (!value.translation_base_url || !value.translation_model || (!props.config?.translation_has_token && !value.translation_token))) return false
  if (value.translation_enabled) {
    const internalReady = Boolean(value.ai_base_url && value.ai_model && ((props.config?.ai_has_token && !value.clear_ai_token) || value.ai_token))
    const externalReady = Boolean(value.external_translation_enabled && value.translation_base_url && value.translation_model && ((props.config?.translation_has_token && !value.clear_translation_token) || value.translation_token))
    if (!internalReady && !externalReady) return false
  }
  return true
})

watch(() => draft.value?.clear_ai_token, (clear) => {
  if (clear && draft.value) draft.value.ai_token = ''
})
watch(() => draft.value?.clear_translation_token, (clear) => {
  if (clear && draft.value) draft.value.translation_token = ''
})

watch(() => props.config, (config) => {
  if (!config) {
    draft.value = null
    baseline.value = ''
    return
  }
  const next: UpdateInstructionRuntimeConfigRequest = {
    max_body_bytes: config.max_body_bytes, parse_timeout_ms: config.parse_timeout_ms, max_inflight_body_bytes: config.max_inflight_body_bytes,
    pass_event_retention_days: config.pass_event_retention_days, aggregate_retention_days: config.aggregate_retention_days,
    raw_content_retention_days: config.raw_content_retention_days,
    ai_enabled: config.ai_enabled, ai_base_url: config.ai_base_url, ai_model: config.ai_model, ai_token: '', clear_ai_token: false,
    ai_timeout_ms: config.ai_timeout_ms, ai_max_concurrency: config.ai_max_concurrency, ai_min_confidence: config.ai_min_confidence,
    ai_per_user_rpm: config.ai_per_user_rpm, ai_per_user_daily_limit: config.ai_per_user_daily_limit,
    ai_global_daily_limit: config.ai_global_daily_limit, ai_prompt_version: config.ai_prompt_version,
    translation_enabled: config.translation_enabled, external_translation_enabled: config.external_translation_enabled,
    translation_base_url: config.translation_base_url, translation_model: config.translation_model, translation_token: '', clear_translation_token: false,
    translation_timeout_ms: config.translation_timeout_ms, translation_max_concurrency: config.translation_max_concurrency,
    translation_chunk_bytes: config.translation_chunk_bytes, translation_max_bytes: config.translation_max_bytes,
    translation_result_ttl_seconds: config.translation_result_ttl_seconds, expected_config_version: config.config_version,
  }
  draft.value = next
  baseline.value = JSON.stringify(next)
}, { immediate: true })

function reset() {
  if (!props.config) return
  const current = props.config
  const next = { ...draft.value!, ai_token: '', clear_ai_token: false, translation_token: '', clear_translation_token: false }
  next.max_body_bytes = current.max_body_bytes
  next.parse_timeout_ms = current.parse_timeout_ms
  next.max_inflight_body_bytes = current.max_inflight_body_bytes
  next.pass_event_retention_days = current.pass_event_retention_days
  next.aggregate_retention_days = current.aggregate_retention_days
  next.raw_content_retention_days = current.raw_content_retention_days
  next.ai_enabled = current.ai_enabled
  next.ai_base_url = current.ai_base_url
  next.ai_model = current.ai_model
  next.ai_timeout_ms = current.ai_timeout_ms
  next.ai_max_concurrency = current.ai_max_concurrency
  next.ai_min_confidence = current.ai_min_confidence
  next.ai_per_user_rpm = current.ai_per_user_rpm
  next.ai_per_user_daily_limit = current.ai_per_user_daily_limit
  next.ai_global_daily_limit = current.ai_global_daily_limit
  next.ai_prompt_version = current.ai_prompt_version
  next.translation_enabled = current.translation_enabled
  next.external_translation_enabled = current.external_translation_enabled
  next.translation_base_url = current.translation_base_url
  next.translation_model = current.translation_model
  next.translation_timeout_ms = current.translation_timeout_ms
  next.translation_max_concurrency = current.translation_max_concurrency
  next.translation_chunk_bytes = current.translation_chunk_bytes
  next.translation_max_bytes = current.translation_max_bytes
  next.translation_result_ttl_seconds = current.translation_result_ttl_seconds
  next.expected_config_version = current.config_version
  draft.value = next
}

function submit() {
  if (draft.value && dirty.value && valid.value && !props.saving) emit('save', { ...draft.value })
}

function formatDate(value: string): string {
  return value ? new Date(value).toLocaleString() : '-'
}

function formatBytes(value: number): string {
  if (!value) return t('admin.instructionAudit.runtime.unknownLimit')
  return `${Math.round((value / MIB) * 10) / 10} MiB`
}
</script>
