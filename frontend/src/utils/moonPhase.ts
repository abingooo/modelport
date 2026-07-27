export type MoonPhaseName =
  | 'new-moon'
  | 'waxing-crescent'
  | 'first-quarter'
  | 'waxing-gibbous'
  | 'full-moon'
  | 'waning-gibbous'
  | 'last-quarter'
  | 'waning-crescent'

export interface MoonPhase {
  cycle: number
  illumination: number
  waxing: boolean
  name: MoonPhaseName
}

const SYNODIC_MONTH_DAYS = 29.530588853
const NEW_MOON_EPOCH = Date.UTC(2000, 0, 6, 18, 14)
const DAY_MS = 24 * 60 * 60 * 1000
const PHASE_NAMES: MoonPhaseName[] = [
  'new-moon',
  'waxing-crescent',
  'first-quarter',
  'waxing-gibbous',
  'full-moon',
  'waning-gibbous',
  'last-quarter',
  'waning-crescent',
]

export function getMoonPhase(date = new Date()): MoonPhase {
  const elapsedDays = (date.getTime() - NEW_MOON_EPOCH) / DAY_MS
  const rawCycle = elapsedDays / SYNODIC_MONTH_DAYS
  const cycle = ((rawCycle % 1) + 1) % 1
  const illumination = (1 - Math.cos(cycle * Math.PI * 2)) / 2
  const phaseIndex = Math.floor((cycle + 1 / 16) * 8) % 8

  return {
    cycle,
    illumination,
    waxing: cycle < 0.5,
    name: PHASE_NAMES[phaseIndex],
  }
}
