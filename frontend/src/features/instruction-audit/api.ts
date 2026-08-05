import { apiClient } from '@/api/client'
import type {
  InstructionBinding,
  InstructionEvent,
  InstructionEventPage,
  InstructionHashEntry,
  InstructionOverview,
  InstructionRuleSet,
  InstructionUserOption,
  SaveInstructionBindingRequest,
  SaveInstructionHashRequest,
  SaveInstructionRuleSetRequest,
} from './types'

const basePath = '/admin/instruction-audit'

export const instructionAuditAPI = {
  async getOverview(): Promise<InstructionOverview> {
    const { data } = await apiClient.get<InstructionOverview>(`${basePath}/overview`)
    return data
  },

  async updateEnabled(enabled: boolean, confirmNoRules = false): Promise<InstructionOverview> {
    const { data } = await apiClient.put<InstructionOverview>(`${basePath}/enabled`, {
      enabled,
      confirm_no_rules: confirmNoRules,
    })
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

  async listBindings(): Promise<InstructionBinding[]> {
    const { data } = await apiClient.get<InstructionBinding[]>(`${basePath}/bindings`)
    return data
  },

  async saveBinding(payload: SaveInstructionBindingRequest): Promise<InstructionBinding> {
    const { data } = await apiClient.post<InstructionBinding>(`${basePath}/bindings`, payload)
    return data
  },

  async deleteBinding(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/bindings/${id}`)
  },

  async searchUsers(query: string): Promise<InstructionUserOption[]> {
    const { data } = await apiClient.get<InstructionUserOption[]>(`${basePath}/users`, {
      params: { q: query },
    })
    return data
  },

  async listEvents(params: {
    page: number
    page_size: number
    user_id?: number
    model?: string
  }): Promise<InstructionEventPage> {
    const { data } = await apiClient.get<InstructionEventPage>(`${basePath}/events`, { params })
    return data
  },

  async getEvent(id: number): Promise<InstructionEvent> {
    const { data } = await apiClient.get<InstructionEvent>(`${basePath}/events/${id}`)
    return data
  },

  async createCandidate(eventId: number, source: 'instructions' | 'input1'): Promise<InstructionHashEntry> {
    const { data } = await apiClient.post<InstructionHashEntry>(`${basePath}/events/${eventId}/candidates`, {
      source,
    })
    return data
  },
}

export default instructionAuditAPI
