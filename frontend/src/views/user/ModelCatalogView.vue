<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-5 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
        <div class="flex flex-1 flex-col gap-3 sm:flex-row">
          <label class="relative block w-full sm:max-w-md">
            <span class="sr-only">{{ t('modelCatalog.searchPlaceholder') }}</span>
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="searchQuery" class="input pl-10" :placeholder="t('modelCatalog.searchPlaceholder')" />
          </label>
          <select v-model="platformFilter" class="input sm:w-44" :aria-label="t('modelCatalog.platformFilter')">
            <option value="">{{ t('modelCatalog.allPlatforms') }}</option>
            <option v-for="platform in platforms" :key="platform" :value="platform">{{ platformLabel(platform) }}</option>
          </select>
          <select v-model="capabilityFilter" class="input sm:w-44" :aria-label="t('modelCatalog.capabilityFilter')">
            <option value="">{{ t('modelCatalog.allCapabilities') }}</option>
            <option v-for="capability in capabilities" :key="capability" :value="capability">
              {{ t(`modelCatalog.capabilities.${capability}`, capability) }}
            </option>
          </select>
        </div>
        <button type="button" class="icon-btn self-end lg:self-auto" :title="t('common.refresh')" :disabled="loading" @click="loadModels">
          <Icon name="refresh" size="md" :class="{ 'animate-spin': loading }" />
        </button>
      </div>

      <div v-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <div v-for="index in 6" :key="index" class="h-64 animate-pulse rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900" />
      </div>

      <div v-else-if="errorMessage" class="py-14 text-center">
        <Icon name="exclamationCircle" size="xl" class="mx-auto text-red-500" />
        <h3 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('modelCatalog.loadFailed') }}</h3>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ errorMessage }}</p>
        <button type="button" class="btn btn-secondary mt-5" @click="loadModels">{{ t('modelCatalog.retry') }}</button>
      </div>

      <EmptyState
        v-else-if="filteredModels.length === 0"
        :title="t('modelCatalog.emptyTitle')"
        :description="t('modelCatalog.emptyDescription')"
        :action-text="hasFilters ? t('modelCatalog.clearFilters') : undefined"
        :action-icon="false"
        @action="clearFilters"
      />

      <div v-else class="grid items-stretch gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <button
          v-for="item in filteredModels"
          :key="`${item.platform}:${item.name}`"
          type="button"
          class="group relative flex min-h-64 flex-col overflow-hidden rounded-md border bg-white p-5 text-left shadow-sm transition hover:-translate-y-0.5 hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:bg-dark-900"
          :class="platformBorderClass(item.platform)"
          @click="selectedModel = item"
        >
          <span class="absolute inset-x-0 top-0 h-1" :class="platformAccentBarClass(item.platform)" />
          <div class="flex items-start justify-between gap-3">
            <span :class="['inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium', platformBadgeClass(item.platform)]">
              <PlatformIcon :platform="item.platform as GroupPlatform" size="sm" />
              {{ platformLabel(item.platform) }}
            </span>
            <span v-if="item.is_recommended" class="inline-flex items-center gap-1 text-xs font-medium text-amber-700 dark:text-amber-300">
              <Icon name="sparkles" size="xs" />{{ t('modelCatalog.recommended') }}
            </span>
          </div>

          <h3 class="mt-4 break-words text-base font-semibold text-gray-950 dark:text-white">{{ item.display_name }}</h3>
          <p class="mt-1 break-all font-mono text-[11px] text-gray-400 dark:text-gray-500">{{ item.name }}</p>
          <p class="mt-3 line-clamp-3 min-h-[3.75rem] text-sm leading-5 text-gray-600 dark:text-gray-300">
            {{ item.description || t('modelCatalog.noDescription') }}
          </p>

          <div class="mt-4 flex flex-wrap gap-1.5">
            <span v-for="capability in item.capabilities" :key="capability" class="badge badge-gray text-[11px]">
              {{ t(`modelCatalog.capabilities.${capability}`, capability) }}
            </span>
          </div>

          <div class="mt-auto flex items-end justify-between gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
            <div>
              <p class="text-[11px] text-gray-400 dark:text-gray-500">{{ t('modelCatalog.startingPrice') }}</p>
              <p class="mt-1 text-sm font-semibold" :class="platformTextClass(item.platform)">{{ priceSummary(item) }}</p>
            </div>
            <div class="text-right text-xs text-gray-500 dark:text-gray-400">
              <p>{{ t('modelCatalog.offerCount', { count: item.offers.length }) }}</p>
              <p v-if="item.context_window > 0" class="mt-1 font-mono">{{ formatCompactNumber(item.context_window) }} ctx</p>
            </div>
          </div>
        </button>
      </div>
    </div>

    <ModelCatalogDetailDialog
      :show="selectedModel !== null"
      :item="selectedModel"
      :base-url="apiBaseUrl"
      @close="selectedModel = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ModelCatalogDetailDialog from '@/components/model-catalog/ModelCatalogDetailDialog.vue'
