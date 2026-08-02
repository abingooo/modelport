import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelPlazaView from '../ModelPlazaView.vue'

const state = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  auth: { isAuthenticated: true },
  app: { fetchPublicSettings: vi.fn() },
  getModelPlaza: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRoute: () => state.route
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => state.auth
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => state.app
}))

vi.mock('@/api/modelPlaza', () => ({
  getModelPlaza: state.getModelPlaza
}))

function mountView() {
  return mount(ModelPlazaView, {
    global: {
      stubs: {
        AppLayout: { template: '<div data-testid="app-layout"><slot /></div>' },
        PlazaNavBar: { template: '<nav data-testid="public-nav" />' },
        ModelPlazaContent: {
          props: { embedded: Boolean },
          template: '<div data-testid="plaza-content" :data-embedded="String(embedded)" />'
        }
      }
    }
  })
}

describe('ModelPlazaView layout modes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.route.query = {}
    state.auth.isAuthenticated = true
    state.getModelPlaza.mockResolvedValue({ groups: [] })
  })

  it('uses the wide public container for the standalone route, including signed-in users', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="model-plaza-public-main"]').classes()).toContain('max-w-[128rem]')
    expect(wrapper.find('[data-testid="public-nav"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="app-layout"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="plaza-content"]').attributes('data-embedded')).toBe('false')
  })

  it('keeps authenticated embedded routes inside AppLayout without the public container', async () => {
    state.route.query = { embedded: '1' }
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="model-plaza-public-main"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="public-nav"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="app-layout"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="plaza-content"]').attributes('data-embedded')).toBe('true')
  })

  it('falls back to the standalone layout when embedded access is unauthenticated', async () => {
    state.route.query = { embedded: '1' }
    state.auth.isAuthenticated = false
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="model-plaza-public-main"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="app-layout"]').exists()).toBe(false)
  })
})
