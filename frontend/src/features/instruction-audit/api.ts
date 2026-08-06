import { apiClient } from '@/api/client'
import type {
  InstructionGroupBinding,
  InstructionGroupOption,
  InstructionEvent,
  InstructionEventDeleteFilter,
  InstructionEventPage,
  InstructionEventFilters,
  InstructionDeletePreview,
  InstructionDeleteResult,
  AddInstructionEventToRuleSetResult,
  InstructionEvidenceReview,
  InstructionHashEntry,
  InstructionOverview,
  InstructionRuleSet,
  SaveInstructionGroupBindingsRequest,
  SaveInstructionHashRequest,
  SaveInstructionRuleSetRequest,
} from './types'

const basePath = '/admin/instruction-audit'

export const instructionAuditAPI = {
  async getOverview(): Promise<InstructionOverview> {
    const { data } = await apiClient.get<InstructionOverview>(`${basePath}/overview`)
    return data
  },

  async updateEnabled(enabled: boolean): Promise<InstructionOverview> {
    const { data } = await apiClient.put<InstructionOverview>(`${basePath}/enabled`, {
      enabled,
    })
    return data
  },

  async updateEvidenceRetention(days: number): Promise<InstructionOverview> {
    const { data } = await apiClient.put<InstructionOverview>(`${basePath}/evidence-retention`, { days })
    return data
  },

  async listHashes(status = ''): Promise<InstructionHashEntry[]> {
    const { data } = await apiClient.get<InstructionHashEntry[]>(`${basePath}/hashes`, {
      params: status ? { status } : undefined,
    })
    return data
  },

  async createHash(payload: SaveInstructionHashRequest): Promise<InstructionHashEntry> {
    const { data } = await apiClient.post<InstructionHashEntry>(`${basePath}/hashes`, payload)
    return data
  },

  async updateHash(id: number, payload: Partial<Omit<SaveInstructionHashRequest, 'digest'>> & {
    clear_valid_from?: boolean
    clear_valid_until?: boolean
  }): Promise<InstructionHashEntry> {
    const { data } = await apiClient.put<InstructionHashEntry>(`${basePath}/hashes/${id}`, payload)
    return data
  },

  async deleteHash(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/hashes/${id}`)
  },

  async listRuleSets(): Promise<InstructionRuleSet[]> {
    const { data } = await apiClient.get<InstructionRuleSet[]>(`${basePath}/rule-sets`)
    return data
  },

  async saveRuleSet(id: number | null, payload: SaveInstructionRuleSetRequest): Promise<InstructionRuleSet> {
    const request = id
      ? apiClient.put<InstructionRuleSet>(`${basePath}/rule-sets/${id}`, payload)
      : apiClient.post<InstructionRuleSet>(`${basePath}/rule-sets`, payload)
    const { data } = await request
    return data
  },

  async deleteRuleSet(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/rule-sets/${id}`)
  },

  async listGroupBindings(): Promise<InstructionGroupBinding[]> {
    const { data } = await apiClient.get<InstructionGroupBinding[]>(`${basePath}/group-bindings`)
    return data
  },

  async saveGroupBindings(payload: SaveInstructionGroupBindingsRequest): Promise<InstructionGroupBinding[]> {
    const { data } = await apiClient.post<InstructionGroupBinding[]>(`${basePath}/group-bindings`, payload)
    return data
  },

  async deleteGroupBinding(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/group-bindings/${id}`)
  },

  async listGroups(): Promise<InstructionGroupOption[]> {
    const { data } = await apiClient.get<InstructionGroupOption[]>(`${basePath}/groups`)
    return data
  },

  async listEvents(params: InstructionEventFilters & {
    page: number
    page_size: number
  }): Promise<InstructionEventPage> {
    const { data } = await apiClient.get<InstructionEventPage>(`${basePath}/events`, { params })
    return data
  },

  async getEvent(id: number): Promise<InstructionEvent> {
    const { data } = await apiClient.get<InstructionEvent>(`${basePath}/events/${id}`)
    return data
  },

  async deleteEvent(id: number): Promise<InstructionDeleteResult> {
    const { data } = await apiClient.delete<InstructionDeleteResult>(`${basePath}/events/${id}`)
    return data
  },

  async batchDeleteEvents(ids: number[]): Promise<InstructionDeleteResult> {
    const { data } = await apiClient.post<InstructionDeleteResult>(`${basePath}/events/batch-delete`, { ids })
    return data
  },

  async previewDeleteEvents(filter: InstructionEventDeleteFilter): Promise<InstructionDeletePreview> {
    const { data } = await apiClient.post<InstructionDeletePreview>(`${basePath}/events/delete-preview`, filter)
    return data
  },

  async deleteEventsByFilter(filter: InstructionEventDeleteFilter, preview: InstructionDeletePreview): Promise<InstructionDeleteResult> {
    const { data } = await apiClient.post<InstructionDeleteResult>(`${basePath}/events/delete-by-filter`, {
      filter,
      snapshot_max_id: preview.snapshot_max_id,
      filter_hash: preview.filter_hash,
      confirm: true,
    })
    return data
  },

  async revealEvidence(id: number): Promise<InstructionEvidenceReview> {
    const { data } = await apiClient.get<InstructionEvidenceReview>(`${basePath}/events/${id}/evidence`)
    return data
  },

  async recordEvidenceCopy(id: number, source: string): Promise<void> {
    await apiClient.post(`${basePath}/events/${id}/evidence-access`, { source })
  },

  async createCandidate(eventId: number, source: 'instructions' | 'input1'): Promise<InstructionHashEntry> {
    const { data } = await apiClient.post<InstructionHashEntry>(`${basePath}/events/${eventId}/candidates`, {
      source,
      review_confirmed: true,
    })
    return data
  },

  async addEventToRuleSet(
    eventId: number,
    ruleSetId: number,
    sources: Array<'instructions' | 'input1'>,
  ): Promise<AddInstructionEventToRuleSetResult> {
    const { data } = await apiClient.post<AddInstructionEventToRuleSetResult>(`${basePath}/events/${eventId}/rule-set`, {
      rule_set_id: ruleSetId,
      sources,
      review_confirmed: true,
    })
    return data
  },
}

export default instructionAuditAPI
