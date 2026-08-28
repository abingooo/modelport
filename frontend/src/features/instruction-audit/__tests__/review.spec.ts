import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const read = (path: string) => readFileSync(resolve(here, path), 'utf8')

describe('instruction audit evidence review', () => {
  it('exposes review, copy logging, retention, and confirmed candidate APIs', () => {
    const api = read('../api.ts')
    expect(api).toContain('/evidence-retention')
    expect(api).toContain('/evidence`')
    expect(api).toContain('/evidence-access`')
    expect(api).toContain('review_confirmed: true')
  })

  it('records copy intent before writing sensitive evidence to the clipboard', () => {
    const dialog = read('../InstructionEvidenceReviewDialog.vue')
    const record = dialog.indexOf('instructionAuditAPI.recordEvidenceCopy')
    const clipboard = dialog.indexOf('await copyToClipboard', record)
    expect(record).toBeGreaterThan(-1)
    expect(clipboard).toBeGreaterThan(record)
    expect(dialog.slice(Math.max(0, record - 80), record)).toContain('stepUp.run')
    expect(dialog).toContain('field.plaintext')
    expect(dialog).toContain('field.sha256')
    expect(dialog).toContain('field.digest_consistent')
    expect(dialog).toContain('reviewConfirmed')
    expect(dialog).toContain('client_type: props.event.client_type')
    expect(dialog).toContain('client_user_agent: props.event.client_user_agent')
    expect(dialog).toContain("copyValue('event_id', String(event.id))")
  })

  it('supports focused event filters and independent statistics queries', () => {
    const view = read('../components/InstructionV2EventsPanel.vue')
    for (const filter of [
      'filters.q', 'filters.eventId', 'filters.userId', 'filters.groupId', 'filters.clientKey',
      'filters.outcome', 'filters.reason', 'filters.aiResult', 'filters.model', 'filters.range',
    ]) {
      expect(view).toContain(filter)
    }
    expect(view).toContain("emit('filters-change', activeFilters)")
    expect(view).toContain('result.group_ids')
    expect(view).toContain('result.client_keys')
    expect(view).toContain('result.ai_results')
  })

  it('supports correlated events, guarded cleanup, resource deletion, and quick trust', () => {
    const api = read('../v2Api.ts')
    const view = read('../components/InstructionV2EventsPanel.vue')

    expect(api).toContain('deleteHash(id: number)')
    expect(api).toContain('deleteScope(id: number)')
    expect(api).toContain('deleteClientProfile(id: number)')
    expect(api).toContain('deleteUserAllowlist(id: number)')
    expect(api).toContain('/events/batch-delete')
    expect(api).toContain('/trust`')
    expect(view).toContain('result.event_id = filters.eventId')
    expect(view).toContain('instructionAuditV2API.trustEvent')
    expect(view).toContain('system_log_q')
    expect(view).not.toContain('min-w-[960px]')
  })

  it('requires plaintext for manual hashes and labels digest-only entries as imports', () => {
    const view = read('../components/InstructionV2TrustedPanel.vue')
    const types = read('../v2Types.ts')
    expect(types).toContain("source: 'manual' | 'import'")
    expect(view).toContain("source: form.inputMode === 'raw' ? 'manual' : 'import'")
    expect(view).toContain("inputMode: 'raw' as 'raw' | 'digest'")
  })
})
