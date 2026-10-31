import { useMemo, type ReactNode } from 'react'
import { ChevronLeft } from 'lucide-react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import useSWR from 'swr'
import { api } from '@/api'
import { FORECAST_INTERVALS, type ForecastInterval } from '@/api/forecast'
import { Page } from '@/components/common/page'
import { Panel, PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { FactorList } from '@/components/forecast/factor-list'
import {
  DirectionLabel,
  baseAsset,
  fmtPct,
  fmtPrice,
  fmtTime,
  scoreText,
} from '@/components/forecast/forecast-format'
import { RecordList } from '@/components/forecast/record-list'
import { LocalMarketChartView } from '@/components/market-chart/local-market-chart-view'
import { ROUTES } from '@/constants/route-paths'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'

const REFRESH_MS = 15000
const WAITING_REFRESH_MS = 3000
const CHART_MARKERS = 40

function Reading({ name, children }: { name: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-vergex-text-muted">{name}</dt>
      <dd className="mt-0.5 truncate text-[13px] font-medium tabular-nums text-vergex-text">
        {children}
      </dd>
    </div>
  )
}

export function ForecastDetailPage() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const { symbol = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const requested = params.get('interval') as ForecastInterval
  const interval = FORECAST_INTERVALS.includes(requested) ? requested : '1h'
  const upper = symbol.toUpperCase()

  const { data, error } = useSWR(
    ['forecast-detail', upper, interval],
    () => api.getForecastDetail(upper, interval),
    {
      keepPreviousData: true,
      refreshInterval: (latest) =>
        latest && latest.last_closed === 0 ? WAITING_REFRESH_MS : REFRESH_MS,
    }
  )
  const current = data?.interval === interval ? data : undefined
  const result = current?.result
  const latest = current?.latest

  const levels = useMemo(
    () =>
      result
        ? [
            {
              price: result.resistance,
              title: label('resistance'),
              tone: 'down' as const,
            },
            {
              price: result.support,
              title: label('support'),
              tone: 'up' as const,
            },
          ]
        : [],
    [result?.resistance, result?.support, language]
  )
  const markers = useMemo(
    () =>
      (current?.records ?? [])
        .filter(
          (record) => record.outcome === 'hit' || record.outcome === 'miss'
        )
        .slice(0, CHART_MARKERS)
        .reverse()
        .map((record) => ({
          time: record.open_time / 1000,
          tone: record.outcome === 'hit' ? ('up' as const) : ('down' as const),
          position:
            record.direction === 'up' ? ('below' as const) : ('above' as const),
        })),
    [current?.records]
  )
  const candles = useMemo(() => current?.candles ?? [], [current?.candles])

  const back = (
    <Link
      to={ROUTES.forecast}
      aria-label={label('tabSignals')}
      className="-ml-1 inline-flex size-8 shrink-0 items-center justify-center rounded-lg text-vergex-text-muted hover:bg-vergex-bg hover:text-vergex-text"
    >
      <ChevronLeft size={20} aria-hidden="true" />
    </Link>
  )

  return (
    <Page
      title={
        <span className="flex min-w-0 items-center gap-1">
          {back}
          <span className="truncate">{baseAsset(upper)}</span>
        </span>
      }
      description={label('detailDescription', { symbol: upper })}
      actions={
        <Segmented
          aria-label={label('interval')}
          value={interval}
          options={FORECAST_INTERVALS.map((value) => ({
            value,
            label: value,
          }))}
          onChange={(next) => setParams({ interval: next }, { replace: true })}
        />
      }
    >
      {error && !current ? (
        <Panel>
          <PanelState role="alert">{label('detailFailed')}</PanelState>
        </Panel>
      ) : !current ? (
        <Panel>
          <PanelState role="status">{label('loading')}</PanelState>
        </Panel>
      ) : (
        <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
          {/* On a phone the reading comes before the chart. */}
          <Panel className="lg:col-start-2 lg:row-start-1">
            <div className="flex items-start justify-between gap-3 px-4 pt-4">
              <div className="min-w-0">
                <p className="text-2xl font-semibold tabular-nums text-vergex-text">
                  {fmtPrice(current.price ?? result?.close)}
                </p>
                <p className="mt-0.5 text-xs text-vergex-text-muted">
                  {label('price')}
                </p>
              </div>
              {result && latest && (
                <div className="shrink-0 text-right">
                  <p className="whitespace-nowrap text-2xl font-semibold tabular-nums text-vergex-text">
                    {latest.calibrated ? '' : `${label('scoreWord')} `}
                    {scoreText(latest.display_prob, latest.calibrated)}
                  </p>
                  <DirectionLabel
                    direction={result.direction}
                    language={language}
                    className="mt-0.5 text-[13px] font-medium"
                  />
                </div>
              )}
            </div>
            {result ? (
              <dl className="grid grid-cols-2 gap-x-4 gap-y-3 p-4">
                <Reading name={label('confidence')}>
                  {result.confidence}
                </Reading>
                <Reading name={label('expectedMove')}>
                  ±{fmtPct(result.expected_move_pct)}
                </Reading>
                <Reading name={label('support')}>
                  {fmtPrice(result.support)}
                </Reading>
                <Reading name={label('resistance')}>
                  {fmtPrice(result.resistance)}
                </Reading>
                <Reading name={label('regime')}>
                  {label(`regimeName.${result.regime}`)}
                </Reading>
                <Reading name={label('horizon')}>
                  {label('horizonBars', { bars: result.horizon })}
                </Reading>
                {latest && (
                  <Reading name={label('updated')}>
                    {fmtTime(latest.open_time)}
                  </Reading>
                )}
                <Reading name={label('derivatives')}>
                  {label(result.has_derivatives ? 'included' : 'notIncluded')}
                </Reading>
              </dl>
            ) : (
              <PanelState role="status">
                {label(`pendingState.${current.pending ?? 'waiting'}`)}
              </PanelState>
            )}
          </Panel>

          <Panel className="min-w-0 overflow-hidden lg:col-start-1 lg:row-span-2 lg:row-start-1">
            {candles.length > 0 ? (
              <LocalMarketChartView
                candles={candles}
                levels={levels}
                markers={markers}
                label={label('chartName', { symbol: upper, interval })}
                failure={label('chartFailed')}
                className="h-[420px] lg:h-[560px]"
              />
            ) : (
              <PanelState role="status" className="h-[420px]">
                {label(`pendingState.${current.pending ?? 'waiting'}`)}
              </PanelState>
            )}
          </Panel>

          {result && result.factors.length > 0 && (
            <div className="min-w-0 lg:col-start-2 lg:row-start-2">
              <FactorList factors={result.factors} language={language} />
            </div>
          )}

          <div className="min-w-0 lg:col-span-2">
            <RecordList
              records={current.records}
              hits={current.hits}
              misses={current.misses}
              language={language}
            />
          </div>
        </div>
      )}
    </Page>
  )
}
