import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const read = (path: string) => readFileSync(resolve(here, path), 'utf8')

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

  it('provides searchable multi-group binding controls', () => {
    const view = read('../InstructionAuditView.vue')
    expect(view).toContain('filteredGroupOptions')
    expect(view).toContain('bindingDialog.groupIds')
    expect(view).toContain('type="checkbox"')
    expect(view).not.toContain('bindingDialog.userId')
    expect(view).not.toContain('bindingDialog.model')
  })

  it('supports editable multi-client scopes without trusting a user-agent internal marker', () => {
    const view = read('../InstructionAuditView.vue')
    expect(view).toContain("clientScope: 'all' as 'all' | 'selected'")
    expect(view).toContain('bindingDialog.clientTypes')
    expect(view).toContain('openBindingDialog(binding)')
    expect(view).toContain('modelport_internal')
    expect(view).toContain('trustedInternalIdentity')
    expect(view).toContain("addArrayFilter('clientTypes', event.client_type)")
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
