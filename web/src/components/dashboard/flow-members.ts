import type { ActivityTotals } from '@/api/activity'
import type { Overview } from '@/api/overview'
import { fmtUsd } from '@/utils/trader-format'
import type { SceneBlock } from './flow-layout'

export type MemberTone = 'on' | 'off' | 'up' | 'down' | 'alert'

export interface Member {
  /** What events name this member by. */
  id: string
  tone: MemberTone
  title: string
}

export interface BlockView {
  sub: string
  /** Set when sub is a problem rather than a description. */
  warn?: boolean
  members: Member[]
  /** Long positions take the first rows of their block, short the rest. */
  shortMembers?: Member[]
}

type Label = (key: string, params?: Record<string, string | number>) => string

// "1 coin", "2 coins": each noun has a singular and a plural key.
export function counted(
  label: Label,
  noun: string,
  count: number,
  params?: Record<string, string | number>
) {
  return label(`flow.${noun}${count === 1 ? 'One' : 'Many'}`, {
    count,
    ...params,
  })
}

export function totalEquity(overview: Overview) {
  const known = overview.accounts.filter((row) => row.total_equity != null)
  if (known.length === 0) return undefined
  return known.reduce((sum, row) => sum + (row.total_equity ?? 0), 0)
}

// What each block of the scene is made of. Every dot is a real member; the phone's
// groups and the scene read the same list.
export function blockViews(
  overview: Overview,
  label: Label
): Record<SceneBlock, BlockView> {
  const running = overview.traders.filter((row) => row.is_running)
  const stopped = overview.traders.filter((row) => !row.is_running)

  const hosts = new Map<string, string[]>()
  overview.models.forEach((model) => {
    const host = model.host || model.provider
    hosts.set(host, [...(hosts.get(host) ?? []), model.name])
  })

  const position = (row: Overview['positions'][number]): Member => ({
    id: `${row.exchange_id}:${row.symbol}:${row.side}`,
    tone: row.unrealized_pnl >= 0 ? 'up' : 'down',
    title: `${row.symbol} ${row.side} ${fmtUsd(row.unrealized_pnl, true)}`,
  })
  const longs = overview.positions.filter((row) => row.side === 'long')
  const shorts = overview.positions.filter((row) => row.side === 'short')
  const partial = overview.positions_partial ? '+' : ''

  const coins = overview.market?.coins ?? 0
  const forecastCoins = overview.forecast?.coins ?? 0

  return {
    market: {
      sub: !overview.market
        ? label('flow.unavailable')
        : coins > 0 && !overview.market.live
          ? label('flow.streamDown')
          : counted(label, 'coin', coins),
      warn: coins > 0 && overview.market?.live === false,
      members: Array.from({ length: coins }, (_, index) => ({
        id: String(index),
        tone: 'on' as const,
        title: '',
      })),
    },
    telegram: {
      sub: label(overview.telegram_bound ? 'flow.bound' : 'flow.notBound'),
      members: overview.telegram_bound
        ? [{ id: '', tone: 'on', title: label('flow.bound') }]
        : [],
    },
    traders: {
      sub:
        overview.traders.length === 0
          ? label('flow.none')
          : label('flow.running', {
              running: running.length,
              total: overview.traders.length,
            }),
      members: [
        ...running.map((row) => ({
          id: row.trader_id,
          tone: 'on' as const,
          title: `${row.trader_name} · ${label('running')}`,
        })),
        ...stopped.map((row) => ({
          id: row.trader_id,
          tone: 'off' as const,
          title: `${row.trader_name} · ${label('stopped')}`,
        })),
      ],
    },
    frontier: {
      sub: overview.runs.length
        ? counted(label, 'plan', overview.runs.length)
        : label('flow.noPlans'),
      members: overview.runs.map((row) => ({
        id: row.id,
        tone: row.state === 'needs_attention' ? 'alert' : 'on',
        title: `${row.symbol} · ${row.state}`,
      })),
    },
    forecast: {
      sub: overview.forecast
        ? counted(label, 'coin', forecastCoins)
        : label('flow.unavailable'),
      members: Array.from({ length: forecastCoins }, (_, index) => ({
        id: String(index),
        tone: 'on' as const,
        title: overview.forecast?.name ?? '',
      })),
    },
    models: {
      sub: hosts.size ? counted(label, 'host', hosts.size) : label('flow.none'),
      members: [...hosts].map(([host, names]) => ({
        id: host,
        tone: 'on' as const,
        title: `${host} · ${names.join(', ')}`,
      })),
    },
    accounts: {
      sub: overview.accounts.length
        ? fmtUsd(totalEquity(overview))
        : label('flow.none'),
      members: overview.accounts.map((row) => ({
        id: row.id,
        tone: row.status === 'ok' ? 'on' : 'off',
        title: `${row.name} · ${label(`account.${row.status}`)}`,
      })),
    },
    positions: {
      sub: overview.positions.length
        ? label('flow.longShort', {
            long: longs.length,
            short: `${shorts.length}${partial}`,
          })
        : partial
          ? label('flow.unread')
          : label('flow.none'),
      warn: overview.positions_partial,
      members: longs.map(position),
      shortMembers: shorts.map(position),
    },
  }
}

export function routeTotal(
  totals: ActivityTotals,
  routes: string[],
  window: 'h1' | 'h24'
) {
  let count = 0
  let tokens = 0
  let items = 0
  let amount = 0
  routes.forEach((route) => {
    const total = totals[route]?.[window]
    count += total?.count ?? 0
    tokens += total?.tokens ?? 0
    items += total?.items ?? 0
    amount += total?.amount ?? 0
  })
  return { count, tokens, items, amount }
}
