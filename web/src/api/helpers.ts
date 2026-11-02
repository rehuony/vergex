import { CryptoService } from '@/utils/crypto'
import { httpClient } from './http-client'

export const API_BASE = '/api'

export { CryptoService, httpClient }

// Helper function to get auth headers
export function getAuthHeaders(): Record<string, string> {
  const token = localStorage.getItem('auth_token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  return headers
}

export async function handleJSONResponse<T>(res: Response): Promise<T> {
  const text = await res.text()
  if (!res.ok) {
    let message = text || res.statusText
    try {
      const data = text ? JSON.parse(text) : null
      if (data && typeof data === 'object') {
        message = data.error || data.message || message
      }
    } catch {
      /* ignore JSON parse errors */
    }
    throw new Error(message || 'Request failed')
  }
  if (!text) {
    return {} as T
  }
  return JSON.parse(text) as T
}

// A request body that carries an API key: plain JSON, or encrypted to the
// server's public key when transport encryption is on.
export async function sealCredentials(request: object): Promise<unknown> {
  const config = await CryptoService.fetchCryptoConfig()
  if (!config.transport_encryption) return request
  await CryptoService.initialize(await CryptoService.fetchPublicKey())
  return CryptoService.encryptSensitiveData(
    JSON.stringify(request),
    localStorage.getItem('user_id') || '',
    sessionStorage.getItem('session_id') || ''
  )
}
