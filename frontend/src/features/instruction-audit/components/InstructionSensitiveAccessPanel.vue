<template>
  <section class="card min-w-0 overflow-hidden" data-test="sensitive-access-panel">
    <div class="flex flex-col gap-3 border-b border-gray-100 px-4 py-4 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between sm:px-6">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">
            {{ t('admin.instructionAudit.sensitiveAccess.title') }}
          </h2>
          <span :class="accessPillClass">
            <Icon :name="hasAccess ? 'check' : 'lock'" size="xs" />
            {{ accessStatusLabel }}
          </span>
        </div>
        <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.instructionAudit.sensitiveAccess.description') }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-secondary shrink-0"
        :disabled="capabilityLoading || !hasAccess || grantsLoading"
        :title="hasAccess ? t('admin.instructionAudit.sensitiveAccess.loadGrants') : t('admin.instructionAudit.sensitiveAccess.lockedHint')"
        data-test="load-sensitive-grants"
        @click="loadGrants"
      >
        <Icon :name="hasAccess ? 'users' : 'lock'" size="sm" :class="{ 'animate-spin': grantsLoading }" />
        {{ grantsLoaded ? t('common.refresh') : t('admin.instructionAudit.sensitiveAccess.loadGrants') }}
      </button>
    </div>

    <div v-if="capabilityLoading" class="space-y-3 px-4 py-5 sm:px-6" aria-busy="true">
      <div class="h-4 w-48 animate-pulse rounded bg-gray-100 dark:bg-dark-700" />
      <div class="h-14 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>

    <div v-else-if="capabilityError" class="px-4 py-5 sm:px-6">
      <div role="alert" class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
        <p class="font-medium">{{ t('admin.instructionAudit.sensitiveAccess.capabilityError') }}</p>
        <p class="mt-1 break-words text-xs">{{ capabilityError }}</p>
        <button type="button" class="btn btn-secondary btn-sm mt-3" @click="$emit('refresh-capability')">
          <Icon name="refresh" size="sm" />{{ t('common.retry') }}
        </button>
      </div>
    </div>

    <div v-else-if="!hasAccess" class="px-4 py-5 sm:px-6">
      <div class="flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300">
        <Icon name="lock" size="md" class="mt-0.5 shrink-0" />
        <div class="min-w-0">
          <p class="text-sm font-medium">{{ t('admin.instructionAudit.sensitiveAccess.accessRequired') }}</p>
          <p class="mt-1 text-xs">{{ t('admin.instructionAudit.sensitiveAccess.lockedHint') }}</p>
        </div>
      </div>
    </div>

    <div v-else-if="!grantsLoaded" class="px-4 py-5 sm:px-6">
      <p class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.instructionAudit.sensitiveAccess.loadHint') }}
      </p>
    </div>

    <div v-else class="min-w-0 space-y-5 px-4 py-5 sm:px-6">
      <div v-if="grantsError" role="alert" class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
        {{ grantsError }}
      </div>

      <form class="grid min-w-0 gap-3 rounded-md border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800 lg:grid-cols-[minmax(0,0.55fr)_minmax(0,1.45fr)_auto] lg:items-end" @submit.prevent="grantAccess">
        <label class="min-w-0">
          <span class="input-label">{{ t('admin.instructionAudit.sensitiveAccess.adminUserId') }}</span>
          <input v-model.trim="targetUserID" type="text" inputmode="numeric" class="input w-full" autocomplete="off" :placeholder="t('admin.instructionAudit.sensitiveAccess.userIdPlaceholder')" />
        </label>
        <label class="min-w-0">
          <span class="input-label">{{ t('admin.instructionAudit.sensitiveAccess.grantReason') }}</span>
          <input v-model="grantReason" type="text" maxlength="255" class="input w-full" :placeholder="t('admin.instructionAudit.sensitiveAccess.reasonPlaceholder')" />
        </label>
        <button type="submit" class="btn btn-primary w-full lg:w-auto" :disabled="mutating || !validTargetUserID">
          <Icon name="plus" size="sm" />{{ t('admin.instructionAudit.sensitiveAccess.grant') }}
        </button>
      </form>

      <div>
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.instructionAudit.sensitiveAccess.activeGrants') }}</h3>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.instructionAudit.sensitiveAccess.grantCount', { count: grants.length }) }}</span>
        </div>
        <div v-if="grantsLoading" class="space-y-2" aria-busy="true">
          <div v-for="index in 2" :key="index" class="h-20 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
        </div>
        <div v-else-if="grants.length" class="grid min-w-0 gap-2 xl:grid-cols-2">
          <article v-for="grant in grants" :key="grant.id" class="flex min-w-0 flex-col gap-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600 sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <p class="break-words text-sm font-medium text-gray-900 dark:text-white">{{ grant.username || grant.email || `#${grant.user_id}` }}</p>
                <span v-if="grant.user_id === capability?.user_id" class="rounded bg-primary-50 px-1.5 py-0.5 text-[11px] font-medium text-primary-700 dark:bg-primary-950/40 dark:text-primary-300">{{ t('admin.instructionAudit.sensitiveAccess.currentAdmin') }}</span>
              </div>
              <p class="mt-0.5 break-all text-xs text-gray-500 dark:text-gray-400">{{ grant.email || '-' }} · #{{ grant.user_id }}</p>
              <p class="mt-1 break-words text-xs text-gray-500 dark:text-gray-400">{{ grant.grant_reason || t('admin.instructionAudit.sensitiveAccess.noReason') }}</p>
              <p class="mt-1 text-[11px] text-gray-400">{{ formatDate(grant.granted_at) }} · {{ sourceLabel(grant.grant_source) }}</p>
            </div>
            <button type="button" class="btn btn-danger btn-sm shrink-0 self-start sm:self-auto" :disabled="mutating" @click="requestRevoke(grant)">
              <Icon name="x" size="sm" />{{ t('admin.instructionAudit.sensitiveAccess.revoke') }}
            </button>
          </article>
        </div>
        <p v-else class="rounded-md border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.instructionAudit.sensitiveAccess.emptyGrants') }}
        </p>
      </div>
    </div>

    <BaseDialog :show="Boolean(revokeTarget)" :title="t('admin.instructionAudit.sensitiveAccess.revokeTitle')" @close="closeRevoke">
      <div class="space-y-4">
        <p class="break-words text-sm text-gray-600 dark:text-gray-300">
          {{ t('admin.instructionAudit.sensitiveAccess.revokeConfirm', { user: revokeTarget?.email || revokeTarget?.username || `#${revokeTarget?.user_id}` }) }}
        </p>
        <label>
          <span class="input-label">{{ t('admin.instructionAudit.sensitiveAccess.revokeReason') }}</span>
          <input v-model="revokeReason" type="text" maxlength="255" class="input w-full" :placeholder="t('admin.instructionAudit.sensitiveAccess.reasonPlaceholder')" />
        </label>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="mutating" @click="closeRevoke">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-danger" :disabled="mutating" data-test="confirm-sensitive-revoke" @click="revokeAccess">{{ t('admin.instructionAudit.sensitiveAccess.revoke') }}</button>
      </template>
    </BaseDialog>
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import instructionAuditAPI from '../api'
import { isInstructionSensitiveAccessDenied } from '../sensitiveAccess'
import type { InstructionSensitiveCapability, InstructionSensitiveGrant } from '../types'

