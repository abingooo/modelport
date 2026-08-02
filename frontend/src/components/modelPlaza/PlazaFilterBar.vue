<template>
  <div
    class="plaza-filter-shell sticky top-[65px] z-20 overflow-hidden rounded-lg border border-gray-200 bg-white/95 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-900/95"
  >
    <div class="border-b border-gray-100 px-3 py-2.5 dark:border-dark-700/70">
      <div class="platform-rail flex min-w-0 items-center gap-1 overflow-x-auto" role="tablist" :aria-label="t('modelPlaza.filters.platformLabel')">
        <button
          type="button"
          class="platform-tab"
          :class="{ 'platform-tab-active': platform === 'all' }"
          :style="platformTabStyle('all')"
          role="tab"
          :aria-selected="platform === 'all'"
          data-testid="platform-tab-all"
          @click="updatePlatform('all')"
        >
          <Icon name="grid" size="sm" />
          <span>{{ t('modelPlaza.filters.all') }}</span>
          <span class="platform-tab-count">{{ totalModelCount }}</span>
        </button>
        <button
          v-for="item in platforms"
          :key="item.id"
          type="button"
          class="platform-tab"
          :class="{ 'platform-tab-active': platform === item.id }"
          :style="platformTabStyle(item.id)"
          role="tab"
          :aria-selected="platform === item.id"
          :data-testid="`platform-tab-${item.id}`"
          @click="updatePlatform(item.id)"
        >
          <PlatformIcon :platform="item.id as GroupPlatform" size="sm" />
          <span>{{ plazaProviderLabel(item.id) }}</span>
          <span class="platform-tab-count">{{ item.count }}</span>
        </button>
      </div>
    </div>

    <div class="flex flex-wrap items-end gap-2.5 p-3">
      <div class="plaza-search relative min-w-0 md:max-w-xs">
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
        class="w-full flex-wrap items-end gap-2.5 md:flex md:w-auto md:min-w-0 md:flex-1"
        :class="mobileFiltersOpen ? 'flex' : 'hidden'"
      >
        <label class="filter-field min-w-0 flex-1 md:w-52 md:flex-none">
          <span class="filter-caption">{{ t('modelPlaza.filters.pricingPlanLabel') }}</span>
          <Select
            :model-value="groupId"
            :options="groupOptions"
            :aria-label="t('modelPlaza.filters.groupLabel')"
            class="plaza-group-select mt-1"
            @update:model-value="updateGroup"
          >
            <template #selected="{ option }">
              <span class="flex min-w-0 items-center gap-2">
                <Icon
                  :name="groupOptionIcon(option)"
                  size="sm"
                  class="hidden flex-shrink-0 text-gray-500 sm:block dark:text-dark-300"
                />
                <span class="min-w-0 flex-1 truncate font-medium">
                  {{ option?.value === 'all' ? t('modelPlaza.filters.allShort') : option?.label }}
                </span>
                <span v-if="option?.badge" class="group-option-badge">{{ option.badge }}</span>
              </span>
            </template>
            <template #option="{ option, selected }">
              <span v-if="option.kind === 'group'" class="select-option-label text-[11px] font-semibold uppercase text-gray-400 dark:text-dark-500">
                {{ option.label }}
              </span>
              <template v-else>
                <span class="group-option-dot" :data-tone="option.tone" aria-hidden="true"></span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-medium">{{ option.label }}</span>
                  <span v-if="option.description" class="mt-0.5 block truncate text-[11px] text-gray-400 dark:text-dark-500">
                    {{ option.description }}
                  </span>
                </span>
                <span v-if="option.badge" class="group-option-badge">{{ option.badge }}</span>
                <Icon v-if="selected" name="check" size="sm" class="flex-shrink-0 text-primary-500" :stroke-width="2" />
              </template>
            </template>
          </Select>
        </label>

        <label class="filter-field min-w-0 flex-1 md:w-40 md:flex-none">
          <span class="filter-caption">{{ t('modelPlaza.filters.billingModeLabel') }}</span>
          <Select
            :model-value="billingMode"
            :options="billingModeOptions"
            :aria-label="t('modelPlaza.filters.billingModeLabel')"
            class="mt-1"
            @update:model-value="updateBillingMode"
          >
            <template #selected="{ option }">
              <span class="min-w-0 flex-1 truncate">
                {{ option?.value === 'all' ? t('modelPlaza.filters.allShort') : option?.label }}
              </span>
            </template>
          </Select>
        </label>

        <div class="filter-field flex-shrink-0">
          <span class="filter-caption">{{ t('modelPlaza.filters.sortLabel') }}</span>
          <button
            type="button"
            class="mt-1 inline-flex h-10 min-w-28 items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white px-3 text-sm font-medium text-gray-700 transition hover:border-primary-300 hover:text-primary-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:border-primary-500/50 dark:hover:text-primary-300"
            :aria-label="sortActionLabel"
            :title="sortActionLabel"
            :aria-pressed="sortMode === 'output'"
            data-testid="plaza-sort-toggle"
            @click="toggleSortMode"
          >
            <Icon name="sort" size="sm" />
            <span>{{ sortModeLabel }}</span>
          </button>
        </div>

        <button
          v-if="hasFilters"
          type="button"
          class="mb-0 inline-flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :title="t('modelPlaza.filters.clear')"
          @click="clearFilters"
        >
          <Icon name="x" size="sm" />
        </button>

        <div class="ml-auto hidden h-10 items-center gap-3 text-xs text-gray-400 lg:flex dark:text-dark-500">
          <span>{{ t('modelPlaza.filters.resultCount', { count: resultCount }) }}</span>
          <span class="h-4 w-px bg-gray-200 dark:bg-dark-700"></span>
          <span class="whitespace-nowrap font-medium text-gray-500 dark:text-dark-300">CNY · ￥ / 1M tokens</span>
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
import { platformAccentColor } from '@/utils/platformColors'
import {
  PLAZA_OFFICIAL_GROUP_ID,
  plazaProviderLabel
} from './modelPlazaPresentation'
import type {
  PlazaFilterGroupData,
  PlazaGroupFilterValue,
  PlazaSortMode
} from './modelPlazaPresentation'

