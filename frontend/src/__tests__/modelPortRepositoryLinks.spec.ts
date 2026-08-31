import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const complianceStore = readFileSync(resolve(here, '../stores/adminCompliance.ts'), 'utf8')
const complianceDialog = readFileSync(
  resolve(here, '../components/admin/AdminComplianceDialog.vue'),
  'utf8'
)
const settingsView = readFileSync(resolve(here, '../views/admin/SettingsView.vue'), 'utf8')
const appHeader = readFileSync(resolve(here, '../components/layout/AppHeader.vue'), 'utf8')
const keyUsageView = readFileSync(resolve(here, '../views/KeyUsageView.vue'), 'utf8')

const upstreamDocsPrefix = 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/'
const modelPortDocsPrefix = 'https://github.com/abingooo/modelport/blob/main/docs/'

describe('ModelPort repository links', () => {
  it('keeps built-in legal and payment documentation inside the ModelPort repository', () => {
    for (const source of [complianceStore, complianceDialog, settingsView]) {
      expect(source).toContain(modelPortDocsPrefix)
      expect(source).not.toContain(upstreamDocsPrefix)
    }
  })

  it('keeps server-provided compliance document URLs ahead of repository fallbacks', () => {
    expect(complianceDialog).toContain(
      "complianceStore.status?.document_url_zh || 'https://github.com/abingooo/modelport/"
    )
    expect(complianceDialog).toContain(
      "complianceStore.status?.document_url_en || 'https://github.com/abingooo/modelport/"
    )
    expect(complianceStore).toContain(
      "partialStatus?.document_url_zh || status.value?.document_url_zh || 'https://github.com/abingooo/modelport/"
    )
    expect(complianceStore).toContain(
      "partialStatus?.document_url_en || status.value?.document_url_en || 'https://github.com/abingooo/modelport/"
    )
  })

  it('uses ModelPort for the built-in public repository command links', () => {
    for (const source of [appHeader, keyUsageView]) {
      expect(source).toContain('https://github.com/abingooo/modelport')
    }
  })
})
