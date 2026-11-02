import type {
  Account,
  AccountState,
  AccountStateResponse,
  CreateAccountRequest,
  Position,
  UpdateAccountConfigRequest,
} from '@/types'
import { API_BASE, httpClient, sealCredentials } from './helpers'
import { normalizePosition } from './trader'

export const accountApi = {
  async getAccounts(): Promise<Account[]> {
    const result = await httpClient.get<Account[]>(`${API_BASE}/accounts`)
    if (!result.success) throw new Error('Failed to fetch accounts')
    return result.data!
  },

  async getAccountStates(): Promise<AccountStateResponse> {
    const result = await httpClient.get<AccountStateResponse>(
      `${API_BASE}/accounts/state`
    )
    if (!result.success || !result.data) {
      throw new Error('Failed to fetch account states')
    }
    return result.data
  },

  /** Checks one account now, past the server's short cache. */
  async verifyAccount(accountId: string): Promise<AccountState> {
    const result = await httpClient.post<AccountState>(
      `${API_BASE}/accounts/${encodeURIComponent(accountId)}/verify`
    )
    if (!result.success || !result.data)
      throw new Error('Failed to verify account')
    return result.data
  },

  async createAccountEncrypted(
    request: CreateAccountRequest
  ): Promise<{ id: string }> {
    const result = await httpClient.post<{ id: string }>(
      `${API_BASE}/accounts`,
      await sealCredentials(request)
    )
    if (!result.success) throw new Error('Failed to create account')
    return result.data!
  },

  async deleteAccount(accountId: string): Promise<void> {
    const result = await httpClient.delete(`${API_BASE}/accounts/${accountId}`)
    if (!result.success) throw new Error('Failed to delete account')
  },

  async updateAccountsEncrypted(
    request: UpdateAccountConfigRequest
  ): Promise<void> {
    const result = await httpClient.put(
      `${API_BASE}/accounts`,
      await sealCredentials(request)
    )
    if (!result.success) throw new Error('Failed to update accounts')
  },

  async getAccountPositions(
    accountId: string,
    silent?: boolean
  ): Promise<Position[]> {
    const result = await httpClient.request<Record<string, unknown>[]>(
      `${API_BASE}/accounts/${encodeURIComponent(accountId)}/positions`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch positions')
    return result.data!.map(normalizePosition)
  },
}
