import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en/admin/instructionAudit'
import zh from '@/i18n/locales/zh/admin/instructionAudit'
import {
  instructionEventReasonOptions,
  instructionNotificationStatuses,
} from '../v2Presentation'

describe('instruction audit V2 presentation contracts', () => {
  it('translates every persisted event reason in both locales', () => {
    const enReasons = en.instructionAudit.v2.reasons as Record<string, string>
    const zhReasons = zh.instructionAudit.v2.reasons as Record<string, string>

    for (const reason of instructionEventReasonOptions) {
      expect(enReasons[reason]).toBeTruthy()
      expect(zhReasons[reason]).toBeTruthy()
    }
  })

  it('translates every unified notification status in both locales', () => {
    const enStatuses = en.instructionAudit.v2.notifications as Record<string, string>
    const zhStatuses = zh.instructionAudit.v2.notifications as Record<string, string>

    for (const status of instructionNotificationStatuses) {
      expect(enStatuses[status]).toBeTruthy()
      expect(zhStatuses[status]).toBeTruthy()
    }
  })

  it('distinguishes sample evidence and displays both notification audiences', () => {
    const here = dirname(fileURLToPath(import.meta.url))
    const dialog = readFileSync(resolve(here, '../components/InstructionV2EvidenceDialog.vue'), 'utf8')

    expect(dialog).toContain("field.storage_kind === 'sample'")
    expect(dialog).toContain('sampleDigestNotApplicable')
    expect(dialog).toContain('detail.user_notification_status')
    expect(dialog).toContain('detail.ops_notification_status')
  })
})
