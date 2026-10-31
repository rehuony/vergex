import useSWR from 'swr'
import { api } from '@/api'
import type { Overview } from '@/api/overview'
import { AppChrome } from '@/components/common/app-chrome'
import { Page } from '@/components/common/page'
import { Panel, PanelState } from '@/components/common/panel'
import { AttentionList } from '@/components/dashboard/attention-list'
import { OverviewMetrics } from '@/components/dashboard/overview-metrics'
import { PositionsOverview } from '@/components/dashboard/positions-overview'
import { RecentOrders } from '@/components/dashboard/recent-orders'
import { SystemTopology } from '@/components/dashboard/system-topology'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import { cn } from '@/utils/cn'

function clock(iso: string) {
  return new Date(iso).toLocaleTimeString('en-GB', { hour12: false })
}

// The home page: a read-only view of the whole system. Everything that changes
// something lives on the page a row leads to.
export function DashboardPage() {
  const { user, token } = useAuth()
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`dashboard.${key}`, language, params)
  const { data: overview, error } = useSWR<Overview>(
    user && token ? 'overview' : null,
    () => api.getOverview(true),
    { refreshInterval: 20000, shouldRetryOnError: false }
  )

  return (
    <AppChrome currentPage="dashboard" animateContent>
      <Page
        title={t('dashboardNav', language)}
        description={label('description')}
        actions={
          overview && (
            <span
              role={error ? 'status' : undefined}
              className={cn(
                'whitespace-nowrap text-xs tabular-nums',
                error ? 'text-[var(--warning)]' : 'text-vergex-text-muted'
              )}
            >
              {label(error ? 'stale' : 'updated', {
                time: clock(overview.generated_at),
              })}
            </span>
          )
        }
      >
        {!overview ? (
          <Panel>
            {error ? (
              <PanelState role="alert">{label('loadFailed')}</PanelState>
            ) : (
              <PanelState role="status">{t('loading', language)}</PanelState>
            )}
          </Panel>
        ) : (
          <div className="flex flex-col gap-4">
            <AttentionList overview={overview} language={language} />
            <OverviewMetrics overview={overview} language={language} />
            <SystemTopology overview={overview} language={language} />
            <div className="grid gap-4 lg:grid-cols-2">
              <PositionsOverview overview={overview} language={language} />
              <RecentOrders overview={overview} language={language} />
            </div>
          </div>
        )}
      </Page>
    </AppChrome>
  )
}
