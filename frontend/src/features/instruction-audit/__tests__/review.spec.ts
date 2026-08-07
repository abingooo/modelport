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

  it('supports searchable, multi-value, URL-persisted audit filters', () => {
    const view = read('../InstructionAuditView.vue')
    for (const filter of [
      'groupIds', 'clientTypes', 'reasons', 'instructionsResults', 'input1Results',
      'userNotifications', 'opsNotifications',
    ]) {
      expect(view).toContain(filter)
    }
    expect(view).toContain('syncEventFilterURL')
    expect(view).toContain('hydrateEventFiltersFromURL')
    expect(view).toContain('type="datetime-local"')
  })

  it('supports correlated events, guarded cleanup, resource deletion, and quick rule creation', () => {
    const api = read('../api.ts')
    const view = read('../InstructionAuditView.vue')

    expect(api).toContain('deleteHash(id: number)')
    expect(api).toContain('deleteRuleSet(id: number)')
    expect(api).toContain('/events/batch-delete')
    expect(api).toContain('/events/delete-preview')
    expect(api).toContain('/events/delete-by-filter')
    expect(api).toContain('/rule-set`')
    expect(view).toContain('event_id: eventFilters.eventId || undefined')
    expect(view).toContain('snapshot_max_id')
    expect(api).toContain('filter_hash: preview.filter_hash')
    expect(view).toContain('openAddToRuleSetDialog(event)')
    expect(view).toContain('system_log_q')
    expect(view).not.toContain('min-w-[960px]')
  })
})
