import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { t, type Language } from '@/i18n'
import {
  getAccountDisplayName,
  getTraderModelLabel,
} from '@/constants/model-providers'
import { buildTraderPath } from '@/constants/route-paths'
import {
  cellClass,
  gainTone,
  headClass,
  numberClass,
  tableClass,
} from '@/components/common/figures'
import { PanelState } from '@/components/common/panel'
import { SortHeader, type ColumnSort } from '@/components/common/sort-header'
import { SettingsLink } from '@/components/settings/settings-link'
import { buttonVariants } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { Account, AgentModel, TraderInfo, TraderMetrics } from '@/types'
import { cn } from '@/utils/cn'
import { fmtPct, fmtUsd } from '@/utils/trader-format'

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
export function sortTraders(
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
export function NoTraders({
  modelCount,
  accountCount,
  language,
  create,
}: {
  modelCount: number
  accountCount: number
  language: Language
  // The way to create one, once nothing else is missing.
  create?: ReactNode
}) {
  const label = (key: string) => t(`traderList.${key}`, language)
  const addClass = buttonVariants({ variant: 'secondary', size: 'sm' })
  return (
    <PanelState role="status">
      <p className="font-medium text-vergex-ink">{t('noTraders', language)}</p>
      {modelCount === 0 ? (
        <>
          <p>{label(accountCount === 0 ? 'needBoth' : 'needModel')}</p>
          <SettingsLink section="models" className={addClass}>
            {label('addModel')}
          </SettingsLink>
        </>
      ) : accountCount === 0 ? (
        <>
          <p>{label('needAccount')}</p>
          <SettingsLink section="accounts" className={addClass}>
            {label('addAccount')}
          </SettingsLink>
        </>
      ) : (
        <>
          <p>{label('createFirst')}</p>
          {create}
        </>
      )}
    </PanelState>
  )
}

interface TradersListProps {
  traders?: TraderInfo[]
  metrics?: Record<string, TraderMetrics>
  failed: boolean
  models: AgentModel[]
  accounts: Account[]
  modelCount: number
  accountCount: number
  search: string
  language: Language
}

// The traders side by side. A row opens that trader, where it is started, stopped,
// edited and deleted. Numbers come from stored data. The header stays in view while a
// long list scrolls inside the panel.
export function TradersList({
  traders,
  metrics,
  failed,
  models,
  accounts,
  modelCount,
  accountCount,
  search,
  language,
}: TradersListProps) {
  const navigate = useNavigate()
  const label = (key: string) => t(`traderList.${key}`, language)
  const [sort, setSort] = useState<Sort | null>(null)

  const describe = (trader: TraderInfo) =>
    [
      label(trader.kind === 'grid' ? 'kindGrid' : 'kindAgent'),
      getTraderModelLabel(trader.agent_model, models),
      getAccountDisplayName(trader.account_id, accounts),
    ].join(' · ')

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase()
    const matching = (traders ?? []).filter(
      (trader) =>
        !term ||
        `${trader.trader_name} ${describe(trader)}`.toLowerCase().includes(term)
    )
    return sortTraders(matching, metrics, sort)
  }, [traders, metrics, models, accounts, search, sort])

  // The first click on a name sorts A to Z, on a number highest first.
  const chooseColumn = (key: SortKey) =>
    setSort((current) =>
      current?.key === key
        ? { key, ascending: !current.ascending }
        : { key, ascending: key === 'name' }
    )

  if (!traders) {
    return failed ? (
      <PanelState role="alert">{label('loadFailed')}</PanelState>
    ) : (
      <PanelState role="status">{t('loading', language)}</PanelState>
    )
  }

  if (traders.length === 0) {
    return (
      <NoTraders
        modelCount={modelCount}
        accountCount={accountCount}
        language={language}
      />
    )
  }

  if (rows.length === 0) {
    return <PanelState role="status">{label('noMatch')}</PanelState>
  }

  const header = { sort, onSort: chooseColumn }
  const number = cn(headClass, 'w-28 text-right')
  return (
    <ScrollArea className="min-h-0 flex-1 rounded-b-2xl">
      <table className={tableClass}>
        <thead className="sticky top-0 z-10">
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
              className={number}
              {...header}
            />
            <SortHeader
              column="return"
              title={label('return')}
              className={number}
              {...header}
            />
            <SortHeader
              column="winRate"
              title={label('winRate')}
              className={cn(number, narrowHidden)}
              {...header}
            />
            <SortHeader
              column="drawdown"
              title={label('drawdown')}
              className={cn(number, narrowHidden)}
              {...header}
            />
            <SortHeader
              column="trades"
              title={label('trades')}
              className={cn(number, narrowHidden)}
              {...header}
            />
          </tr>
        </thead>
        <tbody>
          {rows.map((trader) => {
            const row = metrics?.[trader.trader_id]
            const path = buildTraderPath(trader.trader_id)
            const change = row?.return_pct
            return (
              <tr
                key={trader.trader_id}
                className="cursor-pointer hover:bg-vergex-surface-muted"
                onClick={() => navigate(path)}
              >
                <td className={cn(cellClass, 'h-14')}>
                  <div className="flex min-w-0 items-center gap-2">
                    <span
                      className={cn(
                        'size-2 shrink-0 rounded-full',
                        trader.is_running
                          ? 'bg-vergex-success'
                          : 'bg-vergex-line-strong'
                      )}
                      role="img"
                      aria-label={
                        trader.is_running ? label('running') : label('stopped')
                      }
                    />
                    <Link
                      to={path}
                      className="truncate font-medium text-vergex-ink hover:underline"
                      title={trader.trader_name}
                      onClick={(event) => event.stopPropagation()}
                    >
                      {trader.trader_name}
                    </Link>
                  </div>
                  <div
                    className="mt-0.5 truncate pl-4 text-xs text-vergex-muted"
                    title={describe(trader)}
                  >
                    {describe(trader)}
                  </div>
                </td>
                <td className={cn(cellClass, numberClass)}>
                  {fmtUsd(row?.equity)}
                </td>
                <td className={cn(cellClass, numberClass, gainTone(change))}>
                  {fmtPct(change)}
                </td>
                <td className={cn(cellClass, numberClass, narrowHidden)}>
                  {row?.win_rate == null ? '—' : `${row.win_rate.toFixed(1)}%`}
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
  )
}
