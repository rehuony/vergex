// Number, symbol and time formats for trader data.

export function fmtUsd(n: number | undefined, signed = false): string {
  if (n == null || Number.isNaN(n)) return '$0'
  const sign = signed && n > 0 ? '+' : n < 0 ? '-' : ''
  return `${sign}$${Math.abs(n).toLocaleString('en-US', { maximumFractionDigits: 2 })}`
}
export function fmtPct(n: number | undefined): string {
  if (n == null || Number.isNaN(n)) return '0.00%'
  return `${n >= 0 ? '+' : ''}${n.toFixed(2)}%`
}
/**
 * Price with magnitude-aware precision: 64,416 · 184.2 · 2.3775 · 0.0067. An order or
 * position with no price has none, rather than a price of 0.
 */
export function fmtPx(n: number | undefined): string {
  if (n == null || Number.isNaN(n) || n === 0) return '—'
  const dp = n >= 1000 ? 0 : n >= 100 ? 1 : n >= 1 ? 2 : 4
  return n.toLocaleString('en-US', {
    minimumFractionDigits: dp,
    maximumFractionDigits: dp,
  })
}
export function baseLabel(raw?: string): string {
  if (!raw) return ''
  return raw
    .toUpperCase()
    .replace(/[-_]/g, '')
    .replace(/(USDT|USDC|USD)$/, '')
}
export function parseScanMinutes(scan?: string): number {
  if (!scan) return 15
  const m = scan.match(/(\d+)\s*m/i)
  if (m) return parseInt(m[1], 10)
  const h = scan.match(/(\d+)\s*h/i)
  if (h) return parseInt(h[1], 10) * 60
  const n = parseInt(scan, 10)
  return Number.isFinite(n) && n > 0 ? n : 15
}
/** Local month, day and time: 10-30 09:02. */
export function fmtTime(raw?: string | number): string {
  if (raw == null || raw === '') return ''
  let n = typeof raw === 'number' ? raw : Number(raw)
  if (Number.isFinite(n) && n < 1e12) n *= 1000
  const d = new Date(Number.isFinite(n) ? n : (raw as string))
  if (Number.isNaN(d.getTime())) return ''
  const pad = (v: number) => String(v).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** Hold duration from entry/exit epoch-ms as a compact 45m / 2h10 / 1d3h. */
export function fmtHold(entry?: number, exit?: number): string {
  if (!entry || !exit || exit <= entry) return '—'
  const mins = Math.round((exit - entry) / 60000)
  if (mins < 60) return `${mins}m`
  const h = Math.floor(mins / 60)
  const m = mins % 60
  if (h < 24) return m ? `${h}h${m}` : `${h}h`
  const d = Math.floor(h / 24)
  return `${d}d${h % 24}h`
}

/** Countdown to the next cycle, e.g. 4m 12s; "due now" once it passed. */
export function fmtCountdown(nextCycleMs: number | null): string {
  let countdown = '—'
  if (nextCycleMs) {
    const ms = nextCycleMs - Date.now()
    if (ms <= 0) countdown = 'due now'
    else {
      const s = Math.floor(ms / 1000)
      countdown = `${Math.floor(s / 60)}m ${s % 60}s`
    }
  }
  return countdown
}
