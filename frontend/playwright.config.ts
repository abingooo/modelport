import { defineConfig } from '@playwright/test'

const host = '127.0.0.1'
const port = Number(process.env.MODELPORT_VISUAL_PORT || '4173')
const baseURL = `http://${host}:${port}`

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.visual.spec.ts',
  outputDir: 'test-results/visual',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: [
    ['line'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  use: {
    baseURL,
    browserName: 'chromium',
    locale: 'zh-CN',
    timezoneId: 'Asia/Shanghai',
    serviceWorkers: 'block',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  webServer: {
    command: 'node scripts/serve-visual.mjs',
    url: baseURL,
    env: {
      MODELPORT_VISUAL_HOST: host,
      MODELPORT_VISUAL_PORT: String(port),
    },
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
  projects: [
    {
      name: 'desktop-light',
      metadata: { theme: 'light', motion: 'no-preference', formFactor: 'desktop' },
      use: {
        viewport: { width: 1440, height: 900 },
        colorScheme: 'light',
        deviceScaleFactor: 1,
      },
    },
    {
      name: 'desktop-dark',
      metadata: { theme: 'dark', motion: 'no-preference', formFactor: 'desktop' },
      use: {
        viewport: { width: 1920, height: 1080 },
        colorScheme: 'dark',
        deviceScaleFactor: 1,
      },
    },
    {
      name: 'mobile-light',
      metadata: { theme: 'light', motion: 'no-preference', formFactor: 'mobile' },
      use: {
        viewport: { width: 390, height: 844 },
        colorScheme: 'light',
        deviceScaleFactor: 2,
        hasTouch: true,
        isMobile: true,
      },
    },
    {
      name: 'mobile-dark-reduced',
      metadata: { theme: 'dark', motion: 'reduce', formFactor: 'mobile' },
      use: {
        viewport: { width: 390, height: 844 },
        colorScheme: 'dark',
        deviceScaleFactor: 2,
        hasTouch: true,
        isMobile: true,
      },
    },
  ],
})
