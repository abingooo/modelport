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
import InstructionSensitiveAccessPanel from '../components/InstructionSensitiveAccessPanel.vue'
import InstructionEvidenceReviewDialog from '../InstructionEvidenceReviewDialog.vue'

const mocks = vi.hoisted(() => ({
  createTranslation: vi.fn(), getTranslation: vi.fn(), getHash: vi.fn(), changeHashStatus: vi.fn(), changeHashScope: vi.fn(),
  revealHashRaw: vi.fn(), recordHashRawCopy: vi.fn(), revealEvidence: vi.fn(), recordEvidenceCopy: vi.fn(), listEventAIReviews: vi.fn(),
  listSensitiveAccessGrants: vi.fn(), grantSensitiveAccess: vi.fn(), revokeSensitiveAccess: vi.fn(),
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
        loading: false, loaded: true, error: '', savingReason: '', configVersion: 9, auditEnabled: true,
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
        loading: false, loaded: true, error: '', savingReason: '', configVersion: 9, auditEnabled: true,
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

  it('locks sensitive actions without a grant and does not send protected requests', async () => {
    const panel = mount(InstructionSensitiveAccessPanel, {
      props: {
        capability: { user_id: 7, has_access: false, can_manage: false },
        capabilityLoading: false,
        capabilityError: '',
      },
      global: { stubs: { Icon: IconStub, TotpStepUpDialog: StepUpStub, BaseDialog: BaseDialogStub } },
    })
    const loadButton = panel.get('[data-test="load-sensitive-grants"]')
    expect(loadButton.attributes('disabled')).toBeDefined()
    await loadButton.trigger('click')
    expect(mocks.listSensitiveAccessGrants).not.toHaveBeenCalled()

    mocks.getHash.mockResolvedValue({
      id: 22, digest: 'a'.repeat(64), name: 'locked hash', note: '', observed_source: 'instructions',
      client_name: '', client_version: '', status: 'active', hash_algorithm: 'sha256', normalization_version: 'identity_utf8_v1',
      field_name: 'instructions', raw_content_status: 'stored', content_bytes: 12, sources: [], scopes: [],
      created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z',
    })
    const hashDialog = mount(InstructionHashDetailDialog, {
      props: { show: true, hashId: 22, translationEnabled: true, externalTranslationEnabled: false, sensitiveAccess: false },
      global: { stubs: { BaseDialog: BaseDialogStub, ConfirmDialog: ConfirmStub, TotpStepUpDialog: StepUpStub, Icon: IconStub } },
    })
    await flushPromises()
    const revealButton = hashDialog.findAll('button').find((button) => button.text().includes('admin.instructionAudit.sensitiveAccess.lockedAction'))
    expect(revealButton?.attributes('disabled')).toBeDefined()
    await revealButton?.trigger('click')
    expect(mocks.revealHashRaw).not.toHaveBeenCalled()

    const evidenceDialog = mount(InstructionEvidenceReviewDialog, {
      props: {
        show: true,
        event: {
          id: 9, request_id: 'req-9', user_email: 'user@example.com', group_name: 'OpenAI', client_type: 'codex_cli', client_user_agent: '', model: 'gpt-5', endpoint: '/v1/responses', stage: 'request',
          instructions: { present: true, sha256: 'b'.repeat(64), result: 'mismatch' }, input1: { present: false, sha256: '', result: 'missing' },
          decision: 'blocked', reason: 'hash_mismatch', initial_reason: 'hash_mismatch', final_reason: 'hash_mismatch', final_outcome: 'blocked', policy_action: 'block', rule_set_ids: [], config_version: 1,
          latency_ms: 1, evidence_status: 'stored', user_notification_status: 'sent', ops_notification_status: 'sent', created_at: '2026-08-01T00:00:00Z',
        },
        translationEnabled: true,
        externalTranslationEnabled: false,
        sensitiveAccess: false,
      },
      global: { stubs: { BaseDialog: BaseDialogStub, TotpStepUpDialog: StepUpStub, Icon: IconStub } },
    })
    await flushPromises()
    expect(evidenceDialog.text()).toContain('admin.instructionAudit.sensitiveAccess.lockedHint')
    expect(mocks.revealEvidence).not.toHaveBeenCalled()
  })

  it('clears a translation and reports when the server revokes sensitive access', async () => {
    mocks.createTranslation.mockRejectedValue({ reason: 'INSTRUCTION_SENSITIVE_ACCESS_REQUIRED' })
    const wrapper = mount(InstructionTranslationPanel, {
      props: {
        resourceType: 'event', resourceId: 7, fieldName: 'instructions', original: 'sensitive original',
        enabled: true, externalEnabled: false, sensitiveAccess: true,
      },
      global: { stubs: { Icon: IconStub, TotpStepUpDialog: StepUpStub } },
    })
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('access-denied')).toHaveLength(1)

    await wrapper.setProps({ sensitiveAccess: false })
    await flushPromises()
    expect(wrapper.text()).not.toContain('sensitive original')
    expect(wrapper.text()).toContain('admin.instructionAudit.sensitiveAccess.lockedHint')
  })

  it('removes already revealed hash plaintext as soon as the capability is revoked', async () => {
    mocks.getHash.mockResolvedValue({
      id: 31, digest: 'c'.repeat(64), name: 'sensitive hash', note: '', observed_source: 'instructions',
      client_name: '', client_version: '', status: 'active', hash_algorithm: 'sha256', normalization_version: 'identity_utf8_v1',
      field_name: 'instructions', raw_content_status: 'stored', content_bytes: 18, sources: [], scopes: [],
      created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z',
    })
    mocks.revealHashRaw.mockResolvedValue({
      hash_id: 31, field_name: 'instructions', raw_content_status: 'stored', raw_content: 'plaintext-to-remove',
      content_bytes: 18, sha256: 'c'.repeat(64), recomputed_sha256: 'c'.repeat(64), digest_consistent: true,
    })
    const wrapper = mount(InstructionHashDetailDialog, {
      props: { show: true, hashId: 31, translationEnabled: false, externalTranslationEnabled: false, sensitiveAccess: true },
      global: { stubs: { BaseDialog: BaseDialogStub, ConfirmDialog: ConfirmStub, TotpStepUpDialog: StepUpStub, Icon: IconStub } },
    })
    await flushPromises()
    const revealButton = wrapper.findAll('button').find((button) => button.text().includes('admin.instructionAudit.hashDetail.revealRaw'))
    await revealButton?.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('plaintext-to-remove')

    await wrapper.setProps({ sensitiveAccess: false })
    await flushPromises()
    expect(wrapper.text()).not.toContain('plaintext-to-remove')
  })

  it('loads grants only on demand and clears the current administrator after revocation', async () => {
    const currentGrant = {
      id: 3, user_id: 7, email: 'admin@example.com', username: 'admin', user_status: 'active',
      totp_enabled: true, effective: true, grant_source: 'manual', grant_reason: 'security duty',
      granted_at: '2026-08-01T00:00:00Z',
    }
    mocks.listSensitiveAccessGrants.mockResolvedValue([currentGrant])
    mocks.revokeSensitiveAccess.mockResolvedValue({ ...currentGrant, effective: false })
    const wrapper = mount(InstructionSensitiveAccessPanel, {
      props: {
        capability: { user_id: 7, has_access: true, can_manage: true, grant_id: 3 },
        capabilityLoading: false,
        capabilityError: '',
      },
      global: { stubs: { Icon: IconStub, TotpStepUpDialog: StepUpStub, BaseDialog: BaseDialogStub } },
    })
    expect(mocks.listSensitiveAccessGrants).not.toHaveBeenCalled()
    await wrapper.get('[data-test="load-sensitive-grants"]').trigger('click')
    await flushPromises()
    expect(mocks.listSensitiveAccessGrants).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('admin@example.com')

    const revokeButton = wrapper.findAll('button').find((button) => button.text().includes('admin.instructionAudit.sensitiveAccess.revoke'))
    await revokeButton?.trigger('click')
    await wrapper.get('[data-test="confirm-sensitive-revoke"]').trigger('click')
    await flushPromises()
    expect(mocks.revokeSensitiveAccess).toHaveBeenCalledWith(7, '')
    expect(wrapper.emitted('access-denied')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('admin@example.com')
  })

  it('promotes only the selected temporary AI scope', async () => {
    const temporaryHash = {
      id: 22, digest: 'a'.repeat(64), name: 'AI temporary rule', note: '', observed_source: 'instructions',
      client_name: '', client_version: '', status: 'active', hash_algorithm: 'sha256', normalization_version: 'identity_utf8_v1',
      field_name: 'instructions', raw_content_status: 'stored', content_bytes: 12, sources: [],
      scopes: [{
        rule_set_id: 30, rule_set_name: 'AI temporary scope', rule_set_enabled: true, system_managed: true,
        source_type: 'ai_review', status: 'active', valid_until: '2099-08-02T00:00:00Z', binding_id: 31, group_id: 7,
        group_name: 'OpenAI', client_types: ['codex_cli'], binding_enabled: true, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z',
      }],
      valid_from: '2026-08-01T00:00:00Z', valid_until: null,
      created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z',
    }
    mocks.getHash.mockResolvedValue(temporaryHash)
    mocks.changeHashScope.mockResolvedValue({
      ...temporaryHash,
      scopes: temporaryHash.scopes.map((scope) => ({ ...scope, source_type: 'manual', valid_until: null })),
    })

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

    expect(wrapper.text()).toContain('AI temporary scope')
    expect(wrapper.text()).toContain('admin.instructionAudit.hashDetail.temporaryScope')

    const promote = wrapper.findAll('button').find((button) => button.text().includes('admin.instructionAudit.hashDetail.promote'))
    expect(promote).toBeDefined()
    await promote!.trigger('click')
    await flushPromises()
    expect(mocks.changeHashScope).toHaveBeenCalledWith(22, 30, 'promote')
    expect(mocks.changeHashStatus).not.toHaveBeenCalled()
  })

  it('keeps page overflow bounded and splits major workspaces into components', () => {
    const here = dirname(fileURLToPath(import.meta.url))
    const view = readFileSync(resolve(here, '../InstructionAuditView.vue'), 'utf8')
    expect(view).toContain('w-full min-w-0 max-w-none')
    expect(view).not.toContain('min-w-[960px]')
    expect(view).toContain('<InstructionV2EventsPanel')
    expect(view).toContain('<InstructionV2TrustedPanel')
    expect(view).toContain('<InstructionV2ScopePanel')
    expect(view).toContain('<InstructionV2AISettingsPanel')
    expect(view).toContain('grid-cols-2 gap-2')
  })
})
