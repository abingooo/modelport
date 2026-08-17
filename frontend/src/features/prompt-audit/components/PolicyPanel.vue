<template>
  <section aria-labelledby="prompt-policy-title" class="py-6">
    <div>
      <h2 id="prompt-policy-title" class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.policy.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.policy.description') }}</p>
    </div>

    <div class="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(260px,0.45fr)]">
      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700/60 dark:bg-dark-900/20 sm:p-5">
        <div data-test="inherited-scope">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.policy.inheritedScope') }}</h3>
              <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.policy.inheritedScopeHint') }}</p>
            </div>
            <span class="rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300">
              {{ t(`admin.promptAudit.policy.instructionModes.${instructionMode}`) }}
            </span>
          </div>

          <div class="mt-4">
            <p v-if="scopeLoading" data-test="scope-loading" class="rounded-md bg-gray-50 px-3 py-3 text-sm text-gray-500 dark:bg-dark-900/50 dark:text-dark-300">
              {{ t('admin.promptAudit.policy.scopeLoading') }}
            </p>
            <p v-else-if="scopeError" data-test="scope-error" role="alert" class="rounded-md bg-red-50 px-3 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">
              {{ scopeError }}
            </p>
            <template v-else>
              <p v-if="instructionMode === 'off'" data-test="scope-mode-off" class="rounded-md bg-amber-50 px-3 py-3 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
                {{ t('admin.promptAudit.policy.instructionModeOff') }}
              </p>
              <p v-if="inheritedGroups.length === 0" data-test="scope-empty" class="mt-3 rounded-md border border-dashed border-gray-300 px-3 py-5 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-300">
                {{ t('admin.promptAudit.policy.noInheritedGroups') }}
              </p>
              <ul v-else data-test="scope-groups" class="mt-3 divide-y divide-gray-100 rounded-md border border-gray-200 dark:divide-dark-800 dark:border-dark-700">
                <li v-for="group in inheritedGroups" :key="group.group_id" class="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5 text-sm">
                  <span class="min-w-0 font-medium text-gray-800 dark:text-dark-100">{{ group.name }}</span>
                  <span class="text-xs text-gray-500 dark:text-dark-400">
                    #{{ group.group_id }} · {{ group.platform }} · {{ group.status }} · {{ t('admin.promptAudit.policy.scopeBindings', { count: group.scope_count }) }}
                  </span>
                </li>
              </ul>
            </template>
          </div>

          <div class="mt-3 rounded-md bg-blue-50 px-3 py-3 text-xs leading-5 text-blue-800 dark:bg-blue-950/30 dark:text-blue-200">
            {{ t('admin.promptAudit.policy.nonResponsesOnly') }}
          </div>
          <router-link
            :to="{ path: '/admin/instruction-audit', query: { tab: 'scopes' } }"
            data-test="manage-instruction-scope"
            class="mt-3 inline-flex text-sm font-medium text-primary-600 hover:underline dark:text-primary-400"
          >
            {{ t('admin.promptAudit.policy.manageInstructionScope') }}
          </router-link>
        </div>

        <fieldset class="mt-5 border-t border-gray-100 pt-5 dark:border-dark-800">
          <legend class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.policy.scanners') }}</legend>
          <div class="mt-3 grid gap-2 sm:grid-cols-2">
            <label v-for="scanner in SCANNER_CATALOG" :key="scanner.id" class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-gray-700 hover:bg-gray-50 dark:text-dark-200 dark:hover:bg-dark-800">
              <input type="checkbox" :checked="draft.scanners.includes(scanner.id)" :aria-label="scannerLabel(scanner.id)" @change="toggleScanner(scanner.id)" />
              <span>{{ scannerLabel(scanner.id) }}</span>
            </label>
          </div>
        </fieldset>
      </div>

      <div class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-700/60 dark:bg-dark-900/20 sm:p-5">
        <label class="block text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.policy.workerCount') }}</span>
          <input :value="draft.worker_count" type="number" min="1" max="32" class="input mt-1.5 w-full" :aria-label="t('admin.promptAudit.policy.workerCount')" @input="patch({ worker_count: Number(($event.target as HTMLInputElement).value) })" />
        </label>
        <label class="block text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.policy.queueCapacity') }}</span>
          <input :value="draft.queue_capacity" type="number" min="1" max="100000" class="input mt-1.5 w-full" :aria-label="t('admin.promptAudit.policy.queueCapacity')" @input="patch({ queue_capacity: Number(($event.target as HTMLInputElement).value) })" />
        </label>
        <div class="rounded-lg bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:bg-dark-900/50 dark:text-dark-300">
          <p class="font-medium text-gray-800 dark:text-dark-100">{{ t('admin.promptAudit.policy.strategy') }}</p>
          <p class="mt-1">priority · {{ t('admin.promptAudit.policy.strategyHint') }}</p>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { InstructionAuditMode } from '@/features/instruction-audit/v2Types'
import type { PromptAuditDraft, PromptAuditInheritedGroup } from '../types'
import { cloneData, SCANNER_CATALOG } from '../viewModel'

const props = defineProps<{
  draft: PromptAuditDraft
  inheritedGroups: PromptAuditInheritedGroup[]
  instructionMode: InstructionAuditMode
  scopeLoading: boolean
  scopeError: string
}>()
const emit = defineEmits<{ (event: 'update:draft', value: PromptAuditDraft): void }>()
const { t } = useI18n()

function patch(value: Partial<PromptAuditDraft>) {
  emit('update:draft', { ...cloneData(props.draft), ...value })
}
function toggleScanner(id: string) {
  const selected = new Set(props.draft.scanners)
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
  patch({ scanners: SCANNER_CATALOG.map((item) => item.id).filter((item) => selected.has(item)) })
}
function scannerLabel(id: string): string {
  return t(`admin.promptAudit.scanners.${id}`)
}
</script>
