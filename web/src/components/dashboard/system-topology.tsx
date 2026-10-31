import type { CSSProperties, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Compass } from 'lucide-react'
import type { Overview } from '@/api/overview'
import { getExchangeIcon } from '@/components/common/exchange-icons'
import { getModelIcon } from '@/components/common/model-icons'
import { Panel, PanelState } from '@/components/common/panel'
import { SettingsLink } from '@/components/settings/settings-link'
import { fmtUsd } from '@/components/trader/trader-format'
import { ROUTES } from '@/constants/route-paths'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import { traderPath } from './overview-links'

// Fixed node size, so the connectors can be drawn without measuring the page.
const NODE_HEIGHT = 56
const NODE_GAP = 8
const CONNECTOR_WIDTH = 56
const STEP = NODE_HEIGHT + NODE_GAP

type Health = 'ok' | 'off' | 'warn' | 'fail'

const healthClass: Record<Health, string> = {
  ok: 'bg-vergex-success',
  off: 'bg-[var(--line-strong)]',
  warn: 'bg-[var(--warning)]',
  fail: 'bg-vergex-danger',
}

function Dot({ health, label }: { health: Health; label: string }) {
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={cn('size-2 shrink-0 rounded-full', healthClass[health])}
    />
  )
}

interface Node {
  id: string
  name: string
  /** What the row says beside the connectors. */
  detail: string
  /** Replaces the detail on phones, where no connectors are drawn. */
  phoneDetail?: string
  health: Health
  healthLabel: string
  icon?: ReactNode
  to?: string
  settings?: 'models' | 'exchanges'
}

