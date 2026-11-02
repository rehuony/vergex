import { useRef, useState, type KeyboardEvent } from 'react'
import { Check, ChevronDown, SlidersHorizontal } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/api'
import type { ForecastStrategy } from '@/api/forecast'
import { dialogInputClass } from '@/components/common/dialog'
import { confirmToast, notify } from '@/components/common/notify'
import { PanelState } from '@/components/common/panel'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ROUTES } from '@/constants/route-paths'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import { fmtDate } from './forecast-format'
import { strategiesQuery } from './forecast-queries'

// A short list needs no search box.
const SEARCH_FROM = 7

// The saved strategies, and turning them on and off. Signals and alerts follow the one in
// use and nothing trades on them, so only what deletes records asks first.
export function useStrategies(language: Language) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const client = useQueryClient()
  const { data, error, refetch } = useQuery(strategiesQuery)
  const change = async (run: () => Promise<void>, done: string) => {
    try {
      await run()
      await refetch()
      // Everything Forecast shows follows the strategy in use.
      void client.invalidateQueries({
        predicate: ({ queryKey }) =>
          String(queryKey[0]).startsWith('forecast-'),
      })
      notify.success(done)
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('useFailed'))
    }
  }
  const activate = (strategy: ForecastStrategy) =>
    change(
      () => api.activateForecastStrategy(strategy.id),
      label('strategyActivated', { name: strategy.name })
    )
  const deactivate = (strategy: ForecastStrategy) =>
    change(
      () => api.deactivateForecastStrategy(strategy.id),
      label('strategyStopped', { name: strategy.name })
    )
  // What a retired strategy recorded came from older code, so using it again clears that.
  const revive = async (strategy: ForecastStrategy) => {
    const ok = await confirmToast(
      label('reviveMessage', { name: strategy.name }),
      {
        title: label('reviveTitle'),
        okText: label('reviveOk'),
        cancelText: label('deleteCancel'),
        tone: 'danger',
      }
    )
    if (ok) {
      await change(
        () => api.reviveForecastStrategy(strategy.id),
        label('strategyActivated', { name: strategy.name })
      )
    }
  }
  const toggle = (strategy: ForecastStrategy) =>
    strategy.id === data?.active_strategy_id
      ? deactivate(strategy)
      : strategy.retired
        ? revive(strategy)
        : activate(strategy)
  return { data, error, reload: refetch, toggle }
}

// The strategy in use, beside the Signals tab: the same trigger and rows as the other
// selects, as wide as each other, with a way to where strategies are managed.
export function StrategySelect({ language }: { language: Language }) {
  const label = (key: string) => t(`forecast.${key}`, language)
  const { data, toggle } = useStrategies(language)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const panel = useRef<HTMLDivElement>(null)
  // Holds its place while loading, so nothing beside it moves.
  if (!data) return <div className="h-9 w-52 max-sm:h-10" />

  const usable = data.strategies.filter((strategy) => !strategy.retired)
  const inUse = data.strategies.find(
    (strategy) => strategy.id === data.active_strategy_id
  )
  const term = search.trim().toLowerCase()
  const rows = term
    ? usable.filter((strategy) => strategy.name.toLowerCase().includes(term))
    : usable

  const close = () => {
    setOpen(false)
    setSearch('')
  }
  const choose = (strategy: ForecastStrategy) => {
    close()
    if (strategy.id !== inUse?.id) void toggle(strategy)
  }
  // Up and down move between the rows, and from the search box into them.
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    const options = [
      ...(panel.current?.querySelectorAll<HTMLElement>('[data-strategy]') ??
        []),
    ]
    if (options.length === 0) return
    event.preventDefault()
    const at = options.indexOf(document.activeElement as HTMLElement)
    const step = event.key === 'ArrowDown' ? 1 : -1
    options[(at + step + options.length) % options.length].focus()
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => (next ? setOpen(true) : close())}
    >
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="select"
          className="w-52 gap-2 max-sm:h-10"
        >
          <span className="shrink-0 text-vergex-muted">
            {label('strategy')}
          </span>
          <span className="min-w-0 truncate font-medium">
            {inUse?.name ?? label('noneInUse')}
          </span>
          <ChevronDown
            className="ml-auto size-4 shrink-0 text-vergex-muted"
            aria-hidden="true"
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        ref={panel}
        align="end"
        className="w-(--radix-popover-trigger-width) p-1.5"
        onKeyDown={onKeyDown}
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          panel.current
            ?.querySelector<HTMLElement>('input, [aria-pressed="true"]')
            ?.focus()
        }}
      >
        {usable.length >= SEARCH_FROM && (
          <input
            aria-label={label('searchStrategies')}
            placeholder={label('searchStrategies')}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && rows.length > 0) {
                event.preventDefault()
                choose(rows[0])
              }
            }}
            className={cn(dialogInputClass, 'mb-1 max-sm:text-base')}
          />
        )}
        {rows.length === 0 ? (
          <PanelState role="status">{label('noStrategyMatch')}</PanelState>
        ) : (
          <ScrollArea className="max-h-72">
            <ul>
              {rows.map((strategy) => (
                <li key={strategy.id}>
                  <Button
                    type="button"
                    variant="ghost"
                    data-strategy
                    aria-pressed={strategy.id === inUse?.id}
                    title={`${strategy.name} · ${fmtDate(strategy.created_at)}`}
                    className="h-8 w-full justify-start gap-2 rounded-md px-2.5 text-sm font-normal text-vergex-ink"
                    onClick={() => choose(strategy)}
                  >
                    <span className="min-w-0 truncate">{strategy.name}</span>
                    <Check
                      className={cn(
                        'ml-auto size-4 shrink-0 text-vergex-ink',
                        strategy.id !== inUse?.id && 'invisible'
                      )}
                      aria-hidden="true"
                    />
                  </Button>
                </li>
              ))}
            </ul>
          </ScrollArea>
        )}
        <div className="mt-1.5 border-t border-vergex-line pt-1.5">
          <Link
            to={ROUTES.forecastStrategies}
            onClick={close}
            className={buttonVariants({
              variant: 'ghost',
              className:
                'h-8 w-full justify-start gap-2 rounded-md px-2.5 text-sm font-normal',
            })}
          >
            <SlidersHorizontal className="size-4" aria-hidden="true" />
            {label('manageStrategies')}
          </Link>
        </div>
      </PopoverContent>
    </Popover>
  )
}
