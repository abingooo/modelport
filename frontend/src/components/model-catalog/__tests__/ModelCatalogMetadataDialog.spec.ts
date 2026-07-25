import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelCatalogMetadataDialog from '../ModelCatalogMetadataDialog.vue'
import type { ModelCatalogItem } from '@/api/modelCatalog'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

function model(): ModelCatalogItem {
  return {
    metadata_id: 7,
    platform: 'anthropic',
    name: 'claude-sonnet-4-5',
    display_name: 'Claude Sonnet 4.5',
    description: 'Production model',
    capabilities: ['text', 'reasoning'],
    context_window: 200000,
    interface_formats: ['anthropic', 'openai'],
    scenarios: ['chat'],
    example_overrides: { anthropic: '  custom curl  ' },
    is_recommended: true,
    is_visible: true,
    sort_order: 20,
    available: true,
    offers: [],
  }
}

function mountDialog(item: ModelCatalogItem | null = null) {
  return mount(ModelCatalogMetadataDialog, {
    props: { show: true, item, saving: false },
    global: {
      stubs: {
        BaseDialog: {
          template: '<section><slot /><slot name="footer" /></section>',
        },
        Icon: true,
      },
    },
  })
}

describe('ModelCatalogMetadataDialog', () => {
  beforeEach(() => vi.clearAllMocks())

  it('normalizes create form lists and example overrides before saving', async () => {
    const wrapper = mountDialog()
    await wrapper.get('select').setValue('qwen')
    const textInputs = wrapper.findAll('input[type="text"], input:not([type])')
    await textInputs[0].setValue('qwen-max')
    await textInputs[1].setValue('Qwen Max')
    await textInputs[2].setValue('Text, reasoning, text,  Vision ')
    await textInputs[3].setValue('Chat, coding, chat')

    const formatInputs = wrapper.findAll('input[type="checkbox"]')
    await formatInputs[1].setValue(true)
    const examples = wrapper.findAll('textarea')
    await examples[1].setValue('  curl openai  ')
    await examples[2].setValue('   ')
    await wrapper.get('form').trigger('submit')

    const payload = wrapper.emitted('save')?.[0]?.[0]
    expect(payload).toMatchObject({
      platform: 'qwen',
      model_name: 'qwen-max',
      display_name: 'Qwen Max',
      capabilities: ['text', 'reasoning', 'vision'],
      scenarios: ['chat', 'coding'],
      example_overrides: { openai: 'curl openai' },
    })
  })

  it('loads existing metadata and preserves its identifier on save', async () => {
    const wrapper = mountDialog(model())
    expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('anthropic')

    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({
      id: 7,
      platform: 'anthropic',
      model_name: 'claude-sonnet-4-5',
      display_name: 'Claude Sonnet 4.5',
      capabilities: ['text', 'reasoning'],
      interface_formats: ['anthropic', 'openai'],
      example_overrides: { anthropic: 'custom curl' },
      is_recommended: true,
      is_visible: true,
    })
  })
})
