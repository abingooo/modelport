import type { InstructionAuditMode, InstructionEventOutcome, InstructionHashStatus } from './v2Types'

type Translate = (key: string, params?: Record<string, unknown>) => string

export const instructionEventReasonOptions = [
  'instructions_hash_match',
  'input1_hash_match',
  'fields_empty',
  'user_allowlist',
  'invalid_json',
  'config_unavailable',
  'ai_pass',
  'ai_rejected',
  'ai_reject',
  'ai_uncertain',
  'ai_error',
  'ai_queue_full',
  'persistence_error',
] as const

export const instructionNotificationStatuses = [
  'not_requested',
  'pending',
  'processing',
  'retry',
  'sent',
  'failed',
  'suppressed',
  'no_recipient',
  'enqueue_failed',
] as const

export function translatedValue(t: Translate, namespace: string, value: string): string {
  const key = `${namespace}.${value}`
  const translated = t(key)
  return translated === key ? value.split('_').join(' ') : translated
}

export function modeLabel(t: Translate, mode: InstructionAuditMode): string {
  return translatedValue(t, 'admin.instructionAudit.v2.modes', mode)
}

export function outcomeLabel(t: Translate, outcome: InstructionEventOutcome): string {
  return translatedValue(t, 'admin.instructionAudit.v2.outcomes', outcome)
}

export function reasonLabel(t: Translate, reason: string): string {
  return translatedValue(t, 'admin.instructionAudit.v2.reasons', reason || 'none')
}

export function aiResultLabel(t: Translate, result: string): string {
  return translatedValue(t, 'admin.instructionAudit.v2.aiResults', result || 'not_run')
}

export function fieldStateLabel(t: Translate, state: string): string {
  return translatedValue(t, 'admin.instructionAudit.v2.fieldStates', state || 'not_checked')
}

export function notificationLabel(t: Translate, status: string): string {
  return translatedValue(t, 'admin.instructionAudit.v2.notifications', status || 'not_requested')
}

export function hashStatusLabel(t: Translate, status: InstructionHashStatus): string {
  return translatedValue(t, 'admin.instructionAudit.v2.hashStatuses', status)
}

export function sourceLabel(t: Translate, source: string): string {
  return translatedValue(t, 'admin.instructionAudit.v2.sources', source || 'unknown')
}

export function modePill(mode: InstructionAuditMode): string {
  if (mode === 'enforce') return 'bg-primary-100 text-primary-800 dark:bg-primary-950/60 dark:text-primary-200'
  if (mode === 'observe') return 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

export function outcomePill(outcome: InstructionEventOutcome): string {
  if (outcome === 'blocked') return 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300'
  if (outcome === 'ai_pass') return 'bg-cyan-100 text-cyan-800 dark:bg-cyan-950/50 dark:text-cyan-200'
  if (outcome === 'hash_pass') return 'bg-primary-100 text-primary-800 dark:bg-primary-950/60 dark:text-primary-200'
  if (outcome === 'observe_allow') return 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'
  return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-200'
}

export function hashStatusPill(status: InstructionHashStatus): string {
  if (status === 'active') return 'bg-primary-100 text-primary-800 dark:bg-primary-950/60 dark:text-primary-200'
  if (status === 'candidate') return 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'
  if (status === 'revoked') return 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

export function formatAuditDate(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

export function formatAuditBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  const exponent = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const amount = value / (1024 ** exponent)
  return `${amount >= 10 || exponent === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[exponent]}`
}

export function compactDigest(value: string): string {
  if (!value) return '-'
  return value.length > 20 ? `${value.slice(0, 10)}...${value.slice(-8)}` : value
}
