<template>
  <section class="card overflow-hidden" data-test="instruction-audit-reason-policies">
    <div class="border-b border-gray-100 px-4 py-4 dark:border-dark-700 sm:px-6">
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.policies.title') }}</h2>
      <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.policies.description') }}</p>
    </div>

    <div v-if="error && !policies.length" role="alert" class="m-5 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </div>
    <div v-else-if="loading && !policies.length" class="space-y-3 p-5" aria-busy="true">
      <div v-for="index in 5" :key="index" class="h-20 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>
    <div v-else-if="policies.length" class="divide-y divide-gray-100 dark:divide-dark-700">
      <article v-for="policy in policies" :key="policy.reason" class="px-4 py-4 sm:px-6">
        <div class="grid min-w-0 gap-4 xl:grid-cols-[minmax(180px,1.2fr)_minmax(170px,0.8fr)_minmax(120px,0.6fr)_minmax(120px,0.6fr)_minmax(190px,0.9fr)_auto] xl:items-end">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="break-words text-sm font-semibold text-gray-950 dark:text-white">{{ reasonLabel(policy.reason) }}</h3>
              <span v-if="isForcedBlock(policy.reason)" class="rounded-full bg-red-50 px-2 py-0.5 text-[11px] font-medium text-red-700 dark:bg-red-950/40 dark:text-red-300">
                {{ t('admin.instructionAudit.policies.forcedBlock') }}
              </span>
            </div>
            <p class="mt-1 break-all font-mono text-[11px] text-gray-400">{{ policy.reason }} · v{{ policy.config_version }}</p>
          </div>

          <label>
            <span class="input-label">{{ t('admin.instructionAudit.policies.action') }}</span>
            <select v-model="drafts[policy.reason].action" class="input h-10 py-2 text-sm" :disabled="isForcedBlock(policy.reason)">
              <option value="block">{{ t('admin.instructionAudit.policies.block') }}</option>
              <option value="allow_and_record">{{ t('admin.instructionAudit.policies.allowAndRecord') }}</option>
            </select>
          </label>

          <label class="flex min-h-10 items-center justify-between gap-3 rounded-md border border-gray-200 px-3 py-2 dark:border-dark-600">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.policies.aiReview') }}</span>
            <Toggle v-model="drafts[policy.reason].ai_review_enabled" :disabled="!supportsAI(policy.reason)" />
          </label>

          <label class="flex min-h-10 items-center justify-between gap-3 rounded-md border border-gray-200 px-3 py-2 dark:border-dark-600">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.instructionAudit.policies.alert') }}</span>
            <Toggle v-model="drafts[policy.reason].alert_enabled" />
          </label>

          <label v-if="policy.reason === 'request_too_large' && drafts[policy.reason].action === 'allow_and_record'">
            <span class="input-label">{{ t('admin.instructionAudit.policies.allowUntil') }}</span>
            <input v-model="drafts[policy.reason].allow_until_local" type="datetime-local" class="input h-10 py-2 text-sm" />
          </label>
          <div v-else class="hidden xl:block" />

          <button type="button" class="btn btn-primary h-10 justify-center" :disabled="savingReason === policy.reason || !changed(policy) || !valid(policy)" @click="requestSave(policy)">
            <Icon name="check" size="sm" />
            {{ savingReason === policy.reason ? t('common.saving') : t('common.save') }}
          </button>
        </div>
        <p v-if="drafts[policy.reason].action === 'allow_and_record'" class="mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300">
          {{ t('admin.instructionAudit.policies.allowWarning') }}
        </p>
      </article>
    </div>

    <ConfirmDialog
      :show="Boolean(pendingPolicy)"
      :title="t('admin.instructionAudit.policies.confirmTitle')"
      :message="t('admin.instructionAudit.policies.confirmMessage', { reason: pendingPolicy ? reasonLabel(pendingPolicy.reason) : '' })"
      :confirm-text="t('admin.instructionAudit.policies.confirmAllow')"
      danger
      @confirm="confirmSave"
      @cancel="pendingPolicy = null"
    />
  </section>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { InstructionReasonPolicy, InstructionPolicyAction, UpdateInstructionReasonPolicyRequest } from '../types'

interface PolicyDraft {
  action: InstructionPolicyAction
  ai_review_enabled: boolean
  alert_enabled: boolean
  allow_until_local: string
}

const props = defineProps<{ policies: InstructionReasonPolicy[]; loading: boolean; error: string; savingReason: string; configVersion: number }>()
const emit = defineEmits<{
  (event: 'save', reason: string, payload: UpdateInstructionReasonPolicyRequest): void
}>()
const { t } = useI18n()
const drafts = reactive<Record<string, PolicyDraft>>({})
const pendingPolicy = ref<InstructionReasonPolicy | null>(null)
const forcedBlockReasons = new Set(['config_unavailable', 'ai_error'])

watch(() => props.policies, (policies) => {
  for (const policy of policies) {
    drafts[policy.reason] = {
      action: policy.action,
      ai_review_enabled: policy.ai_review_enabled,
      alert_enabled: policy.alert_enabled,
      allow_until_local: toDateTimeLocal(policy.allow_until),
    }
  }
}, { immediate: true, deep: true })

function isForcedBlock(reason: string): boolean {
  return forcedBlockReasons.has(reason)
}

function supportsAI(reason: string): boolean {
  return reason === 'hash_mismatch' || reason === 'field_invalid'
}

function changed(policy: InstructionReasonPolicy): boolean {
  const draft = drafts[policy.reason]
  if (!draft) return false
  return draft.action !== policy.action
    || draft.ai_review_enabled !== policy.ai_review_enabled
    || draft.alert_enabled !== policy.alert_enabled
    || draft.allow_until_local !== toDateTimeLocal(policy.allow_until)
}

function valid(policy: InstructionReasonPolicy): boolean {
  const draft = drafts[policy.reason]
  if (!draft) return false
  if (isForcedBlock(policy.reason) && draft.action !== 'block') return false
  if (!supportsAI(policy.reason) && draft.ai_review_enabled) return false
  if (policy.reason === 'request_too_large' && draft.action === 'allow_and_record') {
    const until = new Date(draft.allow_until_local)
    const now = Date.now()
    return !Number.isNaN(until.getTime()) && until.getTime() > now && until.getTime() <= now + 24 * 60 * 60 * 1000
  }
  return true
}

function requestSave(policy: InstructionReasonPolicy) {
  if (!valid(policy)) return
  if (drafts[policy.reason].action === 'allow_and_record') {
    pendingPolicy.value = policy
    return
  }
  emitSave(policy, false)
}

function confirmSave() {
  if (!pendingPolicy.value) return
  const policy = pendingPolicy.value
  pendingPolicy.value = null
  emitSave(policy, true)
}

function emitSave(policy: InstructionReasonPolicy, confirmed: boolean) {
  const draft = drafts[policy.reason]
  emit('save', policy.reason, {
    action: draft.action,
    ai_review_enabled: draft.ai_review_enabled,
    alert_enabled: draft.alert_enabled,
    allow_until: draft.action === 'allow_and_record' ? fromDateTimeLocal(draft.allow_until_local) : null,
    expected_config_version: props.configVersion || policy.config_version,
    confirmed,
  })
}

function reasonLabel(reason: string): string {
  return t(`admin.instructionAudit.reasons.${reason}`)
}

function toDateTimeLocal(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function fromDateTimeLocal(value: string): string | null {
  return value ? new Date(value).toISOString() : null
}
</script>
