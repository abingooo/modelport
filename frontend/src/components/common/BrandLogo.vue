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

const modelPortLogoPaths = [
  '/branding/modelport-mark-light.png',
  '/branding/modelport-mark-dark.png',
]

const usesModelPortAssets = computed(
  () =>
    props.siteName.trim().toLowerCase() === 'modelport' &&
    (!props.siteLogo || modelPortLogoPaths.includes(props.siteLogo))
)

const lightAsset = computed(() =>
  props.variant === 'wordmark'
    ? '/branding/modelport-wordmark-light.png'
    : '/branding/modelport-mark-light.png'
)
const darkAsset = computed(() =>
  props.variant === 'wordmark'
    ? '/branding/modelport-wordmark-dark.png'
    : '/branding/modelport-mark-dark.png'
)
</script>
