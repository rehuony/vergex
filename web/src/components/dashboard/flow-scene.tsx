import {
  forwardRef,
  memo,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react'
import { X } from 'lucide-react'
import type { ActivityEvent, ActivityTotals } from '@/api/activity'
import type { Overview } from '@/api/overview'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import {
  LINE_KIND,
  layoutFlow,
  lineOf,
  type BlockPlace,
  type FlowLine,
  type LineKey,
  type SceneBlock,
} from './flow-layout'
import { blockViews, routeTotal, type Member } from './flow-members'
import { createFlowRunner, type FlowRunner } from './flow-runner'
import {
  eventColor,
  lineSummary,
  overallSummary,
  smallUsd,
  type FlowSelection,
} from './flow-text'

type Label = (key: string, params?: Record<string, string | number>) => string

/** Between groups of a burst, so what follows from what stays visible. */
const STAGGER_MS = 240
const MEMBER_LIT_MS = 700
const BURST_SHOWN_MS = 1500
const MAX_GROUPS = 12

// Dot sizes, as shares of the grid's pitch.
const ROUTE_DOT = 0.15
const SLOT_DOT = 0.13
const MEMBER_DOT = 0.36
const STOPPED_DOT = 0.28

const MEMBER_CLASS: Record<Member['tone'], string> = {
  on: 'fill-vergex-text',
  off: 'fill-none stroke-[var(--muted)]',
  up: 'fill-vergex-success',
  down: 'fill-vergex-danger',
  alert: 'fill-vergex-danger',
}

function activate(event: KeyboardEvent, run: () => void) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    run()
  }
}

// The dots of one line. Memoized so that a re-render never touches what the runner
// is drawing on them.
const LineDots = memo(function LineDots({
  line,
  pitch,
}: {
  line: FlowLine
  pitch: number
}) {
  const radius = pitch * ROUTE_DOT
  return (
    <>
      {line.dots.map((dot) => (
        <circle
          key={`${dot.x},${dot.y}`}
          cx={dot.x}
          cy={dot.y}
          r={radius}
          data-rest={radius}
          className="fill-[var(--line-strong)]"
        />
      ))}
    </>
  )
})

function Matrix({
  block,
  place,
  pitch,
  members,
  from = 0,
}: {
  block: string
  place: BlockPlace
  pitch: number
  members: Member[]
  from?: number
}) {
  const shown = Math.max(place.cols * place.rows - from, 0)
  return (
    <>
      {members.slice(0, shown).map((member, index) => {
        const slot = from + index
        return (
          <circle
            key={`${member.id}-${slot}`}
            data-member={`${block}:${member.id}`}
            cx={place.x + (slot % place.cols) * pitch}
            cy={place.y + Math.floor(slot / place.cols) * pitch}
            r={pitch * (member.tone === 'off' ? STOPPED_DOT : MEMBER_DOT)}
            className={MEMBER_CLASS[member.tone]}
          >
            {member.title && <title>{member.title}</title>}
          </circle>
        )
      })}
    </>
  )
}

export interface FlowSceneHandle {
  play: (events: ActivityEvent[]) => void
}

interface FlowSceneProps {
  overview: Overview
  totals: ActivityTotals
  kind: string
  selection: FlowSelection | null
  onSelect: (selection: FlowSelection | null) => void
  /** A line the event list is pointing at. */
  pointed: LineKey | null
  /** Says the data has stopped updating; empty while it is live. */
  staleText: string
  label: Label
}

