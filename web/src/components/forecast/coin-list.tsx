import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Star } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import type { ForecastCoin, ForecastInterval } from '@/api/forecast'
import { SearchField } from '@/components/common/search-field'
import { Segmented } from '@/components/common/segmented'
import { SortHeader, type ColumnSort } from '@/components/common/sort-header'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { buildForecastPath } from '@/constants/route-paths'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import {
  DirectionIcon,
  baseAsset,
  fmtPct,
  fmtPrice,
  scoreText,
} from './forecast-format'

type CoinColumn = 'coin' | 'price' | 'change' | 'score'
type CoinFilter = 'all' | 'watchlist' | 'up' | 'down'

const FILTER_KEY = 'vergex.forecast.filter'
const FILTERS: CoinFilter[] = ['all', 'watchlist', 'up', 'down']

const headClass =
  'sticky top-0 z-10 bg-vergex-surface px-2 py-2 text-[11px] font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'
// The last column stays clear of the overlay scrollbar, which floats over the right edge.
const lastClass = 'pr-4 text-right'

const savedFilter = (): CoinFilter => {
  try {
    const saved = localStorage.getItem(FILTER_KEY) as CoinFilter
    return FILTERS.includes(saved) ? saved : 'all'
  } catch {
    return 'all'
  }
}

// Strong calls first, in either direction; coins with no forecast yet last.
export function sortCoins(
  coins: ForecastCoin[],
  sort: ColumnSort<CoinColumn> | null = null
) {
  const strength = (coin: ForecastCoin) =>
    coin.signal ? Math.abs(coin.signal.display_prob - 0.5) : -1
  const by = (a: ForecastCoin, b: ForecastCoin) => {
    if (!sort) {
      return (
        strength(b) - strength(a) ||
        a.tier - b.tier ||
        a.symbol.localeCompare(b.symbol)
      )
    }
    const direction = sort.ascending ? 1 : -1
    if (sort.key === 'coin') return direction * a.symbol.localeCompare(b.symbol)
    const value = (coin: ForecastCoin) =>
      sort.key === 'price'
        ? coin.price
        : sort.key === 'change'
          ? coin.change_24h_pct
          : (coin.signal?.display_prob ?? null)
    const left = value(a)
    const right = value(b)
    // A coin with no value stays last whichever way the column runs.
    if (left === null || left === undefined) return right === null ? 0 : 1
    if (right === null || right === undefined) return -1
    return direction * (left - right)
  }
  return [...coins].sort(by)
}

// Keeps rows where they are while values refresh, so a row does not move under the
// pointer. The order is worked out again only when the interval or sort changes;
// coins that come and go are added at the end or dropped.
function useStableOrder(
  coins: ForecastCoin[],
  interval: ForecastInterval,
  sort: ColumnSort<CoinColumn> | null
) {
  const [held, setHeld] = useState({ key: '', symbols: [] as string[] })
  const key = `${interval}/${sort?.key}/${sort?.ascending}`
  const present = coins.map((row) => row.symbol)
  let order = held
  if (held.key !== key) {
    if (coins.length > 0) {
      order = { key, symbols: sortCoins(coins, sort).map((r) => r.symbol) }
    }
  } else {
    const kept = held.symbols.filter((symbol) => present.includes(symbol))
    const added = present.filter((symbol) => !held.symbols.includes(symbol))
    if (added.length > 0 || kept.length !== held.symbols.length) {
      order = { key, symbols: [...kept, ...added] }
    }
  }
  if (order !== held) setHeld(order)

  const bySymbol = new Map(coins.map((row) => [row.symbol, row]))
  return order.symbols.flatMap((symbol) => {
    const row = bySymbol.get(symbol)
    return row ? [row] : []
  })
}

