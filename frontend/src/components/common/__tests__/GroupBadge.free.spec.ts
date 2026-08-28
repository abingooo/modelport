import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupBadge from '../GroupBadge.vue'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: { server_utc_offset: '+08:00' } }),
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('GroupBadge free billing', () => {
  it('shows the free label and suppresses custom and peak multipliers', () => {
    const wrapper = mount(GroupBadge, {
      props: {
        name: 'Free harbor',
        platform: 'openai',
        rateMultiplier: 2,
        userRateMultiplier: 0.5,
        isFree: true,
        peakRateEnabled: true,
        peakStart: '09:00',
        peakEnd: '18:00',
        peakRateMultiplier: 3,
      },
      global: { stubs: { PlatformIcon: true } },
    })

    expect(wrapper.text()).toContain('admin.groups.freeBilling.badge')
    expect(wrapper.text()).not.toContain('2x')
    expect(wrapper.text()).not.toContain('0.5x')
    expect(wrapper.text()).not.toContain('3x')
    expect(wrapper.find('[title]').exists()).toBe(false)
  })
})
