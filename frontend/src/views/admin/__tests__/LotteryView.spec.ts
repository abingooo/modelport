import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LotteryView from '../LotteryView.vue'

const lotteryAdmin = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), update: vi.fn(), setStatus: vi.fn(), delete: vi.fn(), entries: vi.fn(), draw: vi.fn() }))
const getAll = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
vi.mock('@/api/lottery', () => ({ default: { admin: lotteryAdmin } }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAll } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }),
}))

const EditorStub = defineComponent({
  name: 'LotteryCampaignEditorDialog', props: ['show', 'subscriptionGroups'],
  template: '<div v-if="show" data-testid="editor" :data-group-count="subscriptionGroups.length" />',
})

function mountView() {
  return mount(LotteryView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    TablePageLayout: { template: '<div><slot name="filters"/><slot name="table"/><slot name="pagination"/></div>' },
    Icon: true, LoadingSpinner: true, EmptyState: { template: '<div data-testid="empty"><slot /></div>' }, Pagination: true,
    LotteryCampaignEditorDialog: EditorStub, LotteryEntriesDialog: true, ConfirmDialog: true,
  } } })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((next) => { resolve = next })
  return { promise, resolve }
}

describe('Admin LotteryView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    lotteryAdmin.list.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    getAll.mockResolvedValue([
      { id: 1, name: 'Monthly', subscription_type: 'subscription' },
      { id: 2, name: 'Standard', subscription_type: 'standard' },
    ])
  })

  it('passes only subscription groups to the prize editor', async () => {
    const wrapper = mountView()
    await flushPromises()
    const create = wrapper.findAll('button').find((item) => item.text().includes('lottery.admin.create'))!
    await create.trigger('click')
    expect(wrapper.get('[data-testid="editor"]').attributes('data-group-count')).toBe('1')
  })

  it('only offers manual completion for instant campaigns', async () => {
    const baseCampaign = {
      status: 'active', state: 'active', starts_at: '2026-01-01T00:00:00Z',
      ends_at: '2026-01-02T00:00:00Z', per_user_limit: 1, minimum_balance: 0,
      required_subscription_group_ids: [], full_draw_participant_limit: null,
      participant_count: 0, entry_count: 0, prizes: [],
    }
    lotteryAdmin.list.mockResolvedValue({
      items: [
        { ...baseCampaign, id: 1, name: 'Instant', mode: 'instant', draw_at: null },
        { ...baseCampaign, id: 2, name: 'Scheduled', mode: 'scheduled', draw_at: '2026-01-02T00:00:00Z' },
      ],
      total: 2, page: 1, page_size: 20, pages: 1,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('button[title="lottery.admin.complete"]')).toHaveLength(1)
  })

  it('allows recovery draw but not pausing after capacity is reached', async () => {
    lotteryAdmin.list.mockResolvedValue({
      items: [{
        id: 3, name: 'Capacity draw', mode: 'scheduled', status: 'active', state: 'awaiting_draw',
        starts_at: '2098-01-01T00:00:00Z', ends_at: '2099-01-01T00:00:00Z', draw_at: '2099-01-02T00:00:00Z',
        full_draw_participant_limit: 10, full_draw_reached_at: '2026-08-06T00:00:00Z', participant_count: 10,
        per_user_limit: 1, minimum_balance: 0, required_subscription_group_ids: [], entry_count: 10, prizes: [],
      }],
      total: 1, page: 1, page_size: 20, pages: 1,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('button').some((button) => button.text().includes('lottery.admin.draw'))).toBe(true)
    expect(wrapper.findAll('button').some((button) => button.text().includes('lottery.admin.pause'))).toBe(false)
    expect(wrapper.findAll('button').some((button) => button.text().includes('lottery.admin.activate'))).toBe(false)
  })

  it('ignores an older list response after a newer admin refresh finishes', async () => {
    const response = (name: string) => ({
      items: [{
        id: name === 'Latest campaign' ? 9 : 8, name, mode: 'instant', status: 'active', state: 'active',
        starts_at: '2026-01-01T00:00:00Z', ends_at: '2026-01-02T00:00:00Z', draw_at: null,
        per_user_limit: 1, minimum_balance: 0, required_subscription_group_ids: [],
        full_draw_participant_limit: null, participant_count: 0, entry_count: 0, prizes: [],
      }],
      total: 1, page: 1, page_size: 20, pages: 1,
    })
    const stale = deferred<ReturnType<typeof response>>()
    const latest = deferred<ReturnType<typeof response>>()
    lotteryAdmin.list.mockReset()
      .mockReturnValueOnce(stale.promise)
      .mockReturnValueOnce(latest.promise)

    const wrapper = mountView()
    await flushPromises()
    ;(wrapper.vm as unknown as { loadCampaigns: () => Promise<void> }).loadCampaigns()

    latest.resolve(response('Latest campaign'))
    await flushPromises()
    stale.resolve(response('Stale campaign'))
    await flushPromises()

    expect(wrapper.text()).toContain('Latest campaign')
    expect(wrapper.text()).not.toContain('Stale campaign')
  })
})
