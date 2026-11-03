import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react'
import { RotateCcw } from 'lucide-react'
import type {
  ActivityEvent,
  ActivityStream,
  ActivityTotals,
  Overview,
} from '@/api/dashboard'
import { PanelLoading } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/utils/cn'
import {
  BLOCKS,
  LINES,
  LINE_KIND,
  STAGES,
  lineOf,
  type LineKey,
  type SceneBlock,
} from './flow-layout'
import { blockViews, routeVolume } from './flow-members'
import { eventColor, lineSummary, type FlowSelection } from './flow-text'
import type { FlowStage } from './flow-stage'

type Label = (key: string, params?: Record<string, string | number>) => string

/** Between groups of a burst, so what follows from what stays visible. */
const STAGGER_MS = 240
const BURST_SHOWN_MS = 1500
const MAX_GROUPS = 12
/** How many coins the hover text names before it says how many more. */
const NAMES_SHOWN = 12

// Text stands directly on the stage, with the canvas behind each stroke to keep it legible.
const onStage = {
  textShadow:
    '0 0 2px var(--canvas), 0 0 2px var(--canvas), 0 0 4px var(--canvas), 0 0 6px var(--canvas)',
}

const blockIds = Object.keys(BLOCKS) as SceneBlock[]
const lineKeys = Object.keys(LINES) as LineKey[]

export interface FlowSceneHandle {
  play: (events: ActivityEvent[]) => void
  /** What the market streams delivered in the feed's latest round. */
  pulse: (flow: ActivityStream) => void
}

interface FlowSceneProps {
  /** Undefined while it is first read: the stage draws, and the names wait for their figures. */
  overview: Overview | undefined
  totals: ActivityTotals
  stream: ActivityStream | null
  kind: string
  selection: FlowSelection | null
  onSelect: (selection: FlowSelection | null) => void
  /** A line the event list is pointing at. */
  pointed: LineKey | null
  /** Says the data has stopped updating; empty while it is live. */
  staleText: string
  /** Called when the stage cannot draw here, so the page can show the list instead. */
  onUnavailable: () => void
  label: Label
}

