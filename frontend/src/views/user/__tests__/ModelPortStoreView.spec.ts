import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPortStoreView from '../ModelPortStoreView.vue'

const push = vi.hoisted(() => vi.fn())
const publicSettings = vi.hoisted(() => ({ payment_enabled: true }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: publicSettings }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

function mountView() {
  return mount(ModelPortStoreView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: true,
      },
    },
  })
}

describe('ModelPortStoreView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    publicSettings.payment_enabled = true
  })

  it('reuses the existing balance recharge and redemption routes', async () => {
    const wrapper = mountView()
    const buttons = wrapper.findAll('button')
    await buttons[0].trigger('click')
    await buttons[1].trigger('click')
    expect(push).toHaveBeenNthCalledWith(1, { path: '/purchase', query: { tab: 'recharge' } })
    expect(push).toHaveBeenNthCalledWith(2, '/redeem')
    expect(wrapper.text()).toContain('modelPortStore.emptyTitle')
  })

  it('disables recharge when payments are unavailable without disabling redemption', () => {
    publicSettings.payment_enabled = false
    const wrapper = mountView()
    const buttons = wrapper.findAll('button')
    expect(buttons[0].attributes('disabled')).toBeDefined()
    expect(buttons[1].attributes('disabled')).toBeUndefined()
  })
})
