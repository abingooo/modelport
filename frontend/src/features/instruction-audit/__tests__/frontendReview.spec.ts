import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { StepUpCancelledError } from '@/composables/useStepUp'
import InstructionAuditReasonPolicies from '../components/InstructionAuditReasonPolicies.vue'
import InstructionAuditResourceState from '../components/InstructionAuditResourceState.vue'
import InstructionAuditRuntimeConfig from '../components/InstructionAuditRuntimeConfig.vue'
import InstructionTranslationPanel from '../components/InstructionTranslationPanel.vue'
import type { InstructionRuntimeConfig, InstructionTranslationJob } from '../types'

const mocks = vi.hoisted(() => ({
  createTranslation: vi.fn(),
  getTranslation: vi.fn(),
  showError: vi.fn(),
  copy: vi.fn(),
  totpStepUp: vi.fn(),
}))

vi.mock('../api', () => ({
  default: {
    createTranslation: mocks.createTranslation,
    getTranslation: mocks.getTranslation,
  },
}))
vi.mock('@/api', () => ({ totpAPI: { stepUp: mocks.totpStepUp } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: vi.fn() }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: mocks.copy }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'stepUp.digitLabel') return `digit ${params?.position}`
        return key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`))
      },
    }),
  }
})

const IconStub = { template: '<i />' }
const StepUpStub = { template: '<div />' }
const ToggleStub = defineComponent({
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: '<button type="button" role="switch" :disabled="disabled" :aria-checked="modelValue" @click="$emit(\'update:modelValue\', !modelValue)" />',
})
const ConfirmStub = { template: '<div />' }

function translationJob(overrides: Partial<InstructionTranslationJob> = {}): InstructionTranslationJob {
  return {
    id: 15,
    resource_type: 'event',
    resource_id: 7,
    field_name: 'instructions',
    target_language: 'zh-CN',
    provider: 'internal',
    status: 'pending',
    error_code: '',
    chunk_count: 2,
    completed_chunks: 0,
    attempts: 0,
    max_attempts: 3,
    result_bytes: 0,
    redaction_count: 0,
    provider_latency_ms: 0,
    expires_at: '2026-08-01T01:00:00Z',
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z',
    ...overrides,
  }
}

function mountTranslationPanel() {
  return mount(InstructionTranslationPanel, {
    props: {
      resourceType: 'event',
      resourceId: 7,
      fieldName: 'instructions',
      original: 'untrusted content',
      enabled: true,
      externalEnabled: false,
    },
    global: { stubs: { Icon: IconStub, TotpStepUpDialog: StepUpStub } },
  })
}

const runtimeConfig: InstructionRuntimeConfig = {
  config_version: 7,
  max_body_bytes: 64 * 1024 * 1024,
  parse_timeout_ms: 500,
  max_inflight_body_bytes: 256 * 1024 * 1024,
  pass_event_retention_days: 7,
  aggregate_retention_days: 365,
  raw_content_retention_days: 30,
  ai_enabled: false,
  ai_base_url: '',
  ai_model: '',
  ai_has_token: false,
  ai_timeout_ms: 3000,
  ai_max_concurrency: 4,
  ai_min_confidence: 0.9,
  ai_per_user_rpm: 5,
  ai_per_user_daily_limit: 10,
  ai_global_daily_limit: 100,
  ai_prompt_version: 'v1',
  translation_enabled: false,
  external_translation_enabled: false,
  translation_base_url: '',
  translation_model: '',
  translation_has_token: false,
  translation_timeout_ms: 5000,
  translation_max_concurrency: 2,
  translation_chunk_bytes: 8192,
  translation_max_bytes: 1024 * 1024,
  translation_result_ttl_seconds: 900,
  updated_at: '2026-08-01T00:00:00Z',
}

describe('instruction-audit frontend review fixes', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('stops after bounded polling retries and can continue the existing translation job', async () => {
    vi.useFakeTimers()
    mocks.createTranslation.mockResolvedValue(translationJob())
    mocks.getTranslation
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockResolvedValueOnce(translationJob({ status: 'succeeded', completed_chunks: 2, translated_text: '安全译文' }))

    const wrapper = mountTranslationPanel()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(800)
    await vi.advanceTimersByTimeAsync(1000)
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(mocks.getTranslation).toHaveBeenCalledTimes(3)
    expect(wrapper.get('[data-test="translation-poll-error"]').text()).toContain('admin.instructionAudit.translation.pollPaused')
    await vi.advanceTimersByTimeAsync(20_000)
    expect(mocks.getTranslation).toHaveBeenCalledTimes(3)

    await wrapper.get('[data-test="translation-continue"]').trigger('click')
    await flushPromises()
    expect(mocks.getTranslation).toHaveBeenCalledTimes(4)
    expect(wrapper.text()).toContain('安全译文')
    wrapper.unmount()
  })

  it('pauses polling after TOTP cancellation and exposes a restart action', async () => {
    vi.useFakeTimers()
    mocks.createTranslation.mockResolvedValue(translationJob())
    mocks.getTranslation.mockRejectedValue(new StepUpCancelledError())

    const wrapper = mountTranslationPanel()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(800)
    await flushPromises()

    expect(wrapper.get('[data-test="translation-poll-error"]').text()).toContain('admin.instructionAudit.translation.pollVerificationCancelled')
    await wrapper.get('[data-test="translation-restart"]').trigger('click')
    expect(wrapper.find('[data-test="translation-poll-error"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.instructionAudit.translation.translate')
    wrapper.unmount()
  })

  it('maps provider error codes to localized messages', async () => {
    mocks.createTranslation.mockResolvedValue(translationJob({ status: 'failed', error_code: 'provider_timeout' }))
    const wrapper = mountTranslationPanel()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('admin.instructionAudit.translation.errors.provider_timeout')
  })

  it('distinguishes initial loading, initial error, stale refresh, empty, and disabled states', async () => {
    const wrapper = mount(InstructionAuditResourceState, {
      props: { loading: true, loaded: false, error: '', hasData: false, emptyDescription: 'nothing here' },
      slots: { default: '<div data-test="resource-content">current data</div>' },
      global: { stubs: { Icon: IconStub } },
    })
    expect(wrapper.find('[data-test="resource-initial-loading"]').exists()).toBe(true)

    await wrapper.setProps({ loading: false, error: 'first request failed' })
    expect(wrapper.get('[data-test="resource-initial-error"]').text()).toContain('first request failed')

    await wrapper.setProps({ loaded: true, hasData: true, error: 'refresh failed' })
    expect(wrapper.get('[data-test="resource-stale-error"]').text()).toContain('refresh failed')
    expect(wrapper.get('[data-test="resource-content"]').text()).toBe('current data')

    await wrapper.setProps({ error: '', hasData: false })
    expect(wrapper.get('[data-test="resource-empty"]').text()).toContain('nothing here')

    await wrapper.setProps({ disabled: true })
    expect(wrapper.find('[data-test="resource-disabled"]').exists()).toBe(true)
  })

  it('keeps stale policy and runtime data visible after refresh errors', () => {
    const policyWrapper = mount(InstructionAuditReasonPolicies, {
      props: {
        policies: [{ reason: 'hash_mismatch', action: 'block', ai_review_enabled: false, alert_enabled: true, config_version: 4, updated_at: '2026-08-01T00:00:00Z' }],
        loading: false,
        loaded: true,
        error: 'policy refresh failed',
        savingReason: '',
        configVersion: 7,
        auditEnabled: false,
      },
      global: { stubs: { Toggle: ToggleStub, ConfirmDialog: ConfirmStub, Icon: IconStub } },
    })
    expect(policyWrapper.get('[data-test="resource-stale-error"]').text()).toContain('policy refresh failed')
    expect(policyWrapper.find('[data-test="resource-disabled"]').exists()).toBe(true)
    expect(policyWrapper.text()).toContain('admin.instructionAudit.reasons.hash_mismatch')

    const runtimeWrapper = mount(InstructionAuditRuntimeConfig, {
      props: { config: runtimeConfig, overview: null, loading: false, saving: false, error: 'runtime refresh failed' },
      global: { stubs: { Toggle: ToggleStub, Icon: IconStub } },
    })
    expect(runtimeWrapper.get('[data-test="resource-stale-error"]').text()).toContain('runtime refresh failed')
    expect(runtimeWrapper.find('form').exists()).toBe(true)
  })

  it('uses a six-column fluid TOTP grid with an accessible name for every digit', () => {
    const visible = ref(true)
    const wrapper = mount(TotpStepUpDialog, {
      props: {
        controller: {
          visible,
          blockedReason: ref(''),
          prompt: vi.fn(),
          onVerified: vi.fn(),
          onCancel: vi.fn(),
          run: vi.fn(),
        },
      },
    })

    const grid = wrapper.get('[data-test="totp-code-grid"]')
    expect(grid.classes()).toContain('grid-cols-6')
    const inputs = grid.findAll('input')
    expect(inputs).toHaveLength(6)
    inputs.forEach((input, index) => {
      expect(input.attributes('aria-label')).toBe(`digit ${index + 1}`)
      expect(input.classes()).toContain('w-full')
      expect(input.classes()).toContain('min-w-0')
      expect(input.classes()).not.toContain('w-10')
    })
  })
})
