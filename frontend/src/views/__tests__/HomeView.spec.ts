import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HomeView from '../HomeView.vue'

const stores = vi.hoisted(() => ({
  auth: {
    isAuthenticated: true,
    isAdmin: false,
    user: { email: 'captain@modelport.link' },
    checkAuth: vi.fn(),
  },
  app: {
    cachedPublicSettings: {
      site_name: 'ModelPort',
      site_logo: '/logo.png',
      site_subtitle: 'Unified model gateway',
      doc_url: 'https://docs.example.com',
      home_content: '',
    },
    siteName: 'ModelPort',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => stores.auth,
  useAppStore: () => stores.app,
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({
    t: (key: string) => ({
      'home.footer.copyrightNotice': '保留所有权利',
      'home.heroDescription': '统一接入、调度与计费，让不同协议与模型能力汇聚于同一个端口',
      'home.heroKicker': '统一模型网关',
    }[key] ?? key),
  }),
}))

function mountView() {
  return mount(HomeView, {
    global: {
      stubs: {
        HarborScene: {
          props: ['dark', 'label', 'providers'],
          template: `
            <div
              data-testid="harbor"
              :aria-label="label"
              :data-dark="String(dark)"
              :data-provider-count="providers.length"
            />
          `,
        },
        BrandLogo: true,
        LocaleSwitcher: true,
        Icon: true,
        RouterLink: {
          props: ['to'],
          template: `
            <a
              :data-to="typeof to === 'string' ? to : to.path"
              :data-embedded="typeof to === 'string' ? undefined : to.query.embedded"
            ><slot /></a>
          `,
        },
      },
    },
  })
}

function routes(wrapper: ReturnType<typeof mountView>) {
  return wrapper.findAll('[data-to]').map((link) => link.attributes('data-to'))
}

describe('HomeView graphic harbor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    localStorage.setItem('theme', 'light')
    document.documentElement.classList.remove('dark')
    stores.auth.isAuthenticated = true
    stores.auth.isAdmin = false
    stores.app.cachedPublicSettings.site_name = 'ModelPort'
    stores.app.cachedPublicSettings.site_logo = '/logo.png'
    stores.app.cachedPublicSettings.site_subtitle = 'Unified model gateway'
    stores.app.cachedPublicSettings.home_content = ''
  })

  it('renders the branded harbor with only the two approved business destinations', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe('ModelPort')
    expect(wrapper.get('.hero-tagline').text()).toBe('One port, All Models.')
    expect(wrapper.get('.chinese-wordmark').text()).toBe('模型港')
    expect(wrapper.get('.kicker-label').text()).toBe('统一模型网关')
    expect(wrapper.get('.hero-description').text()).toBe(
      '统一接入、调度与计费，让不同协议与模型能力汇聚于同一个端口'
    )
    expect(wrapper.get('.hero-description').text()).not.toMatch(/[。.]+$/)
    expect(document.head.querySelector('meta[name="description"]')?.getAttribute('content')).toBe(
      'home.metaDescription'
    )
    expect(wrapper.get('.hero-title img').attributes('src')).toBe(
      '/branding/modelport-wordmark-light.png'
    )
    expect(wrapper.get('.nav-mark').attributes('src')).toBe('/branding/modelport-mark-light.png')
    expect(wrapper.get('[data-testid="harbor"]').attributes('aria-label')).toBe(
      'home.harborSceneLabel'
    )
    expect(Number(wrapper.get('[data-testid="harbor"]').attributes('data-provider-count'))).toBeGreaterThan(8)

    const destinations = routes(wrapper)
    expect(destinations).toContain('/dashboard')
    expect(destinations).toContain('/model-plaza')
    expect(destinations).not.toContain('/available-channels')
    expect(destinations).not.toContain('/keys')
    expect(destinations).not.toContain('/lottery')
    expect(wrapper.findAll('[data-to="/model-plaza"][data-embedded="1"]')).toHaveLength(2)
    expect(wrapper.find('.hero-image').exists()).toBe(false)
    expect(wrapper.find('.mobile-navigation').exists()).toBe(false)
    expect(wrapper.find('.model-current').exists()).toBe(false)
    expect(wrapper.findAll('.provider-chip')).toHaveLength(0)
    expect(wrapper.find('.port-node').exists()).toBe(false)
    expect(wrapper.get('.home-footer').text()).toBe(
      `© ${new Date().getFullYear()} ModelPort 保留所有权利`
    )
    expect(wrapper.get('.home-footer').text()).not.toContain('ModelPort.')
    expect(wrapper.find('.fleet-section').exists()).toBe(false)
    expect(wrapper.find('.protocol-section').exists()).toBe(false)
    expect(wrapper.find('.closing-section').exists()).toBe(false)
  })

  it('routes administrators to the admin dashboard and anonymous users to login', async () => {
    stores.auth.isAdmin = true
    let wrapper = mountView()
    await flushPromises()
    expect(routes(wrapper)).toContain('/admin/dashboard')

    wrapper.unmount()
    stores.auth.isAuthenticated = false
    stores.auth.isAdmin = false
    wrapper = mountView()
    await flushPromises()
    expect(routes(wrapper)).toContain('/login')
    expect(routes(wrapper)).not.toContain('/dashboard')
    expect(wrapper.findAll('[data-to="/model-plaza"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-to="/model-plaza"][data-embedded]')).toHaveLength(0)
  })

  it('keeps provider cargo in the harbor without rendering a separate icon current', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="harbor"]').attributes('data-provider-count')).toBe('11')
    expect(wrapper.find('.model-current').exists()).toBe(false)
    expect(wrapper.find('.lane-east').exists()).toBe(false)
    expect(wrapper.find('.lane-west').exists()).toBe(false)
  })

  it('preserves configured external home content', async () => {
    stores.app.cachedPublicSettings.home_content = 'https://content.example.com/home'
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('iframe').attributes('src')).toBe('https://content.example.com/home')
    expect(wrapper.find('.home-shell').exists()).toBe(false)
  })

  it('preserves custom branding outside ModelPort installations', async () => {
    stores.app.cachedPublicSettings.site_name = 'Acme Gateway'
    stores.app.cachedPublicSettings.site_subtitle = 'One gateway for Acme'
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe('Acme Gateway')
    expect(wrapper.get('.hero-tagline').text()).toBe('One gateway for Acme')
    expect(wrapper.find('.hero-title img').exists()).toBe(false)
    expect(wrapper.get('.home-footer').text()).toBe(
      `© ${new Date().getFullYear()} Acme Gateway 保留所有权利`
    )
  })

  it('keeps theme state synchronized with the scene', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[data-testid="harbor"]').attributes('data-dark')).toBe('false')
    expect(wrapper.get('.theme-action icon-stub').attributes('name')).toBe('sun')

    await wrapper.get('.theme-action').trigger('click')
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(wrapper.get('[data-testid="harbor"]').attributes('data-dark')).toBe('true')
    expect(wrapper.get('.theme-action icon-stub').attributes('name')).toBe('moon')
  })
})
