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

  it('binds an audit scope to one downstream group and an optional client profile', () => {
    const view = read('../components/InstructionV2ScopePanel.vue')
    const types = read('../v2Types.ts')
    expect(view).toContain('scopeForm.groupId')
    expect(view).toContain('scopeForm.clientProfileId')
    expect(view).toContain('client_profile_id: scopeForm.clientProfileId || null')
    expect(types).toContain('group_id: number')
    expect(types).toContain('client_profile_id: number | null')
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
    expect(view).toContain('client?.immutable_internal')
    expect(types).toContain("type: 'prefix' | 'regex'")
    expect(types).toContain('immutable_internal: boolean')
  })

  it('keeps AI candidates scoped and requires explicit promotion', () => {
    const view = read('../components/InstructionV2TrustedPanel.vue')
    const types = read('../v2Types.ts')
    expect(types).toContain('source: string')
    expect(types).toContain("source: 'manual' | 'import'")
    expect(types).toContain("'candidate' | 'active' | 'disabled' | 'revoked'")
    expect(view).toContain("hash.status === 'candidate'")
    expect(view).toContain("instructionAuditV2API.updateHash(hash.id, { status: 'active' })")
    expect(view).toContain('scope_ids: form.scopeIds')
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
