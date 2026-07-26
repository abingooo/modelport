<template>
  <BaseDialog
    :show="show"
    :title="item?.display_name || item?.name || ''"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="item" class="space-y-6">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div class="min-w-0 space-y-2">
          <div class="flex flex-wrap items-center gap-2">
            <span :class="['inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium', platformBadgeClass(item.platform)]">
              <PlatformIcon :platform="item.platform as GroupPlatform" size="sm" />
              {{ platformLabel(item.platform) }}
            </span>
            <span v-for="capability in item.capabilities" :key="capability" class="badge badge-gray">
              {{ t(`modelCatalog.capabilities.${capability}`, capability) }}
            </span>
          </div>
          <p class="break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ item.name }}</p>
          <p class="max-w-3xl text-sm leading-6 text-gray-600 dark:text-gray-300">
            {{ item.description || t('modelCatalog.noDescription') }}
          </p>
        </div>
        <div v-if="item.context_window > 0" class="flex-shrink-0 text-right">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelCatalog.contextWindow') }}</p>
          <p class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">
            {{ formatCompactNumber(item.context_window) }}
          </p>
        </div>
      </div>

      <section>
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('modelCatalog.availableOffers') }}</h4>
        <div class="space-y-3">
          <div
            v-for="offer in item.offers"
            :key="offer.channel_id"
            class="border-l-2 border-gray-200 bg-gray-50/70 px-4 py-3 dark:border-dark-600 dark:bg-dark-800/50"
            :class="platformBorderClass(item.platform)"
          >
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ offer.channel_name }}</span>
            </div>
            <div v-if="offer.pricing" class="mt-3 divide-y divide-gray-200 dark:divide-dark-600">
              <div v-for="group in offer.groups" :key="group.id" class="py-3 first:pt-0 last:pb-0">
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <span class="font-medium text-gray-800 dark:text-gray-200">{{ group.name }}</span>
                  <span v-if="group.is_free" class="badge bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300">
                    {{ t('admin.groups.freeBilling.badge') }}
                  </span>
                  <span v-else class="badge badge-gray">×{{ formatRate(group.rate_multiplier) }}</span>
                  <span v-if="group.peak_rate_enabled && !group.is_free" class="badge badge-warning">
                    {{ t('modelCatalog.pricing.peakRate') }} ×{{ peakRate(group) }}
                  </span>
                </div>
                <div class="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-xs text-gray-600 dark:text-gray-300">
                  <template v-if="offer.pricing.billing_mode === BILLING_MODE_TOKEN">
                    <span>{{ t('modelCatalog.pricing.input') }} <strong>{{ formatGroupPrice(offer.pricing.input_price, group, 1_000_000) }}</strong> / 1M</span>
                    <span>{{ t('modelCatalog.pricing.output') }} <strong>{{ formatGroupPrice(offer.pricing.output_price, group, 1_000_000) }}</strong> / 1M</span>
                    <span v-if="offer.pricing.cache_write_price != null">{{ t('modelCatalog.pricing.cacheWrite') }} <strong>{{ formatGroupPrice(offer.pricing.cache_write_price, group, 1_000_000) }}</strong> / 1M</span>
                    <span v-if="offer.pricing.cache_read_price != null">{{ t('modelCatalog.pricing.cacheRead') }} <strong>{{ formatGroupPrice(offer.pricing.cache_read_price, group, 1_000_000) }}</strong> / 1M</span>
                  </template>
                  <template v-else-if="offer.pricing.billing_mode === BILLING_MODE_IMAGE">
                    <span>{{ t('modelCatalog.pricing.perImage') }} <strong>{{ formatGroupPrice(offer.pricing.image_output_price ?? offer.pricing.per_request_price, group, 1) }}</strong></span>
                    <span v-if="offer.pricing.image_input_price != null">{{ t('modelCatalog.pricing.imageInput') }} <strong>{{ formatGroupPrice(offer.pricing.image_input_price, group, 1_000_000) }}</strong> / 1M</span>
                  </template>
                  <span v-else-if="offer.pricing.billing_mode === BILLING_MODE_PER_REQUEST">
                    {{ t('modelCatalog.pricing.perRequest') }} <strong>{{ formatGroupPrice(offer.pricing.per_request_price, group, 1) }}</strong>
                  </span>
                </div>
                <div v-if="offer.pricing.intervals.length" class="mt-3 space-y-2 border-t border-gray-200 pt-2 dark:border-dark-600">
                  <div v-for="(interval, index) in offer.pricing.intervals" :key="index" class="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-[11px]">
                    <span class="font-medium text-gray-500 dark:text-gray-400">{{ intervalLabel(interval) }}</span>
                    <span class="text-gray-700 dark:text-gray-300">
                      <template v-if="offer.pricing.billing_mode === BILLING_MODE_TOKEN">
                        {{ t('modelCatalog.pricing.input') }} {{ formatGroupPrice(interval.input_price, group, 1_000_000) }} ·
                        {{ t('modelCatalog.pricing.output') }} {{ formatGroupPrice(interval.output_price, group, 1_000_000) }} / 1M
                      </template>
                      <template v-else>
                        {{ formatGroupPrice(interval.per_request_price, group, 1) }} / {{ t('modelCatalog.pricing.requestUnit') }}
                      </template>
                    </span>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('modelCatalog.pricing.notConfigured') }}
            </div>
          </div>
        </div>
      </section>

      <section v-if="item.interface_formats.length">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('modelCatalog.callExample') }}</h4>
          <div class="inline-flex rounded-md bg-gray-100 p-1 dark:bg-dark-800" role="tablist">
            <button
              v-for="format in item.interface_formats"
              :key="format"
              type="button"
              role="tab"
              :aria-selected="activeFormat === format"
              :class="[
                'rounded px-3 py-1.5 text-xs font-medium transition-colors',
                activeFormat === format
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                  : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200',
              ]"
              @click="activeFormat = format"
            >
              {{ t(`modelCatalog.formats.${format}`) }}
            </button>
          </div>
        </div>
        <div class="overflow-hidden rounded-md bg-gray-950">
          <div class="flex items-center justify-between border-b border-white/10 px-4 py-2">
            <span class="text-xs font-medium uppercase text-gray-400">curl</span>
            <button type="button" class="icon-btn text-gray-300 hover:bg-white/10 hover:text-white" :title="t('common.copy')" @click="copyExample">
              <Icon :name="copied ? 'check' : 'copy'" size="sm" />
            </button>
          </div>
          <pre class="max-h-80 overflow-auto p-4 text-xs leading-6 text-gray-100"><code>{{ currentExample }}</code></pre>
        </div>
      </section>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import type { ModelCatalogGroup, ModelCatalogItem, ModelCatalogPricingInterval, ModelInterfaceFormat } from '@/api/modelCatalog'
