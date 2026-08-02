import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppSidebar from '../AppSidebar.vue'

const stores = vi.hoisted(() => ({
  app: {
    sidebarCollapsed: false,
    mobileOpen: false,
    backendModeEnabled: false,
    siteName: 'ModelPort',
    siteLogo: '',
    siteVersion: '0.1.164.6-dev.2',
    publicSettingsLoaded: true,
    sidebarScrollTop: 0,
    cachedPublicSettings: {
      custom_menu_items: [] as Array<{
        id: number
        label: string
        visibility: string
        sort_order: number
        icon_svg: string
      }>,
    },
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn(),
  },
  auth: {
    isAdmin: false,
    isSimpleMode: false,
  },
  admin: {
    customMenuItems: [],
    opsMonitoringEnabled: false,
    paymentEnabled: false,
    fetch: vi.fn(),
  },
  onboarding: {
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => stores.app,
  useAuthStore: () => stores.auth,
  useAdminSettingsStore: () => stores.admin,
  useOnboardingStore: () => stores.onboarding,
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard' }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: {
    affiliate: {},
    availableChannels: {},
    channelMonitor: {},
    payment: {},
    riskControl: {},
  },
  makeSidebarFlag: () => () => true,
}))

const RouterLinkStub = defineComponent({
  props: { to: { type: String, required: true } },
  template: '<a :data-to="to"><slot /></a>',
})

function mountSidebar() {
  return mount(AppSidebar, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        BrandLogo: true,
        VersionBadge: true,
      },
    },
  })
}

function renderedPaths(wrapper: ReturnType<typeof mountSidebar>) {
  return wrapper.get('.sidebar-nav').findAll('[data-to]').map((link) => link.attributes('data-to'))
}

describe('AppSidebar user navigation', () => {
  beforeEach(() => {
    stores.auth.isAdmin = false
    stores.auth.isSimpleMode = false
    stores.app.cachedPublicSettings.custom_menu_items = []
  })

  it('keeps native account flows and omits retired product links for regular users', () => {
    const paths = renderedPaths(mountSidebar())
    expect(paths).not.toContain('/image-site')
    expect(paths).not.toContain('/batch-image')
    expect(paths).not.toContain('/store')
    expect(paths).toContain('/purchase')
    expect(paths).toContain('/subscriptions')
    expect(paths).toContain('/redeem')
    expect(paths).toContain('/orders')
    expect(paths).toContain('/lottery')
  })

  it('orders regular-user navigation by workflow and places custom pages after channel status', () => {
    stores.app.cachedPublicSettings.custom_menu_items = [{
      id: 7,
      label: '生图小站',
      visibility: 'user',
      sort_order: 1,
      icon_svg: '<svg></svg>',
    }]

    expect(renderedPaths(mountSidebar())).toEqual([
      '/dashboard',
      '/keys',
      '/usage',
      '/monitor',
      '/custom/7',
      '/purchase',
      '/subscriptions',
      '/redeem',
      '/orders',
      '/affiliate',
      '/lottery',
      '/profile',
    ])
  })

  it('omits retired product links from the admin personal section', () => {
    stores.auth.isAdmin = true
    const paths = renderedPaths(mountSidebar())
    expect(paths).not.toContain('/image-site')
    expect(paths).not.toContain('/batch-image')
    expect(paths).not.toContain('/store')
    expect(paths).toContain('/purchase')
    expect(paths).toContain('/subscriptions')
    expect(paths).toContain('/redeem')
    expect(paths).toContain('/orders')
    expect(paths).toContain('/lottery')
    expect(paths).toContain('/admin/lottery')
  })

  it('keeps existing simple-mode visibility rules', () => {
    stores.auth.isSimpleMode = true
    const paths = renderedPaths(mountSidebar())
    expect(paths).not.toContain('/image-site')
    expect(paths).not.toContain('/batch-image')
    expect(paths).not.toContain('/store')
    expect(paths).not.toContain('/lottery')
    expect(paths).not.toContain('/admin/lottery')
  })
})
