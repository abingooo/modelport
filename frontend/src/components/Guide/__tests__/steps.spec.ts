import { describe, expect, it } from 'vitest'

import { getAdminSteps, getUserSteps } from '../steps'

const messages: Record<string, string> = {
  'onboarding.admin.welcome.title': 'Welcome to {siteName}',
  'onboarding.admin.welcome.description': '<p>{siteName} admin</p>',
  'onboarding.admin.groupManage.description': '<p>{siteName} groups</p>',
  'onboarding.user.welcome.title': 'Welcome to {siteName}',
  'onboarding.user.welcome.description': '<p>{siteName} user</p>',
}

function t(key: string, named: Record<string, string> = {}): string {
  return (messages[key] ?? key).replace(/\{(\w+)\}/g, (_, name: string) => named[name] ?? `{${name}}`)
}

describe('ModelPort onboarding brand', () => {
  it('uses the configured site name without replacing it with ModelPort', () => {
    const adminSteps = getAdminSteps(t, false, 'Custom Gateway')
    const userSteps = getUserSteps(t, 'Custom Gateway')

    expect(adminSteps[0]?.popover?.title).toBe('Welcome to Custom Gateway')
    expect(adminSteps[1]?.popover?.description).toContain('Custom Gateway groups')
    expect(userSteps[0]?.popover?.title).toBe('Welcome to Custom Gateway')
  })

  it('falls back to ModelPort and escapes configured HTML before rendering rich tour copy', () => {
    expect(getAdminSteps(t)[0]?.popover?.title).toBe('Welcome to ModelPort')
    expect(getUserSteps(t, '<img src=x onerror=alert(1)>')[0]?.popover?.title).toBe(
      'Welcome to &lt;img src=x onerror=alert(1)&gt;'
    )
  })
})
