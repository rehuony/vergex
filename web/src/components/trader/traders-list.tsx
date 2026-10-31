import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { t, type Language } from '@/i18n/translations'
import {
  getExchangeDisplayName,
  getTraderModelLabel,
} from '@/constants/model-providers'
import { buildTraderPath } from '@/constants/route-paths'
import { Panel, PanelState } from '@/components/common/panel'
import { SortHeader, type ColumnSort } from '@/components/common/sort-header'
import { SettingsLink } from '@/components/settings/settings-link'
import { buttonVariants } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { AIModel, Exchange, TraderInfo, TraderMetrics } from '@/types'
import { cn } from '@/utils/cn'
import { getTraderSlug } from '@/utils/trader-slug'
import { fmtPct, fmtUsd } from '@/utils/trader-format'

const headClass =
  'sticky top-0 z-10 bg-[var(--surface)] px-4 py-3 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-4 py-3 whitespace-nowrap'
const numberClass = 'w-28 text-right tabular-nums'
const narrowHidden = 'max-md:hidden'

type SortKey = 'name' | 'equity' | 'return' | 'winRate' | 'drawdown' | 'trades'
type Sort = ColumnSort<SortKey>

const sortValue: Record<
  SortKey,
  (trader: TraderInfo, row?: TraderMetrics) => string | number | undefined
> = {
  name: (trader) => trader.trader_name,
  equity: (_, row) => row?.equity,
  return: (_, row) => row?.return_pct,
  winRate: (_, row) => row?.win_rate ?? undefined,
  drawdown: (_, row) => row?.max_drawdown_pct ?? undefined,
  trades: (_, row) => row?.total_trades,
}

// Running traders come first until a column is chosen; then unknown values stay last.
function sortTraders(
  traders: TraderInfo[],
  metrics: Record<string, TraderMetrics> | undefined,
  sort: Sort | null
) {
  if (!sort) {
    return [...traders].sort(
      (a, b) => Number(Boolean(b.is_running)) - Number(Boolean(a.is_running))
    )
  }
  const value = sortValue[sort.key]
  return [...traders].sort((a, b) => {
    const x = value(a, metrics?.[a.trader_id])
    const y = value(b, metrics?.[b.trader_id])
    if (x == null) return y == null ? 0 : 1
    if (y == null) return -1
    const order =
      typeof x === 'string' ? x.localeCompare(String(y)) : x - Number(y)
    return sort.ascending ? order : -order
  })
}

// What a first trader is still waiting for, and the way to add it.
function NoTraders({
  modelCount,
  exchangeCount,
  language,
}: {
  modelCount: number
  exchangeCount: number
  language: Language
}) {
  const label = (key: string) => t(`traderList.${key}`, language)
  const addClass = buttonVariants({ variant: 'secondary', size: 'sm' })
  return (
    <PanelState role="status">
      <p className="font-medium text-vergex-text">{t('noTraders', language)}</p>
      {modelCount === 0 ? (
        <>
          <p>{label(exchangeCount === 0 ? 'needBoth' : 'needModel')}</p>
          <SettingsLink section="models" className={addClass}>
            {label('addModel')}
          </SettingsLink>
        </>
      ) : exchangeCount === 0 ? (
        <>
          <p>{label('needExchange')}</p>
          <SettingsLink section="exchanges" className={addClass}>
            {label('addExchange')}
          </SettingsLink>
        </>
      ) : (
        <p>{label('createFirst')}</p>
      )}
    </PanelState>
  )
}

interface TradersListProps {
  traders?: TraderInfo[]
  metrics?: Record<string, TraderMetrics>
  failed: boolean
  models: AIModel[]
  exchanges: Exchange[]
  modelCount: number
  exchangeCount: number
  search: string
  language: Language
}