// Every tracked coin at this interval, to move between. Typing a coin that is not
// tracked and pressing Enter opens it, which starts fetching it.
export function CoinList({
  coins,
  interval,
  selected,
  loading,
  failed,
  watchlist,
  onToggleWatch,
  onPreview,
  onChosen,
  language,
  className,
}: {
  coins: ForecastCoin[]
  interval: ForecastInterval
  selected: string
  loading: boolean
  failed: boolean
  watchlist: string[]
  onToggleWatch: (symbol: string) => void
  // Says which coin the pointer is on, or null once it has left.
  onPreview?: (symbol: string | null) => void
  onChosen?: () => void
  language: Language
  className?: string
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<ColumnSort<CoinColumn> | null>(null)
  const [filter, setFilter] = useState(savedFilter)
  // The row the arrow keys are on, for Enter to open from the search box.
  const [cursor, setCursor] = useState<string | null>(null)
  const list = useRef<HTMLDivElement>(null)
  const ordered = useStableOrder(coins, interval, sort)
  const anyShown = ordered.length > 0

  // The coin in view may be far down the list, as may the arrow keys.
  const reveal = (symbol: string | null) => {
    if (!symbol) return
    list.current
      ?.querySelector(`[data-symbol="${symbol}"]`)
      ?.scrollIntoView?.({ block: 'nearest' })
  }
  useEffect(() => reveal(cursor ?? selected), [cursor, selected, anyShown])

  const open = (symbol: string) => {
    navigate(buildForecastPath(symbol, interval), { replace: true })
    onChosen?.()
  }
  const chooseColumn = (key: CoinColumn) =>
    setSort((current) =>
      current?.key === key
        ? { key, ascending: !current.ascending }
        : { key, ascending: key === 'coin' }
    )
  const chooseFilter = (next: CoinFilter) => {
    setFilter(next)
    try {
      localStorage.setItem(FILTER_KEY, next)
    } catch {
      /* Only a convenience: it just won't be remembered. */
    }
  }

  const term = search.trim().toUpperCase()
  const filtered = ordered.filter((row) => {
    switch (filter) {
      case 'watchlist':
        return watchlist.includes(row.symbol)
      case 'up':
      case 'down':
        return row.signal?.direction === filter
    }
    return true
  })
  const shown = term
    ? filtered.filter((row) => row.symbol.includes(term))
    : filtered
  const typed = term.endsWith('USDT') ? term : `${term}USDT`
  const canFetch = /^[A-Z0-9]{1,20}USDT$/.test(typed)

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      if (shown.length === 0) return
      event.preventDefault()
      const at = shown.findIndex((row) => row.symbol === (cursor ?? selected))
      const step = event.key === 'ArrowDown' ? 1 : -1
      setCursor(shown[(at + step + shown.length) % shown.length].symbol)
      return
    }
    if (event.key === 'Escape' && (term || cursor)) {
      event.preventDefault()
      setSearch('')
      setCursor(null)
      return
    }
    if (event.key !== 'Enter' || (!term && !cursor)) return
    event.preventDefault()
    const exact = ordered.find((row) => row.symbol === typed)
    const target =
      cursor && shown.some((row) => row.symbol === cursor)
        ? cursor
        : exact
          ? exact.symbol
          : shown.length === 1
            ? shown[0].symbol
            : canFetch
              ? typed
              : null
    if (!target) return
    setSearch('')
    setCursor(null)
    open(target)
  }

  const emptyMessage = () => {
    if (term)
      return canFetch ? label('fetchCoin', { symbol: typed }) : label('noMatch')
    if (filter === 'watchlist') return label('noWatchlist')
    if (filter === 'up' || filter === 'down') return label('noCalls')
    return label('noCoins')
  }

  const header = { sort, onSort: chooseColumn }
  return (
    <div className={cn('flex min-h-0 flex-col', className)}>
      <div className="shrink-0 space-y-2 p-2" onKeyDown={onKeyDown}>
        <SearchField
          label={label('searchCoins')}
          value={search}
          onChange={(next) => {
            setSearch(next)
            setCursor(null)
          }}
        />
        <Segmented
          aria-label={label('filterCoins')}
          className="flex [&>button]:flex-1"
          value={filter}
          options={FILTERS.map((value) => ({
            value,
            label: label(`filter.${value}`),
          }))}
          onChange={chooseFilter}
        />
      </div>
      {failed && coins.length === 0 ? (
        <PanelState role="alert" className="flex-1">
          {label('loadFailed')}
        </PanelState>
      ) : loading && coins.length === 0 ? (
        <PanelLoading className="flex-1" />
      ) : shown.length === 0 ? (
        <PanelState role="status" className="flex-1">
          {emptyMessage()}
        </PanelState>
      ) : (
        <ScrollArea className="min-h-0 flex-1 rounded-b-2xl" viewportRef={list}>
          <table className="w-full table-fixed text-left text-[13px] tabular-nums">
            <colgroup>
              <col />
              <col className="w-[82px]" />
              <col className="w-[66px]" />
              <col className="w-[70px]" />
            </colgroup>
            <thead className="text-vergex-muted">
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
                  column="change"
                  title={label('change24h')}
                  className={cn(headClass, 'text-right')}
                  {...header}
                />
                <SortHeader
                  column="score"
                  title={label('score')}
                  className={cn(headClass, lastClass)}
                  {...header}
                />
              </tr>
            </thead>
            <tbody>
              {shown.map((row) => {
                const signal = row.signal
                const starred = watchlist.includes(row.symbol)
                const change = row.change_24h_pct
                const state = signal
                  ? row.stale
                    ? label('stale')
                    : undefined
                  : label(`pendingState.${row.pending ?? 'waiting'}`)
                return (
                  <tr
                    key={row.symbol}
                    data-symbol={row.symbol}
                    aria-current={row.symbol === selected ? 'true' : undefined}
                    title={state}
                    className={cn(
                      'scroll-mt-8 cursor-pointer border-t border-vergex-line hover:bg-vergex-surface-muted aria-[current=true]:bg-vergex-ink-tint',
                      row.symbol === cursor && 'bg-vergex-surface-muted'
                    )}
                    onClick={() => open(row.symbol)}
                    onMouseEnter={() => onPreview?.(row.symbol)}
                    onMouseLeave={() => onPreview?.(null)}
                  >
                    <td className={cn(cellClass, 'pl-1 pr-0')}>
                      <span className="flex min-w-0 items-center">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-pressed={starred}
                          aria-label={label(
                            starred ? 'removeWatch' : 'addWatch',
                            { symbol: baseAsset(row.symbol) }
                          )}
                          onClick={(event) => {
                            event.stopPropagation()
                            onToggleWatch(row.symbol)
                          }}
                          className="size-6 shrink-0 max-lg:size-10"
                        >
                          <Star
                            className={cn(
                              'size-3.5',
                              starred
                                ? 'fill-current text-vergex-ink'
                                : 'text-vergex-faint'
                            )}
                            aria-hidden="true"
                          />
                        </Button>
                        <button
                          type="button"
                          data-coin
                          title={row.symbol}
                          className="min-w-0 truncate font-medium text-vergex-ink"
                          onClick={(event) => {
                            event.stopPropagation()
                            open(row.symbol)
                          }}
                        >
                          {baseAsset(row.symbol)}
                        </button>
                      </span>
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        'truncate text-right font-mono text-xs'
                      )}
                    >
                      {fmtPrice(row.price)}
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        'text-right font-mono text-xs',
                        change === null || change === undefined
                          ? 'text-vergex-muted'
                          : change >= 0
                            ? 'text-vergex-success'
                            : 'text-vergex-danger'
                      )}
                    >
                      {fmtPct(change ?? 0, true, 1)}
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        lastClass,
                        'font-medium',
                        (!signal || row.stale) && 'text-vergex-muted'
                      )}
                    >
                      {signal ? (
                        <span className="inline-flex items-center justify-end gap-1">
                          <DirectionIcon direction={signal.direction} />
                          {scoreText(signal.display_prob, signal.calibrated)}
                        </span>
                      ) : (
                        0
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
