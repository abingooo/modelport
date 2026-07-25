<template>
  <BaseDialog :show="show" :title="dialogTitle" width="wide" @close="emit('close')">
    <form id="model-catalog-metadata-form" class="space-y-5" @submit.prevent="submit">
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.platform') }}</span>
          <select v-model="form.platform" class="input" required>
            <option v-for="platform in GROUP_PLATFORM_ORDER" :key="platform" :value="platform">
              {{ platformDisplayName(platform) }}
            </option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.modelName') }}</span>
          <input v-model.trim="form.model_name" class="input font-mono" maxlength="255" required />
        </label>
        <label class="block sm:col-span-2">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.displayName') }}</span>
          <input v-model.trim="form.display_name" class="input" maxlength="255" />
        </label>
        <label class="block sm:col-span-2">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.modelDescription') }}</span>
          <textarea v-model.trim="form.description" class="input min-h-24 resize-y" maxlength="8000" />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.capabilities') }}</span>
          <input v-model="capabilitiesText" class="input" :placeholder="t('modelCatalog.admin.capabilitiesPlaceholder')" />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.scenarios') }}</span>
          <input v-model="scenariosText" class="input" :placeholder="t('modelCatalog.admin.scenariosPlaceholder')" />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.contextWindow') }}</span>
          <input v-model.number="form.context_window" class="input" type="number" min="0" step="1" />
        </label>
        <label class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.sortOrder') }}</span>
          <input v-model.number="form.sort_order" class="input" type="number" step="1" />
        </label>
      </div>

      <fieldset>
        <legend class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.interfaceFormats') }}</legend>
        <div class="flex flex-wrap gap-2">
          <label
            v-for="format in interfaceOptions"
            :key="format"
            :class="[
              'inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm',
              form.interface_formats.includes(format)
                ? 'border-primary-400 bg-primary-50 text-primary-700 dark:border-primary-600 dark:bg-primary-950/30 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-300',
            ]"
          >
            <input v-model="form.interface_formats" type="checkbox" :value="format" class="checkbox" />
            {{ t(`modelCatalog.formats.${format}`) }}
          </label>
        </div>
      </fieldset>

      <div class="grid gap-4 lg:grid-cols-3">
        <label v-for="format in form.interface_formats" :key="format" class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('modelCatalog.admin.exampleOverride', { format: t(`modelCatalog.formats.${format}`) }) }}</span>
          <textarea
            v-model="form.example_overrides[format]"
            class="input min-h-36 resize-y font-mono text-xs"
            maxlength="12000"
          />
        </label>
      </div>

      <div class="flex flex-wrap gap-6 border-t border-gray-200 pt-4 dark:border-dark-700">
        <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.is_visible" type="checkbox" class="checkbox" />
          {{ t('modelCatalog.admin.visible') }}
        </label>
        <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.is_recommended" type="checkbox" class="checkbox" />
          {{ t('modelCatalog.admin.recommended') }}
        </label>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="submit" form="model-catalog-metadata-form" class="btn btn-primary" :disabled="saving || !form.model_name.trim()">
          <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { ModelCatalogItem, ModelCatalogMetadataInput, ModelInterfaceFormat } from '@/api/modelCatalog'
import { GROUP_PLATFORM_ORDER, platformDisplayName } from '@/utils/providerPresets'

const props = defineProps<{ show: boolean; item: ModelCatalogItem | null; saving: boolean }>()
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', input: ModelCatalogMetadataInput): void
}>()
const { t } = useI18n()
const interfaceOptions: ModelInterfaceFormat[] = ['openai', 'anthropic', 'google']
const capabilitiesText = ref('')
const scenariosText = ref('')

const form = reactive<ModelCatalogMetadataInput>({
  platform: 'openai',
  model_name: '',
  display_name: '',
  description: '',
  capabilities: [],
  context_window: 0,
  interface_formats: ['openai'],
  scenarios: [],
  example_overrides: {},
  is_recommended: false,
  is_visible: true,
  sort_order: 0,
})

const dialogTitle = computed(() => props.item
  ? t('modelCatalog.admin.editTitle')
  : t('modelCatalog.admin.createTitle'))

function parseList(value: string): string[] {
  return [...new Set(value.split(',').map((item) => item.trim().toLowerCase()).filter(Boolean))]
}

watch(
  () => [props.show, props.item] as const,
  () => {
    const item = props.item
    form.id = item?.metadata_id || undefined
    form.platform = item?.platform || 'openai'
    form.model_name = item?.name || ''
    form.display_name = item?.display_name === item?.name ? '' : (item?.display_name || '')
    form.description = item?.description || ''
    form.context_window = item?.context_window || 0
    form.interface_formats = [...(item?.interface_formats || ['openai'])]
    form.example_overrides = { ...(item?.example_overrides || {}) }
    form.is_recommended = item?.is_recommended || false
    form.is_visible = item?.is_visible ?? true
    form.sort_order = item?.sort_order || 0
    capabilitiesText.value = (item?.capabilities || []).join(', ')
    scenariosText.value = (item?.scenarios || []).join(', ')
  },
  { immediate: true },
)

function submit() {
  emit('save', {
    ...form,
    display_name: form.display_name.trim(),
    description: form.description.trim(),
    capabilities: parseList(capabilitiesText.value),
    scenarios: parseList(scenariosText.value),
    interface_formats: [...form.interface_formats],
    example_overrides: Object.fromEntries(
      Object.entries(form.example_overrides)
        .map(([format, value]) => [format, value?.trim()])
        .filter((entry) => Boolean(entry[1])),
    ),
  })
}
</script>
