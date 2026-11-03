import type { FuturesEvent } from '@/api/futures'
import { withUsage, type ActivityItem } from '@/components/common/turn-steps'
import { t, type Language } from '@/i18n'
import { validationSummary } from '@/utils/futures/futures-presentation'
import { eventErrorText } from '@/utils/request-errors'

export type { ActivityItem }

const METRICS = new Set(['funding_rate', 'mark_price', 'open_interest'])

const TOOLS = new Set([
  'get_candles',
  'get_market',
  'propose_plans',
  'draw_chart',
  'get_data_plugin',
  'get_entry_plugins',
  'develop_entry_plugin',
  'run_entry_plugin',
])
const STRUCTURES = new Set(['market', 'limit', 'ladder', 'twap'])

export function structureName(kind: string, language: Language, fallback = '') {
  return STRUCTURES.has(kind)
    ? t(`futures.structure.${kind}`, language)
    : fallback
}
export function recordActivity(
  items: ActivityItem[],
  event: FuturesEvent,
  now = Date.now(),
  language: Language = 'en'
): ActivityItem[] {
  const d = event.data
  const text = (key: string, params?: Record<string, string | number>) =>
    t(`futures.activity.${key}`, language, params)
  const toolTitle = (name: string) =>
    text(TOOLS.has(name) ? `tool.${name}` : 'runTool')
  if (event.type === 'planning_sizing')
    return [
      ...items.filter((item) => item.id !== 'planning_sizing'),
      {
        id: 'planning_sizing',
        label: text('sizing'),
        detail: text('planningOnly'),
        state: 'done' as const,
        started: now,
        ended: now,
      },
    ]
  if (event.type === 'usage') return withUsage(items, d, now, language)
  if (event.type === 'model_transport')
    return [
      ...items,
      {
        id: 'model_transport',
        label: text('modelInterface'),
        detail: text(
          d.transport === 'text_tools' ? 'textTools' : 'nativeTools'
        ),
        state: 'done' as const,
        started: now,
        ended: now,
      },
    ]
  if (event.type === 'plugin_data') {
    const plugin = String(d.plugin)
    const metrics = Array.isArray(d.metrics)
      ? (d.metrics as {
          key?: string
          label: string
          value: string
          unit: string
        }[])
      : []
    const id = `plugin_${String(d.plugin)}`
    const previous = items.find((row) => row.id === id)
    const row: ActivityItem = {
      id,
      label:
        plugin === 'funding' || plugin === 'open_interest'
          ? text(`plugin.${plugin}`)
          : text('plugin.other'),
      state: d.error ? 'failed' : 'done',
      started: previous?.started || now,
      ended: now,
      detail: d.error
        ? String(d.error)
        : metrics
            .slice(0, 3)
            .map((metric) => {
              const value = Number(metric.value)
              const display = Number.isFinite(value)
                ? metric.unit === 'fraction'
                  ? `${(value * 100).toFixed(5)}%`
                  : `${value.toLocaleString('en-US', { maximumFractionDigits: 6 })} ${metric.unit === 'contracts' ? text('metric.contracts') : metric.unit}`
                : metric.value
              // The built-in plugins' figures are named here; another's as it names them.
              const name = METRICS.has(metric.key ?? '')
                ? text(`metric.${metric.key}`)
                : metric.label
              return `${name}: ${display}`
            })
            .join(' · '),
    }
    return previous
      ? items.map((item) => (item.id === id ? row : item))
      : [...items, row].slice(-100)
  }
  const endModels = (rows: ActivityItem[]) =>
    rows.map((row) =>
      row.id.startsWith('model_') && row.state === 'running'
        ? {
            ...row,
            state: 'done' as const,
            ended: now,
            detail: text('modelResponded'),
          }
        : row
    )
  const upsert = (
    rows: ActivityItem[],
    item: Partial<ActivityItem> & {
      id: string
      label: string
      state: ActivityItem['state']
    }
  ) => {
    const index = rows.findIndex((row) => row.id === item.id)
    if (index < 0)
      return [...rows, { started: now, detail: '', ...item }].slice(-100)
    return rows.map((row, i) => (i === index ? { ...row, ...item } : row))
  }
  if (event.type === 'iteration')
    return upsert(endModels(items), {
      id: `model_${d.round}`,
      label: text('modelRound', { round: String(d.round) }),
      detail: text('evaluating'),
      state: 'running',
    })
  if (event.type === 'status') {
    if (typeof d.elapsed_seconds !== 'number') return items
    return items.map((row) =>
      row.id.startsWith('model_') && row.state === 'running'
        ? {
            ...row,
            detail: text('waitingModel', {
              seconds: Number(d.elapsed_seconds),
            }),
          }
        : row
    )
  }
  if (event.type === 'tool_start')
    return upsert(endModels(items), {
      id: String(d.id),
      label: toolTitle(String(d.name)),
      detail: text('inProgress'),
      state: 'running',
    })
  if (event.type === 'tool_progress')
    return upsert(items, {
      id: String(d.id),
      label: toolTitle(String(d.name)),
      detail: text('candleProgress', {
        interval: String(d.interval),
        count: String(d.count),
        target: String(d.target_count),
        scope: text('latestHistory'),
      }),
      state: 'running',
    })
  if (event.type === 'tool_result') {
    const detail = [
      d.interval,
      typeof d.count === 'number'
        ? text('candles', {
            count: `${d.count}${typeof d.target_count === 'number' ? ` / ${d.target_count}` : ''}`,
          })
        : undefined,
      d.truncated ? text('limitedHistory') : undefined,
      d.scope ? text('latestHistory') : undefined,
    ]
      .filter(Boolean)
      .join(' · ')
    return upsert(items, {
      id: String(d.id),
      label: toolTitle(String(d.name)),
      state: d.ok ? 'done' : 'failed',
      ended: now,
      detail: d.reason
        ? validationSummary(String(d.reason), language)
        : detail || text(d.ok ? 'completed' : 'stepFailed'),
      calculation: d.reason ? String(d.reason) : undefined,
    })
  }
  if (event.type === 'candidate_start' || event.type === 'candidate_result')
    return upsert(endModels(items), {
      id: String(d.id),
      label:
        d.status === 'deferred'
          ? text('candidateWait', {
              kind: structureName(String(d.kind), language, text('entry')),
            })
          : text('candidate', {
              kind: structureName(
                String(d.kind),
                language,
                text('candidateKind')
              ),
              name: String(d.name),
            }),
      state:
        event.type === 'candidate_start'
          ? 'running'
          : d.status === 'rejected'
            ? 'failed'
            : 'done',
      ...(event.type === 'candidate_result' ? { ended: now } : {}),
      calculation: d.reason ? String(d.reason) : undefined,
      detail: d.reason
        ? validationSummary(String(d.reason), language)
        : event.type === 'candidate_start'
          ? text('checkingCosts')
          : d.status === 'reused'
            ? text('duplicateSkipped')
            : text('passedValidation'),
    })
  if (event.type === 'plans') {
    const count = Array.isArray(d.plans) ? d.plans.length : 0
    const deferred = Array.isArray(d.checks)
      ? d.checks.filter((check) => check.status === 'deferred').length
      : 0
    return upsert(items, {
      id: `save_${items.filter((row) => row.id.startsWith('save_')).length}`,
      label: text(
        d.waiting ? 'waitingEntry' : count ? 'saveDrafts' : 'noDrafts'
      ),
      state: 'done',
      ended: now,
      detail: d.waiting
        ? text('waitingSummary', { deferred })
        : [
            text('newDrafts', { count }),
            deferred ? text('typesWaiting', { deferred }) : '',
            text('rejectedCount', {
              rejected: Array.isArray(d.rejected) ? d.rejected.length : 0,
            }),
            text('noOrders'),
          ]
            .filter(Boolean)
            .join(' · '),
    })
  }
  if (
    event.type === 'outcome' ||
    event.type === 'error' ||
    event.type === 'cancelled'
  ) {
    const ended = (event.type === 'outcome' ? endModels(items) : items).map(
      (row) =>
        row.state === 'running'
          ? {
              ...row,
              state: 'failed' as const,
              ended: now,
              detail: text(
                event.type === 'cancelled' ? 'stoppedByYou' : 'stoppedEarly'
              ),
            }
          : row
    )
    return upsert(ended, {
      id: 'outcome',
      label: text(
        event.type === 'error'
          ? 'interrupted'
          : event.type === 'cancelled'
            ? 'stopped'
            : d.waiting && d.stop_reason === 'completed'
              ? 'waitingAnEntry'
              : 'complete'
      ),
      state: event.type === 'error' ? 'failed' : 'done',
      ended: now,
      detail:
        event.type === 'error'
          ? eventErrorText(d, language)
          : event.type === 'cancelled'
            ? text('cancelledDetail')
            : Number(d.drafts) > 0
              ? text('draftsSaved', { count: Number(d.drafts) })
              : d.waiting
                ? text('noOrderToApprove')
                : text('noneSaved'),
    })
  }
  return items
}