import modelCatalogAPI, { type ModelCatalogItem } from '@/api/modelCatalog'
import type { GroupPlatform } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatCompactNumber } from '@/utils/format'
import { formatScaled } from '@/utils/pricing'
import {
  platformAccentBarClass,
  platformBadgeClass,
  platformBorderClass,
  platformLabel,
  platformTextClass,
} from '@/utils/platformColors'
import { BILLING_MODE_IMAGE, BILLING_MODE_PER_REQUEST, BILLING_MODE_TOKEN } from '@/constants/channel'

const { t } = useI18n()
const appStore = useAppStore()
const models = ref<ModelCatalogItem[]>([])
const loading = ref(false)
const errorMessage = ref('')
const searchQuery = ref('')
const platformFilter = ref('')
const capabilityFilter = ref('')
const selectedModel = ref<ModelCatalogItem | null>(null)
let requestController: AbortController | null = null

const apiBaseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || '')
const platforms = computed(() => [...new Set(models.value.map((item) => item.platform))].sort())
const capabilities = computed(() => [...new Set(models.value.flatMap((item) => item.capabilities))].sort())
const hasFilters = computed(() => Boolean(searchQuery.value || platformFilter.value || capabilityFilter.value))
const filteredModels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return models.value.filter((item) => {
    if (platformFilter.value && item.platform !== platformFilter.value) return false
    if (capabilityFilter.value && !item.capabilities.includes(capabilityFilter.value)) return false
    if (!query) return true
    return [item.name, item.display_name, item.description, platformLabel(item.platform), ...item.capabilities]
      .some((value) => value.toLowerCase().includes(query))
  })
})

function priceSummary(item: ModelCatalogItem): string {
  const priced = item.offers
    .filter((offer) => offer.pricing !== null)
    .flatMap((offer) => offer.groups.map((group) => ({
      pricing: offer.pricing!,
      rate: normalizedRate(group.rate_multiplier),
    })))
  const tokenPrices = priced
    .filter(({ pricing }) => pricing.billing_mode === BILLING_MODE_TOKEN)
    .flatMap(({ pricing, rate }) => [pricing.input_price, ...pricing.intervals.map((interval) => interval.input_price)]
      .filter((price): price is number => price != null)
      .map((price) => price * rate))
  if (tokenPrices.length) return `${formatScaled(Math.min(...tokenPrices), 1_000_000)} / 1M`
  const requestPrices = priced
    .filter(({ pricing }) => pricing.billing_mode === BILLING_MODE_PER_REQUEST)
    .flatMap(({ pricing, rate }) => [pricing.per_request_price, ...pricing.intervals.map((interval) => interval.per_request_price)]
      .filter((price): price is number => price != null)
      .map((price) => price * rate))
  if (requestPrices.length) return `${formatScaled(Math.min(...requestPrices), 1)} / req`
  const imagePrices = priced
    .filter(({ pricing }) => pricing.billing_mode === BILLING_MODE_IMAGE)
    .map(({ pricing, rate }) => {
      const price = pricing.image_output_price ?? pricing.per_request_price
      return price == null ? null : price * rate
    })
    .filter((price): price is number => price != null)
  if (imagePrices.length) return `${formatScaled(Math.min(...imagePrices), 1)} / image`
  return t('modelCatalog.pricing.notConfigured')
}

function normalizedRate(rate: number): number {
  return Number.isFinite(rate) && rate >= 0 ? rate : 1
}

function clearFilters() {
  searchQuery.value = ''
  platformFilter.value = ''
  capabilityFilter.value = ''
}

async function loadModels() {
  requestController?.abort()
  requestController = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    models.value = await modelCatalogAPI.list({ signal: requestController.signal })
  } catch (error: unknown) {
    if (requestController.signal.aborted) return
    errorMessage.value = extractApiErrorMessage(error, t('modelCatalog.loadFailed'))
  } finally {
    if (!requestController.signal.aborted) loading.value = false
  }
}

onMounted(loadModels)
onBeforeUnmount(() => requestController?.abort())
</script>
