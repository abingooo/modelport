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
  InstructionReviewJob,
  InstructionReviewJobPage,
  InstructionRiskAction,
  InstructionRiskActionResult,
  InstructionRiskHash,
  InstructionRiskHashPage,
  InstructionScope,
  InstructionStatistics,
  InstructionUserAllowlistEntry,
  InstructionUserOption,
  InstructionV2Config,
  SaveInstructionAINode,
  SaveInstructionClientProfile,
  SaveInstructionHash,
  SaveInstructionRiskHash,
  SaveInstructionScope,
  SaveInstructionScopeSet,
  UpdateInstructionHash,
  UpdateInstructionV2Config,
} from './v2Types'

const basePath = '/admin/instruction-audit'

function normalizeInstructionHash(hash: InstructionHash): InstructionHash {
  return {
    ...hash,
    source_user_email: hash.source_user_email || '',
    scope_ids: Array.isArray(hash.scope_ids) ? hash.scope_ids : [],
    scopes: Array.isArray(hash.scopes) ? hash.scopes : [],
  }
}

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
  async trustEvent(id: number, fields: string[], name = '', note = '', globalTrust = false): Promise<{ hashes: InstructionHash[] }> {
    const { data } = await apiClient.post<{ hashes: InstructionHash[] }>(`${basePath}/events/${id}/trust`, { fields, name, note, global_trust: globalTrust })
    return { ...data, hashes: Array.isArray(data.hashes) ? data.hashes.map(normalizeInstructionHash) : [] }
  },
  async listHashes(params: { page: number; page_size: number; status?: string; q?: string }): Promise<InstructionHashPage> {
    const { data } = await apiClient.get<InstructionHashPage>(`${basePath}/hashes`, { params })
    return { ...data, items: Array.isArray(data.items) ? data.items.map(normalizeInstructionHash) : [] }
  },
  async getHash(id: number): Promise<InstructionHash> {
    const { data } = await apiClient.get<InstructionHash>(`${basePath}/hashes/${id}`)
    return normalizeInstructionHash(data)
  },
  async createHash(payload: SaveInstructionHash): Promise<InstructionHash> {
    const { data } = await apiClient.post<InstructionHash>(`${basePath}/hashes`, payload)
    return normalizeInstructionHash(data)
  },
  async updateHash(id: number, payload: UpdateInstructionHash): Promise<InstructionHash> {
    const { data } = await apiClient.put<InstructionHash>(`${basePath}/hashes/${id}`, payload)
    return normalizeInstructionHash(data)
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
  async listRiskHashes(params: { page: number; page_size: number; status?: string; q?: string }): Promise<InstructionRiskHashPage> {
    const { data } = await apiClient.get<InstructionRiskHashPage>(`${basePath}/risk-hashes`, { params })
    return data
  },
  async getRiskHash(id: number): Promise<InstructionRiskHash> {
    const { data } = await apiClient.get<InstructionRiskHash>(`${basePath}/risk-hashes/${id}`)
    return data
  },
  async createRiskHash(payload: SaveInstructionRiskHash): Promise<InstructionRiskHash> {
    const { data } = await apiClient.post<InstructionRiskHash>(`${basePath}/risk-hashes`, payload)
    return data
  },
  async updateRiskHash(id: number, action: InstructionRiskAction): Promise<InstructionRiskActionResult> {
    const { data } = await apiClient.put<InstructionRiskActionResult>(`${basePath}/risk-hashes/${id}`, { action })
    return data
  },
  async deleteRiskHash(id: number): Promise<void> {
    await apiClient.delete(`${basePath}/risk-hashes/${id}`)
  },
  async revealRiskHashRaw(id: number): Promise<InstructionEvidenceReview> {
    const { data } = await apiClient.get<InstructionEvidenceReview>(`${basePath}/risk-hashes/${id}/raw`)
    return data
  },
  async recordRiskHashRawCopy(id: number): Promise<void> {
    await apiClient.post(`${basePath}/risk-hashes/${id}/raw-access`, {})
  },
  async listReviewJobs(params: { page: number; page_size: number; status?: string; q?: string }): Promise<InstructionReviewJobPage> {
    const { data } = await apiClient.get<InstructionReviewJobPage>(`${basePath}/review-jobs`, { params })
    return data
  },
  async getReviewJob(id: number): Promise<InstructionReviewJob> {
    const { data } = await apiClient.get<InstructionReviewJob>(`${basePath}/review-jobs/${id}`)
    return data
  },
  async retryReviewJob(id: number): Promise<void> {
    await apiClient.post(`${basePath}/review-jobs/${id}/retry`, {})
  },
  async revealReviewJobRaw(id: number): Promise<InstructionEvidenceReview> {
    const { data } = await apiClient.get<InstructionEvidenceReview>(`${basePath}/review-jobs/${id}/raw`)
    return data
  },
  async recordReviewJobRawCopy(id: number): Promise<void> {
    await apiClient.post(`${basePath}/review-jobs/${id}/raw-access`, {})
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
  async saveScopeSet(payload: SaveInstructionScopeSet): Promise<InstructionScope[]> {
    const { data } = await apiClient.post<InstructionScope[]>(`${basePath}/scopes/batch`, payload)
    return data
  },
  async deleteScopeSet(groupId: number): Promise<void> {
    await apiClient.delete(`${basePath}/scopes/group/${groupId}`)
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
}

export default instructionAuditV2API
