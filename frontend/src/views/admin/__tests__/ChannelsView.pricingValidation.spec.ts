import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { PricingFormEntry } from '@/components/admin/channel/types'
import type { AdminGroup } from '@/types'
import ChannelsView from '../ChannelsView.vue'

const {
  createChannel,
  getAllGroups,
  getWebSearchEmulationConfig,
  listChannels,
  showError,
} = vi.hoisted(() => ({
  createChannel: vi.fn(),
  getAllGroups: vi.fn(),
  getWebSearchEmulationConfig: vi.fn(),
  listChannels: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channels: {
      list: listChannels,
      create: createChannel,
      update: vi.fn(),
      remove: vi.fn(),
      syncPricingModels: vi.fn(),
    },
    groups: {
      getAll: getAllGroups,
    },
    accounts: {
      list: vi.fn(),
      getById: vi.fn(),
    },
    settings: {
      getWebSearchEmulationConfig,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>',
})

const PricingEntryCardStub = defineComponent({
  name: 'PricingEntryCardStub',
  props: { entry: { type: Object, required: true } },
  emits: ['update', 'remove'],
  template: '<div class="pricing-entry-card-stub" />',
})

const openAIGroup = {
  id: 42,
  name: 'OpenAI group',
  platform: 'openai',
  rate_multiplier: 1,
  account_count: 1,
} as AdminGroup

function makePricingEntry(overrides: Partial<PricingFormEntry>): PricingFormEntry {
  return {
    models: ['sora-2'],
    billing_mode: 'video',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    user_visible: false,
    intervals: [],
    ...overrides,
  }
}

function mountView(): VueWrapper {
  return mount(ChannelsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        TablePageLayout: {
          template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>',
        },
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        Icon: true,
        PlatformIcon: true,
        Toggle: true,
        PricingEntryCard: PricingEntryCardStub,
      },
    },
  })
}

async function configureAccountStatsPricing(
  wrapper: VueWrapper,
  entry: PricingFormEntry,
): Promise<void> {
  const createButton = wrapper
    .findAll('button')
    .find(button => button.text().includes('admin.channels.createChannel'))
  expect(createButton).toBeTruthy()
  await createButton!.trigger('click')
  await flushPromises()

  await wrapper.get('#channel-form input[required]').setValue('Video channel')

  const platformLabel = wrapper
    .findAll('label')
    .find(label => label.text().includes('admin.groups.platforms.openai'))
  expect(platformLabel).toBeTruthy()
  await platformLabel!.get('input[type="checkbox"]').setValue(true)
  await nextTick()

  const groupLabel = wrapper
    .findAll('label')
    .find(label => label.text().includes(openAIGroup.name))
  expect(groupLabel).toBeTruthy()
  await groupLabel!.get('input[type="checkbox"]').setValue(true)

  const addRuleButton = wrapper
    .findAll('button')
    .find(button => button.text().includes('admin.channels.form.addRule'))
  expect(addRuleButton).toBeTruthy()
  await addRuleButton!.trigger('click')

  const addPricingButtons = wrapper
    .findAll('button')
    .filter(button => button.text().includes('common.add'))
  expect(addPricingButtons.length).toBeGreaterThan(1)
  await addPricingButtons.at(-1)!.trigger('click')
  await nextTick()

  const pricingCards = wrapper.findAllComponents(PricingEntryCardStub)
  expect(pricingCards).toHaveLength(1)
  pricingCards[0].vm.$emit('update', entry)
  await nextTick()
}

describe('ChannelsView account stats pricing submission validation', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    listChannels.mockResolvedValue({ items: [], total: 0 })
    getAllGroups.mockResolvedValue([openAIGroup])
    getWebSearchEmulationConfig.mockResolvedValue({ enabled: false, providers: [] })
    createChannel.mockResolvedValue({})
  })

  it.each([
    {
      name: 'an empty model list',
      entry: makePricingEntry({ models: [], billing_mode: 'token' }),
      error: 'admin.channels.emptyModelsInPricing',
    },
    {
      name: 'conflicting model patterns',
      entry: makePricingEntry({ models: ['gpt-*', 'gpt-5'], billing_mode: 'token' }),
      error: 'admin.channels.modelConflict',
    },
    {
      name: 'video billing without a fallback or tier price',
      entry: makePricingEntry({}),
      error: 'admin.channels.form.perRequestPriceRequired',
    },
    {
      name: 'a negative top-level price',
      entry: makePricingEntry({ billing_mode: 'token', input_price: -1 }),
      error: 'admin.groups.platforms.openai - sora-2: admin.channels.priceValidation.negative',
    },
    {
      name: 'a non-finite top-level price',
      entry: makePricingEntry({ billing_mode: 'token', input_price: Number.NaN }),
      error: 'admin.groups.platforms.openai - sora-2: admin.channels.priceValidation.finite',
    },
    {
      name: 'an interval without any price',
      entry: makePricingEntry({
        intervals: [{
          min_tokens: 0,
          max_tokens: null,
          tier_label: '1080p',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          per_request_price: null,
          sort_order: 0,
        }],
      }),
      error: 'admin.groups.platforms.openai - sora-2: admin.channels.intervalValidation.missingPrice',
    },
  ])('blocks $name before the create request', async ({ entry, error }) => {
    const wrapper = mountView()
    await flushPromises()
    await configureAccountStatsPricing(wrapper, entry)

    await wrapper.get('#channel-form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(error)
    expect(createChannel).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('preserves an explicit zero video price in an account stats rule', async () => {
    const wrapper = mountView()
    await flushPromises()
    await configureAccountStatsPricing(
      wrapper,
      makePricingEntry({ per_request_price: 0 }),
    )

    await wrapper.get('#channel-form').trigger('submit')
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    expect(createChannel).toHaveBeenCalledTimes(1)
    expect(createChannel.mock.calls[0][0].account_stats_pricing_rules[0].pricing[0])
      .toMatchObject({
        models: ['sora-2'],
        billing_mode: 'video',
        per_request_price: 0,
      })
    wrapper.unmount()
  })
})
