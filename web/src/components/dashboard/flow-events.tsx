import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import type { ActivityEvent, ActivityKind } from '@/api/activity'
import type { Overview } from '@/api/overview'
import { PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { SettingsLink } from '@/components/settings/settings-link'
import { buttonVariants } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { buildFrontierPath, ROUTES } from '@/constants/route-paths'
import { cn } from '@/utils/cn'
import { lineOf, type LineKey } from './flow-layout'
import {
  eventColor,
  eventMatches,
  eventRoute,
  eventStatus,
  eventTime,
  eventWhat,
  isEventBad,
  type FlowSelection,
} from './flow-text'
import { traderPath } from './overview-links'

type Label = (key: string, params?: Record<string, string | number>) => string

export type FlowKind = 'all' | ActivityKind

const KINDS: FlowKind[] = ['all', 'ai', 'funds', 'orders', 'market']

const rowClass = 'block px-4 py-2 hover:bg-vergex-bg hover:duration-0'

// A row leads to the real thing behind it.
function EventLink({
  event,
  overview,
  className,
  children,
}: {
  event: ActivityEvent
  overview: Overview
  className: string
  children: ReactNode
}) {
  if (event.type === 'plan_order') {
    return (
      <Link
        to={buildFrontierPath({ plan: event.ref, tab: 'positions' })}
        className={className}
      >
        {children}
      </Link>
    )
  }
  if (event.type === 'forecast') {
    return (
      <Link to={ROUTES.forecast} className={className}>
        {children}
      </Link>
    )
  }
  if (event.trader_id) {
    const section =
      event.type === 'order' || event.type === 'fee'
        ? 'orders'
        : event.type === 'margin' || event.type === 'realized'
          ? 'positions'
          : `decisions?decision=${event.ref}`
    return (
      <Link
        to={traderPath(overview.traders, event.trader_id, section)}
        className={className}
      >
        {children}
      </Link>
    )
  }
  if (event.from.block === 'traders' && event.from.id) {
    return (
      <Link
        to={traderPath(overview.traders, event.from.id, 'decisions')}
        className={className}
      >
        {children}
      </Link>
    )
  }
  if (event.from.block === 'frontier') {
    return (
      <Link to={ROUTES.planning} className={className}>
        {children}
      </Link>
    )
  }
  if (event.from.block === 'telegram') {
    return (
      <SettingsLink section="telegram" className={className}>
        {children}
      </SettingsLink>
    )
  }
  return (
    <SettingsLink section="models" className={className}>
      {children}
    </SettingsLink>
  )
}

// Everything that happened, newest first. The filter also dims the scene's other
// lines, and pointing at a row shows which line it ran along.
export function FlowEvents({
  overview,
  events,
  loaded,
  stale,
  kind,
  onKind,
  selection,
  onPoint,
  label,
}: {
  overview: Overview
  events: ActivityEvent[]
  loaded: boolean
  stale: boolean
  kind: FlowKind
  onKind: (kind: FlowKind) => void
  selection: FlowSelection | null
  onPoint: (line: LineKey | null) => void
  label: Label
}) {
  const shown = events.filter((event) => eventMatches(event, kind, selection))
  const nothingYet = overview.traders.length === 0 && overview.runs.length === 0

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-14 shrink-0 items-center px-4">
        <Segmented
          aria-label={label('flow.filter')}
          value={kind}
          options={KINDS.map((value) => ({
            value,
            label: label(`flow.kind.${value}`),
          }))}
          onChange={onKind}
        />
      </div>
      {!loaded ? (
        <PanelState
          role={stale ? 'alert' : 'status'}
          className="flex-1 border-t border-[var(--line)]"
        >
          {label(stale ? 'flow.loadFailed' : 'flow.loading')}
        </PanelState>
      ) : shown.length === 0 ? (
        <PanelState
          role="status"
          className="flex-1 border-t border-[var(--line)]"
        >
          {events.length > 0 ? (
            label('flow.noMatch')
          ) : (
            <>
              <p>{label('flow.noEvents')}</p>
              {nothingYet && (
                <div className="flex gap-2">
                  <Link
                    to={ROUTES.traders}
                    className={buttonVariants({
                      variant: 'secondary',
                      size: 'sm',
                    })}
                  >
                    {label('flow.createTrader')}
                  </Link>
                  <Link
                    to={ROUTES.planning}
                    className={buttonVariants({
                      variant: 'secondary',
                      size: 'sm',
                    })}
                  >
                    {label('flow.planTrade')}
                  </Link>
                </div>
              )}
            </>
          )}
        </PanelState>
      ) : (
        <ScrollArea className="max-h-96 min-h-0 flex-1 border-t border-[var(--line)] xl:max-h-none">
          <ul className="divide-y divide-[var(--line)]">
            {shown.map((event) => {
              const route = eventRoute(event, label)
              const what = eventWhat(event, label)
              const status = eventStatus(event, label)
              return (
                <li
                  key={event.id}
                  onMouseEnter={() => onPoint(lineOf(event)?.key ?? null)}
                  onMouseLeave={() => onPoint(null)}
                >
                  <EventLink
                    event={event}
                    overview={overview}
                    className={rowClass}
                  >
                    <span className="flex items-center gap-2 text-[13px] text-vergex-text">
                      <i
                        className="size-1.5 shrink-0 rounded-full"
                        style={{ backgroundColor: eventColor(event) }}
                      />
                      <span className="min-w-0 flex-1 truncate" title={route}>
                        {route}
                      </span>
                      <span className="shrink-0 text-xs tabular-nums text-vergex-text-muted">
                        {eventTime(event)}
                      </span>
                    </span>
                    <span className="mt-0.5 flex items-center gap-2 pl-3.5 text-xs text-vergex-text-muted">
                      <span
                        className="min-w-0 flex-1 truncate tabular-nums"
                        title={event.detail || what}
                      >
                        {what}
                      </span>
                      {status && (
                        <span
                          className={cn(
                            'shrink-0',
                            isEventBad(event) && 'text-vergex-danger'
                          )}
                        >
                          {status}
                        </span>
                      )}
                    </span>
                  </EventLink>
                </li>
              )
            })}
          </ul>
        </ScrollArea>
      )}
    </div>
  )
}
