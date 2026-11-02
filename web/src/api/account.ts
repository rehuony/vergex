import type {
  Account,
  AccountFill,
  AccountState,
  AccountStateResponse,
  CreateAccountRequest,
  Position,
  TraderOrder,
  UpdateAccountConfigRequest,
} from '@/types'
import { API_BASE, httpClient, sealCredentials, serverError } from './helpers'
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

  async renameAccount(accountId: string, name: string): Promise<void> {
    const result = await httpClient.put(
      `${API_BASE}/accounts/${encodeURIComponent(accountId)}/name`,
      { name }
    )
    if (!result.success) throw new Error('Failed to rename account')
  },

  /** Live mark prices by symbol. A coin is left out while none is known. */
  async getAccountMarks(
    accountId: string,
    symbols: string
  ): Promise<Record<string, number>> {
    const result = await httpClient.request<{
      marks: Record<string, number>
    }>(
      `${API_BASE}/accounts/${encodeURIComponent(accountId)}/marks?symbols=${encodeURIComponent(symbols)}`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to fetch mark prices')
    return result.data?.marks ?? {}
  },

  async getAccountOrders(
    accountId: string,
    limit: number,
    silent?: boolean
  ): Promise<TraderOrder[]> {
    const result = await httpClient.request<TraderOrder[]>(
      `${API_BASE}/accounts/${encodeURIComponent(accountId)}/orders?limit=${limit}`,
      { silent }
    )
    if (!result.success) throw serverError(result, 'Failed to fetch orders')
    return result.data ?? []
  },

  async getAccountFills(
    accountId: string,
    limit: number,
    silent?: boolean
  ): Promise<AccountFill[]> {
    const result = await httpClient.request<AccountFill[]>(
      `${API_BASE}/accounts/${encodeURIComponent(accountId)}/fills?limit=${limit}`,
      { silent }
    )
    if (!result.success) throw serverError(result, 'Failed to fetch fills')
    return result.data ?? []
  },

  async getAccountPositions(
    accountId: string,
    silent?: boolean
  ): Promise<Position[]> {
    const result = await httpClient.request<Record<string, unknown>[]>(
      `${API_BASE}/accounts/${encodeURIComponent(accountId)}/positions`,
      { silent }
    )
    if (!result.success) throw serverError(result, 'Failed to fetch positions')
    return result.data!.map(normalizePosition)
  },
}
