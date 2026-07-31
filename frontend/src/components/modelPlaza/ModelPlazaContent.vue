<template>
  <div class="space-y-5" :class="{ 'model-plaza-content-public': !embedded }">
    <div v-if="!embedded" class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-950 dark:text-white sm:text-3xl">{{ t('modelPlaza.title') }}</h1>
        <p class="mt-1.5 max-w-2xl text-sm text-gray-500 dark:text-dark-400">{{ t('modelPlaza.description') }}</p>
      </div>
      <p v-if="pricingSourceLabel" class="text-xs text-gray-400 dark:text-dark-500">
        {{ pricingSourceLabel }}
      </p>
    </div>

    <div
      v-if="descriptionHtml"
      class="plaza-description border-l-2 border-primary-500 bg-primary-50/50 px-4 py-3 text-sm dark:bg-primary-500/5"
      v-html="descriptionHtml"
    ></div>

    <p v-if="!isAuthenticated" class="flex items-center gap-1.5 text-xs text-gray-400 dark:text-dark-500">
      <Icon name="infoCircle" size="xs" />
      {{ t('modelPlaza.anonymousHint') }}
    </p>

    <div v-if="loading" class="flex min-h-[240px] items-center justify-center">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600 dark:border-primary-400/25 dark:border-t-primary-400"></div>
    </div>
    <div
      v-else-if="error"
      class="rounded-lg border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
    >
      {{ t('modelPlaza.loadFailed') }}
    </div>
    <template v-else>
      <PlazaFilterBar
        :platforms="platforms"
        :groups="groupOptions"
        :billing-modes="billingModes"
        :platform="selectedPlatform"
        :group-id="selectedGroupId"
        :show-official-pricing="showOfficialPricing"
        :billing-mode="selectedBillingMode"
        :sort-mode="selectedSortMode"
        :search="searchQuery"
        :result-count="resultCount"
        @update:platform="selectPlatform"
        @update:group-id="selectGroup"
        @update:billing-mode="selectedBillingMode = $event"
        @update:sort-mode="selectSortMode"
        @update:search="searchQuery = $event"
      />

      <div v-if="providerSections.length > 0" class="space-y-8">
        <section v-for="section in providerSections" :key="section.platform" class="min-w-0">
          <header class="mb-3 flex items-center gap-2.5">
            <span class="flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-200">
              <PlatformIcon :platform="section.platform as GroupPlatform" size="md" />
            </span>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ section.label }}</h2>
            <span class="text-xs tabular-nums text-gray-400 dark:text-dark-500">
              {{ t('modelPlaza.provider.modelCount', { count: section.cards.length }) }}
            </span>
            <span class="h-px min-w-6 flex-1 bg-gray-200 dark:bg-dark-700"></span>
          </header>

          <div
            class="plaza-model-grid grid min-w-0 items-stretch gap-4"
            :class="{
              'plaza-model-grid-public': !embedded,
              'plaza-model-grid-public-sparse': !embedded && section.cards.length < 4
            }"
          >
            <PlazaModelCard
              v-for="card in section.cards"
              :key="card.key"
              :card="card"
              :official-pricing="isOfficialGroupSelected"
            />
          </div>
        </section>
      </div>
      <div
        v-else
        class="rounded-lg border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
      >
        {{ filtersActive ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}
      </div>

      <p v-if="embedded && pricingSourceLabel" class="text-right text-xs text-gray-400 dark:text-dark-500">
        {{ pricingSourceLabel }}
      </p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import PlazaFilterBar from './PlazaFilterBar.vue'
import PlazaModelCard from './PlazaModelCard.vue'
import type { GroupPlatform } from '@/types'
import type { ModelPlazaResponse } from '@/api/modelPlaza'
import { useAuthStore } from '@/stores/auth'
import {
  PLAZA_OFFICIAL_GROUP_ID,
  buildPlazaProviderSections,
  plazaBillingMode,
  plazaHasOfficialPricing,
  plazaProviderLabel
} from './modelPlazaPresentation'
import type {
  PlazaFilterGroupData,
  PlazaGroupFilterValue,
  PlazaSortMode
} from './modelPlazaPresentation'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  embedded?: boolean
}>()

const { t, locale } = useI18n()
const authStore = useAuthStore()
const isAuthenticated = computed(() => authStore.isAuthenticated)

