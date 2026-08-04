import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AffiliateView from '../AffiliateView.vue'

const { copyToClipboard, getAffiliateDetail } = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  getAffiliateDetail: vi.fn(),
}))

vi.mock('@/api/user', () => ({
  default: {
    getAffiliateDetail,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

describe('AffiliateView', () => {
  const affiliateCode = 'affiliate-code-that-is-long-enough-to-overflow-a-mobile-viewport'

  beforeEach(() => {
    vi.clearAllMocks()
    copyToClipboard.mockResolvedValue(true)
    getAffiliateDetail.mockResolvedValue({
      user_id: 1,
      aff_code: affiliateCode,
      inviter_id: null,
      aff_count: 0,
      aff_quota: 0,
      aff_frozen_quota: 0,
      aff_history_quota: 0,
      effective_rebate_rate_percent: 10,
      invitees: [],
      reward_program: {
        program: {
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
        },
        paid_amount: 3,
        pending_amount: 1,
        rejected_amount: 0,
        invited_users: 1,
        first_recharge_users: 1,
        invitees: [{
          user_id: 2,
          email_masked: 'f***@e***.com',
          registered_at: '2026-07-30T00:00:00Z',
          registration_status: 'paid',
          registration_reward_status: 'paid',
          registration_reward_amount: 1,
          first_recharge_status: 'pending',
          first_recharge_reward_status: 'pending',
          first_recharge_reward_amount: 2,
        }],
      },
    })
  })

  it('keeps long invite values bounded and renders the native reward progress', async () => {
    const wrapper = mount(AffiliateView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
        },
      },
    })

    await flushPromises()

    const values = wrapper.findAll('code')
    expect(values).toHaveLength(2)
    for (const value of values) {
      expect(value.classes()).toEqual(expect.arrayContaining([
        'min-w-0',
        'flex-1',
        'truncate',
      ]))
      expect(Array.from(value.element.parentElement?.classList ?? [])).toEqual(expect.arrayContaining([
        'flex',
        'min-w-0',
        'items-center',
      ]))
    }

    const copyButtons = wrapper.findAll('button').filter((button) => [
      'affiliate.copyCode',
      'affiliate.copyLink',
    ].includes(button.attributes('title')))
    expect(copyButtons).toHaveLength(2)
    for (const button of copyButtons) {
      expect(button.classes()).toEqual(expect.arrayContaining([
        'icon-btn',
        'shrink-0',
      ]))
    }

    expect(wrapper.text()).toContain('ModelPort Partner')
    expect(wrapper.text()).toContain('affiliate.program.active')
    expect(wrapper.text()).toContain('affiliate.progress.title')
    expect(wrapper.text()).toContain('f***@e***.com')
    expect(wrapper.text()).toContain('affiliate.status.paid')
    expect(wrapper.text()).toContain('affiliate.status.pending')
    expect(wrapper.text()).not.toContain('affiliate.continuous.title')
    expect(wrapper.text()).not.toContain('affiliate.transfer.button')

    await copyButtons[0].trigger('click')
    await copyButtons[1].trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenNthCalledWith(1, affiliateCode, 'affiliate.codeCopied')
    expect(copyToClipboard).toHaveBeenNthCalledWith(
      2,
      `${window.location.origin}/register?aff=${encodeURIComponent(affiliateCode)}`,
      'affiliate.linkCopied',
    )
  })

  it('does not expose continuous rebate controls when fixed rewards are unavailable', async () => {
    getAffiliateDetail.mockResolvedValueOnce({
      user_id: 1,
      aff_code: affiliateCode,
      inviter_id: null,
      aff_count: 0,
      aff_quota: 2,
      aff_frozen_quota: 0,
      aff_history_quota: 4,
      effective_rebate_rate_percent: 20,
      invitees: [],
    })

    const wrapper = mount(AffiliateView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('affiliate.program.unavailable')
    expect(wrapper.text()).not.toContain('affiliate.continuous.title')
    expect(wrapper.text()).not.toContain('affiliate.transfer.button')
    expect(wrapper.text()).not.toContain('affiliate.progress.title')
  })
})