// The Dashboard's main surface. The stage draws in WebGL and everything that reads as text
// stays in the page, moved to where the stage says it belongs, so it keeps the type scale
// and can be focused. Nothing moves here unless an event just happened. It mounts with the
// page, so the stage's code, the largest download on it, comes in beside the first figures.
export const FlowScene = forwardRef<FlowSceneHandle, FlowSceneProps>(
  function FlowScene(
    {
      overview,
      totals,
      stream,
      kind,
      selection,
      onSelect,
      pointed,
      staleText,
      onUnavailable,
      label,
    },
    ref
  ) {
    const box = useRef<HTMLDivElement>(null)
    const stage = useRef<FlowStage | null>(null)
    const onScreen = useRef(true)
    const anchors = useRef(new Map<string, HTMLElement>())
    const timers = useRef(new Set<ReturnType<typeof setTimeout>>())
    const [ready, setReady] = useState(false)
    // The names wait for the first frame, which puts them where they belong.
    const [placed, setPlaced] = useState(false)
    const [hovered, setHovered] = useState<string | null>(null)
    const [bursts, setBursts] = useState<Partial<Record<LineKey, number>>>({})
    const [viewMoved, setViewMoved] = useState(false)

    const latestSelect = useRef(onSelect)
    const latestSelection = useRef(selection)
    useEffect(() => {
      latestSelect.current = onSelect
      latestSelection.current = selection
    })

    const views = useMemo(
      () => overview && blockViews(overview, label, totals, stream),
      [overview, label, totals, stream]
    )

    useEffect(() => {
      const target = box.current
      if (!target) return
      let stopped = false
      const visibility = new IntersectionObserver(([entry]) => {
        onScreen.current = entry.isIntersecting
      })
      visibility.observe(target)
      const live = timers.current
      void import('./flow-stage')
        .then(({ FlowStage: Stage, webglAvailable }) => {
          if (stopped) return
          if (!webglAvailable()) {
            onUnavailable()
            return
          }
          stage.current = new Stage(target, {
            onHover: setHovered,
            onViewMoved: setViewMoved,
            onPick: (key) => {
              if (!key) {
                latestSelect.current(null)
                return
              }
              const next: FlowSelection = key.includes('>')
                ? { type: 'line', key: key as LineKey }
                : { type: 'block', id: key }
              latestSelect.current(
                sameSelection(latestSelection.current, next) ? null : next
              )
            },
            onFrame: (project) => {
              anchors.current.forEach((element, anchor) => {
                const at = project(anchor)
                if (at) {
                  element.style.transform = `translate(${at.x.toFixed(1)}px, ${at.y.toFixed(1)}px) translate(-50%, -100%)`
                }
              })
              setPlaced(true)
            },
          })
          setReady(true)
        })
        .catch(() => {
          if (!stopped) onUnavailable()
        })
      return () => {
        stopped = true
        visibility.disconnect()
        live.forEach(clearTimeout)
        live.clear()
        stage.current?.dispose()
        stage.current = null
      }
    }, [onUnavailable])

    useEffect(() => {
      if (!ready || !views) return
      stage.current?.setData({
        views,
        volumes: Object.fromEntries(
          lineKeys.map((key) => [key, routeVolume(totals, key)])
        ),
      })
    }, [ready, views, totals])

    const later = (run: () => void, ms: number) => {
      const timer = setTimeout(() => {
        timers.current.delete(timer)
        run()
      }, ms)
      timers.current.add(timer)
    }

    useImperativeHandle(
      ref,
      () => ({
        play(events) {
          const drawing = stage.current
          if (!drawing || !onScreen.current || document.hidden) return

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
            const color = eventColor(first)
            const start = () => {
              const fromIds =
                first.symbols ?? (first.from.id ? [first.from.id] : [])
              const toIds = first.symbols ?? (first.to.id ? [first.to.id] : [])
              const drawn = drawing.fire(
                line.key,
                line.reverse,
                color,
                // A call is one event with two legs: the prompt and the reply.
                () => {
                  if (first.type === 'agent_call') {
                    drawing.fire(line.key, !line.reverse, color)
                  }
                  if (first.to.block !== 'other') {
                    drawing.light(first.to.block, toIds, color)
                  }
                }
              )
              if (drawn && first.from.block !== 'other') {
                drawing.light(first.from.block, fromIds, color)
              }
              if (group.length > 1 || !drawn) {
                setBursts((current) => ({
                  ...current,
                  [line.key]: group.length,
                }))
                later(
                  () =>
                    setBursts((current) => {
                      const next = { ...current }
                      delete next[line.key]
                      return next
                    }),
                  BURST_SHOWN_MS
                )
              }
            }
            if (delay) later(start, delay)
            else start()
            delay += STAGGER_MS
          })
        },
        pulse(flow) {
          const drawing = stage.current
          if (!drawing || !onScreen.current || document.hidden) return
          if (flow.messages > 0) {
            drawing.fire('exchange>market', false, 'var(--muted)')
          }
        },
      }),
      []
    )

    useEffect(() => {
      stage.current?.setFocus(
        (key) => {
          if (pointed) return pointed === key
          if (kind !== 'all' && LINE_KIND[key] !== kind) return false
          if (!selection) return true
          if (selection.type === 'line') return selection.key === key
          return key.split('>').includes(selection.id)
        },
        pointed ??
          (hovered?.includes('>') ? hovered : null) ??
          (selection?.type === 'line' ? selection.key : null)
      )
    }, [ready, pointed, hovered, kind, selection])

    const toggle = (next: FlowSelection) =>
      onSelect(sameSelection(selection, next) ? null : next)

    const describe = (focus: string) => {
      if (focus.includes('>')) {
        return lineSummary(focus as LineKey, totals, label)
      }
      const view = views?.[focus as SceneBlock]
      const name = label(`flow.block.${focus}`)
      if (!view) return name
      const names = view.members
        .concat(view.shortMembers ?? [])
        .map((member) => member.title.split(' · ')[0])
        .filter(Boolean)
      if (!names.length) return `${name} · ${view.sub}`
      const more = names.length - NAMES_SHOWN
      return `${name} · ${names.slice(0, NAMES_SHOWN).join(', ')}${more > 0 ? ` +${more}` : ''}`
    }

    const anchor = (name: string) => (element: HTMLElement | null) => {
      if (element) anchors.current.set(name, element)
      else anchors.current.delete(name)
    }

    return (
      <div
        ref={box}
        role="group"
        aria-label={label('flow.sceneLabel')}
        title={hovered ? describe(hovered) : undefined}
        className="relative h-[clamp(480px,calc(56.25vw-28px),calc(100dvh-190px))] cursor-grab select-none overflow-hidden xl:h-full"
      >
        {!placed && <PanelLoading className="absolute inset-0" />}
        {STAGES.map((row) => (
          <span
            key={row.key}
            ref={anchor(`stage:${row.key}`)}
            className={cn(
              'pointer-events-none absolute left-0 top-0 whitespace-nowrap font-mono text-[11px] text-vergex-muted',
              !placed && 'invisible'
            )}
            style={onStage}
          >
            {label(`flow.stage.${row.key}`)}
          </span>
        ))}
        {blockIds.map((id) => {
          const view = views?.[id]
          const name = label(`flow.block.${id}`)
          return (
            <button
              key={id}
              type="button"
              ref={anchor(id)}
              aria-pressed={sameSelection(selection, { type: 'block', id })}
              aria-label={view ? `${name}: ${view.sub}` : name}
              style={onStage}
              className={cn(
                'absolute left-0 top-0 rounded-md px-1.5 py-0.5 text-center font-mono text-[11px] tabular-nums transition-colors hover:bg-vergex-ink-tint hover:duration-0 aria-pressed:bg-vergex-ink-tint',
                !placed && 'invisible'
              )}
              onMouseEnter={() => setHovered(id)}
              onMouseLeave={() => setHovered(null)}
              onFocus={() => setHovered(id)}
              onBlur={() => setHovered(null)}
              onClick={() => toggle({ type: 'block', id })}
            >
              <span className="block whitespace-nowrap font-sans text-xs font-medium text-vergex-ink">
                {name}
              </span>
              <span
                className={cn(
                  'block whitespace-nowrap',
                  view?.warn ? 'text-vergex-warning' : 'text-vergex-muted'
                )}
              >
                {view ? view.sub : <Skeleton className="w-12" />}
              </span>
            </button>
          )
        })}
        {lineKeys.map((key) => (
          // The curtains are drawn, so these are what a keyboard reaches them by.
          <button
            key={key}
            type="button"
            aria-pressed={sameSelection(selection, { type: 'line', key })}
            aria-label={lineSummary(key, totals, label)}
            className="sr-only"
            onFocus={() => setHovered(key)}
            onBlur={() => setHovered(null)}
            onClick={() => toggle({ type: 'line', key })}
          />
        ))}
        {(Object.keys(bursts) as LineKey[]).map((key) => (
          <span
            key={key}
            ref={anchor(key)}
            style={onStage}
            className="pointer-events-none absolute left-0 top-0 font-mono text-[11px] tabular-nums text-vergex-ink"
          >
            ×{bursts[key]}
          </span>
        ))}
        {viewMoved && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={label('flow.resetView')}
            title={label('flow.resetView')}
            className="absolute right-2 top-2 size-8"
            onClick={() => stage.current?.resetView()}
          >
            <RotateCcw className="size-4" aria-hidden="true" />
          </Button>
        )}
        {staleText && (
          <span
            role="status"
            style={onStage}
            className="pointer-events-none absolute left-4 top-3 whitespace-nowrap text-xs tabular-nums text-vergex-warning"
          >
            {staleText}
          </span>
        )}
      </div>
    )
  }
)

function sameSelection(a: FlowSelection | null, b: FlowSelection) {
  if (!a || a.type !== b.type) return false
  return a.type === 'line'
    ? b.type === 'line' && a.key === b.key
    : b.type === 'block' && a.id === b.id
}
