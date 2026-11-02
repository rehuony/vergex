import { useEffect, useState, type FormEvent } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import { futuresAPI } from '@/api/futures'
import { dialogInputClass } from '@/components/common/dialog'
import { PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'

type Market = {
  symbol: string
  base: string
  category: string
  volume_24h: number
  price: string
  execution_supported: boolean
}
type Board = {
  venue: string
  markets: Market[]
  fetched_at: string
  sources: string[]
  warnings?: string[]
}

const MAX_ROWS = 100

const volume = (value: number) =>
  new Intl.NumberFormat('en-US', {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(value)

// A symbol typed by hand keeps a venue-native form such as a "dex:COIN" name.
const typedSymbol = (text: string) =>
  text.trim().includes(':') ? text.trim() : text.trim().toUpperCase()

// MarketPicker shows the open market. It opens a searchable
// list of the venue's markets by volume, loaded when first opened; any other
// native symbol can be typed and opened with Enter.
export function MarketPicker({
  venue,
  selected,
  disabled,
  onSelect,
}: {
  venue: string
  selected: string
  disabled: boolean
  onSelect: (symbol: string) => void
}) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`futures.${key}`, language, params)
  const [open, setOpen] = useState(false)
  const [board, setBoard] = useState<Board | null>(null)
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    if (!open) return
    let active = true
    setLoading(true)
    setError(false)
    futuresAPI<Board>('markets?venue=' + encodeURIComponent(venue))
      .then((next) => {
        if (
          next.venue !== venue ||
          !Array.isArray(next.markets) ||
          next.markets.some(
            (m) =>
              typeof m.symbol !== 'string' ||
              !Number.isFinite(m.volume_24h) ||
              m.volume_24h < 0
          )
        )
          throw new Error('Invalid public market list')
        if (active) setBoard(next)
      })
      .catch(() => {
        if (active) setError(true)
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [open, venue, revision])

  const current = board?.venue === venue ? board : null
  const rows = (current?.markets ?? [])
    .filter((m) =>
      (m.symbol + ' ' + m.base).toLowerCase().includes(search.toLowerCase())
    )
    .slice()
    .sort(
      (a, b) => b.volume_24h - a.volume_24h || a.symbol.localeCompare(b.symbol)
    )

  function choose(symbol: string) {
    setOpen(false)
    setSearch('')
    onSelect(symbol)
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (search.trim()) choose(rows[0]?.symbol ?? typedSymbol(search))
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setSearch('')
      }}
    >
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="select"
          disabled={disabled}
          aria-label={label('market', { symbol: selected })}
          className="max-sm:h-10"
        >
          <span className="font-medium">{selected}</span>
          <ChevronDown
            className="size-4 text-vergex-muted"
            aria-hidden="true"
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[min(360px,calc(100vw-32px))] p-2">
        <form onSubmit={submit}>
          <input
            autoFocus
            aria-label={label('searchMarkets')}
            placeholder={label('searchMarketsHint')}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className={cn(dialogInputClass, 'max-sm:text-base')}
          />
        </form>
        {!current && error ? (
          <PanelState role="alert">
            {label('marketsFailed')}
            <Button
              type="button"
              size="sm"
              onClick={() => setRevision((v) => v + 1)}
            >
              {label('retry')}
            </Button>
          </PanelState>
        ) : !current ? (
          <PanelState role="status">
            {loading ? label('loadingMarkets') : ''}
          </PanelState>
        ) : rows.length === 0 ? (
          <PanelState role="status">
            {search.trim() ? label('openTyped') : label('noMarkets')}
          </PanelState>
        ) : (
          <ScrollArea className="mt-1 max-h-72">
            <ul>
              {rows.slice(0, MAX_ROWS).map((market) => (
                <li key={market.symbol}>
                  <Button
                    type="button"
                    variant="ghost"
                    aria-pressed={selected === market.symbol}
                    title={
                      market.execution_supported
                        ? undefined
                        : label('chartOnlyHint')
                    }
                    className="h-10 w-full justify-start gap-2 rounded-md px-2 font-normal text-vergex-ink"
                    onClick={() => choose(market.symbol)}
                  >
                    <span className="min-w-0 truncate font-medium">
                      {market.symbol}
                    </span>
                    {!market.execution_supported && (
                      <span className="text-xs text-vergex-muted">
                        {label('chartOnly')}
                      </span>
                    )}
                    <span className="ml-auto text-xs tabular-nums text-vergex-muted">
                      {volume(market.volume_24h)}
                    </span>
                    <Check
                      className={cn(
                        'size-4 shrink-0 text-vergex-ink',
                        selected !== market.symbol && 'invisible'
                      )}
                      aria-hidden="true"
                    />
                  </Button>
                </li>
              ))}
            </ul>
          </ScrollArea>
        )}
        {current && (
          <p className="truncate px-2 pt-2 text-[11px] text-vergex-muted">
            {label('marketsMeta', {
              count: rows.length,
              time: new Date(current.fetched_at).toLocaleTimeString('en-GB', {
                hour12: false,
              }),
            })}
            {current.warnings?.length
              ? ` · ${current.warnings.join(' · ')}`
              : ''}
          </p>
        )}
      </PopoverContent>
    </Popover>
  )
}
