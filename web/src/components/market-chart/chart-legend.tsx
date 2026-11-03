import { forwardRef } from 'react'
import { cn } from '@/utils/cn'

// Enough decimals to tell prices apart, as the rest of Forecast shows them.
export const priceDigits = (price: number) =>
  price >= 1000 ? 1 : price >= 1 ? 3 : price >= 0.01 ? 5 : 8

const compactVolume = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 2,
})

// A line floating at a chart's top left, the same on every chart. The chart shows faintly
// through it, and on a narrow chart it wraps between its fields.
export const chartLineClass =
  'absolute left-2 top-2 z-10 flex max-w-[calc(100%-96px)] flex-wrap gap-x-2 rounded-md bg-vergex-surface/85 px-1.5 py-0.5 font-mono text-[11px] tabular-nums'

export interface LegendText {
  open: string
  high: string
  low: string
  close: string
  volume: string
}

// The candle under the pointer, or the newest one, in a line over the chart. It takes no
// pointer events, so it never gets in the way of the chart. The ref is for whoever
// keeps the chart's top clear under it.
export const ChartLegend = forwardRef<
  HTMLParagraphElement,
  {
    candle: {
      open: number
      high: number
      low: number
      close: number
      volume: number
    }
    text: LegendText
  }
>(function ChartLegend({ candle, text }, ref) {
  const digits = priceDigits(candle.close)
  const price = (value: number) =>
    value.toLocaleString('en-US', {
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    })
  const change = candle.open > 0 ? (candle.close / candle.open - 1) * 100 : 0
  const tone =
    candle.close >= candle.open ? 'text-vergex-success' : 'text-vergex-danger'
  const field = (name: string, value: string) => (
    <span className="whitespace-nowrap">
      <span className="text-vergex-muted">{name} </span>
      <span className={tone}>{value}</span>
    </span>
  )
  return (
    <p ref={ref} className={cn(chartLineClass, 'pointer-events-none')}>
      {field(text.open, price(candle.open))}
      {field(text.high, price(candle.high))}
      {field(text.low, price(candle.low))}
      {field(text.close, price(candle.close))}
      <span className={cn('whitespace-nowrap', tone)}>
        {change > 0 ? '+' : ''}
        {change.toFixed(2)}%
      </span>
      {field(text.volume, compactVolume.format(candle.volume))}
    </p>
  )
})
