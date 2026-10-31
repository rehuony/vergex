import type { Overview } from '@/api/overview'
import { fmtUsd } from '@/components/trader/trader-format'
import { Panel } from '@/components/common/panel'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'

// OverviewMetrics is the headline row: how much is in the accounts and how much
// of the system is running. An account that cannot be read leaves a dash or a
// lower bound, never a made-up total.
export function OverviewMetrics({
  overview,
  language,
}: {
  overview: Overview
  language: Language
}) {
  const label = (key: string) => t(`dashboard.${key}`, language)
  const readable = overview.accounts.filter((a) => a.total_equity != null)
  const equity = readable.length
    ? readable.reduce((sum, a) => sum + (a.total_equity ?? 0), 0)
    : undefined
  const running = overview.traders.filter((x) => x.is_running).length
  const services = overview.services.filter((s) => s.state !== 'off')
  const healthy = services.filter((s) => s.state === 'running').length
  const positions = overview.positions.length
  const realized = overview.realized_today

  const metrics: { label: string; value: string; tone?: number | null }[] = [
    {
      label: label('totalEquity'),
      value: fmtUsd(equity),
    },
    {
      label: label('tradersRunning'),
      value: `${running} / ${overview.traders.length}`,
    },
    {
      label: label('servicesHealthy'),
      value: services.length ? `${healthy} / ${services.length}` : '—',
    },
    {
      label: label('positions'),
      value: overview.positions_partial
        ? positions
          ? `${positions}+`
          : '—'
        : String(positions),
    },
    {
      label: label('openOrders'),
      value: overview.open_orders == null ? '—' : String(overview.open_orders),
    },
    {
      label: label('realizedToday'),
      value: realized == null ? '—' : fmtUsd(realized, true),
      tone: realized,
    },
  ]

  return (
    <Panel className="overflow-hidden">
      <dl className="grid grid-cols-2 gap-px bg-[var(--line)] sm:grid-cols-3 lg:grid-cols-6">
        {metrics.map((metric, index) => (
          <div
            key={metric.label}
            className={cn(
              'bg-[var(--surface)] px-4 py-3',
              // Two columns on a phone: the first and last take a full row.
              (index === 0 || index === metrics.length - 1) &&
                'max-sm:col-span-2'
            )}
          >
            <dt
              className="truncate text-xs text-vergex-text-muted"
              title={metric.label}
            >
              {metric.label}
            </dt>
            <dd
              className={cn(
                'mt-1 truncate text-xl font-semibold tabular-nums text-vergex-text',
                metric.tone != null &&
                  metric.tone !== 0 &&
                  (metric.tone > 0
                    ? 'text-vergex-success'
                    : 'text-vergex-danger')
              )}
              title={metric.value}
            >
              {metric.value}
            </dd>
          </div>
        ))}
      </dl>
    </Panel>
  )
}
