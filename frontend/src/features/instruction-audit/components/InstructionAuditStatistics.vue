<template>
  <section aria-labelledby="instruction-audit-statistics-title" data-test="instruction-audit-statistics">
    <div class="mb-3 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 id="instruction-audit-statistics-title" class="text-base font-semibold text-gray-950 dark:text-white">
          {{ t('admin.instructionAudit.statistics.title') }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.instructionAudit.statistics.filteredHint') }}
        </p>
      </div>
      <span v-if="statistics" class="text-xs tabular-nums text-gray-500 dark:text-gray-400">
        {{ t('admin.instructionAudit.statistics.totalAudited', { count: statistics.total }) }}
      </span>
    </div>

    <div v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </div>
    <div v-else-if="loading && !statistics" class="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-7" aria-busy="true">
      <div v-for="index in 7" :key="index" class="h-24 animate-pulse rounded-md bg-gray-100 dark:bg-dark-800" />
    </div>
    <dl v-else-if="statistics" class="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-7">
      <div v-for="metric in metrics" :key="metric.key" class="rounded-md border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800">
        <dt class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</dt>
        <dd class="mt-2 text-2xl font-semibold tabular-nums" :class="metric.color">{{ metric.value }}</dd>
      </div>
    </dl>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { InstructionStatistics } from '../types'

const props = defineProps<{
  statistics: InstructionStatistics | null
  loading: boolean
  error: string
}>()

const { t } = useI18n()

const metrics = computed(() => {
  const statistics = props.statistics
  if (!statistics) return []
  return [
    { key: 'blocked', label: t('admin.instructionAudit.statistics.blocked'), value: statistics.blocked, color: 'text-red-700 dark:text-red-300' },
    { key: 'policy', label: t('admin.instructionAudit.statistics.policyAllow'), value: statistics.policy_allow, color: 'text-amber-700 dark:text-amber-300' },
    { key: 'ai', label: t('admin.instructionAudit.statistics.aiPass'), value: statistics.ai_pass, color: 'text-cyan-700 dark:text-cyan-300' },
    { key: 'hash', label: t('admin.instructionAudit.statistics.hashPass'), value: statistics.hash_pass, color: 'text-primary-700 dark:text-primary-300' },
    { key: 'exception', label: t('admin.instructionAudit.statistics.exceptionPass'), value: statistics.exception_pass, color: 'text-indigo-700 dark:text-indigo-300' },
    { key: 'total', label: t('admin.instructionAudit.statistics.total'), value: statistics.total, color: 'text-gray-950 dark:text-white' },
    { key: 'rate', label: t('admin.instructionAudit.statistics.blockRate'), value: `${(statistics.block_rate * 100).toFixed(2)}%`, color: 'text-gray-950 dark:text-white' },
  ]
})
</script>
