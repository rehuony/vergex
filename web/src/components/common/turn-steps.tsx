import { useEffect, useState } from 'react'
import { Check, ChevronRight, Loader2, TriangleAlert } from 'lucide-react'
import { useLanguage } from '@/contexts/language-context'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'

export interface ActivityItem {
  id: string
  label: string
  detail: string
  calculation?: string
  state: 'running' | 'done' | 'failed'
  started: number
  ended?: number
}

const LIVE_STEPS = 4
// What a turn came to, kept with its steps but not one of them.
const USAGE_ID = 'usage'

// What a turn's model calls came to, read from what the server recorded of them.
function usageText(d: Record<string, unknown>, language: Language) {
  const tokens = (value: unknown) => Number(value).toLocaleString('en-US')
  const cost = Number(d.cost_usd)
  const money = d.unpriced
    ? cost > 0
      ? `≥ $${cost.toFixed(4)}`
      : t('turnSteps.unpriced', language)
    : `$${cost.toFixed(4)}`
  return t('turnSteps.usageDetail', language, {
    prompt: tokens(d.prompt_tokens),
    completion: tokens(d.completion_tokens),
    money,
  })
}

// The steps with what the turn came to among them, in place of any it said before.
export function withUsage(
  items: ActivityItem[],
  usage: Record<string, unknown>,
  now: number,
  language: Language
): ActivityItem[] {
  return [
    ...items.filter((item) => item.id !== USAGE_ID),
    {
      id: USAGE_ID,
      label: t('turnSteps.usage', language),
      detail: usageText(usage, language),
      state: 'done' as const,
      started: now,
      ended: now,
    },
  ]
}

const stepIcon = {
  running: (
    <Loader2 className="size-3.5 animate-spin motion-reduce:animate-none" />
  ),
  done: <Check className="size-3.5" />,
  failed: <TriangleAlert className="size-3.5 text-vergex-danger" />,
}

// TurnSteps is how the model got to its answer, between the question and the answer.
// While it works, one line says it is thinking, with its latest steps under it. Once it is
// done they fold into one line that opens again, and only a step that failed stays out.
export function TurnSteps({
  items,
  busy,
}: {
  items: ActivityItem[]
  busy: boolean
}) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`turnSteps.${key}`, language, params)
  const [now, setNow] = useState(Date.now)
  const [expanded, setExpanded] = useState(false)
  useEffect(() => {
    if (!busy) return
    setNow(Date.now())
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [busy])
  const usage = items.find((row) => row.id === USAGE_ID)
  const steps = items.filter((row) => row.id !== USAGE_ID)
  if (!busy && !steps.length) return null
  const rows = expanded
    ? steps
    : busy
      ? steps.filter((row) => row.id !== 'model_transport').slice(-LIVE_STEPS)
      : steps.filter((row) => row.state === 'failed')
  const ended = busy
    ? now
    : Math.max(...steps.map((row) => row.ended || row.started))
  const seconds = steps.length
    ? Math.max(0, Math.round((ended - steps[0].started) / 1000))
    : 0
  return (
    <section className="text-xs" aria-label={label('name')}>
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1.5 rounded-md text-vergex-muted transition-colors hover:text-vergex-ink hover:duration-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-vergex-ink/25 max-sm:min-h-10"
      >
        {busy && (
          <Loader2
            className="size-3.5 animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        )}
        <span>
          {busy
            ? label(steps.length ? 'thinkingFor' : 'thinking', { seconds })
            : label('thought', { seconds, count: steps.length })}
          {usage && !busy && ` · ${usage.detail}`}
        </span>
        <ChevronRight
          className={cn(
            'size-3.5 transition-transform',
            expanded && 'rotate-90'
          )}
          aria-hidden="true"
        />
      </button>
      {rows.length > 0 && (
        <ol
          className="ml-[7px] mt-1.5 space-y-1 border-l border-vergex-line pl-2.5"
          role="log"
          aria-live="polite"
          aria-relevant="additions text"
        >
          {rows.map((row) => (
            <li key={row.id} className="flex items-start gap-1.5">
              <span
                className="flex h-[18px] shrink-0 items-center text-vergex-muted"
                aria-label={row.state}
              >
                {stepIcon[row.state]}
              </span>
              <p
                className={cn(
                  'min-w-0 flex-1 leading-[18px]',
                  expanded || row.state === 'failed'
                    ? 'break-words'
                    : 'truncate'
                )}
                title={row.calculation ?? row.detail}
              >
                <span className="text-vergex-ink-soft">{row.label}</span>
                {row.detail && (
                  <span className="ml-1.5 text-vergex-muted">{row.detail}</span>
                )}
              </p>
              <time className="mono shrink-0 text-[11px] leading-[18px] text-vergex-muted">
                {Math.max(
                  0,
                  Math.floor(((row.ended || now) - row.started) / 1000)
                )}
                s
              </time>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
