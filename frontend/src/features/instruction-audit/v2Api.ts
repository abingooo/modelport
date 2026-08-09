import { apiClient } from '@/api/client'
import type {
  InstructionAINode,
  InstructionAINodeTestResult,
  InstructionClientProfile,
  InstructionEvent,
  InstructionEventFilters,
  InstructionEventPage,
  InstructionEvidenceReview,
  InstructionGroupOption,
  InstructionHash,
  InstructionHashPage,
  InstructionScope,
  InstructionSensitiveCapability,
  InstructionStatistics,
  InstructionUserAllowlistEntry,
  InstructionUserOption,
  InstructionV2Config,
  SaveInstructionAINode,
  SaveInstructionClientProfile,
  SaveInstructionHash,
  SaveInstructionScope,
  UpdateInstructionHash,
  UpdateInstructionV2Config,
} from './v2Types'

const basePath = '/admin/instruction-audit'

export const instructionAuditV2API = {
  async getConfig(): Promise<InstructionV2Config> {
    const { data } = await apiClient.get<InstructionV2Config>(`${basePath}/config`)
    return data
  },
  async updateConfig(payload: UpdateInstructionV2Config): Promise<InstructionV2Config> {
    const { data } = await apiClient.put<InstructionV2Config>(`${basePath}/config`, payload)
    return data
  },
  async listEvents(params: InstructionEventFilters & { page: number; page_size: number }): Promise<InstructionEventPage> {
    const { data } = await apiClient.get<InstructionEventPage>(`${basePath}/events`, { params })
    return data
  },
  async getStatistics(params: InstructionEventFilters = {}): Promise<InstructionStatistics> {
    const { data } = await apiClient.get<InstructionStatistics>(`${basePath}/statistics`, { params })
    return data
  },
  async getEvent(id: number): Promise<InstructionEvent> {
    const { data } = await apiClient.get<InstructionEvent>(`${basePath}/events/${id}`)
    return data
  },
  async deleteEvent(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/events/${id}`)
  },
  async deleteEvents(ids: number[]): Promise<number> {
    const { data } = await apiClient.post<{ deleted: number }>(`${basePath}/events/batch-delete`, { ids })
    return data.deleted
  },
  async revealEventEvidence(id: number): Promise<InstructionEvidenceReview> {
    const { data } = await apiClient.get<InstructionEvidenceReview>(`${basePath}/events/${id}/evidence`)
    return data
  },
  async recordEventEvidenceCopy(id: number, fieldName: string): Promise<void> {
    await apiClient.post(`${basePath}/events/${id}/evidence-access`, { field_name: fieldName })
  },
  async trustEvent(id: number, fields: string[], name = '', note = ''): Promise<{ hashes: InstructionHash[] }> {
    const { data } = await apiClient.post<{ hashes: InstructionHash[] }>(`${basePath}/events/${id}/trust`, { fields, name, note })
    return data
  },
  async listHashes(params: { page: number; page_size: number; status?: string; q?: string }): Promise<InstructionHashPage> {
    const { data } = await apiClient.get<InstructionHashPage>(`${basePath}/hashes`, { params })
    return data
  },
  async getHash(id: number): Promise<InstructionHash> {
    const { data } = await apiClient.get<InstructionHash>(`${basePath}/hashes/${id}`)
    return data
  },
  async createHash(payload: SaveInstructionHash): Promise<InstructionHash> {
    const { data } = await apiClient.post<InstructionHash>(`${basePath}/hashes`, payload)
    return data
  },
  async updateHash(id: number, payload: UpdateInstructionHash): Promise<InstructionHash> {
    const { data } = await apiClient.put<InstructionHash>(`${basePath}/hashes/${id}`, payload)
    return data
  },
  async deleteHash(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/hashes/${id}`)
  },
  async revealHashRaw(id: number): Promise<InstructionEvidenceReview> {
    const { data } = await apiClient.get<InstructionEvidenceReview>(`${basePath}/hashes/${id}/raw`)
    return data
  },
  async recordHashRawCopy(id: number): Promise<void> {
    await apiClient.post(`${basePath}/hashes/${id}/raw-access`, {})
  },
  async listScopes(): Promise<InstructionScope[]> {
    const { data } = await apiClient.get<InstructionScope[]>(`${basePath}/scopes`)
    return data
  },
  async saveScope(id: number | null, payload: SaveInstructionScope): Promise<InstructionScope> {
    const request = id
      ? apiClient.put<InstructionScope>(`${basePath}/scopes/${id}`, payload)
      : apiClient.post<InstructionScope>(`${basePath}/scopes`, payload)
    const { data } = await request
    return data
  },
  async deleteScope(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/scopes/${id}`)
  },
  async listGroups(): Promise<InstructionGroupOption[]> {
    const { data } = await apiClient.get<InstructionGroupOption[]>(`${basePath}/groups`)
    return data
  },
  async listClientProfiles(): Promise<InstructionClientProfile[]> {
    const { data } = await apiClient.get<InstructionClientProfile[]>(`${basePath}/client-profiles`)
    return data
  },
  async saveClientProfile(id: number | null, payload: SaveInstructionClientProfile): Promise<InstructionClientProfile> {
    const request = id
      ? apiClient.put<InstructionClientProfile>(`${basePath}/client-profiles/${id}`, payload)
      : apiClient.post<InstructionClientProfile>(`${basePath}/client-profiles`, payload)
    const { data } = await request
    return data
  },
  async deleteClientProfile(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/client-profiles/${id}`)
  },
  async listUserAllowlist(): Promise<InstructionUserAllowlistEntry[]> {
    const { data } = await apiClient.get<InstructionUserAllowlistEntry[]>(`${basePath}/user-allowlist`)
    return data
  },
  async searchUsers(q: string): Promise<InstructionUserOption[]> {
    const { data } = await apiClient.get<InstructionUserOption[]>(`${basePath}/users`, { params: { q } })
    return data
  },
  async saveUserAllowlist(userId: number, note: string, enabled = true): Promise<InstructionUserAllowlistEntry> {
    const { data } = await apiClient.post<InstructionUserAllowlistEntry>(`${basePath}/user-allowlist`, {
      user_id: userId,
      note,
      enabled,
    })
    return data
  },
  async deleteUserAllowlist(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/user-allowlist/${id}`)
  },
  async listAINodes(): Promise<InstructionAINode[]> {
    const { data } = await apiClient.get<InstructionAINode[]>(`${basePath}/ai-nodes`)
    return data
  },
  async saveAINode(id: number | null, payload: SaveInstructionAINode): Promise<InstructionAINode> {
    const request = id
      ? apiClient.put<InstructionAINode>(`${basePath}/ai-nodes/${id}`, payload)
      : apiClient.post<InstructionAINode>(`${basePath}/ai-nodes`, payload)
    const { data } = await request
    return data
  },
  async deleteAINode(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/ai-nodes/${id}`)
  },
  async testAINode(id: number): Promise<InstructionAINodeTestResult> {
    const { data } = await apiClient.post<InstructionAINodeTestResult>(`${basePath}/ai-nodes/${id}/test`, {})
    return data
  },
  async getSensitiveCapability(): Promise<InstructionSensitiveCapability> {
    const { data } = await apiClient.get<InstructionSensitiveCapability>(`${basePath}/sensitive-access/me`)
    return data
  },
}

export default instructionAuditV2API
