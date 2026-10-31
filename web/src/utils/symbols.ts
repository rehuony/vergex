function normalizeSymbol(raw: string) {
  const symbol = raw.trim().toUpperCase()
  if (!symbol) return ''
  return symbol.endsWith('USDT') ? symbol : `${symbol}USDT`
}

// Reads symbols typed as "btc, ETHUSDT" into a de-duplicated list.
export function parseSymbols(raw: string) {
  return [
    ...new Set(
      raw
        .split(/[\s,]+/)
        .map(normalizeSymbol)
        .filter(Boolean)
    ),
  ]
}