import { buildModelCatalogExample } from '@/utils/modelCatalogExamples'
import { formatCompactNumber } from '@/utils/format'
import { formatScaled } from '@/utils/pricing'
import { platformBadgeClass, platformBorderClass, platformLabel } from '@/utils/platformColors'
import { BILLING_MODE_IMAGE, BILLING_MODE_PER_REQUEST, BILLING_MODE_TOKEN } from '@/constants/channel'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{ show: boolean; item: ModelCatalogItem | null; baseUrl?: string }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const activeFormat = ref<ModelInterfaceFormat>('openai')
const copied = ref(false)

watch(
  () => [props.show, props.item] as const,
  () => {
    activeFormat.value = props.item?.interface_formats[0] || 'openai'
    copied.value = false
  },
  { immediate: true },
)

const currentExample = computed(() => {
  if (!props.item) return ''
  return buildModelCatalogExample(props.item, activeFormat.value, props.baseUrl)
})

async function copyExample() {
  if (!currentExample.value) return
  await copyToClipboard(currentExample.value)
  copied.value = true
  window.setTimeout(() => { copied.value = false }, 1600)
}

function normalizedRate(rate: number): number {
  return Number.isFinite(rate) && rate >= 0 ? rate : 1
}

function formatRate(rate: number): string {
  return normalizedRate(rate).toFixed(4).replace(/\.?0+$/, '')
}

function peakRate(group: ModelCatalogGroup): string {
  return formatRate(normalizedRate(group.rate_multiplier) * normalizedRate(group.peak_rate_multiplier))
}

function formatGroupPrice(value: number | null, group: ModelCatalogGroup, scale: number): string {
  return formatScaled(value == null ? null : value * normalizedRate(group.rate_multiplier), scale)
}

function intervalLabel(interval: ModelCatalogPricingInterval): string {
  if (interval.tier_label) return interval.tier_label
  const max = interval.max_tokens == null ? '∞' : formatCompactNumber(interval.max_tokens)
  return `${formatCompactNumber(interval.min_tokens)}-${max} ${t('modelCatalog.pricing.tokens')}`
}
</script>
