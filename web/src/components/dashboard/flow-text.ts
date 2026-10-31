import type { ActivityEvent, ActivityTotals } from '@/api/activity'
import { baseLabel, fmtTime, fmtUsd } from '@/utils/trader-format'
import { lineOf, type LineKey } from './flow-layout'
import { counted, routeTotal } from './flow-members'

type Label = (key: string, params?: Record<string, string | number>) => string

function compact(value: number) {
  return value >= 10_000
    ? `${Math.round(value / 1000).toLocaleString('en-US')}k`
    : value.toLocaleString('en-US')
}

function sentence(text: string) {
  const spaced = text.replace(/_/g, ' ').toLowerCase()
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

/** Fees and AI costs can be fractions of a cent, which fmtUsd would round away. */
export function smallUsd(value: number) {
  if (value === 0 || value >= 1) return fmtUsd(value)
  return `$${value.toFixed(value >= 0.01 ? 2 : 4)}`
}

function endName(end: ActivityEvent['from'], label: Label) {
  if (end.block === 'positions' && end.name) {
    const [symbol, side] = end.name.split(' ')
    return `${baseLabel(symbol)} ${side}`
  }
  if (end.block === 'frontier') return label('flow.block.frontier')
  return end.name || label(`flow.block.${end.block}`)
}

export function eventRoute(event: ActivityEvent, label: Label) {
  return `${endName(event.from, label)} → ${endName(event.to, label)}`
}

export function eventTime(event: ActivityEvent) {
  const date = new Date(event.at)
  const today = new Date()
  return date.toDateString() === today.toDateString()
    ? date.toLocaleTimeString('en-GB', { hour12: false })
    : fmtTime(event.at)
}

// What happened, in a phrase. The route says between what.
export function eventWhat(event: ActivityEvent, label: Label) {
  const amount = event.amount ?? 0
  switch (event.type) {
    case 'ai_call':
      return [
        event.detail,
        label('flow.tokens', { count: compact(event.tokens ?? 0) }),
      ]
        .filter(Boolean)
        .join(' · ')
    case 'ai_cost':
      return label('flow.estimated', { amount: smallUsd(amount) })
    case 'decision': {
      if (event.status === 'failed' && !event.action) {
        return label('flow.noDecision')
      }
      if (!event.action) return label('flow.decidedHold')
      const action = `${sentence(event.action)} ${baseLabel(event.symbol)}`
      const more = (event.items ?? 1) - 1
      return more > 0 ? label('flow.andMore', { action, more }) : action
    }
    case 'market_read':
      return counted(label, 'coinRead', event.items ?? 0)
    case 'forecast':
      return counted(label, 'forecast', event.items ?? 0, {
        interval: event.detail ?? '',
      })
    case 'order':
    case 'plan_order':
      return `${sentence(event.action ?? '')} ${baseLabel(event.symbol)} ${(event.quantity ?? 0).toLocaleString('en-US', { maximumFractionDigits: 6 })}`
    case 'fee':
      return label('flow.fee', {
        amount:
          !event.asset || event.asset.startsWith('USD')
            ? smallUsd(amount)
            : `${amount} ${event.asset}`,
      })
    case 'margin':
      return label('flow.margin', { amount: fmtUsd(amount) })
    case 'realized':
      return label('flow.realized', { amount: fmtUsd(amount, true) })
  }
}

/** Empty when an event's outcome needs no saying. */
export function eventStatus(event: ActivityEvent, label: Label) {
  if (!event.status || event.status === 'ok') return ''
  if (event.status === 'failed') return label('flow.failed')
  return sentence(event.status)
}

export function isEventBad(event: ActivityEvent) {
  return ['failed', 'REJECTED', 'EXPIRED'].includes(event.status ?? '')
}

/** The token a line lights up in for this event, also its mark in the list. */
export function eventColor(event: ActivityEvent) {
  if (isEventBad(event) || event.type === 'fee' || event.type === 'ai_cost') {
    return 'var(--color-vergex-danger)'
  }
  if (event.type === 'realized') {
    return (event.amount ?? 0) >= 0
      ? 'var(--color-vergex-success)'
      : 'var(--color-vergex-danger)'
  }
  if (event.kind === 'ai') return 'var(--brand)'
  if (event.kind === 'market') return 'var(--muted)'
  return 'var(--ink)'
}

const AI_ROUTES = [
  'traders>models',
  'frontier>models',
  'telegram>models',
  'other>models',
]

// One line's figures for the last hour and the last day.
export function lineSummary(
  key: LineKey,
  totals: ActivityTotals,
  label: Label
) {
  const [from, to] = key.split('>')
  const name = `${label(`flow.block.${from}`)} → ${label(`flow.block.${to}`)}`
  const window = (id: 'h1' | 'h24') => {
    const total = routeTotal(totals, [key], id)
    if (key.endsWith('>models')) {
      return counted(label, 'call', total.count, {
        tokens: compact(total.tokens),
      })
    }
    if (key.endsWith('>accounts')) {
      return counted(label, 'order', total.count)
    }
    if (key === 'models>ai_cost') {
      return label('flow.estimated', { amount: smallUsd(total.amount) })
    }
    if (key === 'accounts>fees') return smallUsd(total.amount)
    if (key === 'accounts>positions') {
      const realized = routeTotal(totals, ['positions>accounts'], id)
      return label('flow.moneyMoved', {
        margin: fmtUsd(total.amount),
        realized: fmtUsd(realized.amount, true),
      })
    }
    return counted(label, 'read', total.count)
  }
  return `${name} · 1h ${window('h1')} · 24h ${window('h24')}`
}

// The scene's line when nothing is picked: the day across every route.
export function overallSummary(totals: ActivityTotals, label: Label) {
  const calls = routeTotal(totals, AI_ROUTES, 'h24').count
  const orders = routeTotal(
    totals,
    ['traders>accounts', 'frontier>accounts'],
    'h24'
  ).count
  return label('flow.overall', {
    calls: counted(label, 'aiCall', calls),
    orders: counted(label, 'order', orders),
    fees: smallUsd(routeTotal(totals, ['accounts>fees'], 'h24').amount),
    cost: smallUsd(routeTotal(totals, ['models>ai_cost'], 'h24').amount),
  })
}

export function eventMatches(
  event: ActivityEvent,
  kind: string,
  selection: FlowSelection | null
) {
  if (kind !== 'all' && event.kind !== kind) return false
  if (!selection) return true
  if (selection.type === 'line') {
    return lineOf(event)?.key === selection.key
  }
  return event.from.block === selection.id || event.to.block === selection.id
}

export type FlowSelection =
  | { type: 'line'; key: LineKey }
  | { type: 'block'; id: string }
