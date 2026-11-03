import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Minus, Plus, RotateCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { ChartLegend, type LegendText } from './chart-legend'
import type {
  ChartCandle,
  ChartLevel,
  ChartLines,
  ChartMarker,
  LocalMarketChart,
} from './local-market-chart'
import type { ChartForecast } from '@/api/forecast'

const NO_LINES: ChartLines = {}

export interface ChartText extends LegendText {
  zoomIn: string
  zoomOut: string
  reset: string
}

// Shows candles held locally. The charting library loads only when this mounts,
// which keeps it out of the main bundle. Scrolling over it scrolls the page: Ctrl or
// Command with the wheel, a pinch or the buttons zoom, and a drag or a sideways scroll
// moves along time.
export function LocalMarketChartView({
  candles,
  levels,
  markers,
  forecast = null,
  lines = NO_LINES,
  live = null,
  fitFor,
  label,
  failure,
  text,
  className,
}: {
  candles: ChartCandle[]
  levels: ChartLevel[]
  markers: ChartMarker[]
  // The forecast to draw as a band past the candles.
  forecast?: ChartForecast | null
  lines?: ChartLines
  // The forming candle, between refreshes of candles.
  live?: ChartCandle | null
  // Names what is shown: the view fits once for each.
  fitFor: string
  label: string
  failure: string
  text: ChartText
  className?: string
}) {
  const host = useRef<HTMLDivElement>(null)
  const legend = useRef<HTMLParagraphElement>(null)
  const chart = useRef<LocalMarketChart | null>(null)
  const [ready, setReady] = useState(false)
  const [failed, setFailed] = useState(false)
  const [hoveredTime, setHoveredTime] = useState<number | null>(null)

  useEffect(() => {
    let cancelled = false
    import('./local-market-chart')
      .then(({ LocalMarketChart }) => {
        if (cancelled || !host.current) return
        chart.current = new LocalMarketChart(host.current)
        chart.current.onCrosshair(setHoveredTime)
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
    if (ready) chart.current?.setForecast(forecast)
  }, [ready, forecast])
  useEffect(() => {
    if (ready) chart.current?.setLines(lines)
  }, [ready, lines])
  useEffect(() => {
    if (ready && live) chart.current?.update(live)
  }, [ready, live])

  const byTime = useMemo(
    () => new Map(candles.map((candle) => [candle.time, candle])),
    [candles]
  )
  const newest = live ?? candles[candles.length - 1]
  // The band has no candle to read, so the newest real one stays.
  const shownCandle =
    hoveredTime === null
      ? newest
      : hoveredTime === live?.time
        ? live
        : (byTime.get(hoveredTime) ?? newest)
  const hasLegend = ready && Boolean(shownCandle)

  // The legend wraps on a narrow chart, so the chart keeps as much of its top clear
  // as the legend takes, whatever its width.
  useLayoutEffect(() => {
    const element = legend.current
    if (!hasLegend || !element) {
      chart.current?.reserveTop(0)
      return
    }
    const reserve = () => chart.current?.reserveTop(element.offsetHeight)
    reserve()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(reserve)
    observer.observe(element)
    return () => observer.disconnect()
  }, [hasLegend])

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
  return (
    <div className={cn('group relative', className)}>
      <div ref={host} role="img" aria-label={label} className="h-full" />
      {hasLegend && shownCandle && (
        <ChartLegend ref={legend} candle={shownCandle} text={text} />
      )}
      {ready && (
        <div className="absolute bottom-8 left-1/2 z-10 flex -translate-x-1/2 items-center gap-0.5 rounded-full border border-vergex-line bg-vergex-surface p-0.5 opacity-0 shadow-(--shadow-float) transition-opacity focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100">
          {(
            [
              [text.zoomOut, Minus, () => chart.current?.zoomOut()],
              [text.zoomIn, Plus, () => chart.current?.zoomIn()],
              [text.reset, RotateCcw, () => chart.current?.resetView()],
            ] as const
          ).map(([name, Icon, press]) => (
            <Button
              key={name}
              type="button"
              variant="ghost"
              size="icon"
              aria-label={name}
              title={name}
              className="size-7"
              onClick={press}
            >
              <Icon className="size-3.5" aria-hidden="true" />
            </Button>
          ))}
        </div>
      )}
    </div>
  )
}
