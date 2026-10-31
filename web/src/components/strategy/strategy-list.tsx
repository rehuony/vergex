import { t, type Language } from '@/i18n/translations'
import { Panel, PanelState } from '@/components/common/panel'
import { SettingBadge } from '@/components/settings/setting-row'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { Strategy } from '@/types'

export const strategyTypeLabel = (strategy: Strategy, language: Language) =>
  t(
    strategy.config.strategy_type === 'grid_trading'
      ? 'strategy.typeGrid'
      : 'strategy.typeAi',
    language
  )

// The side list of strategies, marking the active one. A long list scrolls
// inside the panel.
export function StrategyList({
  strategies,
  selectedId,
  onSelect,
  language,
  className,
}: {
  strategies: Strategy[]
  selectedId?: string
  onSelect: (strategy: Strategy) => void
  language: Language
  className?: string
}) {
  return (
    <Panel className={className}>
      {strategies.length === 0 ? (
        <PanelState role="status">
          {t('strategy.noStrategies', language)}
        </PanelState>
      ) : (
        <ScrollArea className="max-h-[70vh] rounded-xl">
          <ul className="p-1.5">
            {strategies.map((strategy) => (
              <li key={strategy.id}>
                <Button
                  type="button"
                  variant="ghost"
                  aria-current={selectedId === strategy.id ? 'true' : undefined}
                  onClick={() => onSelect(strategy)}
                  className="h-auto w-full flex-col items-stretch gap-0.5 whitespace-normal px-2.5 py-2 text-left hover:text-vergex-text-muted aria-[current=true]:bg-[var(--brand-soft)]"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="min-w-0 flex-1 truncate text-sm font-medium text-vergex-text">
                      {strategy.name}
                    </span>
                    {strategy.is_active && (
                      <SettingBadge tone="success">
                        {t('strategy.active', language)}
                      </SettingBadge>
                    )}
                  </span>
                  <span className="text-xs font-normal">
                    {strategyTypeLabel(strategy, language)}
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        </ScrollArea>
      )}
    </Panel>
  )
}
