<template>
  <article
    class="model-card flex h-full min-w-0 flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm transition duration-200 hover:-translate-y-0.5 hover:shadow-md dark:border-dark-700 dark:bg-dark-800"
    :style="accentStyle"
  >
    <div class="h-1 w-full flex-shrink-0 bg-[var(--plaza-accent)]"></div>

    <header class="border-b border-gray-100 px-4 pb-3 pt-4 dark:border-dark-700/70 sm:px-5">
      <div class="flex min-w-0 items-start gap-3">
        <div class="provider-mark flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg">
          <PlatformIcon :platform="card.platform as GroupPlatform" size="lg" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
            {{ providerLabel }}
          </p>
          <div class="mt-0.5 flex min-w-0 items-center gap-1.5">
            <h3 class="min-w-0 truncate font-mono text-base font-semibold text-gray-950 dark:text-white">
              {{ card.name }}
            </h3>
            <button
              type="button"
              class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-white"
              :title="copied ? t('modelPlaza.card.copied') : t('modelPlaza.card.copyModel')"
              @click="copyModelName"
            >
              <Icon :name="copied ? 'check' : 'copy'" size="xs" />
            </button>
          </div>
        </div>
      </div>

      <div class="mt-3 flex flex-wrap items-center gap-1.5">
        <span class="billing-badge inline-flex items-center rounded px-2 py-1 text-[11px] font-semibold">
          {{ billingModeLabel }}
        </span>
      </div>
    </header>

    <div class="flex flex-1 flex-col px-4 py-4 sm:px-5">
      <div class="mb-4">
        <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
          {{ t('modelPlaza.card.group') }}
        </p>
        <Select
          v-if="groupOptions.length > 1"
          :model-value="selectedGroupId"
          :options="groupOptions"
          :disabled="officialPricing"
          class="mt-1.5 w-full"
          @update:model-value="selectGroup"
        />
        <div
          v-else
          class="mt-1.5 min-w-0 truncate rounded-lg border border-gray-200 bg-gray-50 px-3 py-2.5 text-sm font-semibold text-gray-800 dark:border-dark-700 dark:bg-dark-900/50 dark:text-gray-200"
        >
          {{ currentGroup.name }}
        </div>

        <div class="mt-2 flex min-h-6 flex-wrap items-center gap-1.5">
          <span
            v-if="isOfficialPricing"
            class="inline-flex items-center rounded bg-sky-50 px-2 py-1 text-[11px] font-semibold text-sky-700 dark:bg-sky-500/10 dark:text-sky-300"
          >
            {{ t('modelPlaza.card.officialReference') }}
          </span>
          <template v-else>
            <span
              v-if="currentGroup.is_free"
              class="inline-flex items-center rounded bg-cyan-50 px-2 py-1 text-[11px] font-semibold text-cyan-700 dark:bg-cyan-500/10 dark:text-cyan-300"
            >
              {{ t('modelPlaza.badges.free') }}
            </span>
            <span
              v-else
              class="inline-flex items-center rounded bg-gray-100 px-2 py-1 font-mono text-[11px] font-semibold text-gray-600 dark:bg-dark-700 dark:text-dark-300"
            >
              {{ formatMultiplier(currentModel.effective_multiplier) }}x
            </span>
            <span
              v-if="currentGroup.is_exclusive"
              class="inline-flex items-center gap-1 rounded bg-fuchsia-50 px-2 py-1 text-[11px] font-medium text-fuchsia-700 dark:bg-fuchsia-500/10 dark:text-fuchsia-300"
            >
              <Icon name="shield" size="xs" />
              {{ t('modelPlaza.badges.exclusive') }}
            </span>
            <span
              v-if="currentGroup.subscription_type === 'subscription'"
              class="inline-flex items-center rounded bg-amber-50 px-2 py-1 text-[11px] font-medium text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"
            >
              {{ t('modelPlaza.badges.subscription') }}
            </span>
            <span
              v-if="isPeakRateActive"
              class="inline-flex items-center gap-1 rounded bg-rose-50 px-2 py-1 text-[11px] font-medium text-rose-700 dark:bg-rose-500/10 dark:text-rose-300"
            >
              <Icon name="clock" size="xs" />
              {{ t('modelPlaza.badges.peak') }}
            </span>
          </template>
        </div>
      </div>

      <template v-if="hasDisplayPricing">
        <div
          v-if="billingMode === BILLING_MODE_TOKEN"
          class="price-grid overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700"
        >
          <div
            v-for="(price, index) in tokenPriceCells"
            :key="price.key"
            class="min-w-0 px-3 py-3"
            :class="{
              'border-l border-gray-200 dark:border-dark-700': index % 2 === 1,
              'border-t border-gray-200 dark:border-dark-700': index >= 2
            }"
          >
            <p class="truncate text-[11px] font-medium text-gray-500 dark:text-dark-400">
              {{ price.label }}
            </p>
            <p class="mt-1 truncate font-mono text-base font-semibold tabular-nums text-gray-950 dark:text-white">
              {{ price.current }}
            </p>
          </div>
        </div>

        <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
          <div
            v-for="(row, index) in requestPriceRows"
            :key="`${row.label}-${index}`"
            class="flex min-h-14 items-center justify-between gap-4 px-3 py-2.5"
            :class="{ 'border-t border-gray-200 dark:border-dark-700': index > 0 }"
          >
            <span class="min-w-0 truncate text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ row.label }}
            </span>
            <span class="flex-shrink-0 font-mono text-base font-semibold tabular-nums text-gray-950 dark:text-white">
              {{ row.price }}
              <span class="font-sans text-[11px] font-normal text-gray-400 dark:text-dark-500">{{ perUnitSuffix }}</span>
            </span>
          </div>
          <p
            v-if="requestPriceRows.length === 0"
            class="px-3 py-6 text-center text-sm text-gray-400 dark:text-dark-500"
          >
            {{ t('modelPlaza.detail.noPricing') }}
          </p>
        </div>

        <div v-if="tokenIntervals.length > 0" class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700/70">
          <button
            type="button"
            class="flex w-full items-center justify-between gap-3 text-left text-xs font-medium text-gray-600 transition hover:text-gray-950 dark:text-dark-300 dark:hover:text-white"
            :aria-expanded="intervalsExpanded"
            @click="intervalsExpanded = !intervalsExpanded"
          >
            <span>{{ t('modelPlaza.card.contextPricing', { count: tokenIntervals.length }) }}</span>
            <Icon :name="intervalsExpanded ? 'chevronUp' : 'chevronDown'" size="xs" />
          </button>
          <div v-if="intervalsExpanded" class="mt-2 divide-y divide-gray-100 dark:divide-dark-700/70">
            <div v-for="(interval, index) in tokenIntervals" :key="index" class="py-3 first:pt-1">
              <p class="mb-2 text-xs font-semibold text-gray-700 dark:text-gray-300">
                {{ tierLabel(interval) }}
              </p>
              <div class="grid grid-cols-2 gap-x-4 gap-y-2">
                <div
                  v-for="field in intervalPriceFields"
                  :key="field.key"
                  class="flex min-w-0 items-baseline justify-between gap-2"
                >
                  <span class="truncate text-[10px] text-gray-400 dark:text-dark-500">{{ field.label }}</span>
                  <span class="flex-shrink-0 font-mono text-xs font-medium tabular-nums text-gray-700 dark:text-gray-300">
                    {{ currentPrice(interval[field.key]) }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>

      <div
        v-else
        class="flex min-h-36 items-center justify-center rounded-lg border border-dashed border-gray-300 text-sm text-gray-400 dark:border-dark-600 dark:text-dark-500"
      >
        {{ t('modelPlaza.detail.noPricing') }}
      </div>

      <div v-if="isOfficialPricing || currentGroup.description || peakNote" class="mt-auto pt-4">
        <p v-if="isOfficialPricing" class="text-xs leading-5 text-sky-700 dark:text-sky-300">
          {{ t('modelPlaza.card.officialReferenceHint') }}
        </p>
        <p v-else-if="currentGroup.description" class="text-xs leading-5 text-gray-500 dark:text-dark-400">
          {{ currentGroup.description }}
        </p>
        <p v-if="peakNote" class="mt-1.5 flex items-start gap-1.5 text-[11px] leading-4 text-rose-600 dark:text-rose-300">
          <Icon name="clock" size="xs" class="mt-0.5 flex-shrink-0" />
          <span>{{ peakNote }}</span>
        </p>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { formatScaled } from '@/utils/pricing'
import { platformAccentColor } from '@/utils/platformColors'
import { formatPeakRateWindow, hasPeakRate, serverTimezoneLabel } from '@/utils/peak-rate'
import { useAppStore } from '@/stores/app'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
import type { GroupPlatform } from '@/types'
import type { UserPricingInterval } from '@/api/channels'
import type { PlazaModelCardData, PlazaModelOffer } from './modelPlazaPresentation'
import {
  PLAZA_OFFICIAL_GROUP_ID,
  plazaBillingMode,
  plazaHasOfficialPricing,
  plazaProviderLabel
} from './modelPlazaPresentation'

const props = defineProps<{
  card: PlazaModelCardData
  officialPricing?: boolean
}>()

const { t } = useI18n()
const appStore = useAppStore()
const selectedGroupId = ref<number | string>(props.card.offers[0]?.group.id ?? 0)
const intervalsExpanded = ref(false)
const copied = ref(false)
let copiedTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.card.offers.map((offer) => offer.group.id),
  (groupIds) => {
    if (
      selectedGroupId.value === PLAZA_OFFICIAL_GROUP_ID &&
      props.card.offers[0]?.model &&
      plazaHasOfficialPricing(props.card.offers[0].model)
    ) {
      return
    }
    if (typeof selectedGroupId.value !== 'number' || !groupIds.includes(selectedGroupId.value)) {
      selectedGroupId.value = groupIds[0] ?? 0
    }
    intervalsExpanded.value = false
  }
)

