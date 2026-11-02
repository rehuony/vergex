import type {
  AgentModel,
  ModelTest,
  TestModelConfigRequest,
  UpdateModelConfigRequest,
} from '@/types'
import { API_BASE, httpClient, sealCredentials } from './helpers'

export type AgentCostPeriod = 'today' | 'week' | 'month' | 'all'

export interface AgentCostTotals {
  total: number
  count: number
  unpriced_count: number
  prompt_tokens: number
  completion_tokens: number
}

export interface AgentCostSummary extends AgentCostTotals {
  by_model: Record<string, AgentCostTotals>
}

export const agentApi = {
  async getModelConfigs(): Promise<AgentModel[]> {
    const result = await httpClient.get<AgentModel[]>(
      `${API_BASE}/agent/models`
    )
    if (!result.success) throw new Error('Failed to fetch model configs')
    if (!Array.isArray(result.data)) return []
    return result.data
  },

  async getSupportedModels(): Promise<AgentModel[]> {
    const result = await httpClient.get<AgentModel[]>(
      `${API_BASE}/agent/providers`
    )
    if (!result.success) throw new Error('Failed to fetch supported models')
    return result.data!
  },

  async updateModelConfigs(request: UpdateModelConfigRequest): Promise<void> {
    const result = await httpClient.put(
      `${API_BASE}/agent/models`,
      await sealCredentials(request)
    )
    if (!result.success) throw new Error('Failed to update model configs')
  },

  // Sends one short prompt with the form's values; a blank key uses the saved
  // one. Resolves with ok=false and the provider's message when it fails.
  async testModelConfig(request: TestModelConfigRequest): Promise<ModelTest> {
    const result = await httpClient.request<ModelTest>(
      `${API_BASE}/agent/models/test`,
      {
        method: 'POST',
        data: await sealCredentials(request),
        // The server waits up to 30 s for the provider.
        timeout: 45000,
      }
    )
    if (!result.success || !result.data)
      throw new Error(result.message || 'Failed to test the model')
    return result.data
  },

  async getAgentCostSummary(
    period: AgentCostPeriod = 'today',
    silent?: boolean,
    traderId?: string
  ): Promise<AgentCostSummary> {
    const trader = traderId ? `&trader_id=${traderId}` : ''
    const result = await httpClient.request<AgentCostSummary>(
      `${API_BASE}/agent/costs/summary?period=${period}${trader}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch agent costs')
    return result.data!
  },
}
