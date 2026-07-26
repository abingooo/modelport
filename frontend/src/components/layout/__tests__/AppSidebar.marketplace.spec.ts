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
    cachedPublicSettings: { custom_menu_items: [] },
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

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({
    canUseBatchImage: { value: true },
    refreshBatchImageAccess: vi.fn(),
  }),
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
  return wrapper.findAll('[data-to]').map((link) => link.attributes('data-to'))
}

describe('AppSidebar marketplace navigation', () => {
  beforeEach(() => {
    stores.auth.isAdmin = false
    stores.auth.isSimpleMode = false
  })

  it('shows marketplace and lottery links to regular users', () => {
    const paths = renderedPaths(mountSidebar())
    expect(paths).toContain('/image-site')
    expect(paths).toContain('/store')
    expect(paths).toContain('/lottery')
  })

  it('shows image studio and store links in the admin personal section', () => {
    stores.auth.isAdmin = true
    const paths = renderedPaths(mountSidebar())
    expect(paths).toContain('/image-site')
    expect(paths).toContain('/store')
    expect(paths).toContain('/lottery')
    expect(paths).toContain('/admin/lottery')
  })

  it('hides both links in simple mode', () => {
    stores.auth.isSimpleMode = true
    const paths = renderedPaths(mountSidebar())
    expect(paths).not.toContain('/image-site')
    expect(paths).not.toContain('/store')
    expect(paths).not.toContain('/lottery')
    expect(paths).not.toContain('/admin/lottery')
  })
})
