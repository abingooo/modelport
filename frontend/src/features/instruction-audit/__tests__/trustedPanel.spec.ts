import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import InstructionV2TrustedPanel from '../components/InstructionV2TrustedPanel.vue'
import type { InstructionHash } from '../v2Types'

const mocks = vi.hoisted(() => ({
  listHashes: vi.fn(),
  revealHashRaw: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('../v2Api', () => ({
  default: {
    listHashes: mocks.listHashes,
    revealHashRaw: mocks.revealHashRaw,
  },
}))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: mocks.showError, showSuccess: vi.fn() }),
}))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const DialogStub = defineComponent({
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})
const EmptyStub = { template: '<span />' }

function hash(id: number, rawStorage: InstructionHash['raw_storage'], overrides: Partial<InstructionHash> = {}): InstructionHash {
  return {
    id,
    sha256: String(id).repeat(64),
    name: `hash-${id}`,
    note: '',
    status: 'active',
    source: rawStorage === 'unavailable' ? 'import' : 'manual',
    observed_field: 'instructions',
    hash_algorithm: 'sha256',
    normalization_version: 'identity_utf8_v1',
    content_bytes: rawStorage === 'unavailable' ? 0 : 12,
    raw_storage: rawStorage,
    stored_bytes: rawStorage === 'unavailable' ? 0 : 28,
    ai_sampled: false,
    source_user_email: '',
    reviewer_model: '',
    prompt_version: '',
    review_reason: '',
    review_category: '',
    global_trust: true,
    content_vault_id: rawStorage === 'unavailable' ? null : 91,
    scope_ids: [],
    scopes: [],
    created_at: '2026-08-10T09:30:00Z',
    updated_at: '2026-08-10T09:30:00Z',
    ...overrides,
  }
}

describe('InstructionV2TrustedPanel plaintext availability', () => {
  beforeEach(() => {
    Object.values(mocks).forEach(mock => mock.mockReset())
    mocks.listHashes.mockResolvedValue({
      items: [hash(1, 'full'), hash(2, 'unavailable')],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    mocks.revealHashRaw.mockResolvedValue({ fields: [] })
  })

  it('reveals vault-backed plaintext and keeps digest-only imports disabled', async () => {
    const wrapper = mount(InstructionV2TrustedPanel, {
      props: { scopes: [], refreshKey: 0 },
      global: {
        stubs: {
          BaseDialog: DialogStub,
          ConfirmDialog: EmptyStub,
          Pagination: EmptyStub,
          Icon: EmptyStub,
        },
      },
    })
    await flushPromises()

    const cards = wrapper.findAll('article')
    expect(cards).toHaveLength(2)
    const vaultButton = cards[0].get('footer button')
    const digestOnlyButton = cards[1].get('footer button')
    expect(vaultButton.attributes('disabled')).toBeUndefined()
    expect(digestOnlyButton.attributes('disabled')).toBeDefined()

    await vaultButton.trigger('click')
    await flushPromises()
    expect(mocks.revealHashRaw).toHaveBeenCalledOnce()
    expect(mocks.revealHashRaw).toHaveBeenCalledWith(1)
  })

  it('shows request-account snapshots and distinguishes deleted accounts and purged events', async () => {
    mocks.listHashes.mockResolvedValue({
      items: [
        hash(1, 'full', { source: 'ai_review', source_event_id: 801, source_user_id: 42, source_user_email: 'active@example.com' }),
        hash(2, 'full', { source: 'ai_review', source_event_id: 802, source_user_id: null, source_user_email: 'deleted@example.com' }),
        hash(3, 'full', { source: 'ai_review', source_event_id: null, source_user_id: 43, source_user_email: 'retained@example.com' }),
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(InstructionV2TrustedPanel, {
      props: { scopes: [], refreshKey: 0 },
      global: {
        stubs: {
          BaseDialog: DialogStub,
          ConfirmDialog: EmptyStub,
          Pagination: EmptyStub,
          Icon: EmptyStub,
        },
      },
    })
    await flushPromises()

    const cards = wrapper.findAll('article')
    expect(cards[0].get('[data-test="hash-source-account"]').text()).toContain('active@example.com')
    expect(cards[0].get('[data-test="hash-source-account"]').text()).toContain('#42')
    expect(cards[0].get('[data-test="hash-source-event"]').text()).toContain('#801')
    expect(cards[1].get('[data-test="hash-source-account"]').text()).toContain('deleted@example.com')
    expect(cards[1].get('[data-test="hash-source-account"]').text()).toContain('admin.instructionAudit.v2.sourceAccountDeleted')
    expect(cards[2].get('[data-test="hash-source-event"]').text()).toContain('admin.instructionAudit.v2.sourceEventPurged')
  })

  it('distinguishes administrator imports from historical records without an account snapshot', async () => {
    mocks.listHashes.mockResolvedValue({
      items: [
        hash(1, 'unavailable', { source: 'import' }),
        hash(2, 'full', { source: 'ai_review' }),
        hash(3, 'full', { source: 'manual', source_event_id: 803, source_user_id: 44, source_user_email: 'trusted@example.com' }),
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(InstructionV2TrustedPanel, {
      props: { scopes: [], refreshKey: 0 },
      global: {
        stubs: {
          BaseDialog: DialogStub,
          ConfirmDialog: EmptyStub,
          Pagination: EmptyStub,
          Icon: EmptyStub,
        },
      },
    })
    await flushPromises()

    const cards = wrapper.findAll('article')
    expect(cards[0].get('[data-test="hash-source-account"]').text()).toContain('admin.instructionAudit.v2.sourceAccountNotApplicable')
    expect(cards[1].get('[data-test="hash-source-account"]').text()).toContain('admin.instructionAudit.v2.sourceAccountUnknown')
    expect(cards[2].get('[data-test="hash-source-account"]').text()).toContain('trusted@example.com')
    expect(cards[2].get('[data-test="hash-source-event"]').text()).toContain('#803')
  })
})