// The trader table. A row opens that trader, where it is started, stopped,
// edited and deleted. Numbers come from stored data. The header stays in view
// while a long list scrolls inside the panel.
export function TradersList({
  traders,
  metrics,
  failed,
  models,
  exchanges,
  modelCount,
  exchangeCount,
  search,
  language,
}: TradersListProps) {
  const navigate = useNavigate()
  const label = (key: string) => t(`traderList.${key}`, language)
  const [sort, setSort] = useState<Sort | null>(null)

  const describe = (trader: TraderInfo) =>
    [
      getTraderModelLabel(trader.ai_model, models),
      getExchangeDisplayName(trader.exchange_id, exchanges),
    ].join(' · ')

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase()
    const matching = (traders ?? []).filter(
      (trader) =>
        !term ||
        `${trader.trader_name} ${describe(trader)} ${trader.strategy_name ?? ''}`
          .toLowerCase()
          .includes(term)
    )
    return sortTraders(matching, metrics, sort)
  }, [traders, metrics, models, exchanges, search, sort])

  // The first click on a name sorts A to Z, on a number highest first.
  const chooseColumn = (key: SortKey) =>
    setSort((current) =>
      current?.key === key
        ? { key, ascending: !current.ascending }
        : { key, ascending: key === 'name' }
    )

  if (!traders) {
    return (
      <Panel>
        {failed ? (
          <PanelState role="alert">{label('loadFailed')}</PanelState>
        ) : (
          <PanelState role="status">{t('loading', language)}</PanelState>
        )}
      </Panel>
    )
  }

  if (traders.length === 0) {
    return (
      <Panel>
        <NoTraders
          modelCount={modelCount}
          exchangeCount={exchangeCount}
          language={language}
        />
      </Panel>
    )
  }

  if (rows.length === 0) {
    return (
      <Panel>
        <PanelState role="status">{label('noMatch')}</PanelState>
      </Panel>
    )
  }

  const header = { sort, onSort: chooseColumn }
  return (
    <Panel className="overflow-hidden">
      <ScrollArea className="max-h-[max(10rem,calc(100dvh-204px))]">
        <table className="w-full text-left text-[13px]">
          <thead className="text-vergex-text-muted">
            <tr>
              <SortHeader
                column="name"
                title={label('name')}
                className={headClass}
                {...header}
              />
              <SortHeader
                column="equity"
                title={label('equity')}
                className={cn(headClass, numberClass)}
                {...header}
              />
              <SortHeader
                column="return"
                title={label('return')}
                className={cn(headClass, numberClass)}
                {...header}
              />
              <SortHeader
                column="winRate"
                title={label('winRate')}
                className={cn(headClass, numberClass, narrowHidden)}
                {...header}
              />
              <SortHeader
                column="drawdown"
                title={label('drawdown')}
                className={cn(headClass, numberClass, narrowHidden)}
                {...header}
              />
              <SortHeader
                column="trades"
                title={label('trades')}
                className={cn(headClass, numberClass, narrowHidden)}
                {...header}
              />
            </tr>
          </thead>
          <tbody>
            {rows.map((trader) => {
              const row = metrics?.[trader.trader_id]
              const path = buildTraderPath(getTraderSlug(trader))
              const change = row?.return_pct
              return (
                <tr
                  key={trader.trader_id}
                  className="cursor-pointer border-t border-[var(--line)] hover:bg-vergex-bg"
                  onClick={() => navigate(path)}
                >
                  <td className={cn(cellClass, 'max-w-0 w-full')}>
                    <div className="flex min-w-0 items-center gap-2">
                      <span
                        className={cn(
                          'size-2 shrink-0 rounded-full',
                          trader.is_running
                            ? 'bg-vergex-success'
                            : 'bg-[var(--line-strong)]'
                        )}
                        role="img"
                        aria-label={
                          trader.is_running
                            ? label('running')
                            : label('stopped')
                        }
                      />
                      <Link
                        to={path}
                        className="truncate text-sm font-medium text-vergex-text hover:underline"
                        title={trader.trader_name}
                        onClick={(event) => event.stopPropagation()}
                      >
                        {trader.trader_name}
                      </Link>
                    </div>
                    <div
                      className="mt-0.5 truncate pl-4 text-xs text-vergex-text-muted"
                      title={describe(trader)}
                    >
                      {describe(trader)}
                    </div>
                  </td>
                  <td className={cn(cellClass, numberClass)}>
                    {fmtUsd(row?.equity)}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      numberClass,
                      change == null
                        ? ''
                        : change >= 0
                          ? 'text-vergex-success'
                          : 'text-vergex-danger'
                    )}
                  >
                    {fmtPct(change)}
                  </td>
                  <td className={cn(cellClass, numberClass, narrowHidden)}>
                    {row?.win_rate == null
                      ? '—'
                      : `${row.win_rate.toFixed(1)}%`}
                  </td>
                  <td className={cn(cellClass, numberClass, narrowHidden)}>
                    {row?.max_drawdown_pct == null
                      ? '—'
                      : `${row.max_drawdown_pct.toFixed(1)}%`}
                  </td>
                  <td className={cn(cellClass, numberClass, narrowHidden)}>
                    {row?.total_trades ?? '—'}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </ScrollArea>
    </Panel>
  )
}
