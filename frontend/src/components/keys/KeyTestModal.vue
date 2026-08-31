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

        <div class="space-y-1.5">
          <span class="input-label">{{ t('keys.testModal.protocolLabel') }}</span>
          <div
            class="grid grid-cols-1 gap-1 rounded-lg bg-gray-100 p-1 sm:grid-cols-3 dark:bg-dark-700"
            role="radiogroup"
            :aria-label="t('keys.testModal.protocolLabel')"
            data-test="protocol-selector"
          >
            <button
              v-for="option in protocolOptions"
              :key="option.value"
              type="button"
              role="radio"
              :data-test="`protocol-${option.value}`"
              :aria-checked="selectedProtocol === option.value"
              :disabled="isRunning"
              :class="[
                'min-h-9 rounded-md px-2 py-2 text-sm font-medium transition-colors disabled:cursor-not-allowed',
                selectedProtocol === option.value
                  ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
                  : 'text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white'
              ]"
              @click="selectedProtocol = option.value"
            >
              {{ option.label }}
            </button>
          </div>
        </div>

        <div class="flex min-h-10 items-center justify-between gap-4">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="key-test-stream">
            {{ t('keys.testModal.streamLabel') }}
          </label>
          <Toggle
            id="key-test-stream"
            :model-value="streamEnabled"
            :disabled="isRunning"
            :aria-label="t('keys.testModal.streamLabel')"
            :class="isRunning ? 'cursor-not-allowed opacity-50' : ''"
            data-test="stream-toggle"
            @update:model-value="setStreamEnabled"
          />
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
          <div class="min-h-20 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('keys.testModal.httpStatus') }}
            </div>
            <div
              class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white"
              data-test="http-status"
            >
              {{ httpStatus ?? '--' }}
            </div>
          </div>
          <div class="min-h-20 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('keys.testModal.requestId') }}
            </div>
            <div
              class="mt-1 break-all text-sm font-semibold leading-6 text-gray-900 dark:text-white"
              data-test="request-id"
            >
              {{ requestId || '--' }}
            </div>
          </div>
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
          data-test="stop-test"
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
          data-test="start-test"
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
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import type { ApiKey } from '@/types'
import { maskApiKey } from '@/utils/maskApiKey'

type TestStatus = 'idle' | 'running' | 'success' | 'error' | 'cancelled'
type TestProtocol = 'chat' | 'responses' | 'messages'

interface ModelOption {
  value: string
  label: string
}

interface ProtocolOption {
  value: TestProtocol
  label: string
}

interface GatewayModel {
  id?: unknown
  display_name?: unknown
}

interface GatewayModelList {
  data?: unknown
}

interface GatewayRequest {
  endpoint: string
  headers: Record<string, string>
  body: Record<string, unknown>
}

interface GatewayResponseEvidence {
  receivedData: boolean
  recognizedFrame: boolean
  terminalFrame: boolean
}

const TEST_TIMEOUT_MS = 120_000
const MAX_OUTPUT_TOKENS = 64
const MAX_ERROR_BODY_LENGTH = 16_384
const MAX_DISPLAY_ERROR_LENGTH = 800
const REQUEST_ID_HEADERS = [
  'x-request-id',
  'x-client-request-id',
  'request-id',
  'openai-request-id'
] as const

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
const selectedProtocol = ref<TestProtocol>('chat')
const streamEnabled = ref(true)
const prompt = ref('')
const loadingModels = ref(false)
const modelLoadError = ref('')
const testStatus = ref<TestStatus>('idle')
const testError = ref('')
const reasoningContent = ref('')
const responseContent = ref('')
const requestStarted = ref(false)
const httpStatus = ref<number | null>(null)
const requestId = ref('')
const firstTokenLatency = ref<number | null>(null)
const totalLatency = ref<number | null>(null)
let modelController: AbortController | null = null
let testController: AbortController | null = null
let requestStartTime = 0
let cancelledByUser = false

const maskedKey = computed(() => maskApiKey(props.apiKey?.key || ''))
const isRunning = computed(() => testStatus.value === 'running')
const modelOptions = computed(() => models.value)
const protocolOptions = computed<ProtocolOption[]>(() => [
  { value: 'chat', label: t('keys.testModal.protocolChat') },
  { value: 'responses', label: t('keys.testModal.protocolResponses') },
  { value: 'messages', label: t('keys.testModal.protocolMessages') }
])
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

  url.username = ''
  url.password = ''
  url.search = ''
  url.hash = ''
  const path = url.pathname
    .replace(/(?:\/v1)?\/(?:chat\/completions|responses|messages|models)\/?$/i, '')
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
  },
  { immediate: true }
)