const props = defineProps<{
  platforms: Array<{ id: string; count: number }>
  groups: PlazaFilterGroupData[]
  billingModes: string[]
  platform: string
  groupId: PlazaGroupFilterValue
  showOfficialPricing: boolean
  billingMode: string
  sortMode: PlazaSortMode
  search: string
  resultCount: number
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: PlazaGroupFilterValue]
  'update:billingMode': [value: string]
  'update:sortMode': [value: PlazaSortMode]
  'update:search': [value: string]
}>()

const { t } = useI18n()
const mobileFiltersOpen = ref(false)

const totalModelCount = computed(() => props.platforms.reduce((sum, item) => sum + item.count, 0))
const visibleGroups = computed(() =>
  props.platform === 'all'
    ? props.groups
    : props.groups.filter((group) => group.platforms.includes(props.platform))
)

function groupCategory(group: PlazaFilterGroupData): 'exclusive' | 'subscription' | 'standard' {
  if (group.isExclusive) return 'exclusive'
  if (group.subscriptionType === 'subscription') return 'subscription'
  return 'standard'
}

function groupBadge(group: PlazaFilterGroupData): string {
  if (group.isFree) return t('modelPlaza.badges.free')
  const multiplier = Math.round(group.effectiveMultiplier * 1000) / 1000
  return `${multiplier}x`
}

const groupOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = [
    {
      value: 'all',
      label: t('modelPlaza.filters.allGroups'),
      description: t('modelPlaza.filters.allGroupsHint'),
      optionType: 'all',
      tone: 'all'
    }
  ]

  if (props.showOfficialPricing) {
    options.push(
      {
        value: '__reference_header__',
        label: t('modelPlaza.filters.groupSections.reference'),
        kind: 'group',
        disabled: true
      },
      {
        value: PLAZA_OFFICIAL_GROUP_ID,
        label: t('modelPlaza.card.officialGroup'),
        description: t('modelPlaza.filters.officialPriceHint'),
        badge: t('modelPlaza.card.officialShort'),
        optionType: 'official',
        tone: 'official'
      }
    )
  }

  const sections = [
    { id: 'exclusive', label: t('modelPlaza.filters.groupSections.exclusive') },
    { id: 'subscription', label: t('modelPlaza.filters.groupSections.subscription') },
    { id: 'standard', label: t('modelPlaza.filters.groupSections.standard') }
  ] as const
  for (const section of sections) {
    const groups = visibleGroups.value.filter((group) => groupCategory(group) === section.id)
    if (groups.length === 0) continue
    options.push({
      value: `__${section.id}_header__`,
      label: section.label,
      kind: 'group',
      disabled: true
    })
    options.push(
      ...groups.map((group) => ({
        value: group.id,
        label: group.name,
        badge: groupBadge(group),
        optionType: 'group',
        tone: group.isFree ? 'free' : section.id
      }))
    )
  }
  return options
})

const billingModeOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('modelPlaza.filters.allBillingModes') },
  ...props.billingModes.map((mode) => ({
    value: mode,
    label: t(`modelPlaza.billingModes.${mode}`)
  }))
])

