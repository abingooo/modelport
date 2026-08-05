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
    const record = dialog.indexOf('await instructionAuditAPI.recordEvidenceCopy')
    const clipboard = dialog.indexOf('await copyToClipboard', record)
    expect(record).toBeGreaterThan(-1)
    expect(clipboard).toBeGreaterThan(record)
    expect(dialog).toContain('field.plaintext')
    expect(dialog).toContain('field.sha256')
    expect(dialog).toContain('field.digest_consistent')
    expect(dialog).toContain('reviewConfirmed')
    expect(dialog).toContain('client_type: props.event.client_type')
    expect(dialog).toContain('client_user_agent: props.event.client_user_agent')
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
})
