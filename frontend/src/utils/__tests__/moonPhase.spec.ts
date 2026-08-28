import { describe, expect, it } from 'vitest'
import { getMoonPhase } from '../moonPhase'

const NEW_MOON_EPOCH = Date.UTC(2000, 0, 6, 18, 14)
const SYNODIC_MONTH_MS = 29.530588853 * 24 * 60 * 60 * 1000

describe('getMoonPhase', () => {
  it('identifies the reference new moon', () => {
    const phase = getMoonPhase(new Date(NEW_MOON_EPOCH))

    expect(phase.name).toBe('new-moon')
    expect(phase.illumination).toBeLessThan(0.001)
    expect(phase.waxing).toBe(true)
  })

  it('distinguishes quarter and full moon phases', () => {
    const firstQuarter = getMoonPhase(new Date(NEW_MOON_EPOCH + SYNODIC_MONTH_MS / 4))
    const fullMoon = getMoonPhase(new Date(NEW_MOON_EPOCH + SYNODIC_MONTH_MS / 2))

    expect(firstQuarter.name).toBe('first-quarter')
    expect(firstQuarter.illumination).toBeCloseTo(0.5, 3)
    expect(fullMoon.name).toBe('full-moon')
    expect(fullMoon.illumination).toBeGreaterThan(0.999)
  })

  it('normalizes dates before the reference epoch', () => {
    const phase = getMoonPhase(new Date(NEW_MOON_EPOCH - SYNODIC_MONTH_MS / 4))

    expect(phase.cycle).toBeGreaterThanOrEqual(0)
    expect(phase.cycle).toBeLessThan(1)
    expect(phase.name).toBe('last-quarter')
  })
})
