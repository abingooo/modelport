import { expect, test, type Page, type TestInfo } from '@playwright/test'

const publicSettings = {
  site_name: 'ModelPort',
  site_logo: '',
  site_subtitle: 'One port, All Models.',
  home_content: '',
  compact_home_enabled: false,
  doc_url: 'https://github.com/abingooo/modelport',
  model_plaza_enabled: true,
  model_plaza_require_auth: false,
  backend_mode_enabled: false,
  registration_enabled: true,
  payment_enabled: false,
  version: '0.1.183.1',
  upstream_version: '0.1.183',
}

type VisualMetadata = {
  theme: 'light' | 'dark'
  motion: 'reduce' | 'no-preference'
  formFactor: 'desktop' | 'mobile'
}

function visualMetadata(testInfo: TestInfo): VisualMetadata {
  return testInfo.project.metadata as VisualMetadata
}

async function mockPublicRuntime(page: Page) {
  await page.route('**/setup/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: { needs_setup: false, step: 'complete' } }),
    })
  })
  await page.route(/\/api\/v1\/settings\/public(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: publicSettings }),
    })
  })
}

async function openHomepage(page: Page, testInfo: TestInfo) {
  const { theme, motion } = visualMetadata(testInfo)
  await page.emulateMedia({ colorScheme: theme, reducedMotion: motion })
  await page.addInitScript((selectedTheme) => {
    window.localStorage.setItem('theme', selectedTheme)
  }, theme)
  await page.goto('/home', { waitUntil: 'domcontentloaded' })
  await expect(page.locator('.home-shell')).toBeVisible()
  await expect(page.locator('.graphic-harbor')).toHaveAttribute('data-scene-ready', 'true')
}

async function assertBrandAssetsLoaded(page: Page, expectedTheme: 'light' | 'dark') {
  const brandAssets = page.locator('img[src*="/branding/modelport-"]')
  await expect.poll(() => brandAssets.count()).toBeGreaterThanOrEqual(3)
  await expect.poll(async () => brandAssets.evaluateAll((images) =>
    images.every((node) => {
      const image = node as HTMLImageElement
      return image.complete && image.naturalWidth > 0 && image.naturalHeight > 0
    })
  )).toBe(true)

  const evidence = await brandAssets.evaluateAll((images) =>
    images.map((node) => {
      const image = node as HTMLImageElement
      return {
        src: image.getAttribute('src') || '',
        complete: image.complete,
        naturalWidth: image.naturalWidth,
        naturalHeight: image.naturalHeight,
      }
    })
  )

  expect(evidence.length).toBeGreaterThanOrEqual(3)
  expect(evidence.every((asset) => asset.complete)).toBe(true)
  expect(evidence.every((asset) => asset.naturalWidth > 0 && asset.naturalHeight > 0)).toBe(true)
  expect(evidence.some((asset) => asset.src.endsWith(`/modelport-mark-${expectedTheme}.png`))).toBe(true)
  expect(evidence.some((asset) => asset.src.endsWith(`/modelport-wordmark-${expectedTheme}.png`))).toBe(true)
  await expect(page.locator('.dock-model-badge svg')).toHaveCount(9)
}

async function assertNoHorizontalOverflowOrKeyOverlap(page: Page) {
  const layout = await page.evaluate(() => {
    const viewportWidth = document.documentElement.clientWidth
    const shell = document.querySelector<HTMLElement>('.home-shell')
    const horizontalOverflow = {
      document: document.documentElement.scrollWidth - viewportWidth,
      body: document.body.scrollWidth - viewportWidth,
      shell: shell ? shell.scrollWidth - shell.clientWidth : 0,
    }

    const rectFor = (selector: string) => {
      const element = document.querySelector<HTMLElement>(selector)
      if (!element) return null
      const style = getComputedStyle(element)
      if (style.display === 'none' || style.visibility === 'hidden') return null
      const rect = element.getBoundingClientRect()
      return {
        selector,
        left: rect.left,
        right: rect.right,
        top: rect.top,
        bottom: rect.bottom,
        width: rect.width,
        height: rect.height,
      }
    }

    const keySelectors = [
      '.brand-link',
      '.nav-actions',
      '.hero-copy',
      '.hero-actions',
      '.primary-action',
      '.secondary-action',
    ]
    const keyRects = keySelectors.map(rectFor).filter((rect) => rect !== null)
    const outOfViewport = keyRects.filter((rect) =>
      rect.left < -1 || rect.right > viewportWidth + 1 || rect.width <= 0 || rect.height <= 0
    )

    const overlapPairs = [
      ['.brand-link', '.nav-actions'],
      ['.home-header', '.hero-copy'],
      ['.primary-action', '.secondary-action'],
    ]
    const overlaps = overlapPairs.flatMap(([leftSelector, rightSelector]) => {
      const left = rectFor(leftSelector)
      const right = rectFor(rightSelector)
      if (!left || !right) return []
      const overlapWidth = Math.min(left.right, right.right) - Math.max(left.left, right.left)
      const overlapHeight = Math.min(left.bottom, right.bottom) - Math.max(left.top, right.top)
      return overlapWidth > 1 && overlapHeight > 1
        ? [{ left: leftSelector, right: rightSelector, overlapWidth, overlapHeight }]
        : []
    })

    return { horizontalOverflow, outOfViewport, overlaps }
  })

  expect(layout.horizontalOverflow).toEqual({ document: 0, body: 0, shell: 0 })
  expect(layout.outOfViewport).toEqual([])
  expect(layout.overlaps).toEqual([])
}

