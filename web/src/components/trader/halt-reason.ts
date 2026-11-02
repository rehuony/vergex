import { t, type Language } from '@/i18n'

// The server writes why a trader stopped in English, in a few set forms.

export function haltReasonText(reason: string, language: Language): string {
  const match = /^(max drawdown|daily loss limit) exceeded: (.+)$/.exec(reason)
  if (!match) return reason
  return t(
    match[1] === 'max drawdown'
      ? 'traderDetail.haltMaxDrawdown'
      : 'traderDetail.haltDailyLoss',
    language,
    { percent: match[2] }
  )
}

export function safeModeReasonText(reason: string, language: Language): string {
  const match = /^Model failed (\d+) consecutive times: (.*)$/s.exec(reason)
  if (!match) return reason
  return t('traderDetail.safeModeModelFailed', language, {
    count: match[1],
    detail: match[2],
  })
}
