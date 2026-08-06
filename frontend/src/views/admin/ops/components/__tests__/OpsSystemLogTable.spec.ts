import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsSystemLogTable from '../OpsSystemLogTable.vue'
import enLocale from '@/i18n/locales/en'
import zhLocale from '@/i18n/locales/zh'

const mockListSystemLogs = vi.fn()
const mockCleanupSystemLogs = vi.fn()
const mockGetSystemLogSinkHealth = vi.fn()
const mockGetRuntimeLogConfig = vi.fn()
const mockRoute = vi.hoisted(() => ({ query: {} as Record<string, string> }))

vi.mock('vue-router', () => ({
  useRoute: () => mockRoute,
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listSystemLogs: (...args: any[]) => mockListSystemLogs(...args),
    cleanupSystemLogs: (...args: any[]) => mockCleanupSystemLogs(...args),
    getSystemLogSinkHealth: (...args: any[]) => mockGetSystemLogSinkHealth(...args),
    getRuntimeLogConfig: (...args: any[]) => mockGetRuntimeLogConfig(...args),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: {
      type: [String, Number],
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const PaginationStub = defineComponent({
  name: 'PaginationStub',
  template: '<div class="pagination-stub" />',
})

const runtimeConfig = {
  level: 'info',
  enable_sampling: false,
  sampling_initial: 100,
  sampling_thereafter: 100,
  caller: true,
  stacktrace_level: 'error',
  retention_days: 30,
}

const sinkHealth = {
  queue_depth: 0,
  queue_capacity: 5000,
  dropped_count: 0,
  write_failed_count: 0,
  written_count: 1,
  avg_write_delay_ms: 0,
}

describe('OpsSystemLogTable host support', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRoute.query = {}
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockListSystemLogs.mockResolvedValue({
      items: [
        {
          id: 1,
          created_at: '2026-07-14T00:10:01Z',
          host: 'api-node-1',
          level: 'warn',
          component: 'app',
          message: 'request failed',
          extra: { event_id: 77 },
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mockCleanupSystemLogs.mockResolvedValue({ deleted: 1 })
    mockGetSystemLogSinkHealth.mockResolvedValue(sinkHealth)
    mockGetRuntimeLogConfig.mockResolvedValue(runtimeConfig)
  })

  it('renders the host and sends it with list and cleanup filters', async () => {
    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('api-node-1')
    expect(wrapper.find('a[href*="event_id=77"]').exists()).toBe(true)

    const hostLabel = wrapper.findAll('label').find((label) => label.text().includes('admin.ops.systemLogs.host'))
    expect(hostLabel).toBeDefined()
    await hostLabel!.find('input').setValue(' api-node-2 ')

    const searchButton = wrapper.findAll('button').find((button) => button.text() === 'admin.ops.systemLogs.search')
    expect(searchButton).toBeDefined()
    await searchButton!.trigger('click')
    await flushPromises()

    expect(mockListSystemLogs).toHaveBeenLastCalledWith(expect.objectContaining({ host: 'api-node-2' }))

    const cleanupButton = wrapper.findAll('button').find((button) => button.text() === 'admin.ops.systemLogs.cleanCurrentFilters')
    expect(cleanupButton).toBeDefined()
    await cleanupButton!.trigger('click')
    await flushPromises()

    expect(mockCleanupSystemLogs).toHaveBeenCalledWith(expect.objectContaining({ host: 'api-node-2' }))
  })

  it('hydrates an instruction-event deep link into system-log filters', async () => {
    mockRoute.query = {
      system_log_q: '"event_id": 77',
      system_log_range: '30d',
    }

    mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub,
        },
      },
    })
    await flushPromises()

    expect(mockListSystemLogs).toHaveBeenCalledWith(expect.objectContaining({
      q: '"event_id": 77',
      time_range: '30d',
    }))
  })

  it.each([
    ['zh', zhLocale],
    ['en', enLocale],
  ])('defines the Host translation for %s', (_name, locale) => {
    expect(locale.admin.ops.systemLogs.host).toBe('Host')
  })
})
