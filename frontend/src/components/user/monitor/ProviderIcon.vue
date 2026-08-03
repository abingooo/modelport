<template>
  <ModelIcon :model="modelKey" :size="`${size}px`" monochrome />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Provider } from '@/api/admin/channelMonitor'
import ModelIcon from '@/components/common/ModelIcon.vue'

const props = withDefaults(defineProps<{
  provider: Provider | string
  size?: number
}>(), {
  size: 20,
})

const providerModelKeys: Record<Provider, string> = {
  openai: 'gpt',
  anthropic: 'claude',
  gemini: 'gemini',
  grok: 'grok',
  deepseek: 'deepseek',
  qwen: 'qwen',
  glm: 'glm',
  kimi: 'kimi',
  doubao: 'doubao',
  minimax: 'minimax',
  mimo: 'mimo',
}

const modelKey = computed(() =>
  providerModelKeys[props.provider as Provider] ?? props.provider ?? 'api',
)
</script>