onBeforeUnmount(abortRequests)

function abortRequests() {
  modelController?.abort()
  testController?.abort()
  modelController = null
  testController = null
}

function resetState() {
  models.value = []
  selectedModel.value = ''
  selectedProtocol.value = preferredProtocol()
  streamEnabled.value = true
  prompt.value = ''
  loadingModels.value = false
  modelLoadError.value = ''
  testStatus.value = 'idle'
  testError.value = ''
  reasoningContent.value = ''
  responseContent.value = ''
  requestStarted.value = false
  httpStatus.value = null
  requestId.value = ''
  firstTokenLatency.value = null
  totalLatency.value = null
}

function preferredProtocol(): TestProtocol {
  const group = props.apiKey?.group
  if (group?.claude_code_only || group?.platform === 'anthropic' || group?.platform === 'antigravity') {
    return 'messages'
  }
  return 'chat'
}

function handleClose() {
  abortRequests()
  emit('close')
}

function setStreamEnabled(value: boolean) {
  if (!isRunning.value) streamEnabled.value = value
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
      if (!isRecord(entry)) return []
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
    modelLoadError.value = safeErrorMessage(error, t('keys.testModal.modelLoadFailed'))
  } finally {
    if (modelController === controller) {
      loadingModels.value = false
      modelController = null
    }
  }
}

function preferredModel(options: ModelOption[]) {
  const likelyTextModel = options.find(
    (option) => !/(?:image|video|embedding|rerank|whisper|speech|tts|realtime)/i.test(option.value)
  )
  return (likelyTextModel || options[0]).value
}

async function startTest() {
  const apiKey = props.apiKey?.key
  const model = selectedModel.value
  const userPrompt = prompt.value.trim()
  if (!apiKey || !model || !userPrompt) return

  testController?.abort()
  const controller = new AbortController()
  const protocol = selectedProtocol.value
  const request = buildGatewayRequest(protocol, apiKey, model, userPrompt, streamEnabled.value)
  testController = controller
  cancelledByUser = false
  testStatus.value = 'running'
  testError.value = ''
  reasoningContent.value = ''
  responseContent.value = ''
  requestStarted.value = true
  httpStatus.value = null
  requestId.value = ''
  firstTokenLatency.value = null
  totalLatency.value = null
  requestStartTime = performance.now()

  let timeoutReached = false
  const timeout = window.setTimeout(() => {
    timeoutReached = true
    controller.abort()
  }, TEST_TIMEOUT_MS)

  try {
    const response = await fetch(`${gatewayBaseUrl.value}${request.endpoint}`, {
      method: 'POST',
      headers: request.headers,
      body: JSON.stringify(request.body),
      signal: controller.signal
    })
    captureResponseMetadata(response)

    if (!response.ok) {
      throw new Error(await readResponseError(response))
    }

    await consumeGatewayResponse(response, protocol)
    if (testController !== controller) return
    totalLatency.value = elapsedMilliseconds()
    testStatus.value = 'success'
  } catch (error) {
    if (testController !== controller) return
    totalLatency.value = elapsedMilliseconds()
    if (isAbortError(error)) {
      testStatus.value = cancelledByUser ? 'cancelled' : 'error'
      if (!cancelledByUser) {
        testError.value = timeoutReached
          ? t('keys.testModal.timeout')
          : t('keys.testModal.failed')
      }
    } else {
      testStatus.value = 'error'
      testError.value = safeErrorMessage(error, t('keys.testModal.failed'))
    }
  } finally {
    window.clearTimeout(timeout)
    if (testController === controller) {
      testController = null
      emit('tested')
    }
  }
}

function buildGatewayRequest(
  protocol: TestProtocol,
  apiKey: string,
  model: string,
  userPrompt: string,
  stream: boolean
): GatewayRequest {
  const headers = {
    Authorization: `Bearer ${apiKey}`,
    'Content-Type': 'application/json',
    Accept: stream ? 'text/event-stream' : 'application/json'
  }

  if (protocol === 'responses') {
    return {
      endpoint: '/responses',
      headers,
      body: {
        model,
        input: userPrompt,
        max_output_tokens: MAX_OUTPUT_TOKENS,
        stream
      }
    }
  }

  if (protocol === 'messages') {
    return {
      endpoint: '/messages',
      headers,
      body: {
        model,
        max_tokens: MAX_OUTPUT_TOKENS,
        messages: [{ role: 'user', content: userPrompt }],
        stream
      }
    }
  }

  return {
    endpoint: '/chat/completions',
    headers,
    body: {
      model,
      messages: [{ role: 'user', content: userPrompt }],
      max_tokens: MAX_OUTPUT_TOKENS,
      stream
    }
  }
}

