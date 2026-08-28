import { sanitizeUrl } from '@/utils/url'

export const MODELPORT_BRAND = Object.freeze({
  siteName: 'ModelPort',
  upstreamName: 'Sub2API',
  homeTitle: '模型港 ModelPort - 统一模型网关',
  mark: Object.freeze({
    light: '/branding/modelport-mark-light.png',
    dark: '/branding/modelport-mark-dark.png',
  }),
  wordmark: Object.freeze({
    light: '/branding/modelport-wordmark-light.png',
    dark: '/branding/modelport-wordmark-dark.png',
  }),
})

export const DEFAULT_SITE_NAME = MODELPORT_BRAND.siteName

const MODELPORT_LOGO_PATHS = new Set<string>([
  MODELPORT_BRAND.mark.light,
  MODELPORT_BRAND.mark.dark,
  MODELPORT_BRAND.wordmark.light,
  MODELPORT_BRAND.wordmark.dark,
])

export function isModelPortSiteName(siteName?: string): boolean {
  return siteName?.trim().toLowerCase() === MODELPORT_BRAND.siteName.toLowerCase()
}

export function usesModelPortBrand(siteName?: string, siteLogo = ''): boolean {
  const normalizedLogo = siteLogo.trim()
  return isModelPortSiteName(siteName)
    && (!normalizedLogo || MODELPORT_LOGO_PATHS.has(normalizedLogo))
}

export function modelPortAsset(variant: 'mark' | 'wordmark', dark: boolean): string {
  return MODELPORT_BRAND[variant][dark ? 'dark' : 'light']
}

export function updateFavicon(logoUrl = ''): void {
  const candidate = logoUrl.trim() || MODELPORT_BRAND.mark.light
  const sanitizedLogoUrl = sanitizeUrl(candidate, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = /\.svg(?:[?#]|$)/i.test(sanitizedLogoUrl)
    ? 'image/svg+xml'
    : /\.png(?:[?#]|$)/i.test(sanitizedLogoUrl)
      ? 'image/png'
      : 'image/x-icon'
  link.href = sanitizedLogoUrl
}