const PLATFORM_STORAGE_KEY = 'modelport_model_plaza_platform'
const SORT_STORAGE_KEY = 'modelport_model_plaza_sort'
const GROUP_STORAGE_PREFIX = 'modelport_model_plaza_group_'

function readStorage(key: string): string {
  try {
    return window.localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

function writeStorage(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // Browsing preferences are optional when storage is unavailable.
  }
}

function parseGroupSelection(value: string): PlazaGroupFilterValue {
  if (value === PLAZA_OFFICIAL_GROUP_ID) return value
  const groupId = Number(value)
  return Number.isInteger(groupId) && groupId > 0 ? groupId : 'all'
}

function groupStorageKey(platform: string): string {
  return `${GROUP_STORAGE_PREFIX}${platform || 'all'}`
}

const storedPlatform = readStorage(PLATFORM_STORAGE_KEY)
const selectedPlatform = ref(storedPlatform || 'all')
const selectedGroupId = ref<PlazaGroupFilterValue>(
  parseGroupSelection(readStorage(groupStorageKey(selectedPlatform.value)))
)
const selectedBillingMode = ref('all')
const selectedSortMode = ref<PlazaSortMode>(
  readStorage(SORT_STORAGE_KEY) === 'output' ? 'output' : 'name'
)
const searchQuery = ref('')

const descriptionHtml = computed(() => {
  const markdown = props.response?.description?.trim()
  if (!markdown) return ''
  return DOMPurify.sanitize(marked.parse(markdown) as string)
})

const platforms = computed(() => {
  return buildPlazaProviderSections(props.response?.groups ?? []).map((section) => ({
    id: section.platform,
    count: section.cards.length
  }))
})

const groupOptions = computed<PlazaFilterGroupData[]>(() =>
  (props.response?.groups ?? []).map((group) => ({
    id: group.id,
    name: group.name,
    platforms: [...new Set(group.models.map((model) => model.platform || group.platform))],
    subscriptionType: group.subscription_type,
    effectiveMultiplier: group.effective_rate_multiplier,
    isFree: group.is_free,
    isExclusive: group.is_exclusive
  }))
)

const billingModes = computed(() => {
  const modes = (props.response?.groups ?? []).flatMap((group) => group.models.map(plazaBillingMode))
  return [...new Set(modes)]
})

const showOfficialPricing = computed(() =>
  (props.response?.groups ?? []).some((group) =>
    group.models.some((model) => {
      const platform = model.platform || group.platform
      return (
        (selectedPlatform.value === 'all' || platform === selectedPlatform.value) &&
        plazaHasOfficialPricing(model)
      )
    })
  )
)
const isOfficialGroupSelected = computed(
  () => selectedGroupId.value === PLAZA_OFFICIAL_GROUP_ID
)

function hasOfficialPricingForPlatform(platform: string): boolean {
  return (props.response?.groups ?? []).some((group) =>
    group.models.some((model) => {
      const modelPlatform = model.platform || group.platform
      return (platform === 'all' || modelPlatform === platform) && plazaHasOfficialPricing(model)
    })
  )
}

function groupSelectionAvailable(value: PlazaGroupFilterValue, platform: string): boolean {
  if (value === 'all') return true
  if (value === PLAZA_OFFICIAL_GROUP_ID) return hasOfficialPricingForPlatform(platform)
  const group = groupOptions.value.find((item) => item.id === value)
  return Boolean(group && (platform === 'all' || group.platforms.includes(platform)))
}

watch(
  platforms,
  (availablePlatforms) => {
    if (availablePlatforms.length === 0) return
    if (
      selectedPlatform.value !== 'all' &&
      !availablePlatforms.some((item) => item.id === selectedPlatform.value)
    ) {
      selectPlatform('all')
      return
    }
    if (!groupSelectionAvailable(selectedGroupId.value, selectedPlatform.value)) {
      selectedGroupId.value = 'all'
    }
  },
  { immediate: true }
)

watch(groupOptions, () => {
  if (!groupSelectionAvailable(selectedGroupId.value, selectedPlatform.value)) {
    selectedGroupId.value = 'all'
  }
})

watch(showOfficialPricing, (available) => {
  if (!available && selectedGroupId.value === PLAZA_OFFICIAL_GROUP_ID) selectedGroupId.value = 'all'
})

const filteredGroups = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  const officialOnly = selectedGroupId.value === PLAZA_OFFICIAL_GROUP_ID
  return (props.response?.groups ?? [])
    .filter(
      (group) =>
        selectedGroupId.value === 'all' || officialOnly || group.id === selectedGroupId.value
    )
    .map((group) => {
      const groupMatches = [group.name, group.description, plazaProviderLabel(group.platform)]
        .join(' ')
        .toLocaleLowerCase()
        .includes(query)
      const models = group.models.filter((model) => {
        const platform = model.platform || group.platform
        if (officialOnly && !plazaHasOfficialPricing(model)) return false
        if (selectedPlatform.value !== 'all' && platform !== selectedPlatform.value) return false
        if (selectedBillingMode.value !== 'all' && plazaBillingMode(model) !== selectedBillingMode.value) return false
        if (!query || groupMatches) return true
        return `${model.name} ${plazaProviderLabel(platform)}`.toLocaleLowerCase().includes(query)
      })
      return { ...group, models }
    })
    .filter((group) => group.models.length > 0)
})

