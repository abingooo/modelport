import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AdminAffiliateRewardReviewsView from '../AdminAffiliateRewardReviewsView.vue'

const mocks = vi.hoisted(() => ({
  getAllGroups: vi.fn(),
  getRewardProgram: vi.fn(),
  getRewardReviewStats: vi.fn(),
  listRewardReviews: vi.fn(),
  reviewReward: vi.fn(),
  updateRewardProgram: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { getAll: mocks.getAllGroups },
  },
  affiliatesAPI: {
    getRewardProgram: mocks.getRewardProgram,
    getRewardReviewStats: mocks.getRewardReviewStats,
    listRewardReviews: mocks.listRewardReviews,
    reviewReward: mocks.reviewReward,
    updateRewardProgram: mocks.updateRewardProgram,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: mocks.showError,
    showSuccess: mocks.showSuccess,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string, fallback?: unknown) => typeof fallback === 'string' ? fallback : key }),
}))

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<section v-if="show" data-dialog><slot /><footer><slot name="footer" /></footer></section>',
})

function mountView() {
  return mount(AdminAffiliateRewardReviewsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        BaseDialog: BaseDialogStub,
        Icon: true,
        LoadingSpinner: true,
        Pagination: true,
        Toggle: true,
      },
    },
  })
}

describe('AdminAffiliateRewardReviewsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getAllGroups.mockResolvedValue([{ id: 50, name: 'Trial', status: 'active' }])
    mocks.getRewardProgram.mockResolvedValue({
      version: 1,
      enabled: true,
      registration: {
        enabled: true,
        inviter_bonus: 1,
        invitee_trial_amount: 3,
        invitee_trial_group_id: 50,
        invitee_trial_days: 3,
      },
      first_recharge: {
        enabled: true,
        inviter_bonus: 2,
        invitee_bonus_percent: 10,
      },
    })
    mocks.getRewardReviewStats.mockResolvedValue({
      pending_count: 2,
      pending_amount: 4,
      paid_count: 8,
      paid_amount: 16,
      rejected_count: 1,
      high_risk_pending_count: 1,
      today_paid_count: 3,
      by_type: {},
    })
    mocks.listRewardReviews.mockResolvedValue({
      items: [
        {
          id: 101,
          inviter_user_id: 1,
          invitee_user_id: 2,
          reward_user_id: 1,
          reward_type: 'invite_register_inviter_bonus',
          reward_amount: 1,
          status: 'pending',
          risk_flags: { risk_level: 'low', risk_score: 0 },
          risk_level: 'low',
          risk_score: 0,
          review_note: '',
          created_at: '2026-07-30T00:00:00Z',
          updated_at: '2026-07-30T00:00:00Z',
          inviter_email: 'inviter@example.com',
          invitee_email: 'invitee@example.com',
          reward_user_email: 'inviter@example.com',
          reviewed_by_email: '',
          order_status: '',
          registration_ip: '203.0.113.9',
        },
        {
          id: 102,
          inviter_user_id: 1,
          invitee_user_id: 3,
          reward_user_id: 3,
          reward_type: 'legacy_custom_reward',
          reward_amount: 3,
          status: 'pending',
          risk_flags: { risk_level: 'unknown' },
          risk_level: 'unknown',
          risk_score: 0,
          review_note: '',
          created_at: '2026-07-30T00:00:00Z',
          updated_at: '2026-07-30T00:00:00Z',
          inviter_email: 'inviter@example.com',
          invitee_email: 'legacy@example.com',
          reward_user_email: 'legacy@example.com',
          reviewed_by_email: '',
          order_status: '',
          registration_ip: '',
        },
        {
          id: 103,
          inviter_user_id: 1,
          invitee_user_id: 4,
          reward_user_id: 1,
          reward_type: 'first_recharge_inviter_bonus',
          reward_amount: 2,
          status: 'pending',
          risk_flags: { risk_level: 'low' },
          risk_level: 'low',
          risk_score: 0,
          review_note: '',
          created_at: '2026-07-05T21:00:00Z',
          updated_at: '2026-07-05T21:00:00Z',
          inviter_email: 'inviter@example.com',
          invitee_email: 'protected@example.com',
          reward_user_email: 'inviter@example.com',
          reviewed_by_email: '',
          order_status: 'COMPLETED',
          registration_ip: '',
          approval_blocked_reason: 'legacy_cutoff',
        },
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    mocks.reviewReward.mockResolvedValue({ review_ids: [101], status: 'paid', effects: [] })
    mocks.updateRewardProgram.mockResolvedValue({})
  })

  it('loads native review data and blocks automatic approval of unknown history', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(mocks.listRewardReviews).toHaveBeenCalledWith(expect.objectContaining({
      status: 'pending_review',
      reward_type: 'all',
      risk: 'all',
    }))
    expect(wrapper.text()).toContain('inviter@example.com')
    expect(wrapper.text()).toContain('legacy_custom_reward')
    expect(wrapper.text()).toContain('admin.affiliates.reviews.stats.pending')

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(3)
    const unknownApprove = rows[1].findAll('button').find((button) =>
      button.text().includes('admin.affiliates.reviews.approve'),
    )
    expect(unknownApprove?.attributes('disabled')).toBeDefined()
    expect(unknownApprove?.attributes('title')).toBe('admin.affiliates.reviews.unsupportedApprove')

    const protectedApprove = rows[2].findAll('button').find((button) =>
      button.text().includes('admin.affiliates.reviews.approve'),
    )
    expect(protectedApprove?.attributes('disabled')).toBeDefined()
    expect(protectedApprove?.attributes('title')).toBe('admin.affiliates.reviews.legacyProtectedApprove')
  })

  it('submits a supported grouped approval through the source API', async () => {
    const wrapper = mountView()
    await flushPromises()

    const approveButton = wrapper.findAll('tbody tr')[0].findAll('button').find((button) =>
      button.text().includes('admin.affiliates.reviews.approve'),
    )
    expect(approveButton).toBeDefined()
    await approveButton!.trigger('click')

    const dialog = wrapper.get('[data-dialog]')
    const confirmButton = dialog.findAll('button').find((button) =>
      button.text().includes('admin.affiliates.reviews.approve'),
    )
    expect(confirmButton).toBeDefined()
    await confirmButton!.trigger('click')
    await flushPromises()

    expect(mocks.reviewReward).toHaveBeenCalledWith(101, 'approve', '')
    expect(mocks.showSuccess).toHaveBeenCalledWith('admin.affiliates.reviews.confirm.approveSuccess')
  })
})
