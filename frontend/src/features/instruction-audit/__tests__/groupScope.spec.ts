import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import InstructionV2ScopePanel from '../components/InstructionV2ScopePanel.vue'
import type { InstructionClientProfile, InstructionGroupOption, InstructionScope } from '../v2Types'

const mocks = vi.hoisted(() => ({
  saveScopeSet: vi.fn(),
  deleteScopeSet: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('../v2Api', () => ({
  default: { saveScopeSet: mocks.saveScopeSet, deleteScopeSet: mocks.deleteScopeSet },
}))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }),
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const here = dirname(fileURLToPath(import.meta.url))
const read = (path: string) => readFileSync(resolve(here, path), 'utf8')

const BaseDialogStub = defineComponent({
  props: ['show'],
  template: '<div v-if="show" data-test="scope-dialog"><slot /><slot name="footer" /></div>',
})
const ConfirmDialogStub = defineComponent({
  props: ['show'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-test="confirm-dialog"><button data-test="confirm-delete" @click="$emit(\'confirm\')">confirm</button></div>',
})
const EmptyStub = { template: '<span />' }

function group(id: number): InstructionGroupOption {
  return { id, name: `group-${id}`, platform: 'openai', status: 'active' }
}

function client(id: number, enabled = true): InstructionClientProfile {
  return {
    id,
    profile_key: `client_${id}`,
    name: `client-${id}`,
    description: '',
    matchers: [],
    priority: id,
    enabled,
    built_in: false,
    immutable_internal: false,
    created_at: '2026-08-11T00:00:00Z',
    updated_at: '2026-08-11T00:00:00Z',
  }
}

function scope(id: number, groupId: number, clientProfileId: number | null, enabled = true): InstructionScope {
  return {
    id,
    group_id: groupId,
    group_name: `group-${groupId}`,
    group_platform: 'openai',
    group_status: 'active',
    client_profile_id: clientProfileId,
    client_profile_key: clientProfileId == null ? '' : `client_${clientProfileId}`,
    client_profile_name: clientProfileId == null ? '' : `client-${clientProfileId}`,
    enabled,
    effective: enabled,
    created_at: '2026-08-11T00:00:00Z',
    updated_at: '2026-08-11T00:00:00Z',
  }
}

function mountScopePanel(scopes: InstructionScope[], clients: InstructionClientProfile[]) {
  return mount(InstructionV2ScopePanel, {
    props: { scopes, clients, groups: [group(10), group(20)], allowlist: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        Icon: EmptyStub,
      },
    },
  })
}

describe('instruction audit group scope', () => {
  it('uses only group-scoped binding APIs', () => {
    const api = read('../api.ts')
    const types = read('../types.ts')
    expect(api).toContain('/group-bindings')
    expect(types).toContain('group_ids: number[]')
    expect(types).toContain('client_types?: InstructionClientType[]')
    expect(api).toContain('listGroups')
    expect(api).not.toContain('/bindings`')
    expect(api).not.toContain('/users')
  })

  it('binds an audit scope to one downstream group and a set of client profiles', () => {
    const view = read('../components/InstructionV2ScopePanel.vue')
    const api = read('../v2Api.ts')
    const types = read('../v2Types.ts')
    expect(view).toContain('scopeForm.groupId')
    expect(view).toContain('scopeForm.clientProfileIds')
    expect(view).toContain('instructionAuditV2API.saveScopeSet')
    expect(api).toContain('/scopes/batch')
    expect(types).toContain('group_id: number')
    expect(types).toContain('client_profile_ids: number[]')
    expect(types).toContain('all_clients: boolean')
    expect(view).not.toContain('client_profile_id: scopeForm.clientProfileId || null')
    expect(view).not.toContain('scopeForm.model')
  })

  it('manages the user allowlist independently from scope and model routing', () => {
    const view = read('../components/InstructionV2ScopePanel.vue')
    const api = read('../v2Api.ts')
    const types = read('../v2Types.ts')
    expect(view).toContain("{ value: 'allowlist' as const")
    expect(view).toContain('<template v-else>')
    expect(view).toContain('instructionAuditV2API.searchUsers')
    expect(view).toContain('instructionAuditV2API.saveUserAllowlist')
    expect(view).toContain('instructionAuditV2API.deleteUserAllowlist')
    expect(api).toContain('/user-allowlist')
    expect(types).toContain('InstructionUserAllowlistEntry')
  })

  it('supports editable prefix or regex client profiles with protected internal identity', () => {
    const view = read('../components/InstructionV2ScopePanel.vue')
    const types = read('../v2Types.ts')
    expect(view).toContain("matcher.type === 'prefix'")
    expect(view).toContain('matcher.case_sensitive')
    expect(view).not.toContain('if (client?.immutable_internal) return')
    expect(view).toContain('immutableInternal: client.immutable_internal')
    expect(view).toContain(':disabled="clientForm.immutableInternal"')
    expect(types).toContain("type: 'prefix' | 'regex'")
    expect(types).toContain('immutable_internal: boolean')
  })

  it('supports scoped or global trusted hashes without candidate promotion', () => {
    const view = read('../components/InstructionV2TrustedPanel.vue')
    const types = read('../v2Types.ts')
    expect(types).toContain('source: string')
    expect(types).toContain("source: 'manual' | 'import'")
    expect(types).toContain("'active' | 'disabled' | 'revoked'")
    expect(types).not.toContain("'candidate' | 'active' | 'disabled' | 'revoked'")
    expect(view).toContain('hash.global_trust')
    expect(view).toContain('scope_ids: form.globalTrust ? [] : form.scopeIds')
    expect(view).toContain('global_trust: form.globalTrust')
    expect(view).not.toContain("hash.status === 'candidate'")
  })

  it('places the global switch under feature switches risk control', () => {
    const settings = read('../../../views/admin/SettingsView.vue')
    const featuresStart = settings.indexOf('<!-- Tab: Features (功能开关) -->')
    const featuresEnd = settings.indexOf('<!-- /Tab: Features -->', featuresStart)
    const securityStart = settings.indexOf('<!-- Tab: Security — Registration, Turnstile, LinuxDo -->')
    const securityEnd = settings.indexOf('<!-- /Tab: Security — Registration, Turnstile, LinuxDo, OIDC -->', securityStart)
    expect(featuresStart).toBeGreaterThan(-1)
    expect(featuresEnd).toBeGreaterThan(featuresStart)
    expect(securityStart).toBeGreaterThan(-1)
    expect(securityEnd).toBeGreaterThan(securityStart)
    const features = settings.slice(featuresStart, featuresEnd)
    const security = settings.slice(securityStart, securityEnd)
    expect(features).toContain("admin.settings.features.riskControl.title")
    expect(features).toContain('requestInstructionAuditEnabled')
    expect(features).toContain('admin.settings.instructionAudit.enabled')
    expect(security).not.toContain('requestInstructionAuditEnabled')
  })
})

describe('InstructionV2ScopePanel grouped client bindings', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.saveScopeSet.mockResolvedValue([])
    mocks.deleteScopeSet.mockResolvedValue(undefined)
  })

  it('renders one card per group with every selected client', () => {
    const wrapper = mountScopePanel(
      [scope(101, 10, 1), scope(102, 10, 2), scope(201, 20, null)],
      [client(1), client(2)],
    )

    expect(wrapper.findAll('.resource-card')).toHaveLength(2)
    const firstGroup = wrapper.get('[data-test="scope-group-10"]')
    expect(firstGroup.get('[data-test="scope-client-summary"]').text()).toContain('client-1')
    expect(firstGroup.get('[data-test="scope-client-summary"]').text()).toContain('client-2')
    expect(wrapper.get('[data-test="scope-group-20"]').text()).toContain('admin.instructionAudit.v2.allClients')
  })

  it('preselects every existing client binding and keeps a disabled bound client visible', async () => {
    const wrapper = mountScopePanel(
      [scope(101, 10, 1), scope(102, 10, 2)],
      [client(1), client(2, false), client(3)],
    )

    await wrapper.findAll('button[title="common.edit"]')[0].trigger('click')

    expect(wrapper.get<HTMLSelectElement>('[data-test="scope-group"]').element.value).toBe('10')
    expect(wrapper.get('[data-test="scope-group"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get<HTMLInputElement>('[data-test="scope-client-1"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-test="scope-client-2"]').element.checked).toBe(true)
    expect(wrapper.get('[data-test="scope-client-2"]').element.closest('label')?.textContent).toContain('common.disabled')
    expect(wrapper.get<HTMLInputElement>('[data-test="scope-client-3"]').element.checked).toBe(false)
  })

  it('replaces a group client set with one saveScopeSet request', async () => {
    const wrapper = mountScopePanel(
      [scope(101, 10, 1), scope(102, 10, 2)],
      [client(1), client(2, false), client(3)],
    )
    await wrapper.findAll('button[title="common.edit"]')[0].trigger('click')
    await wrapper.get('[data-test="scope-client-3"]').setValue(true)

    await wrapper.get('[data-test="scope-dialog"] button.btn-primary').trigger('click')
    await flushPromises()

    expect(mocks.saveScopeSet).toHaveBeenCalledOnce()
    expect(mocks.saveScopeSet).toHaveBeenCalledWith({
      group_id: 10,
      client_profile_ids: [1, 2, 3],
      all_clients: false,
      enabled: true,
    })
    expect(wrapper.emitted('changed')).toHaveLength(1)
  })

  it('saves an all-clients binding with an empty client ID set', async () => {
    const wrapper = mountScopePanel([scope(103, 10, null)], [client(1), client(2)])
    await wrapper.get('button[title="common.edit"]').trigger('click')

    const allClients = wrapper.get<HTMLInputElement>('[data-test="scope-dialog"] fieldset input[type="checkbox"]')
    expect(allClients.element.checked).toBe(true)
    expect(wrapper.get('[data-test="scope-client-1"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="scope-dialog"] button.btn-primary').trigger('click')
    await flushPromises()

    expect(mocks.saveScopeSet).toHaveBeenCalledOnce()
    expect(mocks.saveScopeSet).toHaveBeenCalledWith({
      group_id: 10,
      client_profile_ids: [],
      all_clients: true,
      enabled: true,
    })
  })

  it('preserves all-clients and specific scopes together', async () => {
    const wrapper = mountScopePanel(
      [scope(103, 10, null), scope(104, 10, 1)],
      [client(1), client(2)],
    )
    const summary = wrapper.get('[data-test="scope-group-10"] [data-test="scope-client-summary"]')
    expect(summary.text()).toContain('admin.instructionAudit.v2.allClients')
    expect(summary.text()).toContain('client-1')

    await wrapper.get('button[title="common.edit"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('[data-test="scope-dialog"] fieldset input[type="checkbox"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-test="scope-client-1"]').element.checked).toBe(true)
    await wrapper.get('[data-test="scope-dialog"] button.btn-primary').trigger('click')
    await flushPromises()

    expect(mocks.saveScopeSet).toHaveBeenCalledWith({
      group_id: 10,
      client_profile_ids: [1],
      all_clients: true,
      enabled: true,
    })
  })

  it('requires acknowledgement before normalizing mixed enabled states', async () => {
    const wrapper = mountScopePanel(
      [scope(101, 10, 1, true), scope(102, 10, 2, false)],
      [client(1), client(2)],
    )
    expect(wrapper.get('[data-test="scope-group-10"]').text()).toContain('admin.instructionAudit.v2.mixedScopeStatus')

    await wrapper.get('button[title="common.edit"]').trigger('click')
    const saveButton = wrapper.get('[data-test="scope-dialog"] button.btn-primary')
    expect(wrapper.get('[data-test="mixed-scope-warning"]').exists()).toBe(true)
    expect(saveButton.attributes('disabled')).toBeDefined()
    await wrapper.get('[data-test="acknowledge-mixed-scope"]').setValue(true)
    expect(saveButton.attributes('disabled')).toBeUndefined()
    await saveButton.trigger('click')
    await flushPromises()

    expect(mocks.saveScopeSet).toHaveBeenCalledWith({
      group_id: 10,
      client_profile_ids: [1, 2],
      all_clients: false,
      enabled: false,
    })
  })

  it('deletes every client scope through the group endpoint', async () => {
    const wrapper = mountScopePanel(
      [scope(101, 10, 1), scope(102, 10, 2)],
      [client(1), client(2)],
    )

    await wrapper.get('[data-test="scope-group-10"] button[title="common.delete"]').trigger('click')
    await wrapper.get('[data-test="confirm-delete"]').trigger('click')
    await flushPromises()

    expect(mocks.deleteScopeSet).toHaveBeenCalledOnce()
    expect(mocks.deleteScopeSet).toHaveBeenCalledWith(10)
    expect(wrapper.emitted('changed')).toHaveLength(1)
  })
})
