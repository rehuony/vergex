import { useEffect, useRef, useState } from 'react'
import { t, type Language } from '@/i18n/translations'
import { Panel, PanelState } from '@/components/common/panel'
import { SearchField } from '@/components/common/search-field'
import { SettingBadge } from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { Strategy } from '@/types'

// Past this many strategies the list gets a search box.
const SEARCH_FROM = 10

export const strategyTypeLabel = (strategy: Strategy, language: Language) =>
  t(
    strategy.config.strategy_type === 'grid_trading'
      ? 'strategy.typeGrid'
      : 'strategy.typeAi',
    language
  )

// The side list of strategies, marking the default one and how many traders use
// each. A long list scrolls inside the panel and keeps the open strategy in view.
export function StrategyList({
  strategies,
  selectedId,
  traderCounts,
  onSelect,
  language,
  className,
}: {
  strategies: Strategy[]
  selectedId?: string
  traderCounts: Record<string, number>
  onSelect: (strategy: Strategy) => void
  language: Language
  className?: string
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`strategy.${key}`, language, params)
  const [search, setSearch] = useState('')
  const list = useRef<HTMLUListElement>(null)

  useEffect(() => {
    list.current
      ?.querySelector('[aria-current="true"]')
      ?.scrollIntoView?.({ block: 'nearest' })
  }, [selectedId])

  const term = search.trim().toLowerCase()
  const shown = strategies.filter((strategy) =>
    strategy.name.toLowerCase().includes(term)
  )

  if (strategies.length === 0) {
    return (
      <Panel className={className}>
        <PanelState role="status">{label('noStrategies')}</PanelState>
      </Panel>
    )
  }

  return (
    <Panel className={className}>
      {strategies.length >= SEARCH_FROM && (
        <div className="border-b border-[var(--line)] p-1.5">
          <SearchField
            label={label('search')}
            value={search}
            onChange={setSearch}
          />
        </div>
      )}
      {shown.length === 0 ? (
        <PanelState role="status">{label('noMatch')}</PanelState>
      ) : (
        <ScrollArea className="max-h-[calc(100dvh-254px)] rounded-xl">
          <ul ref={list} className="p-1.5">
            {shown.map((strategy) => {
              const traders = traderCounts[strategy.id] ?? 0
              return (
                <li key={strategy.id}>
                  <Button
                    type="button"
                    variant="ghost"
                    aria-current={
                      selectedId === strategy.id ? 'true' : undefined
                    }
                    title={strategy.name}
                    onClick={() => onSelect(strategy)}
                    className="h-auto w-full flex-col items-stretch gap-0.5 whitespace-normal px-2.5 py-2 text-left hover:text-vergex-text-muted aria-[current=true]:bg-[var(--brand-soft)]"
                  >
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="min-w-0 flex-1 truncate text-sm font-medium text-vergex-text">
                        {strategy.name}
                      </span>
                      {strategy.is_active && (
                        <SettingBadge tone="success">
                          {label('default')}
                        </SettingBadge>
                      )}
                    </span>
                    <span className="text-xs font-normal">
                      {strategyTypeLabel(strategy, language)}
                      {traders > 0 &&
                        ` · ${label('traderCount', { count: traders })}`}
                    </span>
                  </Button>
                </li>
              )
            })}
          </ul>
        </ScrollArea>
      )}
    </Panel>
  )
}
