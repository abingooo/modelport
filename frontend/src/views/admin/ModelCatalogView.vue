<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
          <div class="flex flex-1 flex-col gap-3 sm:flex-row">
            <label class="relative block w-full sm:max-w-md">
              <span class="sr-only">{{ t('modelCatalog.searchPlaceholder') }}</span>
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input v-model="searchQuery" class="input pl-10" :placeholder="t('modelCatalog.searchPlaceholder')" />
            </label>
            <select v-model="platformFilter" class="input sm:w-48">
              <option value="">{{ t('modelCatalog.allPlatforms') }}</option>
              <option v-for="platform in platforms" :key="platform" :value="platform">{{ platformLabel(platform) }}</option>
            </select>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="icon-btn" :title="t('common.refresh')" :disabled="loading" @click="loadModels">
              <Icon name="refresh" size="md" :class="{ 'animate-spin': loading }" />
            </button>
            <button type="button" class="btn btn-primary" @click="openCreate">
              <Icon name="plus" size="sm" />{{ t('modelCatalog.admin.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div class="overflow-hidden rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
          <div v-if="loading" class="flex justify-center py-16"><LoadingSpinner /></div>
          <div v-else-if="errorMessage" class="py-14 text-center">
            <p class="text-sm text-red-600 dark:text-red-400">{{ errorMessage }}</p>
            <button type="button" class="btn btn-secondary mt-4" @click="loadModels">{{ t('modelCatalog.retry') }}</button>
          </div>
          <EmptyState
            v-else-if="filteredModels.length === 0"
            :title="t('modelCatalog.admin.emptyTitle')"
            :description="t('modelCatalog.admin.emptyDescription')"
          />
          <div v-else class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/70">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">{{ t('modelCatalog.admin.model') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">{{ t('modelCatalog.admin.platform') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">{{ t('modelCatalog.admin.capabilities') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">{{ t('modelCatalog.admin.channelStatus') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">{{ t('common.status') }}</th>
                  <th class="px-5 py-3 text-right text-xs font-semibold uppercase text-gray-500 dark:text-gray-400">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="item in filteredModels" :key="`${item.platform}:${item.name}`" class="hover:bg-gray-50/70 dark:hover:bg-dark-800/50">
                  <td class="min-w-64 px-5 py-4 align-top">
                    <p class="font-medium text-gray-900 dark:text-white">{{ item.display_name }}</p>
                    <p class="mt-1 break-all font-mono text-[11px] text-gray-400">{{ item.name }}</p>
                  </td>
                  <td class="px-5 py-4 align-top">
                    <span :class="['inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium', platformBadgeClass(item.platform)]">
                      <PlatformIcon :platform="item.platform as GroupPlatform" size="xs" />
                      {{ platformLabel(item.platform) }}
                    </span>
                  </td>
                  <td class="px-5 py-4 align-top">
                    <div class="flex max-w-56 flex-wrap gap-1">
                      <span v-for="capability in item.capabilities" :key="capability" class="badge badge-gray text-[11px]">{{ capability }}</span>
                    </div>
                  </td>
                  <td class="px-5 py-4 align-top text-sm text-gray-600 dark:text-gray-300">
                    <span v-if="item.available">{{ t('modelCatalog.admin.offerCount', { count: item.offers.length }) }}</span>
                    <span v-else class="text-amber-700 dark:text-amber-300">{{ t('modelCatalog.admin.orphan') }}</span>
                  </td>
                  <td class="px-5 py-4 align-top">
                    <div class="flex flex-wrap gap-1">
                      <span :class="['badge', item.is_visible ? 'badge-success' : 'badge-gray']">
                        {{ item.is_visible ? t('modelCatalog.admin.visible') : t('modelCatalog.admin.hidden') }}
                      </span>
                      <span v-if="item.is_recommended" class="badge badge-warning">{{ t('modelCatalog.recommended') }}</span>
                    </div>
                  </td>
                  <td class="px-5 py-4 align-top">
                    <div class="flex justify-end gap-1">
                      <button type="button" class="icon-btn" :title="t('common.edit')" @click="openEdit(item)"><Icon name="edit" size="sm" /></button>
                      <button
                        v-if="item.metadata_id > 0"
                        type="button"
                        class="icon-btn text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/30"
                        :title="t('common.delete')"
                        @click="pendingDelete = item"
                      ><Icon name="trash" size="sm" /></button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </TablePageLayout>

    <ModelCatalogMetadataDialog
      :show="editorOpen"
      :item="editingItem"
      :saving="saving"
      @close="closeEditor"
      @save="saveMetadata"
    />
    <ConfirmDialog
      :show="pendingDelete !== null"
      :title="t('modelCatalog.admin.deleteTitle')"
      :message="t('modelCatalog.admin.deleteMessage', { name: pendingDelete?.display_name || '' })"
      :confirm-text="t('common.delete')"
      danger
      @confirm="deleteMetadata"
      @cancel="pendingDelete = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ModelCatalogMetadataDialog from '@/components/model-catalog/ModelCatalogMetadataDialog.vue'
import modelCatalogAPI, { type ModelCatalogItem, type ModelCatalogMetadataInput } from '@/api/modelCatalog'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const models = ref<ModelCatalogItem[]>([])
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const searchQuery = ref('')
const platformFilter = ref('')
const editorOpen = ref(false)
const editingItem = ref<ModelCatalogItem | null>(null)
const pendingDelete = ref<ModelCatalogItem | null>(null)
let requestController: AbortController | null = null

const platforms = computed(() => [...new Set(models.value.map((item) => item.platform))].sort())
const filteredModels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return models.value.filter((item) => {
    if (platformFilter.value && item.platform !== platformFilter.value) return false
    if (!query) return true
    return [item.name, item.display_name, item.description, platformLabel(item.platform)]
      .some((value) => value.toLowerCase().includes(query))
  })
})

function openCreate() {
  editingItem.value = null
  editorOpen.value = true
}

function openEdit(item: ModelCatalogItem) {
  editingItem.value = item
  editorOpen.value = true
}

function closeEditor() {
  if (saving.value) return
  editorOpen.value = false
  editingItem.value = null
}

async function loadModels() {
  requestController?.abort()
  requestController = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    models.value = await modelCatalogAPI.listAdmin({ signal: requestController.signal })
  } catch (error: unknown) {
    if (requestController.signal.aborted) return
    errorMessage.value = extractApiErrorMessage(error, t('modelCatalog.loadFailed'))
  } finally {
    if (!requestController.signal.aborted) loading.value = false
  }
}

async function saveMetadata(input: ModelCatalogMetadataInput) {
  if (saving.value) return
  saving.value = true
  try {
    await modelCatalogAPI.saveMetadata(input)
    appStore.showSuccess(t('common.saved'))
    editorOpen.value = false
    editingItem.value = null
    await loadModels()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('modelCatalog.admin.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function deleteMetadata() {
  const item = pendingDelete.value
  if (!item || item.metadata_id <= 0) return
  try {
    await modelCatalogAPI.deleteMetadata(item.metadata_id)
    appStore.showSuccess(t('common.deleted'))
    pendingDelete.value = null
    await loadModels()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('modelCatalog.admin.deleteFailed')))
  }
}

onMounted(loadModels)
onBeforeUnmount(() => requestController?.abort())
</script>
