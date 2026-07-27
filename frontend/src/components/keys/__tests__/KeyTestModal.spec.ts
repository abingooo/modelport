import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiKey } from '@/types'
import KeyTestModal from '../KeyTestModal.vue'

const messages: Record<string, string> = {
  'common.close': 'Close',
  'keys.status.active': 'Active',
  'keys.testModal.title': 'Test API Key',
  'keys.testModal.noGroupTitle': 'Assign a group first',
  'keys.testModal.noGroupDescription': 'No group is assigned',
  'keys.testModal.modelLabel': 'Test Model',
  'keys.testModal.reloadModels': 'Reload models',
  'keys.testModal.searchModel': 'Search models',
  'keys.testModal.loadingModels': 'Loading models',
  'keys.testModal.modelsUnavailable': 'Models unavailable',
  'keys.testModal.selectModel': 'Select a model',
  'keys.testModal.invalidModelResponse': 'Invalid model response',
  'keys.testModal.noModels': 'No models',
  'keys.testModal.modelLoadFailed': 'Failed to load models',
  'keys.testModal.promptLabel': 'Test Message',
  'keys.testModal.promptPlaceholder': 'Enter a test message',
  'keys.testModal.defaultPrompt': 'Reply with Connection successful',
  'keys.testModal.billingNotice': 'This real request is billed',
  'keys.testModal.firstTokenLatency': 'First Token',
  'keys.testModal.totalLatency': 'Total Time',
  'keys.testModal.ready': 'Ready',
  'keys.testModal.running': 'Running',
  'keys.testModal.reasoning': 'Reasoning',
  'keys.testModal.response': 'Response',
  'keys.testModal.noTextResponse': 'No text response',
  'keys.testModal.success': 'Test succeeded',
  'keys.testModal.cancelled': 'Test stopped',
  'keys.testModal.stop': 'Stop Test',
  'keys.testModal.start': 'Start Test',
  'keys.testModal.retry': 'Test Again',
  'keys.testModal.timeout': 'Request timed out',
  'keys.testModal.failed': 'Test failed',
  'keys.testModal.emptyResponse': 'Empty response'
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: `
    <div v-if="show" data-test="dialog">
      <h2>{{ title }}</h2>
      <slot />
      <slot name="footer" />
    </div>
  `
}

