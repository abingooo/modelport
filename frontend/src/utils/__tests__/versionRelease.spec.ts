import { describe, expect, it } from 'vitest'

import { resolveCurrentReleaseUrl } from '../versionRelease'

describe('resolveCurrentReleaseUrl', () => {
  it('links four-part custom versions to the ModelPort release', () => {
    expect(resolveCurrentReleaseUrl('0.1.164.3', 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.164')).toBe(
      'https://github.com/abingooo/modelport/releases/tag/custom-v0.1.164.3'
    )
  })

  it('keeps the official release for upstream versions', () => {
    const officialUrl = 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.164'
    expect(resolveCurrentReleaseUrl('0.1.164', officialUrl)).toBe(officialUrl)
  })

  it('does not link development versions to an official release', () => {
    expect(
      resolveCurrentReleaseUrl(
        '0.1.164.4-dev.1',
        'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.164'
      )
    ).toBe('')
  })

  it('does not expose placeholder release links', () => {
    expect(resolveCurrentReleaseUrl('0.1.164', '#')).toBe('')
  })
})
