import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LotteryView from '../LotteryView.vue'
import type { LotteryCampaign } from '@/api/lottery'

const api = vi.hoisted(() => ({ list: vi.fn(), history: vi.fn(), participate: vi.fn() }))
const stores = vi.hoisted(() => ({ showError: vi.fn(), refreshUser: vi.fn() }))
vi.mock('@/api/lottery', () => ({ default: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: stores.showError }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ refreshUser: stores.refreshUser }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

const campaign: LotteryCampaign = {
  id: 7, name: 'Harbor Launch', description: 'Launch campaign', mode: 'instant', status: 'active', state: 'active',
  starts_at: '2026-07-01T00:00:00Z', ends_at: '2026-08-01T00:00:00Z', per_user_limit: 2,
  minimum_balance: 0, required_subscription_group_ids: [], eligible: true, user_entry_count: 0, entry_count: 3,
  created_at: '2026-07-01T00:00:00Z', updated_at: '2026-07-01T00:00:00Z',
  prizes: [{ id: 1, campaign_id: 7, name: 'Port credit', prize_type: 'balance', balance_amount: 10,
    subscription_validity_days: 0, probability_bps: 2500, inventory: 5, awarded_count: 1, is_enabled: true,
    sort_order: 0, created_at: '2026-07-01T00:00:00Z', updated_at: '2026-07-01T00:00:00Z' }],
}

function response(items: unknown[]) { return { items, total: items.length, page: 1, page_size: 20, pages: 1 } }
function mountView() {
  return mount(LotteryView, {
    global: { stubs: {
      AppLayout: { template: '<main><slot /></main>' }, Icon: true, LoadingSpinner: true,
      EmptyState: { template: '<div data-testid="empty" />' }, Pagination: true,
      LotteryResultDialog: { props: ['show', 'entry'], template: '<div data-testid="result" :data-show="show" :data-status="entry?.status" />' },
    } },
  })
}

describe('LotteryView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.list.mockResolvedValue(response([campaign]))
    api.history.mockResolvedValue(response([]))
  })

  it('prevents duplicate submissions and displays the resolved result', async () => {
    let resolveEntry!: (value: unknown) => void
    api.participate.mockReturnValue(new Promise((resolve) => { resolveEntry = resolve }))
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((item) => item.text().includes('lottery.participate'))!
    await button.trigger('click')
    await button.trigger('click')
    expect(api.participate).toHaveBeenCalledTimes(1)
    expect(api.participate.mock.calls[0][0]).toBe(7)
    expect(api.participate.mock.calls[0][1]).toMatch(/^lottery-7-/)

    resolveEntry({ id: 22, campaign_id: 7, user_id: 1, status: 'won', prize_type: 'balance', balance_amount: 10,
      subscription_validity_days: 0, created_at: '2026-07-02T00:00:00Z' })
    await flushPromises()
    expect(wrapper.get('[data-testid="result"]').attributes('data-status')).toBe('won')
    expect(stores.refreshUser).toHaveBeenCalledOnce()
  })

  it('renders RMB rewards without a dollar symbol', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('lottery.balancePrize')
    expect(wrapper.text()).not.toContain('$')
  })
})
