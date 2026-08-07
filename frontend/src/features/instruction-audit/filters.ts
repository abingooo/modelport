import type { InstructionEventFilters, InstructionStatisticsFilters } from './types'

export function instructionStatisticsFilters(filters: InstructionEventFilters): InstructionStatisticsFilters {
  return {
    from: filters.from,
    to: filters.to,
    group_ids: filters.group_ids,
    user_id: filters.user_id,
    model: filters.model,
    client_types: filters.client_types,
    final_outcomes: filters.final_outcomes,
    final_reasons: filters.final_reasons || filters.reasons,
  }
}