function captureResponseMetadata(response: Response) {
  httpStatus.value = response.status
  requestId.value = ''
  for (const header of REQUEST_ID_HEADERS) {
    const candidate = safeRequestId(response.headers.get(header))
    if (candidate) {
      requestId.value = candidate
      break
    }
  }
}

async function consumeGatewayResponse(response: Response, protocol: TestProtocol) {
  const contentType = (response.headers.get('content-type') || '').toLowerCase()
  if (contentType.includes('text/event-stream')) {
    const evidence = await consumeEventStream(response, protocol)
    if (!evidence.recognizedFrame) {
      throw new Error(t(evidence.receivedData
        ? 'keys.testModal.invalidResponse'
        : 'keys.testModal.emptyResponse'))
    }
    if (!evidence.terminalFrame) {
      throw new Error(t('keys.testModal.incompleteResponse'))
    }
    return
  }

  const raw = await response.text()
  if (!raw.trim()) throw new Error(t('keys.testModal.emptyResponse'))
  let payload: unknown
  try {
    payload = JSON.parse(raw)
  } catch {
    throw new Error(t('keys.testModal.invalidResponse'))
  }
  const evidence = createResponseEvidence()
  consumeProtocolPayload(protocol, payload, '', false, evidence)
  if (!evidence.recognizedFrame) {
    throw new Error(t('keys.testModal.invalidResponse'))
  }
}

async function consumeEventStream(response: Response, protocol: TestProtocol) {
  const reader = response.body?.getReader()
  if (!reader) throw new Error(t('keys.testModal.emptyResponse'))

  const decoder = new TextDecoder()
  const evidence = createResponseEvidence()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    buffer = consumeCompleteEventFrames(buffer, protocol, evidence)
  }

  buffer += decoder.decode()
  if (buffer.trim()) consumeEventFrame(buffer, protocol, evidence)
  return evidence
}

function createResponseEvidence(): GatewayResponseEvidence {
  return { receivedData: false, recognizedFrame: false, terminalFrame: false }
}

function consumeCompleteEventFrames(
  buffer: string,
  protocol: TestProtocol,
  evidence: GatewayResponseEvidence
) {
  let remaining = buffer
  while (true) {
    const boundary = remaining.match(/\r?\n\r?\n/)
    if (!boundary || boundary.index === undefined) return remaining
    const frame = remaining.slice(0, boundary.index)
    remaining = remaining.slice(boundary.index + boundary[0].length)
    consumeEventFrame(frame, protocol, evidence)
  }
}

function consumeEventFrame(
  frame: string,
  protocol: TestProtocol,
  evidence: GatewayResponseEvidence
) {
  let eventName = ''
  const dataLines: string[] = []
  for (const line of frame.split(/\r?\n/)) {
    if (line.startsWith('event:')) {
      eventName = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
    }
  }

  const data = dataLines.join('\n').trim()
  if (!data) return
  evidence.receivedData = true
  if (data === '[DONE]') {
    evidence.terminalFrame = true
    return
  }

  let payload: unknown
  try {
    payload = JSON.parse(data)
  } catch {
    if (/error|failed/i.test(eventName)) {
      throw new Error(sanitizeDisplayText(data) || t('keys.testModal.failed'))
    }
    return
  }
  consumeProtocolPayload(protocol, payload, eventName, true, evidence)
}

function consumeProtocolPayload(
  protocol: TestProtocol,
  payload: unknown,
  eventName: string,
  fromStream: boolean,
  evidence: GatewayResponseEvidence
) {
  if (!isRecord(payload)) return
  throwPayloadError(payload, eventName)

  if (protocol === 'responses') {
    consumeResponsesPayload(payload, eventName, fromStream, evidence)
  } else if (protocol === 'messages') {
    consumeMessagesPayload(payload, fromStream, evidence)
  } else {
    consumeChatPayload(payload, fromStream, evidence)
  }
}

