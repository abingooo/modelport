import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HomeView from '../HomeView.vue'

const stores = vi.hoisted(() => ({
  auth: { isAuthenticated: true, isAdmin: false, user: { email: 'captain@modelport.link' }, checkAuth: vi.fn() },
  app: {
    cachedPublicSettings: { site_name: 'ModelPort', site_logo: '/logo.png', site_subtitle: 'Unified model gateway', doc_url: 'https://docs.example.com', home_content: '' },
    siteName: 'ModelPort', siteLogo: '', docUrl: '', publicSettingsLoaded: true, fetchPublicSettings: vi.fn(),
  },
}))
vi.mock('@/stores', () => ({ useAuthStore: () => stores.auth, useAppStore: () => stores.app }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }),
}))

function mountView() {
  return mount(HomeView, { global: { stubs: {
    HarborScene: { props: ['dark', 'label'], template: '<canvas data-testid="harbor" :aria-label="label" />' },
    BrandLogo: true, LocaleSwitcher: true, PlatformIcon: true, Icon: true,
    RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' },
  } } })
}

describe('HomeView harbor experience', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stores.auth.isAuthenticated = true
    stores.auth.isAdmin = false
    stores.app.cachedPublicSettings.home_content = ''
  })

  it('renders the ModelPort brand, animated scene, and core routes', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('ModelPort')
    expect(wrapper.get('[data-testid="harbor"]').attributes('aria-label')).toBe('home.harbor.sceneLabel')
    const routes = wrapper.findAll('[data-to]').map((link) => link.attributes('data-to'))
    expect(routes).toContain('/model-catalog')
    expect(routes).toContain('/keys')
    expect(routes).toContain('/lottery')
    expect(wrapper.text()).toContain('home.harbor.manifest.title')
  })

  it('keeps administrator and anonymous primary actions correct', async () => {
    stores.auth.isAdmin = true
    let wrapper = mountView()
    await flushPromises()
    expect(wrapper.findAll('[data-to]').map((link) => link.attributes('data-to'))).toContain('/admin/dashboard')

    wrapper.unmount()
    stores.auth.isAuthenticated = false
    stores.auth.isAdmin = false
    wrapper = mountView()
    await flushPromises()
    const routes = wrapper.findAll('[data-to]').map((link) => link.attributes('data-to'))
    expect(routes).toContain('/login')
    expect(routes).not.toContain('/keys')
  })
})
