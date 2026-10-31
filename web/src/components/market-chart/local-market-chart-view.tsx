import { useEffect, useRef, useState } from 'react'
import type {
  ChartCandle,
  ChartLevel,
  ChartLines,
  ChartMarker,
  LocalMarketChart,
} from './local-market-chart'

const NO_LINES: ChartLines = {}
const NO_CANDLES: ChartCandle[] = []

// Shows candles held locally. The charting library loads only when this mounts,
// which keeps it out of the main bundle.
export function LocalMarketChartView({
  candles,
  levels,
  markers,
  projection = NO_CANDLES,
  lines = NO_LINES,
  live = null,
  fitFor,
  onProjectionHover,
  label,
  failure,
  className,
}: {
  candles: ChartCandle[]
  levels: ChartLevel[]
  markers: ChartMarker[]
  projection?: ChartCandle[]
  lines?: ChartLines
  // The forming candle, between refreshes of candles.
  live?: ChartCandle | null
  // Names what is shown: the view fits once for each.
  fitFor: string
  onProjectionHover?: (hovering: boolean) => void
  label: string
  failure: string
  className?: string
}) {
  const host = useRef<HTMLDivElement>(null)
  const chart = useRef<LocalMarketChart | null>(null)
  const hover = useRef(onProjectionHover)
  const [ready, setReady] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    hover.current = onProjectionHover
  }, [onProjectionHover])

  useEffect(() => {
    let cancelled = false
    import('./local-market-chart')
      .then(({ LocalMarketChart }) => {
        if (cancelled || !host.current) return
        chart.current = new LocalMarketChart(host.current)
        chart.current.onCrosshair((onProjection) =>
          hover.current?.(onProjection)
        )
        setReady(true)
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
      chart.current?.dispose()
      chart.current = null
    }
  }, [])

  useEffect(() => {
    chart.current?.forget()
  }, [fitFor])
  useEffect(() => {
    if (ready) chart.current?.setData(candles, levels, markers, fitFor)
  }, [ready, candles, levels, markers, fitFor])
  useEffect(() => {
    if (ready) chart.current?.setProjection(projection)
  }, [ready, projection])
  useEffect(() => {
    if (ready) chart.current?.setLines(lines)
  }, [ready, lines])
  useEffect(() => {
    if (ready && live) chart.current?.update(live)
  }, [ready, live])

  if (failed) {
    return (
      <p
        role="alert"
        className="flex h-full items-center justify-center text-sm text-vergex-danger"
      >
        {failure}
      </p>
    )
  }
  return <div ref={host} role="img" aria-label={label} className={className} />
}
