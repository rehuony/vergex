import { activeLanguage, t } from '@/i18n'
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
        // Where the server names its wording, say it in the user's language.
        if (data.error_key) {
          const text = t(data.error_key, activeLanguage(), data.error_params)
          if (text !== data.error_key) message = text
        }
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

// What the server said went wrong, in the user's language where it names the wording.
export function serverError(
  result: {
    message?: string
    errorKey?: string
    errorParams?: Record<string, string>
  },
  fallback: string
): Error {
  const text =
    result.errorKey && t(result.errorKey, activeLanguage(), result.errorParams)
  return new Error(
    text && text !== result.errorKey ? text : result.message || fallback
  )
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
