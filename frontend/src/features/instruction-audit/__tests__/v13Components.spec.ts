import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import InstructionAuditStatistics from '../components/InstructionAuditStatistics.vue'
import InstructionAuditReasonPolicies from '../components/InstructionAuditReasonPolicies.vue'
import InstructionTranslationPanel from '../components/InstructionTranslationPanel.vue'
import InstructionHashDetailDialog from '../components/InstructionHashDetailDialog.vue'

const mocks = vi.hoisted(() => ({
  createTranslation: vi.fn(), getTranslation: vi.fn(), getHash: vi.fn(), changeHashStatus: vi.fn(),
  showError: vi.fn(), copy: vi.fn(),
}))

vi.mock('../api', () => ({ default: mocks }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: vi.fn() }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: mocks.copy }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`)) }) }
})

const ToggleStub = defineComponent({
  props: ['modelValue', 'disabled'], emits: ['update:modelValue'],
  template: '<button type="button" role="switch" :disabled="disabled" :aria-checked="modelValue" @click="$emit(\'update:modelValue\', !modelValue)" />',
})
const ConfirmStub = defineComponent({
  props: ['show'], emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-test="confirm"><button data-test="confirm-action" @click="$emit(\'confirm\')">confirm</button></div>',
})
const IconStub = { template: '<i />' }
const StepUpStub = { template: '<div />' }
const BaseDialogStub = defineComponent({
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

describe('instruction audit v0.1.170.13 components', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders the five outcomes, total, and block rate as independent statistics', () => {
    const wrapper = mount(InstructionAuditStatistics, {
      props: {
        statistics: { blocked: 2, policy_allow: 3, ai_pass: 4, hash_pass: 5, exception_pass: 6, total: 20, block_rate: 0.1 },
        loading: false, error: '',
      },
    })
    expect(wrapper.findAll('dd')).toHaveLength(7)
    expect(wrapper.text()).toContain('10.00%')
    for (const value of ['2', '3', '4', '5', '6', '20']) expect(wrapper.text()).toContain(value)
  })

  it('requires explicit confirmation before saving a high-risk allow policy', async () => {
    const wrapper = mount(InstructionAuditReasonPolicies, {
      props: {
        policies: [{ reason: 'hash_mismatch', action: 'block', ai_review_enabled: false, alert_enabled: true, config_version: 4, updated_at: '2026-08-01T00:00:00Z' }],
        loading: false, error: '', savingReason: '', configVersion: 9,
      },
      global: { stubs: { Toggle: ToggleStub, ConfirmDialog: ConfirmStub, Icon: IconStub } },
    })
    await wrapper.get('select').setValue('allow_and_record')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(wrapper.emitted('save')).toBeUndefined()
    await wrapper.get('[data-test="confirm-action"]').trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual(['hash_mismatch', expect.objectContaining({ action: 'allow_and_record', expected_config_version: 9, confirmed: true })])
  })

  it('requires explicit confirmation for every reason that can allow and record', async () => {
    const wrapper = mount(InstructionAuditReasonPolicies, {
      props: {
        policies: [{ reason: 'fields_missing', action: 'block', ai_review_enabled: false, alert_enabled: true, config_version: 4, updated_at: '2026-08-01T00:00:00Z' }],
        loading: false, error: '', savingReason: '', configVersion: 9,
      },
      global: { stubs: { Toggle: ToggleStub, ConfirmDialog: ConfirmStub, Icon: IconStub } },
    })
    await wrapper.get('select').setValue('allow_and_record')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(wrapper.emitted('save')).toBeUndefined()
    await wrapper.get('[data-test="confirm-action"]').trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual(['fields_missing', expect.objectContaining({ confirmed: true })])
  })

  it('polls an on-demand translation and renders the short-lived result beside the original', async () => {
    vi.useFakeTimers()
    mocks.createTranslation.mockResolvedValue({
      id: 15, resource_type: 'event', resource_id: 7, field_name: 'instructions', target_language: 'zh-CN', provider: 'internal',
      status: 'pending', error_code: '', chunk_count: 2, completed_chunks: 0, attempts: 0, max_attempts: 3,
      result_bytes: 0, redaction_count: 1, provider_latency_ms: 0, expires_at: '2026-08-01T01:00:00Z', created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z',
    })
    mocks.getTranslation
      .mockResolvedValueOnce({
        id: 15, resource_type: 'event', resource_id: 7, field_name: 'instructions', target_language: 'zh-CN', provider: 'internal',
        status: 'retry', error_code: 'provider_timeout', chunk_count: 2, completed_chunks: 0, attempts: 1, max_attempts: 3,
        result_bytes: 0, redaction_count: 1, provider_latency_ms: 5, expires_at: '2026-08-01T01:00:00Z', created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:01Z',
      })
      .mockResolvedValueOnce({
      id: 15, resource_type: 'event', resource_id: 7, field_name: 'instructions', target_language: 'zh-CN', provider: 'internal',
      status: 'succeeded', error_code: '', chunk_count: 2, completed_chunks: 2, attempts: 1, max_attempts: 3,
      result_bytes: 12, redaction_count: 1, provider_latency_ms: 5, translated_text: '安全译文', expires_at: '2026-08-01T01:00:00Z', created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:01Z',
      })
    const wrapper = mount(InstructionTranslationPanel, {
      props: { resourceType: 'event', resourceId: 7, fieldName: 'instructions', original: 'untrusted content', enabled: true, externalEnabled: false },
      global: { stubs: { Icon: IconStub, TotpStepUpDialog: StepUpStub } },
    })
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.createTranslation).toHaveBeenCalledWith(expect.objectContaining({ resource_id: 7, provider: 'internal' }))
    await vi.advanceTimersByTimeAsync(800)
    await flushPromises()
    expect(mocks.getTranslation).toHaveBeenCalledWith(15)
    expect(wrapper.text()).toContain('admin.instructionAudit.translation.processing')
    await vi.advanceTimersByTimeAsync(800)
    await flushPromises()
    expect(mocks.getTranslation).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('安全译文')
    expect(wrapper.text()).toContain('untrusted content')
  })

  it('offers promotion for an active temporary AI hash and clears its expiry through the status API', async () => {
    const temporaryHash = {
      id: 22, digest: 'a'.repeat(64), name: 'AI temporary rule', note: '', observed_source: 'instructions',
      client_name: '', client_version: '', status: 'active', hash_algorithm: 'sha256', normalization_version: 'identity_utf8_v1',
      field_name: 'instructions', raw_content_status: 'stored', content_bytes: 12, sources: [],
      valid_from: '2026-08-01T00:00:00Z', valid_until: '2026-08-02T00:00:00Z',
      created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z',
    }
    mocks.getHash.mockResolvedValue(temporaryHash)
    mocks.changeHashStatus.mockResolvedValue({ ...temporaryHash, valid_until: null })

    const wrapper = mount(InstructionHashDetailDialog, {
      props: { show: true, hashId: 22, translationEnabled: false, externalTranslationEnabled: false },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          ConfirmDialog: ConfirmStub,
          TotpStepUpDialog: StepUpStub,
          Icon: IconStub,
        },
      },
    })
    await flushPromises()

    const promote = wrapper.findAll('button').find((button) => button.text().includes('admin.instructionAudit.hashDetail.promote'))
    expect(promote).toBeDefined()
    await promote!.trigger('click')
    await flushPromises()
    expect(mocks.changeHashStatus).toHaveBeenCalledWith(22, 'active')
  })

  it('keeps page overflow bounded and splits major workspaces into components', () => {
    const here = dirname(fileURLToPath(import.meta.url))
    const view = readFileSync(resolve(here, '../InstructionAuditView.vue'), 'utf8')
    expect(view).toContain('w-full min-w-0 max-w-none')
    expect(view).not.toContain('min-w-[960px]')
    expect(view).toContain('<InstructionAuditStatistics')
    expect(view).toContain('<InstructionAuditRuntimeConfig')
    expect(view).toContain('<InstructionAuditReasonPolicies')
    expect(view).toContain('<InstructionHashDetailDialog')
    expect(view).toContain('table-fixed')
  })
})