function NodeBody({ node }: { node: Node }) {
  return (
    <>
      <Dot health={node.health} label={node.healthLabel} />
      {node.icon && (
        <span className="grid size-5 shrink-0 place-items-center">
          {node.icon}
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span
          className="block truncate text-[13px] font-medium text-vergex-text"
          title={node.name}
        >
          {node.name}
        </span>
        <span
          className={cn(
            'block truncate text-xs text-vergex-text-muted',
            node.phoneDetail && 'max-md:hidden'
          )}
          title={node.detail}
        >
          {node.detail}
        </span>
        {node.phoneDetail && (
          <span className="block truncate text-xs text-vergex-text-muted md:hidden">
            {node.phoneDetail}
          </span>
        )}
      </span>
    </>
  )
}

const nodeClass =
  'flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 hover:border-[var(--line-strong)]'

function NodeLink({ node }: { node: Node }) {
  const style = { height: NODE_HEIGHT }
  if (node.settings) {
    return (
      <div style={style}>
        <SettingsLink
          section={node.settings}
          className={cn(nodeClass, 'h-full')}
        >
          <NodeBody node={node} />
        </SettingsLink>
      </div>
    )
  }
  return (
    <Link to={node.to ?? '#'} className={nodeClass} style={style}>
      <NodeBody node={node} />
    </Link>
  )
}

function Column({
  title,
  nodes,
  height,
}: {
  title: string
  nodes: Node[]
  height: number
}) {
  return (
    <div className="min-w-0">
      <h3 className="mb-2 h-4 text-xs leading-4 text-vergex-text-muted">
        {title}
      </h3>
      {/* A shorter column sits centred against the tallest. */}
      <ul
        className="flex flex-col md:pt-[var(--column-offset)]"
        style={
          {
            gap: NODE_GAP,
            '--column-offset': `${offset(nodes.length, height)}px`,
          } as CSSProperties
        }
      >
        {nodes.map((node) => (
          <li key={node.id}>
            <NodeLink node={node} />
          </li>
        ))}
      </ul>
    </div>
  )
}

function offset(count: number, height: number) {
  return (height - columnHeight(count)) / 2
}

function columnHeight(count: number) {
  return count === 0 ? 0 : count * STEP - NODE_GAP
}

interface Edge {
  from: string
  to: string
  active: boolean
}

function Connectors({
  left,
  right,
  edges,
  height,
}: {
  left: Node[]
  right: Node[]
  edges: Edge[]
  height: number
}) {
  const y = (nodes: Node[], id: string) => {
    const index = nodes.findIndex((node) => node.id === id)
    if (index < 0) return null
    return offset(nodes.length, height) + index * STEP + NODE_HEIGHT / 2
  }
  return (
    <svg
      aria-hidden="true"
      className="mt-6 max-md:hidden"
      width={CONNECTOR_WIDTH}
      height={height}
    >
      {edges.map((edge) => {
        const start = y(left, edge.from)
        const end = y(right, edge.to)
        if (start == null || end == null) return null
        const mid = CONNECTOR_WIDTH / 2
        return (
          <path
            key={`${edge.from}-${edge.to}`}
            d={`M0 ${start} C${mid} ${start} ${mid} ${end} ${CONNECTOR_WIDTH} ${end}`}
            fill="none"
            stroke={edge.active ? 'var(--brand)' : 'var(--line-strong)'}
            strokeWidth={edge.active ? 1.5 : 1}
          />
        )
      })}
    </svg>
  )
}

const ACCOUNT_HEALTH: Record<string, Health> = {
  ok: 'ok',
  disabled: 'off',
  missing_credentials: 'off',
  unavailable: 'warn',
  invalid_credentials: 'fail',
  permission_denied: 'fail',
}

const SERVICE_HEALTH: Record<string, Health> = {
  running: 'ok',
  off: 'off',
  degraded: 'warn',
  failed: 'fail',
}

// SystemTopology shows how the system is wired right now: which model each
// runner asks and which account it trades. Every node opens where it is managed.
export function SystemTopology({
  overview,
  language,
}: {
  overview: Overview
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`dashboard.${key}`, language, params)

  const modelName = (id: string) =>
    overview.models.find((model) => model.id === id)?.name
  const accountName = (id: string) =>
    overview.accounts.find((account) => account.id === id)?.name

  const models: Node[] = overview.models.map((model) => {
    const used = overview.traders.filter(
      (trader) => trader.ai_model_id === model.id
    )
    return {
      id: model.id,
      name: model.name,
      detail: label('traderCount', { count: used.length }),
      health: 'ok',
      healthLabel: label('enabled'),
      icon: getModelIcon(model.provider, { width: 18, height: 18 }),
      settings: 'models',
    }
  })

  const activeAccounts = [
    ...new Set(overview.runs.map((run) => run.exchange_id)),
  ]
  const runners: Node[] = [
    ...overview.traders.map(
      (trader): Node => ({
        id: trader.trader_id,
        name: trader.trader_name,
        detail: trader.strategy_name || '—',
        phoneDetail: [
          modelName(trader.ai_model_id),
          accountName(trader.exchange_id),
        ]
          .filter(Boolean)
          .join(' → '),
        health: trader.is_running ? 'ok' : 'off',
        healthLabel: trader.is_running ? label('running') : label('stopped'),
        to: traderPath(overview.traders, trader.trader_id),
      })
    ),
    {
      id: 'frontier',
      name: 'Frontier',
      detail: overview.runs.length
        ? label('activePlans', { count: overview.runs.length })
        : label('noActivePlans'),
      health: overview.runs.some((run) => run.state === 'needs_attention')
        ? 'fail'
        : overview.runs.length
          ? 'ok'
          : 'off',
      healthLabel: overview.runs.length ? label('running') : label('idle'),
      icon: <Compass size={16} className="text-vergex-text-muted" />,
      to: ROUTES.planning,
    },
  ]

  const accounts: Node[] = overview.accounts.map((account) => ({
    id: account.id,
    name: account.name,
    detail:
      account.total_equity != null
        ? fmtUsd(account.total_equity)
        : label(`account.${account.status}`),
    health: ACCOUNT_HEALTH[account.status] ?? 'warn',
    healthLabel: label(`account.${account.status}`),
    icon: getExchangeIcon(account.exchange_type, { width: 18, height: 18 }),
    settings: 'exchanges',
  }))

  const modelEdges: Edge[] = overview.traders.map((trader) => ({
    from: trader.ai_model_id,
    to: trader.trader_id,
    active: trader.is_running,
  }))
  const accountEdges: Edge[] = [
    ...overview.traders.map((trader) => ({
      from: trader.trader_id,
      to: trader.exchange_id,
      active: trader.is_running,
    })),
    ...activeAccounts.map((account) => ({
      from: 'frontier',
      to: account,
      active: true,
    })),
  ]

  const height = Math.max(
    columnHeight(models.length),
    columnHeight(runners.length),
    columnHeight(accounts.length)
  )
  const empty = !models.length && !accounts.length && !overview.traders.length

  return (
    <Panel title={label('orchestration')}>
      {empty ? (
        <PanelState role="status">
          <p className="font-medium text-vergex-text">
            {label('nothingConnected')}
          </p>
          <span className="flex gap-4">
            <SettingsLink section="models" className="text-vergex-brand">
              {label('connectModel')}
            </SettingsLink>
            <SettingsLink section="exchanges" className="text-vergex-brand">
              {label('connectAccount')}
            </SettingsLink>
          </span>
        </PanelState>
      ) : (
        <div className="px-4 pb-4">
          <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_56px_minmax(0,1fr)_56px_minmax(0,1fr)] md:gap-0">
            <Column title={label('aiModels')} nodes={models} height={height} />
            <Connectors
              left={models}
              right={runners}
              edges={modelEdges}
              height={height}
            />
            <Column title={label('runners')} nodes={runners} height={height} />
            <Connectors
              left={runners}
              right={accounts}
              edges={accountEdges}
              height={height}
            />
            <Column
              title={label('accounts')}
              nodes={accounts}
              height={height}
            />
          </div>
          <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-[var(--line)] pt-3 text-xs">
            <span className="text-vergex-text-muted">{label('services')}</span>
            {overview.services.map((service) => {
              const state = label(`serviceState.${service.state}`)
              return (
                <span
                  key={service.name}
                  className="flex items-center gap-1.5 whitespace-nowrap text-vergex-text"
                  title={service.detail || state}
                >
                  <Dot
                    health={SERVICE_HEALTH[service.state] ?? 'warn'}
                    label={state}
                  />
                  {label(`service.${service.name}`)}
                </span>
              )
            })}
          </div>
        </div>
      )}
    </Panel>
  )
}
