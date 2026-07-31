<template>
  <div class="plaza-filter-shell sticky top-[65px] z-20 rounded-lg border border-gray-200 bg-white/95 p-3 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-900/95">
    <div class="flex flex-wrap items-center gap-2.5">
      <div class="relative min-w-0 flex-1 md:max-w-sm">
        <Icon
          name="search"
          size="sm"
          class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500"
        />
        <input
          :value="search"
          type="search"
          :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          class="input h-10 rounded-lg pl-9 pr-9 text-sm"
          @input="$emit('update:search', ($event.target as HTMLInputElement).value)"
        />
        <button
          v-if="search"
          type="button"
          class="absolute right-2.5 top-1/2 -translate-y-1/2 rounded p-1 text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :title="t('modelPlaza.filters.clear')"
          @click="$emit('update:search', '')"
        >
          <Icon name="x" size="xs" />
        </button>
      </div>

      <button
        type="button"
        class="relative inline-flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-600 transition hover:border-gray-300 hover:text-gray-900 md:hidden dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:text-white"
        :aria-expanded="mobileFiltersOpen"
        :title="t('modelPlaza.filters.more')"
        @click="mobileFiltersOpen = !mobileFiltersOpen"
      >
        <Icon name="filter" size="sm" />
        <span
          v-if="activeFilterCount > 0"
          class="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary-600 px-1 text-[10px] font-semibold text-white"
        >
          {{ activeFilterCount }}
        </span>
      </button>

      <div
        class="w-full flex-wrap items-center gap-2.5 md:flex md:w-auto md:min-w-0 md:flex-1"
        :class="mobileFiltersOpen ? 'flex' : 'hidden'"
      >
        <Select
          :model-value="platform"
          :options="platformOptions"
          :aria-label="t('modelPlaza.filters.platformLabel')"
          class="min-w-0 flex-1 md:w-44 md:flex-none"
          @update:model-value="updatePlatform"
        >
          <template #selected="{ option }">
            <span class="flex min-w-0 items-center gap-2">
              <PlatformIcon
                v-if="option?.platform"
                :platform="option.platform as GroupPlatform"
                size="sm"
                class="flex-shrink-0"
              />
              <span class="truncate">{{ option?.label }}</span>
            </span>
          </template>
          <template #option="{ option }">
            <PlatformIcon
              v-if="option.platform"
              :platform="option.platform as GroupPlatform"
              size="sm"
              class="flex-shrink-0"
            />
            <span class="select-option-label truncate">{{ option.label }}</span>
          </template>
        </Select>

        <Select
          :model-value="groupId"
          :options="groupOptions"
          :aria-label="t('modelPlaza.filters.groupLabel')"
          class="min-w-0 flex-1 md:w-44 md:flex-none"
          @update:model-value="updateGroup"
        />

        <Select
          :model-value="billingMode"
          :options="billingModeOptions"
          :aria-label="t('modelPlaza.filters.billingModeLabel')"
          class="min-w-0 flex-1 md:w-36 md:flex-none"
          @update:model-value="updateBillingMode"
        />

        <button
          v-if="hasFilters"
          type="button"
          class="inline-flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :title="t('modelPlaza.filters.clear')"
          @click="clearFilters"
        >
          <Icon name="x" size="sm" />
        </button>

        <div class="ml-auto hidden items-center gap-3 text-xs text-gray-400 lg:flex dark:text-dark-500">
          <span>{{ t('modelPlaza.filters.resultCount', { count: resultCount }) }}</span>
          <span class="h-4 w-px bg-gray-200 dark:bg-dark-700"></span>
          <span class="whitespace-nowrap font-medium text-gray-500 dark:text-dark-300">USD · $ / 1M tokens</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import type { GroupPlatform } from '@/types'
import { plazaProviderLabel } from './modelPlazaPresentation'

const props = defineProps<{
  platforms: string[]
  groups: Array<{ id: number; name: string; platforms: string[] }>
  billingModes: string[]
  platform: string
  groupId: number | 'all'
  billingMode: string
  search: string
  resultCount: number
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number | 'all']
  'update:billingMode': [value: string]
  'update:search': [value: string]
}>()

const { t } = useI18n()
const mobileFiltersOpen = ref(false)

const platformOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('modelPlaza.filters.all') },
  ...props.platforms.map((platform) => ({
    value: platform,
    label: plazaProviderLabel(platform),
    platform: platform as GroupPlatform
  }))
])

const visibleGroups = computed(() =>
  props.platform === 'all'
    ? props.groups
    : props.groups.filter((group) => group.platforms.includes(props.platform))
)

const groupOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('modelPlaza.filters.allGroups') },
  ...visibleGroups.value.map((group) => ({ value: group.id, label: group.name }))
])

const billingModeOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('modelPlaza.filters.allBillingModes') },
  ...props.billingModes.map((mode) => ({
    value: mode,
    label: t(`modelPlaza.billingModes.${mode}`)
  }))
])

const activeFilterCount = computed(
  () => Number(props.platform !== 'all') + Number(props.groupId !== 'all') + Number(props.billingMode !== 'all')
)
const hasFilters = computed(() => activeFilterCount.value > 0 || props.search.trim() !== '')

function updatePlatform(value: string | number | boolean | null) {
  emit('update:platform', typeof value === 'string' ? value : 'all')
}

function updateGroup(value: string | number | boolean | null) {
  emit('update:groupId', typeof value === 'number' ? value : 'all')
}

function updateBillingMode(value: string | number | boolean | null) {
  emit('update:billingMode', typeof value === 'string' ? value : 'all')
}

function clearFilters() {
  emit('update:platform', 'all')
  emit('update:groupId', 'all')
  emit('update:billingMode', 'all')
  emit('update:search', '')
}
</script>