function throwPayloadError(payload: Record<string, unknown>, eventName: string) {
  const payloadType = typeof payload.type === 'string' ? payload.type : eventName
  const isFailure = payloadType === 'error'
    || payloadType.endsWith('.failed')
    || payloadType.endsWith('.error')
  const message = nestedMessage(payload.error)
    || (isFailure ? nestedMessage(payload.response) || nestedMessage(payload) : '')
  if (message) throw new Error(sanitizeDisplayText(message))
  if (isFailure) throw new Error(t('keys.testModal.failed'))
}

function consumeChatPayload(
  payload: Record<string, unknown>,
  fromStream: boolean,
  evidence: GatewayResponseEvidence
) {
  if (!Array.isArray(payload.choices) || payload.choices.length === 0) return
  const choice = payload.choices[0]
  if (!isRecord(choice)) return

  const hasDelta = isRecord(choice.delta)
  const hasMessage = isRecord(choice.message)
  const hasLegacyText = typeof choice.text === 'string'
  const hasFinishReason = choice.finish_reason !== undefined && choice.finish_reason !== null
  if (!hasDelta && !hasMessage && !hasLegacyText && !hasFinishReason) return

  evidence.recognizedFrame = true
  if (fromStream && hasFinishReason) evidence.terminalFrame = true
  const source = hasDelta ? choice.delta as Record<string, unknown>
    : hasMessage ? choice.message as Record<string, unknown>
      : choice
  appendReasoning(contentText(source.reasoning_content) || contentText(source.reasoning))
  appendResponse(contentText(source.content) || contentText(source.text))
}

function consumeResponsesPayload(
  payload: Record<string, unknown>,
  eventName: string,
  fromStream: boolean,
  evidence: GatewayResponseEvidence
) {
  const eventType = typeof payload.type === 'string' ? payload.type : eventName
  if (/response\.(?:output_text|refusal)\.delta$/i.test(eventType)) {
    evidence.recognizedFrame = true
    appendResponse(contentText(payload.delta))
    return
  }
  if (/response\.reasoning(?:_summary|_summary_text|_text)?\.delta$/i.test(eventType)) {
    evidence.recognizedFrame = true
    appendReasoning(contentText(payload.delta))
    return
  }

  const terminalEvent = /response\.(?:completed|done|incomplete)$/i.test(eventType)
  if (terminalEvent) {
    evidence.recognizedFrame = true
    evidence.terminalFrame = true
  }
  if (fromStream && !terminalEvent) return
  const response = isRecord(payload.response) ? payload.response : payload
  const validEnvelope = Array.isArray(response.output)
    || typeof response.output_text === 'string'
    || response.object === 'response'
    || response.type === 'response'
  if (!fromStream && !validEnvelope) return
  evidence.recognizedFrame = true
  const output = extractResponsesOutput(response)
  if (!responseContent.value) appendResponse(output.text)
  if (!reasoningContent.value) appendReasoning(output.reasoning)
}

function extractResponsesOutput(response: Record<string, unknown>) {
  let text = contentText(response.output_text)
  const hasDirectText = Boolean(text)
  let reasoning = contentText(response.reasoning_content)
  if (!Array.isArray(response.output)) return { text, reasoning }

  for (const item of response.output) {
    if (!isRecord(item)) continue
    const type = typeof item.type === 'string' ? item.type : ''
    if (type === 'reasoning') {
      reasoning += contentText(item.summary) || contentText(item.content)
    } else if (!hasDirectText && (type === 'message' || type === 'output_text' || !type)) {
      text += contentText(item.content) || contentText(item.text)
    }
  }
  return { text, reasoning }
}

function consumeMessagesPayload(
  payload: Record<string, unknown>,
  fromStream: boolean,
  evidence: GatewayResponseEvidence
) {
  const payloadType = typeof payload.type === 'string' ? payload.type : ''
  if (fromStream && [
    'message_start',
    'content_block_start',
    'content_block_delta',
    'content_block_stop',
    'message_delta',
    'message_stop'
  ].includes(payloadType)) {
    evidence.recognizedFrame = true
    if (payloadType === 'message_stop') evidence.terminalFrame = true
  }
  if (payloadType === 'content_block_delta' && isRecord(payload.delta)) {
    const deltaType = typeof payload.delta.type === 'string' ? payload.delta.type : ''
    if (deltaType === 'thinking_delta') {
      appendReasoning(contentText(payload.delta.thinking))
    } else if (deltaType === 'text_delta' || !deltaType) {
      appendResponse(contentText(payload.delta.text))
    }
    return
  }

  if (payloadType === 'content_block_start' && isRecord(payload.content_block)) {
    appendAnthropicBlock(payload.content_block)
    return
  }
  if (fromStream) return

  const message = isRecord(payload.message) ? payload.message : payload
  const validEnvelope = payloadType === 'message'
    || Array.isArray(message.content)
    || typeof message.content === 'string'
  if (!validEnvelope) return
  evidence.recognizedFrame = true
  if (Array.isArray(message.content)) {
    for (const block of message.content) {
      if (isRecord(block)) appendAnthropicBlock(block)
    }
  } else {
    appendResponse(contentText(message.content))
  }
}

