import type { AccountFill, TraderOrder } from '@/types'

type Column<T> = [string, (row: T) => string | number | undefined]

const COLUMNS: Column<TraderOrder>[] = [
  ['created_at', (o) => new Date(o.created_at).toISOString()],
  ['symbol', (o) => o.symbol],
  ['side', (o) => o.side],
  ['position_side', (o) => o.position_side],
  ['type', (o) => o.type],
  ['action', (o) => o.order_action],
  ['status', (o) => o.status],
  ['quantity', (o) => o.quantity],
  ['filled_quantity', (o) => o.filled_quantity],
  ['avg_fill_price', (o) => o.avg_fill_price],
  ['commission', (o) => o.commission],
  ['commission_asset', (o) => o.commission_asset],
]

const SOURCE: Column<{ source?: string }> = ['source', (row) => row.source]

const FILL_COLUMNS: Column<AccountFill>[] = [
  ['executed_at', (f) => new Date(f.created_at).toISOString()],
  SOURCE,
  ['symbol', (f) => f.symbol],
  ['side', (f) => f.side],
  ['quantity', (f) => f.quantity],
  ['price', (f) => f.price],
  ['commission', (f) => f.commission],
  ['commission_asset', (f) => f.commission_asset],
  ['realized_pnl', (f) => f.realized_pnl],
]

function field(value: string | number | undefined) {
  const text = String(value ?? '')
  // A leading = + - @ would run as a formula when opened in a spreadsheet.
  const safe =
    /^[=+\-@]/.test(text) && typeof value === 'string' ? `'${text}` : text
  return /[",\n]/.test(safe) ? `"${safe.replace(/"/g, '""')}"` : safe
}

function toCsv<T>(columns: Column<T>[], items: T[]) {
  const rows = items.map((item) =>
    columns.map(([, read]) => field(read(item))).join(',')
  )
  return [columns.map(([name]) => name).join(','), ...rows].join('\n')
}

export const ordersToCsv = (orders: TraderOrder[]) => toCsv(COLUMNS, orders)

// An account's orders come from anyone, so they also say from whom.
export const accountOrdersToCsv = (orders: TraderOrder[]) =>
  toCsv([SOURCE, ...COLUMNS], orders)

export const fillsToCsv = (fills: AccountFill[]) => toCsv(FILL_COLUMNS, fills)

export function downloadCsv(filename: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/csv' }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}
