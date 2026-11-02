import { describe, expect, it } from 'vitest'
import { handleJSONResponse } from '@/api/helpers'
import {
  haltReasonText,
  safeModeReasonText,
} from '@/components/trader/halt-reason'
import {
  codedText,
  formatRequestError,
  eventErrorText,
} from '@/utils/request-errors'

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
    expect(formatRequestError(protocolError, 'zh-CN')).toContain('网络协议异常')
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
      'Request failed. Check the service status'
    )
    expect(formatRequestError('bad\u0000response')).toBe('bad response')
  })
})

describe('wording from the server', () => {
  it("uses the wording for a code, and the server's words without one", () => {
    expect(
      codedText('accountError', 'INVALID_CREDENTIALS', 'raw', 'zh-CN')
    ).toBe('账户密钥无效')
    expect(
      codedText('accountError', 'EXCHANGE_UNAVAILABLE', 'raw', 'zh-CN')
    ).toBe('raw')
    expect(codedText('accountError', undefined, 'raw', 'zh-CN')).toBe('raw')
  })

  it('turns a refusal the server named into the language of the page', async () => {
    document.documentElement.lang = 'zh-CN'
    const refusal = new Response(
      JSON.stringify({
        error: 'Net reward/risk 0.5 is below minimum 1',
        error_key: 'futures.rejection.rewardRisk',
        error_params: { ratio: '0.5', minimum: '1' },
      }),
      { status: 400 }
    )
    await expect(handleJSONResponse(refusal)).rejects.toThrow(
      '扣除手续费和滑点后，净盈亏比 0.5 低于最低要求 1'
    )
    // A key with no wording keeps what the server said.
    const unknown = new Response(
      JSON.stringify({
        error: 'Plain',
        error_key: 'futures.rejection.nothing',
      }),
      { status: 400 }
    )
    await expect(handleJSONResponse(unknown)).rejects.toThrow('Plain')
    document.documentElement.lang = 'en'
  })

  it('reads why a trader stopped in the language of the page', () => {
    expect(haltReasonText('max drawdown exceeded: 12.30%', 'zh-CN')).toBe(
      '权益从峰值回撤 12.30%，超过上限'
    )
    expect(haltReasonText('daily loss limit exceeded: 5.00%', 'en')).toBe(
      'Lost 5.00% today, past the limit'
    )
    expect(haltReasonText('something else', 'zh-CN')).toBe('something else')
    expect(
      safeModeReasonText('Model failed 3 consecutive times: timeout', 'zh-CN')
    ).toBe('模型连续 3 次失败：timeout')
  })
})

describe('eventErrorText', () => {
  it('words a coded failure in the language in use, with the provider line after it', () => {
    const data = {
      message: 'The provider does not offer this model.',
      error_key: 'agentError.model_not_found',
      detail: 'HTTP 400: you passed x',
    }
    expect(eventErrorText(data, 'zh-CN')).toBe(
      '服务商没有这个模型。请在 设置 → Agent 模型 检查名称 (HTTP 400: you passed x)'
    )
    expect(eventErrorText({ ...data, detail: '' }, 'en')).toBe(
      'The provider does not offer this model. Check its name in Settings → Agent models'
    )
  })

  it("keeps the server's words where it gave no code it has wording for", () => {
    expect(
      eventErrorText({ message: 'Planning ran out of time' }, 'zh-CN')
    ).toBe('Planning ran out of time')
    expect(
      eventErrorText(
        { message: 'Other', error_key: 'agentError.unknown' },
        'en'
      )
    ).toBe('Other')
  })
})
