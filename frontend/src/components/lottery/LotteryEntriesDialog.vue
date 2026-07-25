<template>
  <BaseDialog :show="show" :title="campaign ? `${t('lottery.admin.entryDialog.title')} · ${campaign.name}` : t('lottery.admin.entryDialog.title')" width="extra-wide" @close="emit('close')">
    <div class="min-h-48">
      <div v-if="loading" class="flex justify-center py-16"><LoadingSpinner /></div>
      <div v-else-if="error" class="py-12 text-center">
        <p class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        <button type="button" class="btn btn-secondary mt-4" @click="emit('refresh')">{{ t('lottery.retry') }}</button>
      </div>
      <EmptyState v-else-if="entries.length === 0" :title="t('lottery.admin.entryDialog.empty')" />
      <div v-else class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('lottery.admin.entryDialog.user') }}</th>
              <th class="px-4 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('lottery.admin.entryDialog.result') }}</th>
              <th class="px-4 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('lottery.admin.entryDialog.reward') }}</th>
              <th class="px-4 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('lottery.admin.entryDialog.time') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="entry in entries" :key="entry.id">
              <td class="px-4 py-3 text-sm text-gray-800 dark:text-gray-200"><p>{{ entry.user_email }}</p><p class="mt-0.5 text-xs text-gray-400">#{{ entry.user_id }}</p></td>
              <td class="px-4 py-3"><span :class="['badge', statusClass(entry.status)]">{{ t(`lottery.entryStatus.${entry.status}`) }}</span></td>
              <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">
                <span v-if="entry.status !== 'won'">-</span>
                <span v-else-if="entry.prize_type === 'balance'">{{ formatCurrency(entry.balance_amount) }}</span>
                <span v-else class="inline-flex items-center gap-2">
                  <span>{{ entry.prize_name }}</span>
                  <button v-if="entry.reward_code" type="button" class="icon-btn" :title="t('lottery.result.copyCode')" @click="copyToClipboard(entry.reward_code, t('common.copiedToClipboard'))"><Icon name="copy" size="xs" /></button>
                </span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(entry.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination v-if="total > pageSize" :page="page" :page-size="pageSize" :total="total" :show-page-size-selector="false" @update:page="emit('page', $event)" />
    </div>

    <template #footer>
      <div class="flex w-full flex-col-reverse gap-2 sm:flex-row sm:justify-between">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
        <button v-if="canDraw" type="button" class="btn btn-primary" :disabled="drawing" @click="emit('draw')">
          <span v-if="drawing" class="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" />
          <Icon v-else name="sparkles" size="sm" />{{ drawing ? t('lottery.admin.drawing') : t('lottery.admin.draw') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import type { LotteryCampaign, LotteryEntry } from '@/api/lottery'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{ show: boolean; campaign: LotteryCampaign | null; entries: LotteryEntry[]; loading: boolean; error: string; page: number; pageSize: number; total: number; drawing: boolean }>()
const emit = defineEmits<{ (event: 'close' | 'refresh' | 'draw'): void; (event: 'page', page: number): void }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const canDraw = computed(() => {
  const campaign = props.campaign
  return Boolean(campaign && campaign.mode === 'scheduled' && campaign.status === 'active' && campaign.draw_at && Date.now() >= new Date(campaign.draw_at).getTime())
})
function statusClass(status: LotteryEntry['status']) {
  if (status === 'won') return 'badge-success'
  if (status === 'pending') return 'badge-info'
  return 'badge-gray'
}
</script>
