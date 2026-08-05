import { apiClient } from '@/api/client'
import type {
  InstructionGroupBinding,
  InstructionGroupOption,
  InstructionEvent,
  InstructionEventPage,
  InstructionEventFilters,
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
}

export default instructionAuditAPI