const activeFilterCount = computed(
  () => Number(props.groupId !== 'all') + Number(props.billingMode !== 'all')
)
const hasFilters = computed(
  () => props.platform !== 'all' || activeFilterCount.value > 0 || props.search.trim() !== ''
)
const sortModeLabel = computed(() =>
  props.sortMode === 'output'
    ? t('modelPlaza.filters.sortByOutput')
    : t('modelPlaza.filters.sortByName')
)
const sortActionLabel = computed(() =>
  props.sortMode === 'output'
    ? t('modelPlaza.filters.switchToNameSort')
    : t('modelPlaza.filters.switchToOutputSort')
)

type GroupOptionIcon = 'grid' | 'badge' | 'gift' | 'shield' | 'users' | 'key'

function groupOptionIcon(option: Record<string, unknown> | null): GroupOptionIcon {
  if (option?.optionType === 'official') return 'badge'
  if (option?.tone === 'free') return 'gift'
  if (option?.tone === 'exclusive') return 'shield'
  if (option?.tone === 'subscription') return 'users'
  if (option?.optionType === 'group') return 'key'
  return 'grid'
}

function platformTabStyle(platformId: string): Record<string, string> {
  return {
    '--platform-accent': platformId === 'all' ? '#0f9f9a' : platformAccentColor(platformId)
  }
}

function updatePlatform(value: string) {
  emit('update:platform', value)
}

function updateGroup(value: string | number | boolean | null) {
  if (typeof value === 'number' || value === 'all' || value === PLAZA_OFFICIAL_GROUP_ID) {
    emit('update:groupId', value)
    return
  }
  emit('update:groupId', 'all')
}

function updateBillingMode(value: string | number | boolean | null) {
  emit('update:billingMode', typeof value === 'string' ? value : 'all')
}

function toggleSortMode() {
  emit('update:sortMode', props.sortMode === 'name' ? 'output' : 'name')
}

function clearFilters() {
  emit('update:platform', 'all')
  emit('update:groupId', 'all')
  emit('update:billingMode', 'all')
  emit('update:search', '')
}
</script>

<style scoped>
.platform-rail {
  scrollbar-width: none;
}

.platform-rail::-webkit-scrollbar {
  display: none;
}

.plaza-search {
  width: calc(100% - 3.125rem);
  flex: none;
}

.platform-tab {
  display: inline-flex;
  height: 38px;
  flex: 0 0 auto;
  align-items: center;
  gap: 7px;
  border-radius: 6px;
  padding: 0 11px;
  color: rgb(107 114 128);
  font-size: 12px;
  font-weight: 600;
  transition: background-color 150ms ease, color 150ms ease, box-shadow 150ms ease;
}

.platform-tab:hover {
  background: rgb(243 244 246);
  color: rgb(31 41 55);
}

.platform-tab-active {
  background: color-mix(in srgb, var(--platform-accent) 10%, transparent);
  color: var(--platform-accent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--platform-accent) 24%, transparent);
}

.platform-tab-count {
  display: inline-flex;
  min-width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: rgb(243 244 246);
  padding: 0 6px;
  color: rgb(107 114 128);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

.platform-tab-active .platform-tab-count {
  background: color-mix(in srgb, var(--platform-accent) 13%, transparent);
  color: inherit;
}

.filter-caption {
  display: block;
  color: rgb(107 114 128);
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
}

.group-option-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 8px;
  border-radius: 2px;
  background: #94a3b8;
}

.group-option-dot[data-tone='official'] { background: #0ea5e9; }
.group-option-dot[data-tone='free'] { background: #0891b2; }
.group-option-dot[data-tone='exclusive'] { background: #c026d3; }
.group-option-dot[data-tone='subscription'] { background: #d97706; }
.group-option-dot[data-tone='standard'] { background: #64748b; }
.group-option-dot[data-tone='all'] { background: #0f9f9a; }

.group-option-badge {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  border-radius: 4px;
  background: rgb(243 244 246);
  padding: 2px 5px;
  color: rgb(75 85 99);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 600;
  line-height: 1.25;
}

.dark .platform-tab {
  color: rgb(156 163 175);
}

.dark .platform-tab:hover {
  background: rgb(31 41 55);
  color: rgb(229 231 235);
}

.dark .platform-tab-active {
  color: color-mix(in srgb, var(--platform-accent) 70%, white);
}

.dark .platform-tab-count,
.dark .group-option-badge {
  background: rgb(31 41 55);
  color: rgb(209 213 219);
}

.dark .filter-caption {
  color: rgb(156 163 175);
}

@media (min-width: 768px) {
  .plaza-search {
    width: auto;
    flex: 1 1 0%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .platform-tab {
    transition: none;
  }
}
</style>
