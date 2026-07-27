<template>
  <BaseDialog
    :show="show"
    :title="t('keys.testModal.title')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div
        v-if="apiKey"
        class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800"
      >
        <div class="flex min-w-0 items-center gap-3">
          <div
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-600 text-white shadow-sm"
          >
            <Icon name="key" size="md" :stroke-width="2" />
          </div>
          <div class="min-w-0">
            <div class="truncate font-semibold text-gray-900 dark:text-white">
              {{ apiKey.name }}
            </div>
            <div class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
              <code>{{ maskedKey }}</code>
              <span v-if="apiKey.group">{{ apiKey.group.name }}</span>
            </div>
          </div>
        </div>
        <span :class="statusBadgeClass">
          {{ t(`keys.status.${apiKey.status}`) }}
        </span>
      </div>

      <div
        v-if="apiKey && !apiKey.group"
        class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800/70 dark:bg-amber-950/30"
      >
        <div class="flex gap-3">
          <Icon
            name="exclamationTriangle"
            size="md"
            class="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400"
          />
          <div>
            <div class="font-medium text-amber-900 dark:text-amber-200">
              {{ t('keys.testModal.noGroupTitle') }}
            </div>
            <p class="mt-1 text-sm text-amber-700 dark:text-amber-300">
              {{ t('keys.testModal.noGroupDescription') }}
            </p>
          </div>
        </div>
      </div>

      <template v-else-if="apiKey">
        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-3">
            <label class="input-label" for="key-test-model">
              {{ t('keys.testModal.modelLabel') }}
            </label>
            <button
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :disabled="loadingModels || isRunning"
              :title="t('keys.testModal.reloadModels')"
              @click="loadModels"
            >
              <Icon name="refresh" size="sm" :class="loadingModels ? 'animate-spin' : ''" />
            </button>
          </div>
          <Select
            id="key-test-model"
            v-model="selectedModel"
            :options="modelOptions"
            :placeholder="modelPlaceholder"
            :searchable="true"
            :search-placeholder="t('keys.testModal.searchModel')"
            :disabled="loadingModels || isRunning || modelOptions.length === 0"
          />
          <p v-if="modelLoadError" class="text-sm text-red-600 dark:text-red-400">
            {{ modelLoadError }}
          </p>
        </div>

        <TextArea
          v-model="prompt"
          :label="t('keys.testModal.promptLabel')"
          :placeholder="t('keys.testModal.promptPlaceholder')"
          :disabled="isRunning"
          rows="3"
        />

        <div
          class="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:border-amber-800/70 dark:bg-amber-950/30 dark:text-amber-200"
        >
          <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0" />
          <span>{{ t('keys.testModal.billingNotice') }}</span>
        </div>

        <div v-if="requestStarted" class="grid grid-cols-2 gap-2">
          <div class="rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('keys.testModal.firstTokenLatency') }}
            </div>
            <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ firstTokenLatency === null ? '--' : `${firstTokenLatency} ms` }}
            </div>
          </div>
          <div class="rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('keys.testModal.totalLatency') }}
            </div>
            <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ totalLatency === null ? '--' : `${totalLatency} ms` }}
            </div>
          </div>
        </div>

        <div
          class="min-h-[150px] max-h-[300px] overflow-y-auto rounded-lg border border-gray-700 bg-gray-950 p-4 font-mono text-sm shadow-inner"
          data-test="key-test-output"
        >
          <div v-if="testStatus === 'idle'" class="flex items-center gap-2 text-gray-500">
            <Icon name="play" size="sm" />
            <span>{{ t('keys.testModal.ready') }}</span>
          </div>
          <div v-else-if="isRunning" class="flex items-center gap-2 text-amber-300">
            <Icon name="refresh" size="sm" class="animate-spin" />
            <span>{{ t('keys.testModal.running') }}</span>
          </div>

          <div v-if="reasoningContent" class="mt-3 whitespace-pre-wrap break-words text-gray-400">
            <div class="mb-1 text-xs font-semibold uppercase text-gray-500">
              {{ t('keys.testModal.reasoning') }}
            </div>
            {{ reasoningContent }}
          </div>
          <div v-if="responseContent" class="mt-3 whitespace-pre-wrap break-words text-emerald-300">
            <div class="mb-1 text-xs font-semibold uppercase text-emerald-500">
              {{ t('keys.testModal.response') }}
            </div>
            {{ responseContent }}<span v-if="isRunning" class="animate-pulse">_</span>
          </div>
          <div
            v-else-if="testStatus === 'success' && !reasoningContent"
            class="mt-3 text-emerald-300"
          >
            {{ t('keys.testModal.noTextResponse') }}
          </div>

          <div
            v-if="testStatus === 'success'"
            class="mt-4 flex items-center gap-2 border-t border-gray-800 pt-3 text-emerald-400"
          >
            <Icon name="checkCircle" size="sm" />
            <span>{{ t('keys.testModal.success') }}</span>
          </div>
          <div
            v-else-if="testStatus === 'error'"
            class="mt-4 flex items-start gap-2 border-t border-gray-800 pt-3 text-red-400"
          >
            <Icon name="xCircle" size="sm" class="mt-0.5 shrink-0" />
            <span class="break-words">{{ testError }}</span>
          </div>
          <div
            v-else-if="testStatus === 'cancelled'"
            class="mt-4 flex items-center gap-2 border-t border-gray-800 pt-3 text-gray-400"
          >
            <Icon name="xCircle" size="sm" />
            <span>{{ t('keys.testModal.cancelled') }}</span>
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex w-full items-center justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.close') }}
        </button>
        <button
          v-if="isRunning"
          type="button"
          class="btn btn-secondary text-red-600 hover:text-red-700 dark:text-red-400"
          @click="cancelTest"
        >
          <Icon name="x" size="sm" class="mr-2" />
          {{ t('keys.testModal.stop') }}
        </button>
        <button
          v-else-if="apiKey?.group"
          type="button"
          class="btn btn-primary"
          :disabled="loadingModels || !selectedModel || !prompt.trim()"
          @click="startTest"
        >
          <Icon name="play" size="sm" class="mr-2" />
          {{ testStatus === 'idle' ? t('keys.testModal.start') : t('keys.testModal.retry') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import Icon from '@/components/icons/Icon.vue'
import type { ApiKey } from '@/types'
import { maskApiKey } from '@/utils/maskApiKey'

type TestStatus = 'idle' | 'running' | 'success' | 'error' | 'cancelled'

interface ModelOption {
  value: string
  label: string
}

interface GatewayModel {
  id?: unknown
  display_name?: unknown
}

interface GatewayModelList {
  data?: unknown
}

interface StreamChoice {
  delta?: unknown
  message?: unknown
}

interface StreamPayload {
  choices?: unknown
  error?: unknown
}

const props = defineProps<{
  show: boolean
  apiKey: ApiKey | null
  baseUrl?: string
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'tested'): void
}>()

const { t } = useI18n()
const models = ref<ModelOption[]>([])
const selectedModel = ref('')
const prompt = ref('')
const loadingModels = ref(false)
const modelLoadError = ref('')
const testStatus = ref<TestStatus>('idle')
const testError = ref('')
const reasoningContent = ref('')
const responseContent = ref('')
const requestStarted = ref(false)
const firstTokenLatency = ref<number | null>(null)
const totalLatency = ref<number | null>(null)
let modelController: AbortController | null = null
let testController: AbortController | null = null
let requestStartTime = 0
let cancelledByUser = false

const maskedKey = computed(() => maskApiKey(props.apiKey?.key || ''))
const isRunning = computed(() => testStatus.value === 'running')
const modelOptions = computed(() => models.value)
const modelPlaceholder = computed(() => {
  if (loadingModels.value) return t('keys.testModal.loadingModels')
  if (modelLoadError.value) return t('keys.testModal.modelsUnavailable')
  return t('keys.testModal.selectModel')
})
const statusBadgeClass = computed(() => [
  'shrink-0 rounded-full px-2.5 py-1 text-xs font-semibold',
  props.apiKey?.status === 'active'
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    : props.apiKey?.status === 'expired' || props.apiKey?.status === 'quota_exhausted'
      ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
      : 'bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
])

const gatewayBaseUrl = computed(() => {
  const fallback = new URL(window.location.origin)
  let url = fallback
  try {
    const candidate = new URL(props.baseUrl?.trim() || window.location.origin, window.location.origin)
    if (candidate.protocol === 'http:' || candidate.protocol === 'https:') {
      url = candidate
    }
  } catch {
    url = fallback
  }

  url.search = ''
  url.hash = ''
  const path = url.pathname
    .replace(/\/(?:chat\/completions|models)\/?$/i, '')
    .replace(/\/+$/, '')
  url.pathname = /\/v1$/i.test(path) ? path : `${path}/v1`
  return url.toString().replace(/\/$/, '')
})

watch(
  [() => props.show, () => props.apiKey?.id],
  async ([show]) => {
    abortRequests()
    resetState()
    if (!show || !props.apiKey) return

    prompt.value = t('keys.testModal.defaultPrompt')
    if (props.apiKey.group) {
      await loadModels()
    }
  }
)

function abortRequests() {
  modelController?.abort()
  testController?.abort()
  modelController = null
  testController = null
}

function resetState() {
  models.value = []
  selectedModel.value = ''
  prompt.value = ''
  loadingModels.value = false
  modelLoadError.value = ''
  testStatus.value = 'idle'
  testError.value = ''
  reasoningContent.value = ''
  responseContent.value = ''
  requestStarted.value = false
  firstTokenLatency.value = null
  totalLatency.value = null
}

function handleClose() {
  abortRequests()
  emit('close')
}

async function loadModels() {
  if (!props.apiKey?.key || !props.apiKey.group) return

  modelController?.abort()
  const controller = new AbortController()
  modelController = controller
  loadingModels.value = true
  modelLoadError.value = ''
  models.value = []
  selectedModel.value = ''

  try {
    const response = await fetch(`${gatewayBaseUrl.value}/models`, {
      headers: {
        Authorization: `Bearer ${props.apiKey.key}`,
        Accept: 'application/json'
      },
      signal: controller.signal
    })
    if (!response.ok) {
      throw new Error(await readResponseError(response))
    }

    const payload = await response.json() as GatewayModelList
    if (modelController !== controller) return
    if (!Array.isArray(payload.data)) {
      throw new Error(t('keys.testModal.invalidModelResponse'))
    }

    const seen = new Set<string>()
    models.value = payload.data.flatMap((entry): ModelOption[] => {
      if (!entry || typeof entry !== 'object') return []
      const model = entry as GatewayModel
      if (typeof model.id !== 'string' || !model.id.trim() || seen.has(model.id)) return []
      seen.add(model.id)
      return [{
        value: model.id,
        label: typeof model.display_name === 'string' && model.display_name.trim()
          ? model.display_name
          : model.id
      }]
    })

    if (models.value.length === 0) {
      throw new Error(t('keys.testModal.noModels'))
    }
    selectedModel.value = preferredModel(models.value)
  } catch (error) {
    if (isAbortError(error)) return
    modelLoadError.value = errorMessage(error, t('keys.testModal.modelLoadFailed'))
  } finally {
    if (modelController === controller) {
      loadingModels.value = false
      modelController = null
    }
  }
}

function preferredModel(options: ModelOption[]) {
  const likelyChatModel = options.find((option) => !/(?:image|video|embedding|rerank|whisper|speech|tts|realtime)/i.test(option.value))
  return (likelyChatModel || options[0]).value
}

async function startTest() {
  if (!props.apiKey?.key || !selectedModel.value || !prompt.value.trim()) return

  testController?.abort()
  const controller = new AbortController()
  testController = controller
  cancelledByUser = false
  testStatus.value = 'running'
  testError.value = ''
  reasoningContent.value = ''
  responseContent.value = ''
  requestStarted.value = true
  firstTokenLatency.value = null
  totalLatency.value = null
  requestStartTime = performance.now()

  const timeout = window.setTimeout(() => controller.abort(), 120_000)
  try {
    const response = await fetch(`${gatewayBaseUrl.value}/chat/completions`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${props.apiKey.key}`,
        'Content-Type': 'application/json',
        Accept: 'text/event-stream'
      },
      body: JSON.stringify({
        model: selectedModel.value,
        messages: [{ role: 'user', content: prompt.value.trim() }],
        stream: true
      }),
      signal: controller.signal
    })

    if (!response.ok) {
      throw new Error(await readResponseError(response))
    }

    const contentType = response.headers.get('content-type') || ''
    if (contentType.includes('application/json')) {
      const payload = await response.json() as StreamPayload
      consumePayload(payload)
    } else {
      await consumeEventStream(response)
    }

    if (testController !== controller) return
    totalLatency.value = elapsedMilliseconds()
    testStatus.value = 'success'
  } catch (error) {
    if (testController !== controller) return
    totalLatency.value = elapsedMilliseconds()
    if (isAbortError(error)) {
      testStatus.value = cancelledByUser ? 'cancelled' : 'error'
      if (!cancelledByUser) testError.value = t('keys.testModal.timeout')
    } else {
      testStatus.value = 'error'
      testError.value = errorMessage(error, t('keys.testModal.failed'))
    }
  } finally {
    window.clearTimeout(timeout)
    if (testController === controller) {
      testController = null
      emit('tested')
    }
  }
}

async function consumeEventStream(response: Response) {
  const reader = response.body?.getReader()
  if (!reader) throw new Error(t('keys.testModal.emptyResponse'))

  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split(/\r?\n/)
    buffer = lines.pop() || ''
    for (const line of lines) consumeEventLine(line)
  }

  buffer += decoder.decode()
  if (buffer.trim()) consumeEventLine(buffer)
}

function consumeEventLine(line: string) {
  const trimmed = line.trim()
  if (!trimmed.startsWith('data:')) return
  const data = trimmed.slice(5).trim()
  if (!data || data === '[DONE]') return

  let payload: StreamPayload
  try {
    payload = JSON.parse(data) as StreamPayload
  } catch {
    return
  }
  consumePayload(payload)
}

function consumePayload(payload: StreamPayload) {
  const upstreamError = nestedMessage(payload.error)
  if (upstreamError) throw new Error(upstreamError)
  if (!Array.isArray(payload.choices) || payload.choices.length === 0) return

  const choice = payload.choices[0] as StreamChoice
  const source = isRecord(choice.delta)
    ? choice.delta
    : isRecord(choice.message)
      ? choice.message
      : null
  if (!source) return

  const reasoning = contentText(source.reasoning_content)
  const content = contentText(source.content)
  if (reasoning || content) markFirstToken()
  if (reasoning) reasoningContent.value += reasoning
  if (content) responseContent.value += content
}

function contentText(value: unknown): string {
  if (typeof value === 'string') return value
  if (!Array.isArray(value)) return ''
  return value.map((part) => {
    if (typeof part === 'string') return part
    if (!isRecord(part)) return ''
    if (typeof part.text === 'string') return part.text
    if (isRecord(part.text) && typeof part.text.value === 'string') return part.text.value
    return ''
  }).join('')
}

function markFirstToken() {
  if (firstTokenLatency.value === null) {
    firstTokenLatency.value = elapsedMilliseconds()
  }
}

function elapsedMilliseconds() {
  return Math.max(0, Math.round(performance.now() - requestStartTime))
}

function cancelTest() {
  cancelledByUser = true
  testController?.abort()
}

async function readResponseError(response: Response) {
  const fallback = `HTTP ${response.status}`
  try {
    const text = await response.text()
    if (!text.trim()) return fallback
    const payload = JSON.parse(text) as unknown
    return nestedMessage(payload) || fallback
  } catch {
    return fallback
  }
}

function nestedMessage(value: unknown): string {
  if (typeof value === 'string') return value
  if (!isRecord(value)) return ''
  if (typeof value.message === 'string') return value.message
  if (typeof value.detail === 'string') return value.detail
  return nestedMessage(value.error)
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback
}

function isAbortError(error: unknown) {
  return (error instanceof DOMException && error.name === 'AbortError')
    || (isRecord(error) && error.name === 'AbortError')
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object'
}
</script>
