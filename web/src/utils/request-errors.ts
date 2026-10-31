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
    return language === 'zh'
      ? '网络协议异常，未能读取响应。请检查客户端连接配置。'
      : 'A network protocol error prevented the response from being read. Check the client connection configuration.'
  }
  const text = raw
    .replace(/\p{Cc}/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!text)
    return language === 'zh'
      ? '请求失败，请检查服务状态。'
      : 'Request failed. Check the service status.'
  return text.length > 500 ? `${text.slice(0, 500)}…` : text
}
import type { Language } from '@/i18n/translations'
