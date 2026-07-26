<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-3 xl:flex-row xl:items-center">
          <div class="flex flex-1 flex-col gap-3 sm:flex-row sm:flex-wrap">
            <div class="relative w-full sm:w-72">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="filters.search"
                type="search"
                class="input pl-10"
                :placeholder="t('modelPricing.searchPlaceholder')"
              />
            </div>
            <Select
              v-model="filters.platform"
              :options="platformOptions"
              class="w-full sm:w-48"
            />
            <Select
              v-model="filters.groupId"
              :options="groupOptions"
              class="w-full sm:w-56"
            />
            <Select
              v-model="filters.billingCategory"
              :options="billingOptions"
              class="w-full sm:w-44"
            />
          </div>

          <div class="flex items-center justify-between gap-3 sm:justify-end">
            <span class="text-sm tabular-nums text-gray-500 dark:text-gray-400">
              {{ t('modelPricing.resultCount', { count: filteredRows.length }) }}
            </span>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="loadPricing"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div class="table-wrapper">
          <table class="min-w-[1280px]">
            <thead>
              <tr>
                <th class="w-[24%]">{{ t('modelPricing.columns.model') }}</th>
                <th class="w-[12%]">{{ t('modelPricing.columns.platform') }}</th>
                <th class="w-[14%]">{{ t('modelPricing.columns.channel') }}</th>
                <th class="w-[22%]">{{ t('modelPricing.columns.groups') }}</th>
                <th>{{ t('modelPricing.columns.billingMethod') }}</th>
                <th class="text-right">{{ t('modelPricing.columns.input') }}</th>
                <th class="text-right">{{ t('modelPricing.columns.output') }}</th>
                <th class="text-right">{{ t('modelPricing.columns.cacheWrite') }}</th>
                <th class="text-right">{{ t('modelPricing.columns.cacheRead') }}</th>
                <th class="text-right">{{ t('modelPricing.columns.perRequest') }}</th>
              </tr>
            </thead>
            <tbody>
              <template v-if="loading">
                <tr v-for="index in 6" :key="index">
                  <td v-for="column in 10" :key="column">
                    <div class="h-4 animate-pulse rounded bg-gray-100 dark:bg-dark-700" />
                  </td>
                </tr>
              </template>
              <tr v-else-if="filteredRows.length === 0">
                <td colspan="10" class="py-16 text-center">
                  <Icon name="database" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
                  <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">
                    {{ emptyLabel }}
                  </p>
                </td>
              </tr>
              <tr v-for="row in filteredRows" v-else :key="row.key">
                <td>
                  <span class="font-mono text-sm font-semibold text-gray-900 dark:text-gray-100">
                    {{ row.model.name }}
                  </span>
                </td>
                <td>
                  <span class="inline-flex items-center gap-1.5 text-sm font-medium" :class="platformTextClass(row.platform)">
                    <PlatformIcon :platform="row.platform as GroupPlatform" size="sm" />
                    {{ platformLabel(row.platform) }}
                  </span>
                </td>
                <td class="text-sm text-gray-600 dark:text-gray-300">{{ row.channelName }}</td>
                <td>
                  <div class="flex flex-wrap gap-1.5">
                    <GroupBadge
                      v-for="group in visibleGroups(row)"
                      :key="group.id"
                      :name="group.name"
                      :platform="group.platform as GroupPlatform"
                      :subscription-type="group.subscription_type as SubscriptionType"
                      :rate-multiplier="group.rate_multiplier"
                      :is-free="group.is_free"
                      :user-rate-multiplier="userGroupRates[group.id] ?? null"
                      always-show-rate
                    />
                  </div>
                </td>
                <td>
                  <span class="billing-badge" :class="billingBadgeClass(row)">
                    {{ billingLabel(row) }}
                  </span>
                </td>
                <td class="text-right"><span class="price-value">{{ formatUsagePrice(row, row.model.pricing?.input_price) }}</span></td>
                <td class="text-right"><span class="price-value">{{ formatUsagePrice(row, row.model.pricing?.output_price) }}</span></td>
                <td class="text-right"><span class="price-value">{{ formatUsagePrice(row, row.model.pricing?.cache_write_price) }}</span></td>
                <td class="text-right"><span class="price-value">{{ formatUsagePrice(row, row.model.pricing?.cache_read_price) }}</span></td>
                <td class="text-right">
                  <div v-if="requestPrices(row).length" class="space-y-1">
                    <div v-for="price in requestPrices(row)" :key="price.label" class="flex items-center justify-end gap-2">
                      <span class="max-w-28 truncate text-xs text-gray-500 dark:text-gray-400" :title="price.label">{{ price.label }}</span>
                      <span class="price-value">{{ price.value }}</span>
                    </div>
                  </div>
                  <span v-else class="price-value">-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { platformTextClass } from '@/utils/platformColors'