const props = defineProps<{
  capability: InstructionSensitiveCapability | null
  capabilityLoading: boolean
  capabilityError: string
}>()

const emit = defineEmits<{
  (event: 'refresh-capability'): void
  (event: 'access-denied', error?: unknown): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const stepUp = useStepUp()
const grants = ref<InstructionSensitiveGrant[]>([])
const grantsLoading = ref(false)
const grantsLoaded = ref(false)
const grantsError = ref('')
const mutating = ref(false)
const targetUserID = ref('')
const grantReason = ref('')
const revokeReason = ref('')
const revokeTarget = ref<InstructionSensitiveGrant | null>(null)

const hasAccess = computed(() => Boolean(props.capability?.has_access && props.capability?.can_manage))
const validTargetUserID = computed(() => /^[1-9]\d*$/.test(targetUserID.value))
const accessStatusLabel = computed(() => {
  if (props.capabilityLoading) return t('admin.instructionAudit.sensitiveAccess.checking')
  return hasAccess.value
    ? t('admin.instructionAudit.sensitiveAccess.authorized')
    : t('admin.instructionAudit.sensitiveAccess.notAuthorized')
})
const accessPillClass = computed(() => {
  const base = 'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium'
  return hasAccess.value
    ? `${base} bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-300`
    : `${base} bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300`
})

async function loadGrants() {
  if (!hasAccess.value || grantsLoading.value) return
  grantsLoading.value = true
  grantsError.value = ''
  try {
    grants.value = await stepUp.run(() => instructionAuditAPI.listSensitiveAccessGrants())
    grantsLoaded.value = true
  } catch (error) {
    handleError(error)
  } finally {
    grantsLoading.value = false
  }
}

async function grantAccess() {
  if (!hasAccess.value || !validTargetUserID.value || mutating.value) return
  mutating.value = true
  grantsError.value = ''
  try {
    const item = await stepUp.run(() => instructionAuditAPI.grantSensitiveAccess(Number(targetUserID.value), grantReason.value.trim()))
    const existingIndex = grants.value.findIndex((grant) => grant.user_id === item.user_id)
    if (existingIndex >= 0) grants.value.splice(existingIndex, 1, item)
    else grants.value.push(item)
    grants.value.sort((left, right) => left.user_id - right.user_id)
    targetUserID.value = ''
    grantReason.value = ''
    appStore.showSuccess(t('admin.instructionAudit.sensitiveAccess.granted'))
  } catch (error) {
    handleError(error)
  } finally {
    mutating.value = false
  }
}

function requestRevoke(grant: InstructionSensitiveGrant) {
  revokeTarget.value = grant
  revokeReason.value = ''
}

function closeRevoke() {
  if (mutating.value) return
  revokeTarget.value = null
  revokeReason.value = ''
}

async function revokeAccess() {
  if (!hasAccess.value || !revokeTarget.value || mutating.value) return
  const targetUser = revokeTarget.value.user_id
  mutating.value = true
  grantsError.value = ''
  try {
    await stepUp.run(() => instructionAuditAPI.revokeSensitiveAccess(targetUser, revokeReason.value.trim()))
    grants.value = grants.value.filter((grant) => grant.user_id !== targetUser)
    revokeTarget.value = null
    revokeReason.value = ''
    appStore.showSuccess(t('admin.instructionAudit.sensitiveAccess.revoked'))
    if (targetUser === props.capability?.user_id) emit('access-denied')
  } catch (error) {
    handleError(error)
  } finally {
    mutating.value = false
  }
}

function handleError(error: unknown) {
  if (isStepUpCancelled(error)) return
  if (isInstructionSensitiveAccessDenied(error)) {
    grants.value = []
    grantsLoaded.value = false
    emit('access-denied', error)
    return
  }
  grantsError.value = extractI18nErrorMessage(error, t, 'admin.instructionAudit.errors', t('common.error'))
  appStore.showError(grantsError.value)
}

function sourceLabel(source: string): string {
  return t(`admin.instructionAudit.sensitiveAccess.sources.${source}`, source || '-')
}

function formatDate(value: string): string {
  return value ? new Date(value).toLocaleString() : '-'
}
</script>
