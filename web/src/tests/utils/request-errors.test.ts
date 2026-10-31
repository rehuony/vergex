import { describe, expect, it } from 'vitest'
import { formatRequestError } from '@/utils/request-errors'

const protocolError =
  'failed to build trading context: request failed: Get "https://api.example.test/v1?secret=hidden": net/http: HTTP/1.x transport connection broken: malformed HTTP response "\\x00\\x00\\x12\\x04"'

describe('readable request errors', () => {
  it('replaces binary protocol dumps without leaking the request', () => {
    for (const error of [
      protocolError,
      new Error(protocolError),
      { message: protocolError },
    ]) {
      const text = formatRequestError(error)
      expect(text).toContain('network protocol error')
      expect(text).not.toMatch(/\\x00|https:|secret=|successful/)
      expect(text.length).toBeLessThan(300)
    }
    expect(formatRequestError(protocolError, 'zh')).toContain('网络协议异常')
  })

  it('preserves actionable ordinary errors and bounds unexpected output', () => {
    expect(formatRequestError(new Error('snapshot expired'))).toBe(
      'snapshot expired'
    )
    expect(formatRequestError('missing symbol')).toBe('missing symbol')
    expect(formatRequestError('long '.repeat(500)).length).toBeLessThanOrEqual(
      501
    )
    expect(formatRequestError(null)).toBe(
      'Request failed. Check the service status.'
    )
    expect(formatRequestError('bad\u0000response')).toBe('bad response')
  })
})
