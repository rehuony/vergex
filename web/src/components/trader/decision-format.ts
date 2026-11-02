import { formatRequestError } from '@/utils/request-errors'
import type { DecisionRecord } from '@/types'

// How a cycle's stored records read in the decision log.

export type LogTone = 'ok' | 'warn' | 'risk' | 'info'

// Classify an execution-log line by what happened to the order.
export function logTone(line: string): LogTone {
  const s = line.toLowerCase()
  if (s.includes('succeed') || s.includes('success') || line.includes('✓'))
    return 'ok'
  if (
    s.includes('throttle') ||
    s.includes('re-entry') ||
    s.includes('cooldown') ||
    s.includes('blocked')
  )
    return 'warn'
  if (
    s.includes('risk') ||
    s.includes('fail') ||
    s.includes('reject') ||
    s.includes('error') ||
    s.includes('denied') ||
    line.includes('✗') ||
    line.includes('❌')
  )
    return 'risk'
  return 'info'
}

// Tidy a verbose log line to its gist without inventing anything: drop the
// glyph the backend prefixes and the symbol the row already shows, and shorten
// the throttle wording. Anything else passes through.
export function cleanLog(raw: string): string {
  let s = raw
    .replace(/^[\s└>•·]*(?:[✓✗\p{Extended_Pictographic}]️?)?\s*/u, '')
    .trim()
  const throttle = s.match(
    /closed\s+([0-9smhd.]+)\s+ago;\s*wait\s+([0-9smhd.]+)/i
  )
  if (throttle) {
    const ago = throttle[1].replace(/(\d)0s$/, '$1').replace(/0s$/, '')
    const wait = throttle[2].replace(/(\d)0s$/, '$1').replace(/0s$/, '')
    return `throttle · closed ${ago} ago, wait ${wait}`
  }
  s = s.replace(
    /^[A-Z0-9:_-]{2,12}\s+(open_long|open_short|close_long|close_short)\s+/i,
    ''
  )
  return formatRequestError(s)
}

export type ActionSide = 'long' | 'short' | 'flat'

export function actionSide(action: string): ActionSide {
  const a = action.toLowerCase()
  if (a === 'open_long' || a === 'close_short') return 'long'
  if (a === 'open_short' || a === 'close_long') return 'short'
  return 'flat'
}

// The local date and time. Go's zero time (0001-01-01) means none was recorded.
export function fmtDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime()) || d.getFullYear() <= 1) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function tradeActions(record: DecisionRecord) {
  return (record.decisions ?? []).filter(
    (a) => a.action !== 'hold' && a.action !== 'wait'
  )
}

// A cycle's own flag only says the cycle ran, so failed orders also count as a
// fault.
export function cycleFault(record: DecisionRecord) {
  const failed = tradeActions(record).filter((a) => !a.success).length
  return { failed, fault: !record.success || failed > 0 }
}
