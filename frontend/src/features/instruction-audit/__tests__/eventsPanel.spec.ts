import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import InstructionV2EventsPanel from '../components/InstructionV2EventsPanel.vue'
import type { InstructionEvent, InstructionEventPage } from '../v2Types'

const mocks = vi.hoisted(() => ({
  listEvents: vi.fn(),
  deleteEvent: vi.fn(),
  deleteEvents: vi.fn(),
  trustEvent: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('../v2Api', () => ({
  default: {
    listEvents: mocks.listEvents,
    deleteEvent: mocks.deleteEvent,
    deleteEvents: mocks.deleteEvents,
    trustEvent: mocks.trustEvent,
  },
}))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }),
}))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return { ...actual, useRoute: () => ({ query: {} }) }
})
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params?.count == null ? key : `${key}(${params.count})`,
    }),
  }
})

const ConfirmStub = defineComponent({
  props: ['show'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-test="confirm-dialog"><button data-test="confirm-action" @click="$emit(\'confirm\')">confirm</button></div>',
})
const EmptyStub = { template: '<span />' }
const RouterLinkStub = defineComponent({
  props: ['to'],
  template: '<a><slot /></a>',
})
const PaginationStub = defineComponent({
  emits: ['update:page', 'update:page-size'],
  template: '<button data-test="next-page" @click="$emit(\'update:page\', 2)">next</button>',
})

function auditEvent(id: number): InstructionEvent {
  const field = { state: 'valid' as const, sha256: String(id).repeat(64), bytes: 12, partial: false, ai_sampled: false }
  return {
    id,
    request_id: `request-${id}`,
    user_id: id,
    user_email: `user-${id}@example.test`,
    api_key_id: id,
    api_key_name: `key-${id}`,
    group_id: id,
    group_name: `group-${id}`,
    scope_id: id,
    client_profile_id: id,
    client_key: 'codex_cli',
    client_name: 'Codex CLI',
    client_user_agent: 'codex-cli/test',
    model: 'gpt-test',
    endpoint: '/v1/responses',
    stage: 'http',
    mode: 'observe',
    decision: 'allow',
    outcome: 'observe_allow',
    reason: '',
    instructions: field,
    input1: field,
    matched_hash_id: null,
    ai_result: 'not_run',
    ai_reviewed_field: '',
    ai_sampled: false,
    audit_latency_ms: 5,
    ai_latency_ms: 0,
    body_bytes: 12,
    config_version: 1,
    evidence_status: 'stored',
    user_notification_status: '',
    ops_notification_status: '',
    created_at: '2026-08-11T00:00:00Z',
    selected_field: 'instructions',
    selected_sha256: field.sha256,
    review_job_id: null,
  }
}

function eventPage(items: InstructionEvent[]): InstructionEventPage {
  return { items, total: items.length, page: 1, page_size: 20, pages: items.length ? 1 : 0 }
}

function mountPanel() {
  return mount(InstructionV2EventsPanel, {
    props: { groups: [], clients: [], refreshKey: 0 },
    global: {
      stubs: {
        ConfirmDialog: ConfirmStub,
        Icon: EmptyStub,
        InstructionV2EvidenceDialog: EmptyStub,
        Pagination: PaginationStub,
        RouterLink: RouterLinkStub,
      },
    },
  })
}

describe('InstructionV2EventsPanel current-page selection', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.listEvents.mockResolvedValue(eventPage([auditEvent(1), auditEvent(2)]))
    mocks.deleteEvents.mockResolvedValue(2)
  })

  it('selects and clears every event on the current page', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    const selectionButton = wrapper.get('[data-test="select-current-page"]')
    const deleteButton = wrapper.get('[data-test="delete-selected"]')
    expect(selectionButton.attributes('aria-pressed')).toBe('false')
    expect(selectionButton.text()).toContain('admin.instructionAudit.v2.selectCurrentPage')
    expect(deleteButton.attributes('disabled')).toBeDefined()
    expect(deleteButton.classes()).toContain('btn-secondary')

    await selectionButton.trigger('click')

    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-1"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-2"]').element.checked).toBe(true)
    expect(selectionButton.attributes('aria-pressed')).toBe('true')
    expect(selectionButton.text()).toContain('admin.instructionAudit.v2.clearCurrentPageSelection')
    expect(deleteButton.attributes('disabled')).toBeUndefined()
    expect(deleteButton.classes()).toContain('btn-danger')
    expect(deleteButton.text()).toContain('(2)')

    await selectionButton.trigger('click')

    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-1"]').element.checked).toBe(false)
    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-2"]').element.checked).toBe(false)
    expect(deleteButton.attributes('disabled')).toBeDefined()
  })

  it('clears the old selection before applying a new filter', async () => {
    mocks.listEvents
      .mockResolvedValueOnce(eventPage([auditEvent(1), auditEvent(2)]))
      .mockResolvedValueOnce(eventPage([auditEvent(3)]))
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[data-test="select-current-page"]').trigger('click')
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeUndefined()

    const search = wrapper.get('input[type="search"]')
    await search.setValue('request-3')
    await search.trigger('keyup.enter')
    await flushPromises()

    expect(mocks.listEvents).toHaveBeenLastCalledWith(expect.objectContaining({ q: 'request-3', page: 1, page_size: 20 }))
    expect(wrapper.find('[data-test="select-event-1"]').exists()).toBe(false)
    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-3"]').element.checked).toBe(false)
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="select-current-page"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-3"]').element.checked).toBe(true)
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeUndefined()
  })

  it('clears the current-page selection before loading another page', async () => {
    let resolveNextPage!: (page: InstructionEventPage) => void
    mocks.listEvents
      .mockResolvedValueOnce({ ...eventPage([auditEvent(1), auditEvent(2)]), total: 3, pages: 2 })
      .mockImplementationOnce(() => new Promise((resolve) => { resolveNextPage = resolve }))
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[data-test="select-current-page"]').trigger('click')
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-test="next-page"]').trigger('click')
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="select-event-1"]').attributes('disabled')).toBeDefined()
    resolveNextPage({ ...eventPage([auditEvent(3)]), total: 3, page: 2, pages: 2 })
    await flushPromises()

    expect(mocks.listEvents).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2, page_size: 20 }))
    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-3"]').element.checked).toBe(false)
  })

  it('ignores a stale filter response that arrives after the latest request', async () => {
    let resolveOld!: (page: InstructionEventPage) => void
    let resolveLatest!: (page: InstructionEventPage) => void
    mocks.listEvents
      .mockResolvedValueOnce(eventPage([auditEvent(1)]))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveLatest = resolve }))
    const wrapper = mountPanel()
    await flushPromises()

    const search = wrapper.get('input[type="search"]')
    await search.setValue('old-filter')
    await search.trigger('keyup.enter')
    await search.setValue('latest-filter')
    await search.trigger('keyup.enter')

    resolveLatest(eventPage([auditEvent(3)]))
    await flushPromises()
    resolveOld(eventPage([auditEvent(2)]))
    await flushPromises()

    expect(wrapper.find('[data-test="select-event-2"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="select-event-3"]').exists()).toBe(true)
    await wrapper.get('[data-test="select-current-page"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('[data-test="select-event-3"]').element.checked).toBe(true)
  })

  it('clears stale results and selection when a filtered request fails', async () => {
    mocks.listEvents
      .mockResolvedValueOnce(eventPage([auditEvent(1), auditEvent(2)]))
      .mockRejectedValueOnce(new Error('filter request failed'))
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[data-test="select-current-page"]').trigger('click')
    const search = wrapper.get('input[type="search"]')
    await search.setValue('new-filter')
    await search.trigger('keyup.enter')
    await flushPromises()

    expect(wrapper.find('[data-test="select-event-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="select-event-2"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="select-current-page"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  })

  it('clears stale results when loading another page fails', async () => {
    mocks.listEvents
      .mockResolvedValueOnce({ ...eventPage([auditEvent(1), auditEvent(2)]), total: 21, pages: 2 })
      .mockRejectedValueOnce(new Error('page request failed'))
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()

    expect(mocks.listEvents).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2, page_size: 20 }))
    expect(wrapper.find('[data-test="select-event-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="select-event-2"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="select-current-page"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeDefined()
  })

  it('deletes all selected events after confirmation', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[data-test="select-current-page"]').trigger('click')
    await wrapper.get('[data-test="delete-selected"]').trigger('click')
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(true)

    await wrapper.get('[data-test="confirm-action"]').trigger('click')
    await flushPromises()

    expect(mocks.deleteEvents).toHaveBeenCalledOnce()
    expect(mocks.deleteEvents).toHaveBeenCalledWith([1, 2])
    expect(wrapper.get('[data-test="delete-selected"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
  })

  it('moves back to the last valid page after deleting the final page', async () => {
    mocks.listEvents
      .mockResolvedValueOnce({ ...eventPage([auditEvent(1)]), total: 21, pages: 2 })
      .mockResolvedValueOnce({ ...eventPage([auditEvent(21)]), total: 21, page: 2, pages: 2 })
      .mockResolvedValueOnce({ ...eventPage([auditEvent(1)]), total: 20, page: 1, pages: 1 })
    mocks.deleteEvents.mockResolvedValueOnce(1)
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="select-event-21"]').exists()).toBe(true)

    await wrapper.get('[data-test="select-current-page"]').trigger('click')
    await wrapper.get('[data-test="delete-selected"]').trigger('click')
    await wrapper.get('[data-test="confirm-action"]').trigger('click')
    await flushPromises()

    expect(mocks.deleteEvents).toHaveBeenCalledWith([21])
    expect(mocks.listEvents).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 20 }))
    expect(wrapper.find('[data-test="select-event-21"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="select-event-1"]').exists()).toBe(true)
  })
})
