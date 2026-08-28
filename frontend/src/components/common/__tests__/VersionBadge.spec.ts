import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAppStore, useAuthStore } from '@/stores'
import type { User } from '@/types'
import VersionBadge from '../VersionBadge.vue'

const systemApiMocks = vi.hoisted(() => ({
  checkUpdates: vi.fn(),
  performUpdate: vi.fn(),
  getVersion: vi.fn(),
  restartService: vi.fn(),
  getRollbackVersions: vi.fn(),
  rollback: vi.fn(),
}))

vi.mock('@/api/admin/system', () => ({
  ...systemApiMocks,
  default: systemApiMocks,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        const messages: Record<string, string> = {
          'version.currentVersion': 'Current Version',
          'version.modelPortVersion': 'ModelPort Version',
          'version.basedOnUpstream': 'Based on {name} v{version}',
          'version.upToDate': 'Up to date',
          'version.updateAvailable': 'Update available',
          'version.latestVersion': 'Latest Version',
          'version.refresh': 'Refresh',
          'version.rollback': 'Rollback',
          'version.updateNow': 'Update Now',
          'version.updating': 'Updating',
          'version.updateQueued': 'Update queued',
          'version.containerRecreating': 'Verifying digest and recreating container',
          'version.manualModeHint': 'Online updates are disabled',
          'version.sourceModeHint': 'Use git pull',
          'version.rollbackManualHint': 'Online rollback is disabled',
          'version.rollbackDockerWarning': 'Digest-pinned Docker rollback',
          'version.rollbackSelectVersion': 'Select version',
          'version.rollbackConfirm': 'Roll back to {version}',
          'version.noRollbackVersions': 'No rollback versions',
        }
        return (messages[key] || key).replace(
          /\{(\w+)\}/g,
          (_, name: string) => params?.[name] || ''
        )
      },
    }),
  }
})

describe('VersionBadge product lineage', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    systemApiMocks.performUpdate.mockResolvedValue({
      message: 'Docker update queued',
      need_restart: false,
      update_queued: true,
    })
    systemApiMocks.rollback.mockResolvedValue({
      message: 'Docker rollback queued',
      need_restart: false,
      update_queued: true,
    })
    systemApiMocks.getRollbackVersions.mockResolvedValue({ versions: [] })
    systemApiMocks.getVersion.mockResolvedValue({ version: '0.1.183.1' })
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('shows ModelPort and its Sub2API baseline as separate versions', async () => {
    const authStore = useAuthStore()
    const appStore = useAppStore()
    authStore.user = { role: 'admin' } as User
    appStore.versionLoaded = true
    appStore.currentVersion = '0.1.183.1'
    appStore.latestVersion = '0.1.183.1'
    appStore.hasUpdate = false

    const wrapper = mount(VersionBadge, {
      props: {
        version: '0.1.183.1',
        upstreamVersion: '0.1.183',
      },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.get('button').attributes('title')).toContain('ModelPort v0.1.183.1')
    expect(wrapper.get('button').attributes('title')).toContain('Sub2API v0.1.183')

    await wrapper.get('button').trigger('click')

    expect(wrapper.text()).toContain('ModelPort Version')
    expect(wrapper.get('[data-testid="upstream-version"]').text()).toBe(
      'Based on Sub2API v0.1.183'
    )
  })

  it('does not offer online actions or legacy install.sh commands in manual mode', async () => {
    const authStore = useAuthStore()
    const appStore = useAppStore()
    authStore.user = { role: 'admin' } as User
    appStore.versionLoaded = true
    appStore.currentVersion = '0.1.183.1'
    appStore.latestVersion = '0.1.183.2'
    appStore.hasUpdate = true
    appStore.buildType = 'release'
    appStore.updateMode = 'manual'

    const wrapper = mount(VersionBadge, {
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('button').trigger('click')

    expect(wrapper.find('[data-testid="update-now"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Online updates are disabled')
    expect(wrapper.html()).not.toContain('install.sh')
    expect(wrapper.html()).not.toContain('ghcr.io/abingooo/modelport:custom-v')
  })

  it('shows the host-side queue state for Docker updates', async () => {
    vi.useFakeTimers()
    const authStore = useAuthStore()
    const appStore = useAppStore()
    authStore.user = { role: 'admin' } as User
    appStore.versionLoaded = true
    appStore.currentVersion = '0.1.183.1'
    appStore.latestVersion = '0.1.183.2'
    appStore.hasUpdate = true
    appStore.buildType = 'release'
    appStore.updateMode = 'docker'

    const wrapper = mount(VersionBadge, {
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('button').trigger('click')
    await wrapper.get('[data-testid="update-now"]').trigger('click')
    await flushPromises()

    expect(systemApiMocks.performUpdate).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="update-queued"]').text()).toContain('Update queued')
  })

  it('queues rollback through the digest-verifying host updater without manual tag commands', async () => {
    vi.useFakeTimers()
    systemApiMocks.getRollbackVersions.mockResolvedValue({
      versions: [{
        version: '0.1.183.0',
        published_at: '2026-08-01T00:00:00Z',
        html_url: 'https://github.com/abingooo/modelport/releases/tag/custom-v0.1.183.0',
      }],
    })
    const authStore = useAuthStore()
    const appStore = useAppStore()
    authStore.user = { role: 'admin' } as User
    appStore.versionLoaded = true
    appStore.currentVersion = '0.1.183.1'
    appStore.latestVersion = '0.1.183.1'
    appStore.hasUpdate = false
    appStore.buildType = 'release'
    appStore.updateMode = 'docker'

    const wrapper = mount(VersionBadge, {
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('button').trigger('click')
    await wrapper.get('[data-testid="rollback-toggle"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="rollback-version"]').trigger('click')

    expect(wrapper.html()).not.toContain('install.sh')
    expect(wrapper.html()).not.toContain('image: ghcr.io/abingooo/modelport:')
    await wrapper.get('[data-testid="rollback-confirm"]').trigger('click')
    await flushPromises()

    expect(systemApiMocks.rollback).toHaveBeenCalledWith('0.1.183.0')
    expect(wrapper.get('[data-testid="update-queued"]').exists()).toBe(true)
  })

  it('stops the host update poll when the version badge unmounts', async () => {
    vi.useFakeTimers()
    const authStore = useAuthStore()
    const appStore = useAppStore()
    authStore.user = { role: 'admin' } as User
    appStore.versionLoaded = true
    appStore.currentVersion = '0.1.183.1'
    appStore.latestVersion = '0.1.183.2'
    appStore.hasUpdate = true
    appStore.buildType = 'release'
    appStore.updateMode = 'docker'

    const wrapper = mount(VersionBadge, {
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('button').trigger('click')
    await wrapper.get('[data-testid="update-now"]').trigger('click')
    await flushPromises()

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(2000)
    expect(systemApiMocks.getVersion).not.toHaveBeenCalled()
  })

  it('closes the version panel when another page control is clicked', async () => {
    const authStore = useAuthStore()
    const appStore = useAppStore()
    authStore.user = { role: 'admin' } as User
    appStore.versionLoaded = true
    appStore.currentVersion = '0.1.183.1'
    appStore.latestVersion = '0.1.183.1'
    appStore.hasUpdate = false

    const outside = document.createElement('button')
    document.body.appendChild(outside)
    const wrapper = mount(VersionBadge, {
      attachTo: document.body,
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('button').trigger('click')
    expect(wrapper.find('#version-details-panel').exists()).toBe(true)

    outside.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.get('button[aria-controls="version-details-panel"]').attributes('aria-expanded')).toBe('false')

    wrapper.unmount()
    outside.remove()
  })
})
