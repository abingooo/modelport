import { extractApiErrorCode } from '@/utils/apiError'

const sensitiveAccessDenialCodes = new Set([
  'INSTRUCTION_SENSITIVE_ACCESS_REQUIRED',
  'INSTRUCTION_SENSITIVE_ACCESS_UNAVAILABLE',
  'INSTRUCTION_SENSITIVE_HUMAN_SESSION_REQUIRED',
  'STEP_UP_ADMIN_API_KEY_FORBIDDEN',
])

export function isInstructionSensitiveAccessDenied(error: unknown): boolean {
  const code = extractApiErrorCode(error)
  return Boolean(code && sensitiveAccessDenialCodes.has(code))
}