// The Dashboard's main surface: every block's members as dots, and the routes between
// them. Nothing moves here unless an event just happened, and each one lights its
// route once. It draws at real size, so text stays on the type scale at any width.
export const FlowScene = forwardRef<FlowSceneHandle, FlowSceneProps>(
  function FlowScene(
    { overview, totals, kind, selection, onSelect, pointed, staleText, label },
    ref
  ) {
    const box = useRef<HTMLDivElement>(null)
    const svg = useRef<SVGSVGElement>(null)
    const runner = useRef<FlowRunner | null>(null)
    const onScreen = useRef(true)
    const [size, setSize] = useState({ width: 0, height: 0 })
    const [hovered, setHovered] = useState<string | null>(null)
    const [bursts, setBursts] = useState<Partial<Record<LineKey, number>>>({})

    useEffect(() => {
      const target = box.current
      if (!target) return
      const measure = () =>
        setSize({ width: target.clientWidth, height: target.clientHeight })
      measure()
      const resizes = new ResizeObserver(measure)
      resizes.observe(target)
      const visibility = new IntersectionObserver(([entry]) => {
        onScreen.current = entry.isIntersecting
      })
      visibility.observe(target)
      const flow = createFlowRunner()
      runner.current = flow
      return () => {
        resizes.disconnect()
        visibility.disconnect()
        flow.stop()
      }
    }, [])

    const layout = useMemo(
      () => (size.width > 0 ? layoutFlow(size.width, size.height) : null),
      [size]
    )
    const views = useMemo(() => blockViews(overview, label), [overview, label])

    useImperativeHandle(
      ref,
      () => ({
        play(events) {
          const flow = runner.current
          const root = svg.current
          if (!flow || !root || !onScreen.current || document.hidden) return

          // Events along the same line in one burst share a run and show a count.
          const groups = new Map<string, ActivityEvent[]>()
          events.forEach((event) => {
            const line = lineOf(event)
            if (!line) return
            const key = `${line.key}|${line.reverse}|${eventColor(event)}`
            groups.set(key, [...(groups.get(key) ?? []), event])
          })

          let delay = 0
          ;[...groups.values()].slice(0, MAX_GROUPS).forEach((group) => {
            const first = group[0]
            const line = lineOf(first)
            if (!line) return
            const dots = Array.from(
              root.querySelectorAll<SVGCircleElement>(
                `[data-line="${line.key}"] circle`
              )
            )
            const ordered = line.reverse ? [...dots].reverse() : dots
            const color = eventColor(first)
            const member = (end: ActivityEvent['from']) =>
              root.querySelector<SVGCircleElement>(
                `[data-member="${end.block}:${CSS.escape(end.id ?? '')}"]`
              )
            const flash = (dot: SVGCircleElement | null, at: number) => {
              if (!dot) return
              setTimeout(() => {
                dot.style.fill = color
                setTimeout(
                  () => dot.style.removeProperty('fill'),
                  MEMBER_LIT_MS
                )
              }, at)
            }
            const drawn = flow.fire(ordered, color, {
              delay,
              // A call is one event with two legs: the prompt and the reply.
              done:
                first.type === 'agent_call'
                  ? () => flow.fire([...ordered].reverse(), color)
                  : undefined,
            })
            if (drawn) {
              flash(member(first.from), delay)
              flash(member(first.to), delay + 400)
            }
            if (group.length > 1 || !drawn) {
              setBursts((current) => ({
                ...current,
                [line.key]: group.length,
              }))
              setTimeout(
                () =>
                  setBursts((current) => {
                    const next = { ...current }
                    delete next[line.key]
                    return next
                  }),
                BURST_SHOWN_MS
              )
            }
            delay += STAGGER_MS
          })
        },
      }),
      []
    )

    const lineActive = (key: LineKey) => {
      if (pointed) return pointed === key
      if (kind !== 'all' && LINE_KIND[key] !== kind) return false
      if (!selection) return true
      if (selection.type === 'line') return selection.key === key
      return key.split('>').includes(selection.id)
    }

    const selected = (next: FlowSelection) =>
      selection?.type === next.type &&
      (next.type === 'line'
        ? selection.type === 'line' && selection.key === next.key
        : selection.type === 'block' && selection.id === next.id)

    const toggle = (next: FlowSelection) =>
      onSelect(selected(next) ? null : next)

    const describe = (focus: string | null) => {
      if (focus?.includes('>')) {
        return lineSummary(focus as LineKey, totals, label)
      }
      if (focus) {
        const names = views[focus as SceneBlock].members
          .concat(views[focus as SceneBlock].shortMembers ?? [])
          .map((member) => member.title.split(' · ')[0])
          .filter(Boolean)
        const name = label(`flow.block.${focus}`)
        return names.length ? `${name} · ${names.join(', ')}` : name
      }
      return overallSummary(totals, label)
    }

    const pickedKey = selection
      ? selection.type === 'line'
        ? selection.key
        : selection.id
      : null
    const readout = describe(hovered ?? pickedKey)
    const cost = routeTotal(totals, ['models>agent_cost'], 'h24').amount
    const fees = routeTotal(totals, ['accounts>fees'], 'h24').amount
    const today = overview.realized_today
    const labelGap = layout ? layout.pitch / 2 + 6 : 0

    return (
      <div className="flex h-full min-h-0 flex-col">
        <div
          ref={box}
          className="relative h-[400px] min-h-0 overflow-hidden xl:h-auto xl:flex-1"
          onClick={(event) => {
            if (event.target === event.currentTarget) onSelect(null)
          }}
        >
          {layout && (
            <svg
              ref={svg}
              width={layout.width}
              height={layout.height}
              role="group"
              aria-label={label('flow.sceneLabel')}
              className="absolute inset-0 select-none"
            >
              {layout.lines.map((line) => {
                const burst = bursts[line.key]
                const end = line.points[line.points.length - 1]
                return (
                  <g
                    key={line.key}
                    style={{ opacity: lineActive(line.key) ? 1 : 0.25 }}
                    className="transition-opacity"
                  >
                    <g data-line={line.key}>
                      <LineDots line={line} pitch={layout.pitch} />
                    </g>
                    {burst && (
                      <text
                        x={end[0] - 4}
                        y={end[1] - 8}
                        textAnchor="end"
                        className="fill-vergex-text text-xs tabular-nums"
                      >
                        ×{burst}
                      </text>
                    )}
                    <polyline
                      points={line.points.join(' ')}
                      fill="none"
                      stroke="transparent"
                      strokeWidth={16}
                      role="button"
                      tabIndex={0}
                      aria-label={lineSummary(line.key, totals, label)}
                      aria-pressed={selected({ type: 'line', key: line.key })}
                      className="cursor-pointer outline-none focus-visible:stroke-[var(--brand-soft)]"
                      onMouseEnter={() => setHovered(line.key)}
                      onMouseLeave={() => setHovered(null)}
                      onClick={() => toggle({ type: 'line', key: line.key })}
                      onKeyDown={(event) =>
                        activate(event, () =>
                          toggle({ type: 'line', key: line.key })
                        )
                      }
                    />
                  </g>
                )
              })}
              {(Object.keys(layout.blocks) as SceneBlock[]).map((id) => {
                const place = layout.blocks[id]
                const view = views[id]
                const capacity = place.cols * place.rows
                const shortFrom = id === 'positions' ? capacity / 2 : capacity
                const hidden =
                  Math.max(view.members.length - shortFrom, 0) +
                  Math.max(
                    (view.shortMembers?.length ?? 0) - (capacity - shortFrom),
                    0
                  )
                return (
                  <g
                    key={id}
                    role="button"
                    tabIndex={0}
                    aria-label={`${label(`flow.block.${id}`)}: ${view.sub}`}
                    aria-pressed={selected({ type: 'block', id })}
                    className="group cursor-pointer outline-none"
                    onMouseEnter={() => setHovered(id)}
                    onMouseLeave={() => setHovered(null)}
                    onClick={() => toggle({ type: 'block', id })}
                    onKeyDown={(event) =>
                      activate(event, () => toggle({ type: 'block', id }))
                    }
                  >
                    <rect
                      x={place.x - 8}
                      y={place.y - 40}
                      width={place.cols * layout.pitch + 64}
                      height={place.rows * layout.pitch + 40}
                      rx={8}
                      fill="transparent"
                      className="group-focus-visible:stroke-[var(--brand)]"
                    />
                    <text
                      x={place.x - 4}
                      y={place.y - labelGap - 15}
                      className="fill-vergex-text text-[13px] font-medium"
                    >
                      {label(`flow.block.${id}`)}
                      {hidden > 0 && (
                        <tspan className="fill-[var(--muted)] text-xs font-normal">
                          {` +${hidden}`}
                        </tspan>
                      )}
                    </text>
                    <text
                      x={place.x - 4}
                      y={place.y - labelGap}
                      className={cn(
                        'text-xs tabular-nums',
                        view.warn
                          ? 'fill-[var(--warning)]'
                          : 'fill-[var(--muted)]'
                      )}
                    >
                      {view.sub}
                    </text>
                    {Array.from({ length: capacity }, (_, slot) => (
                      <circle
                        key={slot}
                        cx={place.x + (slot % place.cols) * layout.pitch}
                        cy={
                          place.y + Math.floor(slot / place.cols) * layout.pitch
                        }
                        r={layout.pitch * SLOT_DOT}
                        className="fill-[var(--line-strong)]"
                      />
                    ))}
                    <Matrix
                      block={id}
                      place={place}
                      pitch={layout.pitch}
                      members={view.members}
                    />
                    {view.shortMembers && (
                      <Matrix
                        block={id}
                        place={place}
                        pitch={layout.pitch}
                        members={view.shortMembers}
                        from={shortFrom}
                      />
                    )}
                  </g>
                )
              })}
              <text
                x={layout.costLabel.x - 4}
                y={layout.costLabel.y - 3}
                className="fill-vergex-text text-[13px] font-medium"
              >
                {label('flow.block.agent_cost')}
              </text>
              <text
                x={layout.costLabel.x - 4}
                y={layout.costLabel.y + 13}
                className="fill-[var(--muted)] text-xs tabular-nums"
              >
                {label('flow.estimatedDay', { amount: smallUsd(cost) })}
              </text>
              <text
                x={layout.feesLabel.x - 4}
                y={layout.feesLabel.y}
                className="fill-vergex-text text-[13px] font-medium"
              >
                {label('flow.block.fees')}
              </text>
              <text
                x={layout.feesLabel.x - 4}
                y={layout.feesLabel.y + 15}
                className="fill-[var(--muted)] text-xs tabular-nums"
              >
                {label('flow.day', { amount: smallUsd(fees) })}
              </text>
              <text
                x={layout.todayLabel.x - 4}
                y={layout.todayLabel.y}
                className={cn(
                  'text-xs tabular-nums',
                  today == null
                    ? 'fill-[var(--muted)]'
                    : today >= 0
                      ? 'fill-vergex-success'
                      : 'fill-vergex-danger'
                )}
              >
                {label('flow.today', {
                  amount:
                    today == null
                      ? '—'
                      : `${today > 0 ? '+' : today < 0 ? '-' : ''}$${Math.abs(today).toFixed(2)}`,
                })}
              </text>
            </svg>
          )}
        </div>
        <div className="flex h-10 shrink-0 items-center gap-2 border-t border-[var(--line)] pl-4 pr-2 text-xs text-vergex-text-muted">
          <span
            className="min-w-0 flex-1 truncate tabular-nums"
            title={readout}
          >
            {readout}
          </span>
          {staleText && (
            <span
              role="status"
              className="shrink-0 whitespace-nowrap tabular-nums text-[var(--warning)]"
            >
              {staleText}
            </span>
          )}
          {selection && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={label('flow.clear')}
              className="size-7 shrink-0"
              onClick={() => onSelect(null)}
            >
              <X className="size-3.5" aria-hidden="true" />
            </Button>
          )}
        </div>
      </div>
    )
  }
)
