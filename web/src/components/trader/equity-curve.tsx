import { useEffect, useRef, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { IChartApi, ISeriesApi, UTCTimestamp } from 'lightweight-charts'
import { api } from '@/api'
import { THEME_EVENT } from '@/contexts/theme-context'
import { t, type Language } from '@/i18n'
import type { EquityPoint, EquityRange } from '@/types'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { pollEvery } from '@/utils/poll'

const RANGES: EquityRange[] = ['24h', '7d', '30d', 'all']

function colors() {
  const css = getComputedStyle(document.documentElement)
  const token = (name: string) => css.getPropertyValue(name).trim()
  return {
    surface: token('--surface'),
    text: token('--muted'),
    grid: token('--line'),
    up: token('--success'),
    down: token('--danger'),
  }
}

// The curve is green above the starting balance and red below it. The chart
// library loads on first use so other pages do not carry it.
function EquityChart({
  points,
  base,
}: {
  points: EquityPoint[]
  base: number
}) {
  const host = useRef<HTMLDivElement>(null)
  const chart = useRef<IChartApi | null>(null)
  const series = useRef<ISeriesApi<'Baseline'> | null>(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let disposed = false
    let removeThemeListener = () => {}
    void import('lightweight-charts').then(
      ({ BaselineSeries, ColorType, createChart }) => {
        if (disposed || !host.current) return
        const theme = () => {
          const c = colors()
          return {
            chart: {
              layout: {
                attributionLogo: false,
                background: { type: ColorType.Solid, color: c.surface },
                textColor: c.text,
              },
              grid: {
                vertLines: { color: c.grid },
                horzLines: { color: c.grid },
              },
            },
            series: { topLineColor: c.up, bottomLineColor: c.down },
          }
        }
        const initial = theme()
        chart.current = createChart(host.current, {
          ...initial.chart,
          autoSize: true,
          rightPriceScale: { borderVisible: false },
          timeScale: { borderVisible: false, timeVisible: true },
          // A vertical swipe scrolls the page, not the chart.
          handleScroll: { vertTouchDrag: false },
        })
        series.current = chart.current.addSeries(BaselineSeries, {
          ...initial.series,
          lineWidth: 2,
          priceLineVisible: false,
          topFillColor1: 'transparent',
          topFillColor2: 'transparent',
          bottomFillColor1: 'transparent',
          bottomFillColor2: 'transparent',
          priceFormat: { type: 'price', precision: 2, minMove: 0.01 },
        })
        const repaint = () => {
          const next = theme()
          chart.current?.applyOptions(next.chart)
          series.current?.applyOptions(next.series)
        }
        window.addEventListener(THEME_EVENT, repaint)
        removeThemeListener = () =>
          window.removeEventListener(THEME_EVENT, repaint)
        setReady(true)
      }
    )
    return () => {
      disposed = true
      removeThemeListener()
      chart.current?.remove()
      chart.current = null
      series.current = null
    }
  }, [])

  useEffect(() => {
    if (!ready || !series.current) return
    const seen = new Set<number>()
    const rows = points.filter((p) => !seen.has(p.time) && seen.add(p.time))
    series.current.applyOptions({
      baseValue: { type: 'price', price: base || rows[0]?.equity || 0 },
    })
    series.current.setData(
      rows.map((p) => ({ time: p.time as UTCTimestamp, value: p.equity }))
    )
    chart.current?.timeScale().fitContent()
  }, [ready, points, base])

  return <div ref={host} className="absolute inset-0" />
}

export function EquityCurve({
  traderId,
  language,
}: {
  traderId: string
  language: Language
}) {
  const [range, setRange] = useState<EquityRange>('7d')
  const { data, error, isLoading } = useQuery({
    queryKey: ['equity', traderId, range],
    queryFn: () => api.getEquity(traderId, range, true),
    ...pollEvery(60000),
    placeholderData: keepPreviousData,
  })
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const points = data?.points ?? []
  return (
    <section className="border-b border-vergex-line">
      <header className="flex flex-wrap items-center justify-between gap-2 px-4 py-2">
        <h3 className="text-[13px] font-medium text-vergex-ink">
          {label('equity')}
        </h3>
        <Segmented
          aria-label={label('equityRange')}
          value={range}
          onChange={setRange}
          options={RANGES.map((value) => ({
            value,
            label: label(`range.${value}`),
          }))}
        />
      </header>
      <div className="relative h-64 px-2 pb-2">
        {points.length > 0 ? (
          <div className="relative h-full">
            <EquityChart points={points} base={data?.initial_balance ?? 0} />
          </div>
        ) : error ? (
          <PanelState role="alert" className="h-full">
            {label('equityFailed')}
          </PanelState>
        ) : isLoading ? (
          <PanelLoading className="h-full" />
        ) : (
          <PanelState role="status" className="h-full">
            {label('noEquity')}
          </PanelState>
        )}
      </div>
    </section>
  )
}
