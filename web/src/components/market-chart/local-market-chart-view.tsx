import { useEffect, useRef, useState } from 'react'
import type {
  ChartCandle,
  ChartLevel,
  ChartMarker,
  LocalMarketChart,
} from './local-market-chart'

// Shows candles held locally. The charting library loads only when this mounts,
// which keeps it out of the main bundle.
export function LocalMarketChartView({
  candles,
  levels,
  markers,
  label,
  failure,
  className,
}: {
  candles: ChartCandle[]
  levels: ChartLevel[]
  markers: ChartMarker[]
  label: string
  failure: string
  className?: string
}) {
  const host = useRef<HTMLDivElement>(null)
  const chart = useRef<LocalMarketChart | null>(null)
  const [ready, setReady] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    import('./local-market-chart')
      .then(({ LocalMarketChart }) => {
        if (cancelled || !host.current) return
        chart.current = new LocalMarketChart(host.current)
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
    if (ready) chart.current?.setData(candles, levels, markers)
  }, [ready, candles, levels, markers])

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
