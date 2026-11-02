function errorText(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (
    error &&
    typeof error === 'object' &&
    'message' in error &&
    typeof error.message === 'string'
  ) {
    return error.message
  }
  return ''
}

// Covers both current responses and stored decision records containing raw
// HTTP/2 frames. Displaying a readable error must never imply a successful
// call, nor modify the stored audit record.
export function formatRequestError(
  error: unknown,
  language: Language = 'en'
): string {
  const raw = errorText(error)
  const protocol =
    /malformed http response|http protocol error|http\/1\.x transport connection broken/i.test(
      raw
    )
  if (protocol || /(?:\\x[\da-f]{2}){2,}/i.test(raw)) {
    return t('requestProtocolError', language)
  }
  const text = raw
    .replace(/\p{Cc}/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!text) return t('requestFailed', language)
  return text.length > 500 ? `${text.slice(0, 500)}…` : text
}
import { t, type Language } from '@/i18n'

// What the server said in the user's language, where it gave a code the interface has
// wording for, and in the server's own words otherwise.
export function codedText(
  section: string,
  code: string | undefined,
  fallback: string,
  language: Language,
  params?: Record<string, string | number>
): string {
  if (!code) return fallback
  const key = `${section}.${code}`
  const text = t(key, language, params)
  return text === key ? fallback : text
}
