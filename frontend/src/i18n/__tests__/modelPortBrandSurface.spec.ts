import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('ModelPort user-visible brand surfaces', () => {
  it.each([
    ['zh', zh],
    ['en', en],
  ] as const)('%s setup, onboarding, and SMTP defaults do not expose the upstream brand', (_locale, messages) => {
    const brandMessages = [
      messages.setup.title,
      messages.setup.description,
      messages.onboarding.admin.welcome.title,
      messages.onboarding.admin.welcome.description,
      messages.onboarding.admin.groupManage.description,
      messages.onboarding.user.welcome.title,
      messages.onboarding.user.welcome.description,
      messages.admin.settings.smtp.fromNamePlaceholder,
    ]

    expect(messages.setup.title).toContain('ModelPort')
    expect(messages.setup.description).toContain('ModelPort')
    expect(messages.admin.settings.smtp.fromNamePlaceholder).toBe('{siteName}')
    for (const message of brandMessages) {
      expect(message).not.toContain('Sub2API')
    }
  })
})