const SelectStub = {
  props: ['modelValue', 'options', 'disabled'],
  emits: ['update:modelValue'],
  template: `
    <select
      data-test="model-select"
      :value="modelValue"
      :disabled="disabled"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
}

const TextAreaStub = {
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: `
    <textarea
      data-test="test-prompt"
      :value="modelValue"
      :disabled="disabled"
      @input="$emit('update:modelValue', $event.target.value)"
    />
  `
}

function createKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 17,
    user_id: 4,
    key: 'sk-user-specific-test-key-1234',
    name: 'My downstream key',
    group_id: 8,
    group: {
      id: 8,
      name: 'DeepSeek Production',
      platform: 'deepseek',
      subscription_type: 'standard',
      rate_multiplier: 1,
      is_free: false
    } as ApiKey['group'],
    status: 'active',
    ip_whitelist: [],
    ip_blacklist: [],
    last_used_at: null,
    last_used_ip: null,
    quota: 0,
    quota_used: 0,
    expires_at: null,
    created_at: '2026-07-27T00:00:00Z',
    updated_at: '2026-07-27T00:00:00Z',
    current_concurrency: 0,
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_7d: 0,
    usage_5h: 0,
    usage_1d: 0,
    usage_7d: 0,
    window_5h_start: null,
    window_1d_start: null,
    window_7d_start: null,
    reset_5h_at: null,
    reset_1d_at: null,
    reset_7d_at: null,
    ...overrides
  }
}

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'Content-Type': 'application/json' }
  })
}

function streamResponse(chunks: string[]) {
  const encoder = new TextEncoder()
  let index = 0
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (index >= chunks.length) {
        controller.close()
        return
      }
      controller.enqueue(encoder.encode(chunks[index++]))
    }
  })
  return new Response(body, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' }
  })
}

function mountModal(apiKey: ApiKey = createKey(), baseUrl = 'https://api.modelport.test/v1/chat/completions') {
  return mount(KeyTestModal, {
    props: {
      show: false,
      apiKey,
      baseUrl
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        TextArea: TextAreaStub,
        Icon: true
      }
    }
  })
}

describe('KeyTestModal', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('loads models with the selected key and runs a billed streaming gateway test', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse({
        object: 'list',
        data: [
          { id: 'gpt-image-1', display_name: 'GPT Image 1' },
          { id: 'deepseek-reasoner', display_name: 'DeepSeek Reasoner' }
        ]
      }))
      .mockResolvedValueOnce(streamResponse([
        'data: {"choices":[{"delta":{"reasoning_content":"checking"}}]}\n\n',
        'data: {"choices":[{"delta":{"content":"Connection ',
        'successful"}}]}\n\ndata: [DONE]\n\n'
      ]))

    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      'https://api.modelport.test/v1/models',
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer sk-user-specific-test-key-1234'
        })
      })
    )
    expect((wrapper.get('[data-test="model-select"]').element as HTMLSelectElement).value)
      .toBe('deepseek-reasoner')
    expect(wrapper.text()).toContain('This real request is billed')
    expect(wrapper.text()).toContain('sk-use...1234')
    expect(wrapper.text()).not.toContain('sk-user-specific-test-key-1234')

    const startButton = wrapper.findAll('button').find((button) => button.text().includes('Start Test'))
    expect(startButton).toBeTruthy()
    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    const request = vi.mocked(fetch).mock.calls[1]
    expect(request[0]).toBe('https://api.modelport.test/v1/chat/completions')
    expect(request[1]).toEqual(expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({
        Authorization: 'Bearer sk-user-specific-test-key-1234',
        Accept: 'text/event-stream'
      })
    }))
    expect(JSON.parse(request[1]!.body as string)).toEqual({
      model: 'deepseek-reasoner',
      messages: [{ role: 'user', content: 'Reply with Connection successful' }],
      stream: true
    })
    expect(wrapper.text()).toContain('checking')
    expect(wrapper.text()).toContain('Connection successful')
    expect(wrapper.text()).toMatch(/First Token\d+ ms/)
    expect(wrapper.text()).toMatch(/Total Time\d+ ms/)
    expect(wrapper.text()).toContain('Test succeeded')
    expect(wrapper.emitted('tested')).toHaveLength(1)
  })

  it('shows the gateway authentication error returned while loading models', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse({
      error: { message: 'API key is inactive' }
    }, 401))

    const wrapper = mountModal(createKey({ status: 'inactive' }))
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('API key is inactive')
    expect(fetch).toHaveBeenCalledTimes(1)
    expect((wrapper.get('[data-test="model-select"]').element as HTMLSelectElement).disabled).toBe(true)
  })

  it('does not call the gateway when the key has no group', async () => {
    const wrapper = mountModal(createKey({ group_id: null, group: undefined }))
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('Assign a group first')
    expect(fetch).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="model-select"]').exists()).toBe(false)
  })

  it('supports a successful non-streaming JSON fallback response', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse({ data: [{ id: 'glm-5', display_name: 'GLM-5' }] }))
      .mockResolvedValueOnce(jsonResponse({
        choices: [{ message: { content: 'Connection successful' } }]
      }))

    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const startButton = wrapper.findAll('button').find((button) => button.text().includes('Start Test'))
    await startButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Connection successful')
    expect(wrapper.text()).toContain('Test succeeded')
  })

  it('does not let an aborted request overwrite state after the modal closes', async () => {
    let rejectRequest: ((error: Error) => void) | undefined
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse({ data: [{ id: 'deepseek-chat' }] }))
      .mockImplementationOnce((_input, init) => new Promise<Response>((_resolve, reject) => {
        rejectRequest = reject
        init?.signal?.addEventListener('abort', () => {
          const error = new Error('aborted')
          error.name = 'AbortError'
          reject(error)
        })
      }))

    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const startButton = wrapper.findAll('button').find((button) => button.text().includes('Start Test'))
    await startButton!.trigger('click')
    await flushPromises()
    expect(rejectRequest).toBeTypeOf('function')

    await wrapper.setProps({ show: false })
    await flushPromises()

    expect((wrapper.vm as unknown as { testStatus: string }).testStatus).toBe('idle')
    expect(wrapper.emitted('tested')).toBeUndefined()
  })
})
