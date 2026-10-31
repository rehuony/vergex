import type { ActivityTotals } from '@/api/activity'
import type { Overview } from '@/api/overview'
import { cn } from '@/utils/cn'
import type { SceneBlock } from './flow-layout'
import { blockViews, type Member } from './flow-members'
import { overallSummary, type FlowSelection } from './flow-text'

type Label = (key: string, params?: Record<string, string | number>) => string

const GROUPS: SceneBlock[][] = [
  ['market'],
  ['telegram', 'traders', 'frontier', 'forecast'],
  ['models', 'accounts', 'positions'],
]

const MAX_DOTS = 18

const TONE: Record<Member['tone'], string> = {
  on: 'bg-vergex-text',
  off: 'border border-[var(--muted)]',
  up: 'bg-vergex-success',
  down: 'bg-vergex-danger',
  alert: 'bg-vergex-danger',
}

// The scene on a phone, where routes will not fit: the same blocks and members,
// grouped from what feeds the system down to where it sends things.
export function FlowGroups({
  overview,
  totals,
  selection,
  onSelect,
  staleText,
  label,
}: {
  overview: Overview
  totals: ActivityTotals
  selection: FlowSelection | null
  onSelect: (selection: FlowSelection | null) => void
  staleText: string
  label: Label
}) {
  const views = blockViews(overview, label)
  return (
    <div>
      {GROUPS.map((group, index) => (
        <ul
          key={group[0]}
          className={cn(index > 0 && 'border-t border-[var(--line-strong)]')}
        >
          {group.map((id) => {
            const view = views[id]
            const members = [...view.members, ...(view.shortMembers ?? [])]
            const active = selection?.type === 'block' && selection.id === id
            return (
              <li
                key={id}
                className="border-t border-[var(--line)] first:border-0"
              >
                <button
                  type="button"
                  aria-pressed={active}
                  onClick={() =>
                    onSelect(active ? null : { type: 'block', id })
                  }
                  className={cn(
                    'grid min-h-11 w-full grid-cols-[88px_minmax(0,1fr)_auto] items-center gap-2 px-4 text-left',
                    active && 'bg-vergex-bg'
                  )}
                >
                  <span className="text-[13px] font-medium text-vergex-text">
                    {label(`flow.block.${id}`)}
                  </span>
                  <span className="flex flex-wrap items-center gap-[3px]">
                    {members.slice(0, MAX_DOTS).map((member, slot) => (
                      <i
                        key={`${member.id}-${slot}`}
                        className={cn(
                          'size-1.5 rounded-full',
                          TONE[member.tone]
                        )}
                      />
                    ))}
                    {members.length > MAX_DOTS && (
                      <span className="pl-1 text-xs text-vergex-text-muted">
                        +{members.length - MAX_DOTS}
                      </span>
                    )}
                  </span>
                  <span
                    className={cn(
                      'whitespace-nowrap text-xs tabular-nums',
                      view.warn
                        ? 'text-[var(--warning)]'
                        : 'text-vergex-text-muted'
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
      <div className="flex min-h-10 items-center gap-2 border-t border-[var(--line)] px-4 py-2 text-xs text-vergex-text-muted">
        <span className="min-w-0 flex-1 tabular-nums">
          {overallSummary(totals, label)}
        </span>
        {staleText && (
          <span role="status" className="text-[var(--warning)]">
            {staleText}
          </span>
        )}
      </div>
    </div>
  )
}
