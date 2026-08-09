<template>
  <div class="min-w-0" :aria-busy="loading">
    <div
      v-if="disabled"
      class="m-4 flex min-w-0 items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300 sm:mx-5"
      data-test="resource-disabled"
      role="status"
    >
      <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0" />
      <span class="min-w-0 break-words">{{ t('admin.instructionAudit.states.disabled') }}</span>
    </div>

    <div v-if="!loaded && !error" class="space-y-3 p-5" data-test="resource-initial-loading" role="status">
      <span class="sr-only">{{ t('admin.instructionAudit.states.loading') }}</span>
      <div v-for="index in skeletonRows" :key="index" class="h-16 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>
    <div
      v-else-if="!loaded && error"
      class="m-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300 sm:m-5"
      data-test="resource-initial-error"
      role="alert"
    >
      <p class="font-medium">{{ t('admin.instructionAudit.states.loadFailed') }}</p>
      <p class="mt-1 break-words text-xs">{{ error }}</p>
    </div>
    <template v-else>
      <div
        v-if="loading"
        class="mx-4 mt-4 flex min-w-0 items-center gap-2 rounded-md border border-primary-200 bg-primary-50 px-3 py-2 text-xs text-primary-700 dark:border-primary-900/60 dark:bg-primary-950/30 dark:text-primary-300 sm:mx-5"
        data-test="resource-refreshing"
        role="status"
        aria-live="polite"
      >
        <Icon name="refresh" size="sm" class="shrink-0 animate-spin" />
        <span class="min-w-0 break-words">{{ t('admin.instructionAudit.states.refreshing') }}</span>
      </div>
      <div
        v-if="error"
        class="mx-4 mt-4 flex min-w-0 items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-300 sm:mx-5"
        data-test="resource-stale-error"
        role="alert"
      >
        <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" />
        <span class="min-w-0 break-words">
          <span class="font-medium">{{ t('admin.instructionAudit.states.refreshFailed') }}</span>
          <span class="mt-0.5 block text-xs">{{ error }}</span>
        </span>
      </div>
      <slot v-if="hasData" />
      <EmptyState v-else data-test="resource-empty" :description="emptyDescription" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'

withDefaults(defineProps<{
  loading: boolean
  loaded: boolean
  error: string
  hasData: boolean
  emptyDescription: string
  disabled?: boolean
  skeletonRows?: number
}>(), {
  disabled: false,
  skeletonRows: 3,
})

const { t } = useI18n()
</script>
