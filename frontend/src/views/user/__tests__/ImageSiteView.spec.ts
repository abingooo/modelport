import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ImageSiteView from '../ImageSiteView.vue'

const publicSettings = vi.hoisted(() => ({ image_site_url: '' }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: publicSettings }),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

function mountView() {
  return mount(ImageSiteView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: true,
      },
    },
  })
}

describe('ImageSiteView', () => {
  beforeEach(() => { publicSettings.image_site_url = '' })

  it('renders a protected external link for a valid configured destination', () => {
    publicSettings.image_site_url = 'https://images.modelport.link/create'
    const wrapper = mountView()
    const link = wrapper.get('a')
    expect(link.attributes('href')).toBe('https://images.modelport.link/create')
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
    expect(wrapper.text()).toContain('images.modelport.link')
  })

  it.each([
    'javascript:alert(1)',
    'https://user:password@images.modelport.link/create',
    `https://images.modelport.link/${'a'.repeat(2049)}`,
  ])('does not render a link for an unsafe destination: %s', (destination) => {
    publicSettings.image_site_url = destination
    const wrapper = mountView()
    expect(wrapper.find('a').exists()).toBe(false)
    expect(wrapper.text()).toContain('imageSite.unavailableTitle')
  })
})
