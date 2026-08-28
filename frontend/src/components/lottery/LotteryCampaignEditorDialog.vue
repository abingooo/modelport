<template>
  <BaseDialog :show="show" :title="campaign ? t('lottery.admin.editor.editTitle') : t('lottery.admin.editor.createTitle')" width="extra-wide" @close="emit('close')">
    <form id="lottery-campaign-form" class="space-y-6" @submit.prevent="submit">
      <div class="grid gap-4 md:grid-cols-2">
        <label class="md:col-span-2">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.name') }}</span>
          <input v-model.trim="form.name" class="input" maxlength="160" required />
        </label>
        <label class="md:col-span-2">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.description') }}</span>
          <textarea v-model.trim="form.description" class="input min-h-24 resize-y" maxlength="8000" />
        </label>
        <label>
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.mode') }}</span>
          <select v-model="form.mode" class="input">
            <option value="instant">{{ t('lottery.mode.instant') }}</option>
            <option value="scheduled">{{ t('lottery.mode.scheduled') }}</option>
          </select>
        </label>
        <label>
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.status') }}</span>
          <select v-model="form.status" class="input">
            <option value="draft">{{ t('lottery.state.draft') }}</option>
            <option value="active">{{ t('lottery.state.active') }}</option>
            <option value="paused">{{ t('lottery.state.paused') }}</option>
            <option value="completed">{{ t('lottery.state.completed') }}</option>
          </select>
        </label>
        <label>
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.startsAt') }}</span>
          <input v-model="form.starts_at" type="datetime-local" class="input" required />
        </label>
        <label>
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.endsAt') }}</span>
          <input v-model="form.ends_at" type="datetime-local" class="input" required />
        </label>
        <label v-if="form.mode === 'scheduled'">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.drawAt') }}</span>
          <input v-model="form.draw_at" type="datetime-local" class="input" required />
        </label>
        <label v-if="form.mode === 'scheduled'" class="flex min-h-16 items-center gap-3 rounded-md border border-gray-200 px-3 py-2 dark:border-dark-700">
          <input v-model="form.full_draw_enabled" data-testid="full-draw-toggle" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.fullDrawEnabled') }}</span>
        </label>
        <label v-if="form.mode === 'scheduled' && form.full_draw_enabled">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.fullDrawParticipantLimit') }}</span>
          <input v-model.number="form.full_draw_participant_limit" data-testid="full-draw-limit" type="number" class="input" min="1" max="1000000" required />
        </label>
        <label>
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.perUserLimit') }}</span>
          <input v-model.number="form.per_user_limit" type="number" class="input" min="1" max="1000" required />
        </label>
        <label>
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('lottery.admin.editor.minimumBalance') }}</span>
          <input v-model.number="form.minimum_balance" type="number" class="input" min="0" step="0.01" required />
        </label>
      </div>

      <fieldset>
        <legend class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('lottery.admin.editor.requiredGroups') }}</legend>
        <p v-if="subscriptionGroups.length === 0" class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('lottery.admin.editor.noGroupRequired') }}</p>
        <div v-else class="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <label v-for="group in subscriptionGroups" :key="group.id" class="flex min-h-10 items-center gap-2 rounded-md border border-gray-200 px-3 py-2 text-sm dark:border-dark-700">
            <input v-model="form.required_subscription_group_ids" type="checkbox" :value="group.id" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            <span class="min-w-0 truncate text-gray-700 dark:text-gray-200">{{ group.name }}</span>
          </label>
        </div>
      </fieldset>

      <section>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('lottery.admin.editor.prizes') }}</h4>
            <p :class="['mt-1 text-xs', probabilityTotal > 10000 ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400']">
              {{ t('lottery.admin.editor.probabilityTotal', { bps: probabilityTotal, percent: formatProbability(probabilityTotal) }) }}
            </p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" @click="addPrize"><Icon name="plus" size="sm" />{{ t('lottery.admin.editor.addPrize') }}</button>
        </div>

        <p v-if="probabilityTotal > 10000" class="mt-2 text-sm text-red-600 dark:text-red-400">{{ t('lottery.admin.editor.probabilityExceeded') }}</p>

        <div class="mt-4 space-y-3">
          <div v-for="(prize, index) in form.prizes" :key="prize.local_id" class="rounded-md border border-gray-200 p-4 dark:border-dark-700">
            <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <label class="sm:col-span-2">
                <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('lottery.admin.editor.prizeName') }}</span>
                <input v-model.trim="prize.name" class="input" maxlength="160" required />
              </label>
              <label>
                <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('lottery.admin.editor.prizeType') }}</span>
                <select v-model="prize.prize_type" class="input" @change="normalizePrizeType(prize)">
                  <option value="balance">{{ t('lottery.admin.editor.balance') }}</option>
                  <option value="subscription_code">{{ t('lottery.admin.editor.subscriptionCode') }}</option>
                </select>
              </label>
              <label v-if="prize.prize_type === 'balance'">
                <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('lottery.admin.editor.balanceAmount') }}</span>
                <input v-model.number="prize.balance_amount" type="number" class="input" min="0.00000001" step="0.01" required />
              </label>
              <label v-else>
                <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('lottery.admin.editor.subscriptionGroup') }}</span>
                <select v-model.number="prize.subscription_group_id" class="input" required>
                  <option :value="null" disabled>-</option>
                  <option v-for="group in subscriptionGroups" :key="group.id" :value="group.id">{{ group.name }}</option>
                </select>
              </label>
              <label v-if="prize.prize_type === 'subscription_code'">
                <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('lottery.admin.editor.validityDays') }}</span>
                <input v-model.number="prize.subscription_validity_days" type="number" class="input" min="1" max="3650" required />
              </label>
              <label>
                <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('lottery.admin.editor.probabilityBps') }}</span>
                <input v-model.number="prize.probability_bps" type="number" class="input" min="1" max="10000" required />
              </label>
              <label>
                <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('lottery.admin.editor.inventory') }}</span>
                <input v-model.number="prize.inventory" type="number" class="input" min="1" required />
              </label>
              <label class="flex items-end gap-2 pb-2 text-sm text-gray-700 dark:text-gray-200">
                <input v-model="prize.is_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                {{ t('lottery.admin.editor.enabled') }}
              </label>
              <div class="flex items-end justify-end">
                <button type="button" class="icon-btn text-red-600 dark:text-red-400" :title="t('lottery.admin.editor.removePrize')" :disabled="form.prizes.length === 1" @click="removePrize(index)"><Icon name="trash" size="sm" /></button>
              </div>
            </div>
          </div>
        </div>
      </section>

      <p v-if="validationError" class="text-sm text-red-600 dark:text-red-400">{{ validationError }}</p>
    </form>

    <template #footer>
      <div class="flex w-full flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('lottery.admin.editor.cancel') }}</button>
        <button type="submit" form="lottery-campaign-form" class="btn btn-primary" :disabled="saving || probabilityTotal > 10000">
          <span v-if="saving" class="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" />
          <Icon v-else name="check" size="sm" />{{ saving ? t('lottery.admin.editor.saving') : t('lottery.admin.editor.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminGroup } from '@/types'
import type { LotteryCampaign, LotteryCampaignInput, LotteryCampaignStatus, LotteryMode, LotteryPrizeType } from '@/api/lottery'
import { formatDateTimeLocalInput } from '@/utils/format'

interface EditorPrize {
  local_id: string
  name: string
  prize_type: LotteryPrizeType
  balance_amount: number
  subscription_group_id: number | null
  subscription_validity_days: number
  probability_bps: number
  inventory: number
  is_enabled: boolean
}

interface EditorForm {
  name: string
  description: string
  mode: LotteryMode
  status: LotteryCampaignStatus
  starts_at: string
  ends_at: string
  draw_at: string
  full_draw_enabled: boolean
  full_draw_participant_limit: number
  per_user_limit: number
  minimum_balance: number
  required_subscription_group_ids: number[]
  prizes: EditorPrize[]
}

const props = defineProps<{ show: boolean; campaign: LotteryCampaign | null; saving: boolean; subscriptionGroups: AdminGroup[] }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'save', input: LotteryCampaignInput): void }>()
const { t } = useI18n()
const validationError = ref('')
let prizeCounter = 0

function localID() { prizeCounter += 1; return `prize-${prizeCounter}` }
function localDate(value: string | Date) { return formatDateTimeLocalInput(Math.floor(new Date(value).getTime() / 1000)) }
function futureDate(minutes: number) { return localDate(new Date(Date.now() + minutes * 60_000)) }
function emptyPrize(): EditorPrize {
  return { local_id: localID(), name: '', prize_type: 'balance', balance_amount: 1, subscription_group_id: null, subscription_validity_days: 0, probability_bps: 1000, inventory: 1, is_enabled: true }
}
function emptyForm(): EditorForm {
  return {
    name: '', description: '', mode: 'instant', status: 'draft', starts_at: futureDate(10), ends_at: futureDate(1450),
    draw_at: futureDate(1450), full_draw_enabled: false, full_draw_participant_limit: 100,
    per_user_limit: 1, minimum_balance: 0, required_subscription_group_ids: [], prizes: [emptyPrize()],
  }
}

const form = reactive<EditorForm>(emptyForm())
const probabilityTotal = computed(() => form.prizes.reduce((sum, prize) => sum + (prize.is_enabled ? Number(prize.probability_bps) || 0 : 0), 0))

function resetForm() {
  const campaign = props.campaign
  const next = campaign ? {
    name: campaign.name,
    description: campaign.description,
    mode: campaign.mode,
    status: campaign.status,
    starts_at: localDate(campaign.starts_at),
    ends_at: localDate(campaign.ends_at),
    draw_at: campaign.draw_at ? localDate(campaign.draw_at) : localDate(campaign.ends_at),
    full_draw_enabled: campaign.full_draw_participant_limit != null,
    full_draw_participant_limit: campaign.full_draw_participant_limit ?? 100,
    per_user_limit: campaign.per_user_limit,
    minimum_balance: campaign.minimum_balance,
    required_subscription_group_ids: [...campaign.required_subscription_group_ids],
    prizes: campaign.prizes.map((prize) => ({
      local_id: localID(), name: prize.name, prize_type: prize.prize_type, balance_amount: prize.balance_amount,
      subscription_group_id: prize.subscription_group_id ?? null, subscription_validity_days: prize.subscription_validity_days,
      probability_bps: prize.probability_bps, inventory: prize.inventory, is_enabled: prize.is_enabled,
    })),
  } satisfies EditorForm : emptyForm()
  Object.assign(form, next)
  validationError.value = ''
}

function addPrize() { form.prizes.push(emptyPrize()) }
function removePrize(index: number) { if (form.prizes.length > 1) form.prizes.splice(index, 1) }
function normalizePrizeType(prize: EditorPrize) {
  if (prize.prize_type === 'balance') {
    prize.balance_amount = prize.balance_amount > 0 ? prize.balance_amount : 1
    prize.subscription_group_id = null
    prize.subscription_validity_days = 0
  } else {
    prize.balance_amount = 0
    prize.subscription_validity_days = prize.subscription_validity_days > 0 ? prize.subscription_validity_days : 30
  }
}
function formatProbability(bps: number) { return (bps / 100).toFixed(bps % 100 === 0 ? 0 : 2) }

function submit() {
  validationError.value = ''
  const startsAt = new Date(form.starts_at)
  const endsAt = new Date(form.ends_at)
  const drawAt = form.mode === 'scheduled' ? new Date(form.draw_at) : null
  if (!form.name || Number.isNaN(startsAt.getTime()) || Number.isNaN(endsAt.getTime()) || form.prizes.length === 0) {
    validationError.value = t('lottery.admin.editor.required'); return
  }
  if (endsAt <= startsAt) { validationError.value = t('lottery.admin.editor.invalidWindow'); return }
  if (drawAt && (Number.isNaN(drawAt.getTime()) || drawAt < endsAt)) { validationError.value = t('lottery.admin.editor.drawBeforeEnd'); return }
  const fullDrawParticipantLimit = Number(form.full_draw_participant_limit)
  if (form.mode === 'scheduled' && form.full_draw_enabled
    && (!Number.isInteger(fullDrawParticipantLimit) || fullDrawParticipantLimit < 1 || fullDrawParticipantLimit > 1000000)) {
    validationError.value = t('lottery.admin.editor.fullDrawLimitInvalid'); return
  }
  if (probabilityTotal.value > 10000) { validationError.value = t('lottery.admin.editor.probabilityExceeded'); return }
  const invalidPrize = form.prizes.some((prize) => !prize.name || prize.probability_bps < 1 || prize.inventory < 1
    || (prize.prize_type === 'balance' ? prize.balance_amount <= 0 : !prize.subscription_group_id || prize.subscription_validity_days < 1))
  if (invalidPrize) { validationError.value = t('lottery.admin.editor.required'); return }

  emit('save', {
    name: form.name,
    description: form.description,
    mode: form.mode,
    status: form.status,
    starts_at: startsAt.toISOString(),
    ends_at: endsAt.toISOString(),
    draw_at: drawAt?.toISOString() ?? null,
    full_draw_participant_limit: form.mode === 'scheduled' && form.full_draw_enabled ? fullDrawParticipantLimit : null,
    per_user_limit: Number(form.per_user_limit),
    minimum_balance: Number(form.minimum_balance),
    required_subscription_group_ids: [...form.required_subscription_group_ids],
    prizes: form.prizes.map((prize, index) => ({
      name: prize.name,
      prize_type: prize.prize_type,
      balance_amount: prize.prize_type === 'balance' ? Number(prize.balance_amount) : 0,
      subscription_group_id: prize.prize_type === 'subscription_code' ? Number(prize.subscription_group_id) : null,
      subscription_validity_days: prize.prize_type === 'subscription_code' ? Number(prize.subscription_validity_days) : 0,
      probability_bps: Number(prize.probability_bps),
      inventory: Number(prize.inventory),
      is_enabled: prize.is_enabled,
      sort_order: index,
    })),
  })
}

watch(() => props.show, (show) => { if (show) resetForm() })
watch(() => form.mode, (mode) => {
  if (mode === 'scheduled' && !form.draw_at) form.draw_at = form.ends_at
  if (mode === 'instant') form.full_draw_enabled = false
})
</script>