const providerSections = computed(() =>
  buildPlazaProviderSections(filteredGroups.value, {
    sortMode: selectedSortMode.value,
    officialPricing: isOfficialGroupSelected.value
  })
)
const resultCount = computed(() =>
  providerSections.value.reduce((count, section) => count + section.cards.length, 0)
)
const filtersActive = computed(
  () =>
    selectedPlatform.value !== 'all' ||
    selectedGroupId.value !== 'all' ||
    selectedBillingMode.value !== 'all' ||
    searchQuery.value.trim() !== ''
)

function selectGroup(value: PlazaGroupFilterValue) {
  selectedGroupId.value = value
  writeStorage(groupStorageKey(selectedPlatform.value), String(value))
  if (value === PLAZA_OFFICIAL_GROUP_ID) selectedBillingMode.value = 'token'
}

function selectPlatform(value: string) {
  selectedPlatform.value = value
  writeStorage(PLATFORM_STORAGE_KEY, value)
  const storedGroup = parseGroupSelection(readStorage(groupStorageKey(value)))
  selectedGroupId.value = groupSelectionAvailable(storedGroup, value) ? storedGroup : 'all'
}

function selectSortMode(value: PlazaSortMode) {
  selectedSortMode.value = value
  writeStorage(SORT_STORAGE_KEY, value)
}

const pricingSourceLabel = computed(() => {
  const response = props.response
  if (!response?.official_pricing_source) return ''
  const updatedAt = response.official_pricing_updated_at
    ? new Intl.DateTimeFormat(locale.value, { year: 'numeric', month: 'short', day: 'numeric' }).format(
        new Date(response.official_pricing_updated_at)
      )
    : ''
  return updatedAt
    ? t('modelPlaza.source.withDate', { source: response.official_pricing_source, date: updatedAt })
    : t('modelPlaza.source.name', { source: response.official_pricing_source })
})
</script>

<style scoped>
.plaza-description {
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.plaza-model-grid {
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 17rem), 1fr));
}

.plaza-model-grid-public {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 17rem), 1fr));
}

.plaza-model-grid-public-sparse {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 17rem), 26rem));
  justify-content: start;
}

.plaza-description :deep(h1),
.plaza-description :deep(h2),
.plaza-description :deep(h3) {
  @apply mb-2 mt-3 font-semibold text-gray-900 first:mt-0 dark:text-white;
}

.plaza-description :deep(p) {
  @apply mb-2 text-gray-700 last:mb-0 dark:text-dark-200;
}

.plaza-description :deep(a) {
  @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300;
}

.plaza-description :deep(ul) {
  @apply mb-2 list-disc pl-5;
}

.plaza-description :deep(ol) {
  @apply mb-2 list-decimal pl-5;
}

.plaza-description :deep(li) {
  @apply mb-0.5 text-gray-700 dark:text-dark-200;
}

.plaza-description :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs dark:bg-dark-800;
}

.plaza-description :deep(blockquote) {
  @apply my-2 border-l-4 border-gray-300 pl-3 text-gray-600 dark:border-dark-600 dark:text-dark-300;
}
</style>
