<template>
  <div class="relative flex min-h-screen items-center justify-center overflow-hidden bg-gray-50 p-4 dark:bg-dark-950">
    <div class="pointer-events-none absolute inset-x-0 top-0 h-1 bg-primary-600 dark:bg-primary-500"></div>
    <div
      class="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(13,110,242,0.035)_1px,transparent_1px),linear-gradient(90deg,rgba(13,110,242,0.035)_1px,transparent_1px)] bg-[size:64px_64px] dark:opacity-40"
    ></div>

    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-lg shadow-sm"
          >
            <BrandLogo
              :site-name="siteName"
              :site-logo="siteLogo"
              image-class="h-full w-full object-contain"
            />
          </div>
          <BrandLogo
            v-if="isModelPortBrand"
            variant="wordmark"
            :site-name="siteName"
            :site-logo="siteLogo"
            image-class="mx-auto mb-3 h-10 w-auto max-w-[260px] object-contain"
          />
          <h1 v-else class="mb-2 [overflow-wrap:anywhere] text-3xl font-bold text-gray-900 dark:text-white">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-lg p-6 shadow-glass sm:p-8">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import BrandLogo from '@/components/common/BrandLogo.vue'
import { DEFAULT_SITE_NAME, usesModelPortBrand } from '@/utils/branding'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || DEFAULT_SITE_NAME)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const isModelPortBrand = computed(() => usesModelPortBrand(siteName.value, siteLogo.value))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