const currentOffer = computed<PlazaModelOffer>(
  () => props.card.offers.find((offer) => offer.group.id === selectedGroupId.value) ?? props.card.offers[0]!
)
const currentGroup = computed(() => currentOffer.value.group)
const currentModel = computed(() => currentOffer.value.model)
const displayPricing = computed(() => currentModel.value.display_pricing)
const isOfficialPricing = computed(() => selectedGroupId.value === PLAZA_OFFICIAL_GROUP_ID)
const hasOfficialPricing = computed(() => plazaHasOfficialPricing(currentModel.value))
const activePricing = computed(() =>
  isOfficialPricing.value ? currentModel.value.pricing : displayPricing.value
)
const hasDisplayPricing = computed(() =>
  isOfficialPricing.value ? hasOfficialPricing.value : activePricing.value != null
)
const billingMode = computed(() =>
  plazaBillingMode(currentModel.value, isOfficialPricing.value) as BillingMode
)
const providerLabel = computed(() => plazaProviderLabel(props.card.platform))
const accentStyle = computed(() => ({ '--plaza-accent': platformAccentColor(props.card.platform) }))
const groupOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = props.card.offers.map(({ group }) => ({ value: group.id, label: group.name }))
  if (hasOfficialPricing.value) {
    options.push({ value: PLAZA_OFFICIAL_GROUP_ID, label: t('modelPlaza.card.officialGroup') })
  }
  return options
})

