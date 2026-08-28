<template>
  <BaseDialog :show="show" :title="t('lottery.result.title')" width="narrow" @close="emit('close')">
    <div v-if="entry" class="text-center">
      <div :class="['mx-auto flex h-16 w-16 items-center justify-center rounded-full', resultIconClass]">
        <Icon :name="resultIcon" size="xl" />
      </div>

      <h3 class="mt-4 text-lg font-semibold text-gray-950 dark:text-white">{{ resultTitle }}</h3>
      <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ resultDescription }}</p>

      <div v-if="entry.reward_code" class="mt-5 rounded-md border border-teal-200 bg-teal-50 p-4 text-left dark:border-teal-900 dark:bg-teal-950/30">
        <p class="text-xs font-medium uppercase text-teal-700 dark:text-teal-300">{{ t('lottery.result.code', { name: entry.prize_name }) }}</p>
        <p class="mt-2 break-all font-mono text-sm font-semibold text-gray-950 dark:text-white">{{ entry.reward_code }}</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary btn-sm" @click="copyCode">
            <Icon name="copy" size="sm" />{{ t('lottery.result.copyCode') }}
          </button>
          <RouterLink to="/redeem" class="btn btn-primary btn-sm" @click="emit('close')">
            <Icon name="gift" size="sm" />{{ t('lottery.result.redeemCode') }}
          </RouterLink>
        </div>
      </div>

      <p v-if="entry.replayed" class="mt-4 text-xs text-amber-700 dark:text-amber-300">{{ t('lottery.result.replayed') }}</p>
    </div>

    <template #footer>
      <button type="button" class="btn btn-primary w-full" @click="emit('close')">{{ t('lottery.result.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { LotteryEntry } from '@/api/lottery'
import { formatCurrency } from '@/utils/format'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{ show: boolean; entry: LotteryEntry | null }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const resultIcon = computed<'gift' | 'clock' | 'xCircle'>(() => {
  if (props.entry?.status === 'won') return 'gift'
  if (props.entry?.status === 'pending') return 'clock'
  return 'xCircle'
})

const resultIconClass = computed(() => {
  if (props.entry?.status === 'won') return 'bg-teal-100 text-teal-700 dark:bg-teal-950 dark:text-teal-300'
  if (props.entry?.status === 'pending') return 'bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-300'
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300'
})

const resultTitle = computed(() => {
  if (props.entry?.status === 'won') return t('lottery.result.wonTitle')
  if (props.entry?.status === 'pending') return t('lottery.result.pendingTitle')
  return t('lottery.result.noWinTitle')
})

const resultDescription = computed(() => {
  const entry = props.entry
  if (!entry || entry.status === 'not_won') return t('lottery.result.noWinDescription')
  if (entry.status === 'pending') return t('lottery.result.pendingDescription')
  if (entry.prize_type === 'balance') return t('lottery.result.balance', { amount: formatCurrency(entry.balance_amount).replace('￥', '') })
  return t('lottery.result.code', { name: entry.prize_name })
})

async function copyCode() {
  if (props.entry?.reward_code) await copyToClipboard(props.entry.reward_code, t('common.copiedToClipboard'))
}
</script>

