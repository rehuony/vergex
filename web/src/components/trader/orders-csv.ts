import type { TraderOrder } from '@/types'

const COLUMNS: [string, (order: TraderOrder) => string | number][] = [
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

function field(value: string | number) {
  const text = String(value ?? '')
  // A leading = + - @ would run as a formula when opened in a spreadsheet.
  const safe =
    /^[=+\-@]/.test(text) && typeof value === 'string' ? `'${text}` : text
  return /[",\n]/.test(safe) ? `"${safe.replace(/"/g, '""')}"` : safe
}

export function ordersToCsv(orders: TraderOrder[]) {
  const rows = orders.map((order) =>
    COLUMNS.map(([, read]) => field(read(order))).join(',')
  )
  return [COLUMNS.map(([name]) => name).join(','), ...rows].join('\n')
}

export function downloadCsv(filename: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/csv' }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}