async function canvasPixelEvidence(page: Page) {
  return page.locator('.graphic-harbor canvas').evaluate((node) => {
    const canvas = node as HTMLCanvasElement
    const context = canvas.getContext('2d')
    if (!context) return null

    const columns = 24
    const rows = 16
    let nonTransparentSamples = 0
    const colors = new Set<string>()
    for (let row = 0; row < rows; row += 1) {
      for (let column = 0; column < columns; column += 1) {
        const x = Math.min(canvas.width - 1, Math.floor(((column + 0.5) * canvas.width) / columns))
        const y = Math.min(canvas.height - 1, Math.floor(((row + 0.5) * canvas.height) / rows))
        const [red, green, blue, alpha] = context.getImageData(x, y, 1, 1).data
        if (alpha > 0) nonTransparentSamples += 1
        if (alpha > 0) colors.add(`${red >> 4}:${green >> 4}:${blue >> 4}:${alpha >> 4}`)
      }
    }

    return {
      width: canvas.width,
      height: canvas.height,
      totalSamples: columns * rows,
      nonTransparentSamples,
      distinctColorBuckets: colors.size,
    }
  })
}

async function attachScreenshot(page: Page, testInfo: TestInfo, name: string) {
  const screenshotPath = testInfo.outputPath(`${name}.png`)
  await page.screenshot({ path: screenshotPath, fullPage: true, animations: 'disabled' })
  await testInfo.attach(name, { path: screenshotPath, contentType: 'image/png' })
}

test.beforeEach(async ({ page }) => {
  await mockPublicRuntime(page)
})

test('homepage renders assets, layout, theme, motion, and nonblank Canvas', async ({ page }, testInfo) => {
  const pageErrors: string[] = []
  const failedResources: string[] = []
  page.on('pageerror', (error) => pageErrors.push(error.message))
  page.on('requestfailed', (request) => {
    if (['document', 'stylesheet', 'script', 'image', 'font'].includes(request.resourceType())) {
      failedResources.push(`${request.resourceType()}: ${request.url()}`)
    }
  })

  await openHomepage(page, testInfo)
  const { theme, motion } = visualMetadata(testInfo)
  const shell = page.locator('.home-shell')
  const scene = page.locator('.graphic-harbor')

  await expect(shell).toHaveClass(theme === 'dark' ? /is-dark/ : /home-shell/)
  await expect(scene).toHaveAttribute('data-renderer', 'canvas2d')
  await expect(scene).toHaveAttribute('data-celestial-body', theme === 'dark' ? 'moon' : 'sun')
  await expect(scene).toHaveClass(motion === 'reduce' ? /is-reduced/ : /graphic-harbor/)
  await assertBrandAssetsLoaded(page, theme)
  await assertNoHorizontalOverflowOrKeyOverlap(page)

  const pixels = await canvasPixelEvidence(page)
  expect(pixels).not.toBeNull()
  expect(pixels?.width).toBeGreaterThan(100)
  expect(pixels?.height).toBeGreaterThan(100)
  expect(pixels?.nonTransparentSamples).toBe(pixels?.totalSamples)
  expect(pixels?.distinctColorBuckets).toBeGreaterThan(8)

  if (motion === 'reduce') {
    const animationName = await page.locator('.dock-model-badge').first().evaluate((node) =>
      getComputedStyle(node).animationName
    )
    expect(animationName).toBe('none')
  }

  await attachScreenshot(page, testInfo, 'homepage')
  expect(failedResources).toEqual([])
  expect(pageErrors).toEqual([])
})

test('homepage keeps a usable static harbor when Canvas is unavailable', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop-light', 'One deterministic project covers the CSS fallback.')
  await page.addInitScript(() => {
    const originalGetContext = HTMLCanvasElement.prototype.getContext
    HTMLCanvasElement.prototype.getContext = function getContext(
      this: HTMLCanvasElement,
      contextId: string,
      ...args: unknown[]
    ) {
      if (contextId === '2d') return null
      return originalGetContext.call(this, contextId, ...args)
    } as typeof HTMLCanvasElement.prototype.getContext
  })

  await openHomepage(page, testInfo)
  const scene = page.locator('.graphic-harbor')
  await expect(scene).toHaveAttribute('data-renderer', 'fallback')
  await expect(scene).toHaveClass(/is-fallback/)
  await expect(scene.locator('.fallback-harbor')).toBeVisible()
  await expect(scene.locator('canvas')).toBeHidden()
  await expect(scene).toHaveAttribute('role', 'img')
  await expect(scene.locator('.fallback-terminal')).toBeVisible()
  await expect(scene.locator('.fallback-lighthouse')).toBeVisible()
  await expect(scene.locator('.fallback-ship')).toHaveCount(2)
  await assertBrandAssetsLoaded(page, 'light')
  await assertNoHorizontalOverflowOrKeyOverlap(page)
  await attachScreenshot(page, testInfo, 'homepage-static-fallback')
})
