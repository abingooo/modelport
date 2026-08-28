<template>
  <img
    v-if="usesModelPortAssets"
    :src="lightAsset"
    :alt="`${siteName} logo`"
    :class="[imageClass, 'dark:hidden']"
  />
  <img
    v-if="usesModelPortAssets"
    :src="darkAsset"
    :alt="`${siteName} logo`"
    :class="[imageClass, 'hidden dark:block']"
  />
  <img
    v-else
    :src="siteLogo || '/logo.svg'"
    :alt="`${siteName} logo`"
    :class="imageClass"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { modelPortAsset, usesModelPortBrand } from '@/utils/branding'

const props = withDefaults(
  defineProps<{
    siteName: string
    siteLogo?: string
    variant?: 'mark' | 'wordmark'
    imageClass?: string
  }>(),
  {
    siteLogo: '',
    variant: 'mark',
    imageClass: '',
  }
)

// Never fall back to the legacy Sub2API mark when a deployment has no custom
// logo. The ModelPort mark is the neutral product fallback; an explicit custom
// logo is still honored for white-label deployments.
const usesModelPortAssets = computed(
  () => !props.siteLogo.trim() || usesModelPortBrand(props.siteName, props.siteLogo)
)

const lightAsset = computed(() => modelPortAsset(props.variant, false))
const darkAsset = computed(() => modelPortAsset(props.variant, true))
</script>
