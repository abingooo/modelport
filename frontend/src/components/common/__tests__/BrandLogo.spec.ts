import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import BrandLogo from '../BrandLogo.vue'

describe('BrandLogo', () => {
  it('renders light and dark ModelPort marks', () => {
    const wrapper = mount(BrandLogo, {
      props: { siteName: 'ModelPort', imageClass: 'brand-image' },
    })

    const images = wrapper.findAll('img')
    expect(images).toHaveLength(2)
    expect(images[0].attributes('src')).toBe('/branding/modelport-mark-light.png')
    expect(images[0].classes()).toContain('dark:hidden')
    expect(images[1].attributes('src')).toBe('/branding/modelport-mark-dark.png')
    expect(images[1].classes()).toContain('dark:block')
  })

  it('renders the ModelPort wordmark variant', () => {
    const wrapper = mount(BrandLogo, {
      props: {
        siteName: 'ModelPort',
        siteLogo: '/branding/modelport-mark-light.png',
        variant: 'wordmark',
      },
    })

    const images = wrapper.findAll('img')
    expect(images).toHaveLength(2)
    expect(images[0].attributes('src')).toBe(
      '/branding/modelport-wordmark-light.png'
    )
    expect(images[1].attributes('src')).toBe('/branding/modelport-wordmark-dark.png')
  })

  it('keeps a custom site logo for other brands', () => {
    const wrapper = mount(BrandLogo, {
      props: { siteName: 'Another Site', siteLogo: '/custom-logo.png' },
    })

    const images = wrapper.findAll('img')
    expect(images).toHaveLength(1)
    expect(images[0].attributes('src')).toBe('/custom-logo.png')
  })

  it('honors a custom logo for a site still named ModelPort', () => {
    const wrapper = mount(BrandLogo, {
      props: { siteName: 'ModelPort', siteLogo: '/custom-logo.png' },
    })

    const images = wrapper.findAll('img')
    expect(images).toHaveLength(1)
    expect(images[0].attributes('src')).toBe('/custom-logo.png')
  })
})
