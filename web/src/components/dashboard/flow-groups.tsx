import type { ActivityStream, ActivityTotals, Overview } from '@/api/dashboard'
import { cn } from '@/utils/cn'
import type { SceneBlock } from './flow-layout'
import { blockViews, type Member } from './flow-members'
import { overallSummary, type FlowSelection } from './flow-text'

type Label = (key: string, params?: Record<string, string | number>) => string

const GROUPS: SceneBlock[][] = [
  ['exchange', 'market'],
  ['telegram', 'traders', 'futures', 'forecast'],
  ['models', 'accounts', 'positions'],
]

// As many as fit on one line beside the figure on a phone, so a block with
// hundreds of members is as tall as one with none.
const MAX_DOTS = 7

const TONE: Record<Member['tone'], string> = {
  on: 'bg-vergex-ink',
  off: 'border border-vergex-muted',
  up: 'bg-vergex-success',
  down: 'bg-vergex-danger',
  alert: 'bg-vergex-danger',
}

// The scene on a phone, where routes will not fit: the same blocks and members,
// grouped from what feeds the system down to where it sends things.
export function FlowGroups({
  overview,
  totals,
  stream,
  selection,
  onSelect,
  staleText,
  label,
}: {
  overview: Overview
  totals: ActivityTotals
  stream: ActivityStream | null
  selection: FlowSelection | null
  onSelect: (selection: FlowSelection | null) => void
  staleText: string
  label: Label
}) {
  const views = blockViews(overview, label, totals, stream)
  const summary = overallSummary(totals, label)
  return (
    <div>
      {GROUPS.map((group) => (
        <ul
          key={group[0]}
          className="border-t border-vergex-line-strong first:border-t-0"
        >
          {group.map((id) => {
            const view = views[id]
            const members = [...view.members, ...(view.shortMembers ?? [])]
            const active = selection?.type === 'block' && selection.id === id
            return (
              <li
                key={id}
                className="border-t border-vergex-line first:border-0"
              >
                <button
                  type="button"
                  aria-pressed={active}
                  onClick={() =>
                    onSelect(active ? null : { type: 'block', id })
                  }
                  className={cn(
                    'grid h-11 w-full grid-cols-[88px_minmax(0,1fr)_auto] items-center gap-2 px-4 text-left',
                    active && 'bg-vergex-surface-muted'
                  )}
                >
                  <span className="text-[13px] font-medium text-vergex-ink">
                    {label(`flow.block.${id}`)}
                  </span>
                  <span className="flex min-w-0 items-center gap-1">
                    <span className="flex min-w-0 items-center gap-[3px] overflow-hidden">
                      {members.slice(0, MAX_DOTS).map((member, slot) => (
                        <i
                          key={`${member.id}-${slot}`}
                          className={cn(
                            'size-1.5 shrink-0 rounded-full',
                            TONE[member.tone]
                          )}
                        />
                      ))}
                    </span>
                    {members.length > MAX_DOTS && (
                      <span className="shrink-0 text-xs text-vergex-muted">
                        +{members.length - MAX_DOTS}
                      </span>
                    )}
                  </span>
                  <span
                    className={cn(
                      'whitespace-nowrap text-xs tabular-nums',
                      view.warn ? 'text-vergex-warning' : 'text-vergex-muted'
                    )}
                  >
                    {view.sub}
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      ))}
      <div className="flex h-10 items-center gap-2 border-t border-vergex-line px-4 text-xs text-vergex-muted">
        <span className="min-w-0 flex-1 truncate tabular-nums" title={summary}>
          {summary}
        </span>
        {staleText && (
          <span
            role="status"
            className="shrink-0 whitespace-nowrap text-vergex-warning"
          >
            {staleText}
          </span>
        )}
      </div>
    </div>
  )
}
