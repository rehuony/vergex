import { t, type Language } from '@/i18n'
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

// The log lines the server writes most, which are English where they are stored. What
// follows a known start, such as an exchange's own words, stays as it came.
const LOG_LINES: [RegExp, string][] = [
  [/^model call duration: (?<ms>\d+) ms$/, 'modelDuration'],
  [/^succeeded$/, 'succeeded'],
  [/^blocked: (?<detail>.*)$/s, 'blocked'],
  [/^failed: (?<detail>.*)$/s, 'failed'],
  [
    /^(?<failed>\d+) of (?<total>\d+) trade actions failed or were blocked$/,
    'someBlocked',
  ],
  [/^No candidate coins available, cycle skipped$/, 'noCandidates'],
  [
    /^Halted with nothing held, cycle skipped: (?<detail>.*)$/s,
    'haltedSkipped',
  ],
  [/^Grid cycle completed with (?<count>\d+) decisions$/, 'gridCycle'],
  [/^Failed to get agent decision: (?<detail>.*)$/s, 'noDecision'],
]

// logText is a stored log line as this page reads it, in the language in use.
export function logText(raw: string, language: Language): string {
  const line = cleanLog(raw)
  for (const [pattern, key] of LOG_LINES) {
    const match = pattern.exec(line)
    if (match) {
      return t(`traderDetail.log.${key}`, language, { ...match.groups })
    }
  }
  return line
}

// actionName is what an action is called. One the interface has no name for stays as it is.
export function actionName(action: string, language: Language): string {
  const name = action.toLowerCase()
  const key = `traderDetail.actionName.${name}`
  const text = t(key, language)
  return text === key ? name : text
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
// fault. A rule stopping an order is the rule working, and is no fault.
export function cycleFault(record: DecisionRecord) {
  const trades = tradeActions(record)
  const blocked = trades.filter((a) => a.blocked).length
  const failed = trades.filter((a) => !a.success && !a.blocked).length
  return {
    failed,
    blocked,
    total: trades.length - blocked,
    fault: !record.success || failed > 0,
  }
}
