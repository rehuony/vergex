import { useState, type KeyboardEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ForecastInterval, ForecastSignal } from '@/api/forecast'
import { SearchField } from '@/components/common/search-field'
import { SortHeader, type ColumnSort } from '@/components/common/sort-header'
import { PanelState } from '@/components/common/panel'
import { ScrollArea } from '@/components/ui/scroll-area'
import { buildForecastPath } from '@/constants/route-paths'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import {
  DirectionIcon,
  baseAsset,
  fmtPrice,
  scoreText,
} from './forecast-format'

type CoinColumn = 'coin' | 'price' | 'score'

const headClass =
  'sticky top-0 z-10 bg-[var(--surface)] px-3 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-3 py-2 whitespace-nowrap'

// Strong calls first, in either direction; coins with no forecast yet last.
export function sortSignals(
  signals: ForecastSignal[],
  sort: ColumnSort<CoinColumn> | null = null
) {
  const strength = (signal: ForecastSignal) =>
    signal.record ? Math.abs(signal.record.display_prob - 0.5) : -1
  const by = (a: ForecastSignal, b: ForecastSignal) => {
    if (!sort) {
      return (
        strength(b) - strength(a) ||
        a.tier - b.tier ||
        a.symbol.localeCompare(b.symbol)
      )
    }
    const direction = sort.ascending ? 1 : -1
    if (sort.key === 'coin') return direction * a.symbol.localeCompare(b.symbol)
    const value = (signal: ForecastSignal) =>
      sort.key === 'price'
        ? signal.price
        : (signal.record?.display_prob ?? null)
    const left = value(a)
    const right = value(b)
    // A coin with no value stays last whichever way the column runs.
    if (left === null || left === undefined) return right === null ? 0 : 1
    if (right === null || right === undefined) return -1
    return direction * (left - right)
  }
  return [...signals].sort(by)
}

// Keeps rows where they are while values refresh, so a row does not move under the
// pointer. The order is worked out again only when the interval or sort changes;
// coins that come and go are added at the end or dropped.
function useStableOrder(
  signals: ForecastSignal[],
  interval: ForecastInterval,
  sort: ColumnSort<CoinColumn> | null
) {
  const [held, setHeld] = useState({ key: '', symbols: [] as string[] })
  const key = `${interval}/${sort?.key}/${sort?.ascending}`
  const present = signals.map((row) => row.symbol)
  let order = held
  if (held.key !== key) {
    if (signals.length > 0) {
      order = { key, symbols: sortSignals(signals, sort).map((r) => r.symbol) }
    }
  } else {
    const kept = held.symbols.filter((symbol) => present.includes(symbol))
    const added = present.filter((symbol) => !held.symbols.includes(symbol))
    if (added.length > 0 || kept.length !== held.symbols.length) {
      order = { key, symbols: [...kept, ...added] }
    }
  }
  if (order !== held) setHeld(order)

  const bySymbol = new Map(signals.map((row) => [row.symbol, row]))
  return order.symbols.flatMap((symbol) => {
    const row = bySymbol.get(symbol)
    return row ? [row] : []
  })
}

// Every tracked coin at this interval, to move between. Typing a coin that is not
// tracked and pressing Enter opens it, which starts fetching it.
export function CoinList({
  signals,
  interval,
  selected,
  loading,
  failed,
  onChosen,
  language,
  className,
}: {
  signals: ForecastSignal[]
  interval: ForecastInterval
  selected: string
  loading: boolean
  failed: boolean
  onChosen?: () => void
  language: Language
  className?: string
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<ColumnSort<CoinColumn> | null>(null)
  const ordered = useStableOrder(signals, interval, sort)

  const open = (symbol: string) => {
    navigate(buildForecastPath(symbol, interval))
    onChosen?.()
  }
  const chooseColumn = (key: CoinColumn) =>
    setSort((current) =>
      current?.key === key
        ? { key, ascending: !current.ascending }
        : { key, ascending: key === 'coin' }
    )

  const term = search.trim().toUpperCase()
  const shown = term
    ? ordered.filter((row) => row.symbol.includes(term))
    : ordered
  const typed = term.endsWith('USDT') ? term : `${term}USDT`
  const canFetch = /^[A-Z0-9]{1,20}USDT$/.test(typed)

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Enter' || !term) return
    event.preventDefault()
    const exact = ordered.find((row) => row.symbol === typed)
    if (exact || shown.length !== 1) {
      if (exact) open(exact.symbol)
      else if (canFetch) open(typed)
    } else {
      open(shown[0].symbol)
    }
  }

  const header = { sort, onSort: chooseColumn }
  return (
    <div className={cn('flex min-h-0 flex-col', className)}>
      <div className="shrink-0 p-2" onKeyDown={onKeyDown}>
        <SearchField
          label={label('searchCoins')}
          value={search}
          onChange={setSearch}
        />
      </div>
      {failed && signals.length === 0 ? (
        <PanelState role="alert">{label('loadFailed')}</PanelState>
      ) : loading && signals.length === 0 ? (
        <PanelState role="status">{label('loading')}</PanelState>
      ) : shown.length === 0 ? (
        <PanelState role="status">
          {term
            ? canFetch
              ? label('fetchCoin', { symbol: typed })
              : label('noMatch')
            : label('noCoins')}
        </PanelState>
      ) : (
        <ScrollArea className="min-h-0 flex-1 rounded-b-xl">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead className="text-vergex-text-muted">
              <tr>
                <SortHeader
                  column="coin"
                  title={label('coin')}
                  className={headClass}
                  {...header}
                />
                <SortHeader
                  column="price"
                  title={label('price')}
                  className={cn(headClass, 'text-right')}
                  {...header}
                />
                <SortHeader
                  column="score"
                  title={label('score')}
                  className={cn(headClass, 'text-right')}
                  {...header}
                />
              </tr>
            </thead>
            <tbody>
              {shown.map((row) => {
                const record = row.record
                const state = record
                  ? row.stale
                    ? label('stale')
                    : undefined
                  : label(`pendingState.${row.pending ?? 'waiting'}`)
                return (
                  <tr
                    key={row.symbol}
                    aria-current={row.symbol === selected ? 'true' : undefined}
                    title={state}
                    className="cursor-pointer border-t border-[var(--line)] hover:bg-vergex-bg aria-[current=true]:bg-[var(--brand-soft)]"
                    onClick={() => open(row.symbol)}
                  >
                    <td className={cn(cellClass, 'max-w-0 w-full')}>
                      <button
                        type="button"
                        className="block max-w-full truncate font-medium text-vergex-text"
                        onClick={(event) => {
                          event.stopPropagation()
                          open(row.symbol)
                        }}
                      >
                        {baseAsset(row.symbol)}
                      </button>
                    </td>
                    <td className={cn(cellClass, 'text-right')}>
                      {fmtPrice(row.price)}
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        'text-right font-medium',
                        (!record || row.stale) && 'text-vergex-text-muted'
                      )}
                    >
                      {record ? (
                        <span className="inline-flex items-center justify-end gap-1">
                          <DirectionIcon direction={record.direction} />
                          {scoreText(record.display_prob, record.calibrated)}
                        </span>
                      ) : (
                        '—'
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </ScrollArea>
      )}
    </div>
  )
}