import { formatScaled } from '@/utils/pricing'
import {
  collectAccessibleGroups,
  filterModelPricingRows,
  flattenModelPricingRows,
  modelBillingCategory,
  type ModelBillingCategory,
  type ModelPricingRow,
} from './modelPricingRows'

const { t } = useI18n()
const appStore = useAppStore()
const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const filters = reactive({
  search: '',
  platform: null as string | null,
  groupId: null as number | null,
  billingCategory: null as ModelBillingCategory | null,
})

const rows = computed(() => flattenModelPricingRows(channels.value))
const filteredRows = computed(() => filterModelPricingRows(rows.value, filters))
const accessibleGroups = computed(() => collectAccessibleGroups(channels.value))

const platformOptions = computed<SelectOption[]>(() => {
  const platforms = [...new Set(rows.value.map((row) => row.platform))].sort()
  return [
    { value: null, label: t('modelPricing.filters.allPlatforms') },
    ...platforms.map((platform) => ({ value: platform, label: platformLabel(platform) })),
  ]
})

const groupOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('modelPricing.filters.allGroups') },
  ...accessibleGroups.value.map((group) => ({
    value: group.id,
    label: `${group.name} · ${platformLabel(group.platform)}`,
  })),
])

const billingOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('modelPricing.filters.allBillingMethods') },
  { value: 'usage', label: t('modelPricing.billing.usage') },
  { value: 'request', label: t('modelPricing.billing.request') },
])

const emptyLabel = computed(() =>
  channels.value.length === 0
    ? t('modelPricing.empty.unconfigured')
    : t('modelPricing.empty.filtered'),
)

function platformLabel(platform: string): string {
  return t(`admin.groups.platforms.${platform}`, platform)
}

function formatUsagePrice(row: ModelPricingRow, value: number | null | undefined): string {
  if (modelBillingCategory(row.model) !== 'usage') return '-'
  return value == null ? '-' : formatScaled(value, 1_000_000)
}

function requestPrices(row: ModelPricingRow): Array<{ label: string; value: string }> {
	if (modelBillingCategory(row.model) !== 'request' || !row.model.pricing) return []
	const prices = row.model.pricing.intervals
	  .filter(interval => interval.per_request_price != null)
	  .map((interval, index) => ({
	    label: interval.tier_label || `#${index + 1}`,
	    value: formatScaled(interval.per_request_price as number, 1),
	  }))
	const fallback = row.model.pricing.billing_mode === 'image'
	  ? row.model.pricing.image_output_price ?? row.model.pricing.per_request_price
	  : row.model.pricing.per_request_price
	if (fallback != null) {
	  prices.unshift({ label: t('modelPricing.tiers.default'), value: formatScaled(fallback, 1) })
	}
	return prices
}

function billingLabel(row: ModelPricingRow): string {
  const category = modelBillingCategory(row.model)
  return t(`modelPricing.billing.${category}`)
}

function billingBadgeClass(row: ModelPricingRow): string {
  switch (modelBillingCategory(row.model)) {
    case 'usage':
      return 'bg-cyan-50 text-cyan-700 dark:bg-cyan-900/25 dark:text-cyan-300'
    case 'request':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/25 dark:text-amber-300'
    default:
      return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
  }
}

function visibleGroups(row: ModelPricingRow) {
  if (filters.groupId == null) return row.groups
  return row.groups.filter((group) => group.id === filters.groupId)
}

async function loadPricing() {
  loading.value = true
  try {
    const [availableChannels, rates] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates().catch(() => ({} as Record<number, number>)),
    ])
    channels.value = availableChannels
    userGroupRates.value = rates
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadPricing)
</script>

<style scoped>
.price-value {
  @apply font-mono text-sm tabular-nums text-gray-800 dark:text-gray-200;
}

.billing-badge {
  @apply inline-flex whitespace-nowrap rounded px-2 py-1 text-xs font-medium;
}
</style>