function appendAnthropicBlock(block: Record<string, unknown>) {
  const type = typeof block.type === 'string' ? block.type : ''
  if (type === 'thinking') {
    appendReasoning(contentText(block.thinking))
  } else if (type === 'text' || !type) {
    appendResponse(contentText(block.text))
  }
}

function contentText(value: unknown): string {
  if (typeof value === 'string') return value
  if (Array.isArray(value)) return value.map(contentText).join('')
  if (!isRecord(value)) return ''
  if (typeof value.text === 'string') return value.text
  if (isRecord(value.text) && typeof value.text.value === 'string') return value.text.value
  if (typeof value.value === 'string') return value.value
  return ''
}

function appendReasoning(value: string) {
  if (!value) return
  markFirstToken()
  reasoningContent.value += value
}

function appendResponse(value: string) {
  if (!value) return
  markFirstToken()
  responseContent.value += value
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
    const raw = (await response.text()).slice(0, MAX_ERROR_BODY_LENGTH)
    if (!raw.trim()) return fallback
    let message = raw
    try {
      message = nestedMessage(JSON.parse(raw)) || fallback
    } catch {
      // Plain-text and HTML gateway errors are escaped by Vue and redacted below.
    }
    return sanitizeDisplayText(message) || fallback
  } catch {
    return fallback
  }
}

function nestedMessage(value: unknown, depth = 0): string {
  if (depth > 5) return ''
  if (typeof value === 'string') return value
  if (!isRecord(value)) return ''
  if (typeof value.message === 'string') return value.message
  if (typeof value.detail === 'string') return value.detail
  return nestedMessage(value.error, depth + 1)
    || nestedMessage(value.message, depth + 1)
    || nestedMessage(value.detail, depth + 1)
}

function safeErrorMessage(error: unknown, fallback: string) {
  const message = error instanceof Error && error.message ? error.message : fallback
  return sanitizeDisplayText(message) || fallback
}

function sanitizeDisplayText(value: string) {
  let sanitized = ''
  for (const character of value) {
    const code = character.charCodeAt(0)
    const isUnsafeControl = (code < 32 && code !== 9 && code !== 10 && code !== 13) || code === 127
    sanitized += isUnsafeControl ? ' ' : character
  }
  const currentKey = props.apiKey?.key
  if (currentKey) sanitized = sanitized.split(currentKey).join('[REDACTED]')
  sanitized = sanitized
    .replace(/(bearer\s+)[a-z0-9._~+/=-]+/gi, '$1[REDACTED]')
    .replace(/((?:x-api-key|api[_-]?key)\s*[:=]\s*)[^\s,;]+/gi, '$1[REDACTED]')
    .replace(/\b(?:sk|rk|pk|key)-[a-z0-9._-]{8,}\b/gi, '[REDACTED]')
    .replace(/(["']?(?:(?:account|channel|route)(?:[_-]?id)?|(?:upstream|base|proxy)[_-]?(?:url|host))["']?\s*[:=]\s*["']?)[^\s,;"'}]+/gi, '$1[REDACTED]')
    .replace(/\b(?:https?|wss?):\/\/[^\s,;]+/gi, '[REDACTED_URL]')
    .trim()
  return sanitized.slice(0, MAX_DISPLAY_ERROR_LENGTH)
}

function safeRequestId(value: string | null) {
  if (!value) return ''
  const sanitized = sanitizeDisplayText(value)
  return /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$/.test(sanitized) ? sanitized : ''
}

function isAbortError(error: unknown) {
  return (error instanceof DOMException && error.name === 'AbortError')
    || (isRecord(error) && error.name === 'AbortError')
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object'
}
</script>
