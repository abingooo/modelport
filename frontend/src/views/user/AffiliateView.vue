<template>
  <AppLayout>
    <div v-if="loading" class="flex justify-center py-20">
      <LoadingSpinner />
    </div>

    <div v-else-if="detail" class="space-y-6">
      <section
        class="overflow-hidden rounded-xl border border-primary-200 bg-white shadow-sm dark:border-primary-900/60 dark:bg-dark-800"
      >
        <div class="border-b border-primary-100 bg-primary-50/70 px-5 py-5 dark:border-primary-900/50 dark:bg-primary-950/20 sm:px-6">
          <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <div class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase text-primary-600 dark:text-primary-400">
                <Icon name="users" size="sm" />
                <span>ModelPort Partner</span>
              </div>
              <h2 class="text-xl font-semibold text-gray-950 dark:text-white">{{ t('affiliate.title') }}</h2>
              <p class="mt-1 max-w-2xl text-sm text-gray-600 dark:text-dark-300">{{ t('affiliate.description') }}</p>
            </div>
            <div
              v-if="rewardDashboard"
              class="inline-flex w-fit items-center gap-2 rounded-full px-3 py-1.5 text-xs font-medium"
              :class="rewardDashboard.program.enabled
                ? 'bg-cyan-100 text-cyan-800 dark:bg-cyan-950/50 dark:text-cyan-300'
                : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'"
            >
              <span class="h-1.5 w-1.5 rounded-full" :class="rewardDashboard.program.enabled ? 'bg-cyan-500' : 'bg-gray-400'" />
              {{ rewardDashboard.program.enabled ? t('affiliate.program.active') : t('affiliate.program.inactive') }}
            </div>
          </div>
        </div>

        <div class="grid gap-px bg-gray-200 dark:bg-dark-700 lg:grid-cols-2">
          <div class="bg-white p-5 dark:bg-dark-800 sm:p-6">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('affiliate.yourCode') }}</p>
            <div class="mt-2 flex min-w-0 items-center gap-3">
              <code class="min-w-0 flex-1 truncate text-lg font-semibold text-gray-950 dark:text-white">{{ detail.aff_code }}</code>
              <button type="button" class="icon-btn shrink-0" :title="t('affiliate.copyCode')" @click="copyCode">
                <Icon name="copy" size="sm" />
              </button>
            </div>
          </div>
          <div class="bg-white p-5 dark:bg-dark-800 sm:p-6">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('affiliate.inviteLink') }}</p>
            <div class="mt-2 flex min-w-0 items-center gap-3">
              <code class="min-w-0 flex-1 truncate text-sm text-gray-700 dark:text-gray-300">{{ inviteLink }}</code>
              <button type="button" class="icon-btn shrink-0" :title="t('affiliate.copyLink')" @click="copyInviteLink">
                <Icon name="copy" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </section>

      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div class="card border-l-4 border-l-primary-500 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.stats.invitedUsers') }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">{{ formatCount(invitedUsers) }}</p>
        </div>
        <div class="card border-l-4 border-l-cyan-500 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.stats.firstRechargeUsers') }}</p>
          <p class="mt-2 text-2xl font-semibold text-cyan-700 dark:text-cyan-300">{{ formatCount(firstRechargeUsers) }}</p>
        </div>
        <div class="card border-l-4 border-l-amber-400 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.stats.pendingReward') }}</p>
          <p class="mt-2 text-2xl font-semibold text-amber-700 dark:text-amber-300">{{ formatCurrency(pendingReward) }}</p>
        </div>
        <div class="card border-l-4 border-l-teal-500 p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.stats.paidReward') }}</p>
          <p class="mt-2 text-2xl font-semibold text-teal-700 dark:text-teal-300">{{ formatCurrency(paidReward) }}</p>
        </div>
      </div>

      <section class="card p-5 sm:p-6">
          <div class="flex items-center gap-3">
            <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary-100 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300">
              <Icon name="gift" size="md" />
            </div>
            <div>
              <h3 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('affiliate.program.title') }}</h3>
              <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.program.description') }}</p>
            </div>
          </div>

          <div v-if="rewardDashboard" class="mt-5 divide-y divide-gray-100 border-y border-gray-100 dark:divide-dark-700 dark:border-dark-700">
            <div class="flex flex-col gap-2 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('affiliate.program.registration.title') }}</p>
                <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                  {{ registrationRuleText }}
                </p>
              </div>
              <span :class="programRuleBadge(rewardDashboard.program.registration.enabled)">
                {{ rewardDashboard.program.registration.enabled ? t('affiliate.program.enabled') : t('affiliate.program.disabled') }}
              </span>
            </div>
            <div class="flex flex-col gap-2 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('affiliate.program.firstRecharge.title') }}</p>
                <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                  {{ firstRechargeRuleText }}
                </p>
              </div>
              <span :class="programRuleBadge(rewardDashboard.program.first_recharge.enabled)">
                {{ rewardDashboard.program.first_recharge.enabled ? t('affiliate.program.enabled') : t('affiliate.program.disabled') }}
              </span>
            </div>
          </div>
          <p v-else class="mt-5 text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.program.unavailable') }}</p>
          <div class="mt-4 flex items-start gap-2 rounded-lg bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
            <Icon name="clock" size="sm" class="mt-0.5 shrink-0" />
            <span>{{ t('affiliate.program.reviewNotice') }}</span>
          </div>
      </section>

      <section v-if="rewardDashboard" class="card overflow-hidden">
        <div class="flex flex-col gap-2 border-b border-gray-200 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <div>
            <h3 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('affiliate.progress.title') }}</h3>
            <p class="mt-0.5 text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.progress.description') }}</p>
          </div>
          <span class="text-xs text-gray-400 dark:text-dark-500">{{ t('affiliate.progress.masked') }}</span>
        </div>
        <div v-if="rewardDashboard.invitees.length === 0" class="py-12 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('affiliate.invitees.empty') }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[760px] text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">
              <tr>
                <th class="px-5 py-3 font-medium sm:px-6">{{ t('affiliate.invitees.columns.email') }}</th>
                <th class="px-5 py-3 font-medium">{{ t('affiliate.progress.registration') }}</th>
                <th class="px-5 py-3 font-medium">{{ t('affiliate.progress.firstRecharge') }}</th>
                <th class="px-5 py-3 text-right font-medium sm:px-6">{{ t('affiliate.invitees.columns.joinedAt') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in rewardDashboard.invitees" :key="item.user_id">
                <td class="px-5 py-4 font-medium text-gray-900 dark:text-white sm:px-6">{{ item.email_masked || '-' }}</td>
                <td class="px-5 py-4">
                  <span :class="statusBadge(item.registration_reward_status)">{{ statusText(item.registration_reward_status) }}</span>
                  <span v-if="item.registration_reward_amount > 0" class="ml-2 text-xs text-gray-500 dark:text-dark-400">
                    {{ formatCurrency(item.registration_reward_amount) }}
                  </span>
                </td>
                <td class="px-5 py-4">
                  <span :class="statusBadge(item.first_recharge_reward_status)">{{ statusText(item.first_recharge_reward_status) }}</span>
                  <span v-if="item.first_recharge_reward_amount > 0" class="ml-2 text-xs text-gray-500 dark:text-dark-400">
                    {{ formatCurrency(item.first_recharge_reward_amount) }}
                  </span>
                </td>
                <td class="px-5 py-4 text-right text-gray-500 dark:text-dark-400 sm:px-6">{{ formatDateTime(item.registered_at) || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import userAPI from '@/api/user'
import type { UserAffiliateDetail } from '@/types'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const loading = ref(true)
const detail = ref<UserAffiliateDetail | null>(null)

const rewardDashboard = computed(() => detail.value?.reward_program || null)
const invitedUsers = computed(() => rewardDashboard.value?.invited_users ?? detail.value?.aff_count ?? 0)
const firstRechargeUsers = computed(() => rewardDashboard.value?.first_recharge_users ?? 0)
const pendingReward = computed(() => rewardDashboard.value?.pending_amount ?? 0)
const paidReward = computed(() => rewardDashboard.value?.paid_amount ?? 0)
const inviteLink = computed(() => {
  if (!detail.value) return ''
  const path = `/register?aff=${encodeURIComponent(detail.value.aff_code)}`
  return typeof window === 'undefined' ? path : `${window.location.origin}${path}`
})
const registrationRuleText = computed(() => {
  const config = rewardDashboard.value?.program.registration
  if (!config) return ''
  return t('affiliate.program.registration.rule', {
    inviter: formatCurrency(config.inviter_bonus),
    invitee: formatCurrency(config.invitee_trial_amount),
    days: config.invitee_trial_days,
  })
})
const firstRechargeRuleText = computed(() => {
  const config = rewardDashboard.value?.program.first_recharge
  if (!config) return ''
  return t('affiliate.program.firstRecharge.rule', {
    inviter: formatCurrency(config.inviter_bonus),
    percent: config.invitee_bonus_percent,
  })
})

function formatCount(value: number): string {
  return value.toLocaleString()
}
function programRuleBadge(enabled: boolean): string {
  return enabled
    ? 'badge border-cyan-200 bg-cyan-50 text-cyan-700 dark:border-cyan-900 dark:bg-cyan-950/40 dark:text-cyan-300'
    : 'badge badge-gray'
}
function statusBadge(status: string): string {
  if (status === 'paid') return 'badge border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-900 dark:bg-teal-950/40 dark:text-teal-300'
  if (status === 'pending' || status === 'approved') return 'badge border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300'
  if (status === 'rejected') return 'badge badge-danger'
  return 'badge badge-gray'
}
function statusText(status: string): string {
  const known = ['pending', 'approved', 'paid', 'rejected', 'none']
  return known.includes(status) ? t(`affiliate.status.${status}`) : status
}
async function loadAffiliateDetail(silent = false): Promise<void> {
  if (!silent) loading.value = true
  try {
    detail.value = await userAPI.getAffiliateDetail()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.loadFailed')))
  } finally {
    if (!silent) loading.value = false
  }
}
async function copyCode(): Promise<void> {
  if (detail.value?.aff_code) await copyToClipboard(detail.value.aff_code, t('affiliate.codeCopied'))
}
async function copyInviteLink(): Promise<void> {
  if (inviteLink.value) await copyToClipboard(inviteLink.value, t('affiliate.linkCopied'))
}
onMounted(() => void loadAffiliateDetail())
</script>
