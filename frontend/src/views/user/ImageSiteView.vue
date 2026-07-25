<template>
  <AppLayout>
    <div class="mx-auto flex min-h-[calc(100vh-12rem)] max-w-5xl items-center justify-center py-8">
      <section class="w-full border-y border-gray-200 py-12 dark:border-dark-700 sm:py-16">
        <div class="mx-auto max-w-2xl text-center">
          <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-md bg-cyan-50 text-cyan-700 dark:bg-cyan-950/40 dark:text-cyan-300">
            <Icon name="sparkles" size="xl" />
          </div>
          <h1 class="mt-6 text-2xl font-semibold text-gray-950 dark:text-white">{{ t('imageSite.title') }}</h1>
          <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('imageSite.description') }}</p>

          <template v-if="safeImageSiteUrl">
            <div class="mx-auto mt-8 flex max-w-lg items-center gap-3 border-y border-gray-200 px-2 py-4 text-left dark:border-dark-700">
              <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-gray-300">
                <Icon name="globe" size="md" />
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('imageSite.destination') }}</p>
                <p class="truncate font-mono text-sm text-gray-800 dark:text-gray-200">{{ destinationHost }}</p>
              </div>
            </div>
            <a
              :href="safeImageSiteUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="btn btn-primary mt-8 inline-flex items-center gap-2"
            >
              {{ t('imageSite.open') }}
              <Icon name="externalLink" size="sm" />
            </a>
          </template>

          <div v-else class="mt-8 border-y border-amber-200 bg-amber-50/60 px-5 py-6 dark:border-amber-900/60 dark:bg-amber-950/20">
            <p class="font-medium text-amber-900 dark:text-amber-200">{{ t('imageSite.unavailableTitle') }}</p>
            <p class="mt-1 text-sm text-amber-700 dark:text-amber-300">{{ t('imageSite.unavailableDescription') }}</p>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()
const safeImageSiteUrl = computed(() => {
  const value = appStore.cachedPublicSettings?.image_site_url || ''
  if (value.trim().length > 2048) return ''

  const sanitized = sanitizeUrl(value)
  if (!sanitized) return ''

  const parsed = new URL(sanitized)
  return parsed.username || parsed.password ? '' : sanitized
})
const destinationHost = computed(() => {
  if (!safeImageSiteUrl.value) return ''
  try {
    return new URL(safeImageSiteUrl.value).host
  } catch {
    return ''
  }
})
</script>
