import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import LotteryCampaignEditorDialog from '../LotteryCampaignEditorDialog.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }),
}))

function mountEditor() {
  return mount(LotteryCampaignEditorDialog, {
    props: {
      show: true, campaign: null, saving: false,
      subscriptionGroups: [{ id: 8, name: 'Pro Monthly', subscription_type: 'subscription' }] as any,
    },
    global: { stubs: {
      BaseDialog: { props: ['show', 'title'], template: '<section v-if="show"><slot/><footer><slot name="footer"/></footer></section>' },
      Icon: true,
    } },
  })
}

describe('LotteryCampaignEditorDialog', () => {
  it('validates the probability boundary and emits a normalized campaign payload', async () => {
    const wrapper = mountEditor()
    const textInputs = wrapper.findAll('input').filter((input) => input.attributes('type') === undefined)
    await textInputs[0].setValue('Port launch draw')
    await textInputs[1].setValue('Balance reward')

    const numberInputs = wrapper.findAll('input[type="number"]')
    const probability = numberInputs.find((input) => input.attributes('max') === '10000')!
    await probability.setValue('10001')
    expect(wrapper.text()).toContain('lottery.admin.editor.probabilityExceeded')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()

    await probability.setValue('2500')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    const payload = wrapper.emitted('save')?.[0]?.[0] as any
    expect(payload).toMatchObject({ name: 'Port launch draw', mode: 'instant', draw_at: null })
    expect(payload.prizes[0]).toMatchObject({ prize_type: 'balance', probability_bps: 2500, subscription_group_id: null })
  })
})
