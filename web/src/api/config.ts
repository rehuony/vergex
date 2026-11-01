import type {
  AgentModel,
  Exchange,
  ExchangeAccountState,
  ExchangeAccountStateResponse,
  ModelTest,
  TestModelConfigRequest,
  UpdateModelConfigRequest,
  UpdateExchangeConfigRequest,
  CreateExchangeRequest,
} from '@/types'
import { API_BASE, httpClient, CryptoService } from './helpers'

// A request body that carries an API key: plain JSON, or encrypted to the
// server's public key when transport encryption is on.
async function sealCredentials(request: object): Promise<unknown> {
  const config = await CryptoService.fetchCryptoConfig()
  if (!config.transport_encryption) return request
  await CryptoService.initialize(await CryptoService.fetchPublicKey())
  return CryptoService.encryptSensitiveData(
    JSON.stringify(request),
    localStorage.getItem('user_id') || '',
    sessionStorage.getItem('session_id') || ''
  )
}

export const configApi = {
  async getModelConfigs(): Promise<AgentModel[]> {
    const result = await httpClient.get<AgentModel[]>(`${API_BASE}/models`)
    if (!result.success) throw new Error('Failed to fetch model configs')
    if (!Array.isArray(result.data)) return []
    return result.data
  },

  async getSupportedModels(): Promise<AgentModel[]> {
    const result = await httpClient.get<AgentModel[]>(
      `${API_BASE}/supported-models`
    )
    if (!result.success) throw new Error('Failed to fetch supported models')
    return result.data!
  },

  async getPromptTemplates(): Promise<string[]> {
    const res = await fetch(`${API_BASE}/prompt-templates`)
    if (!res.ok) throw new Error('Failed to fetch prompt templates')
    const data = await res.json()
    if (Array.isArray(data.templates)) {
      return data.templates.map((item: { name: string }) => item.name)
    }
    return []
  },

  async updateModelConfigs(request: UpdateModelConfigRequest): Promise<void> {
    const result = await httpClient.put(
      `${API_BASE}/models`,
      await sealCredentials(request)
    )
    if (!result.success) throw new Error('Failed to update model configs')
  },

  // Sends one short prompt with the form's values; a blank key uses the saved
  // one. Resolves with ok=false and the provider's message when it fails.
  async testModelConfig(request: TestModelConfigRequest): Promise<ModelTest> {
    const result = await httpClient.request<ModelTest>(
      `${API_BASE}/models/test`,
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

  async getExchangeConfigs(): Promise<Exchange[]> {
    const result = await httpClient.get<Exchange[]>(`${API_BASE}/exchanges`)
    if (!result.success) throw new Error('Failed to fetch exchange configs')
    return result.data!
  },

  async getExchangeAccountState(): Promise<ExchangeAccountStateResponse> {
    const result = await httpClient.get<ExchangeAccountStateResponse>(
      `${API_BASE}/exchanges/account-state`
    )
    if (!result.success || !result.data) {
      throw new Error('Failed to fetch exchange account states')
    }
    return result.data
  },

  /** Checks one account now, past the server's short cache. */
  async verifyExchangeAccount(
    exchangeId: string
  ): Promise<ExchangeAccountState> {
    const result = await httpClient.post<ExchangeAccountState>(
      `${API_BASE}/exchanges/${encodeURIComponent(exchangeId)}/verify`
    )
    if (!result.success || !result.data)
      throw new Error('Failed to verify exchange account')
    return result.data
  },

  async getSupportedExchanges(): Promise<Exchange[]> {
    const result = await httpClient.get<Exchange[]>(
      `${API_BASE}/supported-exchanges`
    )
    if (!result.success) throw new Error('Failed to fetch supported exchanges')
    return result.data!
  },

  async updateExchangeConfigs(
    request: UpdateExchangeConfigRequest
  ): Promise<void> {
    const result = await httpClient.put(`${API_BASE}/exchanges`, request)
    if (!result.success) throw new Error('Failed to update exchange configs')
  },

  async createExchange(
    request: CreateExchangeRequest
  ): Promise<{ id: string }> {
    const result = await httpClient.post<{ id: string }>(
      `${API_BASE}/exchanges`,
      request
    )
    if (!result.success) throw new Error('Failed to create exchange account')
    return result.data!
  },

  async createExchangeEncrypted(
    request: CreateExchangeRequest
  ): Promise<{ id: string }> {
    // Check if transport encryption is enabled
    const config = await CryptoService.fetchCryptoConfig()

    if (!config.transport_encryption) {
      // Transport encryption disabled, send plaintext
      const result = await httpClient.post<{ id: string }>(
        `${API_BASE}/exchanges`,
        request
      )
      if (!result.success) throw new Error('Failed to create exchange account')
      return result.data!
    }

    // Fetch RSA public key
    const publicKey = await CryptoService.fetchPublicKey()

    // Initialize crypto service
    await CryptoService.initialize(publicKey)

    // Get user info
    const userId = localStorage.getItem('user_id') || ''
    const sessionId = sessionStorage.getItem('session_id') || ''

    // Encrypt sensitive data
    const encryptedPayload = await CryptoService.encryptSensitiveData(
      JSON.stringify(request),
      userId,
      sessionId
    )

    // Send encrypted data
    const result = await httpClient.post<{ id: string }>(
      `${API_BASE}/exchanges`,
      encryptedPayload
    )
    if (!result.success) throw new Error('Failed to create exchange account')
    return result.data!
  },

  async deleteExchange(exchangeId: string): Promise<void> {
    const result = await httpClient.delete(
      `${API_BASE}/exchanges/${exchangeId}`
    )
    if (!result.success) throw new Error('Failed to delete exchange account')
  },

  async updateExchangeConfigsEncrypted(
    request: UpdateExchangeConfigRequest
  ): Promise<void> {
    // Check if transport encryption is enabled
    const config = await CryptoService.fetchCryptoConfig()

    if (!config.transport_encryption) {
      // Transport encryption disabled, send plaintext
      const result = await httpClient.put(`${API_BASE}/exchanges`, request)
      if (!result.success) throw new Error('Failed to update exchange configs')
      return
    }

    // Fetch RSA public key
    const publicKey = await CryptoService.fetchPublicKey()

    // Initialize crypto service
    await CryptoService.initialize(publicKey)

    // Get user info from localStorage
    const userId = localStorage.getItem('user_id') || ''
    const sessionId = sessionStorage.getItem('session_id') || ''

    // Encrypt sensitive data
    const encryptedPayload = await CryptoService.encryptSensitiveData(
      JSON.stringify(request),
      userId,
      sessionId
    )

    // Send encrypted data
    const result = await httpClient.put(
      `${API_BASE}/exchanges`,
      encryptedPayload
    )
    if (!result.success) throw new Error('Failed to update exchange configs')
  },

  async getServerIP(): Promise<{
    public_ip: string
    message: string
  }> {
    const result = await httpClient.get<{
      public_ip: string
      message: string
    }>(`${API_BASE}/server-ip`)
    if (!result.success) throw new Error('Failed to fetch server IP')
    return result.data!
  },
}