watch(
  () => props.officialPricing,
  (officialPricing) => {
    if (officialPricing && hasOfficialPricing.value) {
      selectedGroupId.value = PLAZA_OFFICIAL_GROUP_ID
    } else if (!officialPricing && selectedGroupId.value === PLAZA_OFFICIAL_GROUP_ID) {
      selectedGroupId.value = props.card.offers[0]?.group.id ?? 0
    }
    intervalsExpanded.value = false
  },
  { immediate: true }
)

const billingModeLabel = computed(() => t(`modelPlaza.billingModes.${billingMode.value}`))
const isPeakRateActive = computed(
  () =>
    !isOfficialPricing.value &&
    !currentGroup.value.is_free &&
    currentGroup.value.applied_peak_multiplier !== 1
)

const peakNote = computed(() => {
  if (isOfficialPricing.value || currentGroup.value.is_free || !hasPeakRate(currentGroup.value)) return ''
  const window = formatPeakRateWindow(
    currentGroup.value,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
  return t('modelPlaza.detail.peakNote', {
    window,
    multiplier: currentGroup.value.peak_rate_multiplier
  })
})

type TokenPriceKey = 'input_price' | 'output_price' | 'cache_write_price' | 'cache_read_price'

const intervalPriceFields = computed<Array<{ key: TokenPriceKey; label: string }>>(() => [
  { key: 'input_price', label: t('modelPlaza.table.input') },
  { key: 'output_price', label: t('modelPlaza.table.output') },
  { key: 'cache_write_price', label: t('modelPlaza.table.cacheWrite') },
  { key: 'cache_read_price', label: t('modelPlaza.table.cacheRead') }
])

const tokenIntervals = computed(() => activePricing.value?.intervals ?? [])

const tokenPriceCells = computed(() =>
  intervalPriceFields.value.map((field) => ({
    key: field.key,
    label: field.label,
    current: currentTokenPrice(field.key)
  }))
)

const requestPriceRows = computed(() => {
  const pricing = activePricing.value
  if (!pricing) return []
  const rows = pricing.intervals
    .filter((interval) => interval.per_request_price != null)
    .map((interval) => ({ label: tierLabel(interval), price: currentPrice(interval.per_request_price) }))
  if (rows.length > 0) return rows
  if (pricing.per_request_price == null) return []
  return [{ label: t('modelPlaza.card.defaultSpecification'), price: currentPrice(pricing.per_request_price) }]
})

const perUnitSuffix = computed(() => {
  if (billingMode.value === BILLING_MODE_IMAGE) return t('modelPlaza.table.perUnitImage')
  if (billingMode.value === BILLING_MODE_VIDEO) return t('modelPlaza.table.perUnitSecond')
  return t('modelPlaza.table.perUnitRequest')
})

function selectGroup(value: string | number | boolean | null) {
  if (typeof value === 'number' || value === PLAZA_OFFICIAL_GROUP_ID) {
    selectedGroupId.value = value
    intervalsExpanded.value = false
  }
}

function currentTokenPrice(key: TokenPriceKey): string {
  const value = activePricing.value?.[key]
  if (value != null) return currentPrice(value)
  if (tokenIntervals.value.some((interval) => interval[key] != null)) return t('modelPlaza.card.tiered')
  return '-'
}

function currentPrice(value: number | null | undefined): string {
  if (value == null) return '-'
  if (!isOfficialPricing.value && (currentGroup.value.is_free || value === 0)) {
    return t('modelPlaza.badges.free')
  }
  return formatPlazaPrice(value, billingMode.value === BILLING_MODE_TOKEN ? 1_000_000 : 1)
}

function formatPlazaPrice(value: number, scale: number): string {
  return formatScaled(value, scale, 2).replace(/^\$/, '￥')
}

function tierLabel(interval: UserPricingInterval): string {
  if (interval.tier_label) return interval.tier_label
  if (interval.max_tokens == null) return `>${formatTokenCount(interval.min_tokens)}`
  if (interval.min_tokens === 0) return `≤${formatTokenCount(interval.max_tokens)}`
  return `${formatTokenCount(interval.min_tokens)}–${formatTokenCount(interval.max_tokens)}`
}

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) return `${trimNumber(value / 1_000_000)}M`
  if (value >= 1_000) return `${trimNumber(value / 1_000)}K`
  return String(value)
}

function trimNumber(value: number): string {
  return String(Math.round(value * 100) / 100)
}

function formatMultiplier(value: number): string {
  return Number.isInteger(value) ? String(value) : String(Math.round(value * 1000) / 1000)
}

async function copyModelName() {
  try {
    await navigator.clipboard.writeText(props.card.name)
    copied.value = true
    if (copiedTimer) clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => {
      copied.value = false
    }, 1600)
  } catch {
    copied.value = false
  }
}

onBeforeUnmount(() => {
  if (copiedTimer) clearTimeout(copiedTimer)
})
</script>

<style scoped>
.model-card {
  --plaza-accent: #0f9f9a;
}

.provider-mark {
  color: var(--plaza-accent);
  background-color: color-mix(in srgb, var(--plaza-accent) 10%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--plaza-accent) 20%, transparent);
}

.billing-badge {
  color: color-mix(in srgb, var(--plaza-accent) 80%, black);
  background-color: color-mix(in srgb, var(--plaza-accent) 9%, transparent);
}

.dark .billing-badge {
  color: color-mix(in srgb, var(--plaza-accent) 68%, white);
}

.price-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
</style>
