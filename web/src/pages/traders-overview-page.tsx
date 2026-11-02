import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { StatStrip } from '@/components/common/figures'
import { Panel, PanelState } from '@/components/common/panel'
import { SearchField } from '@/components/common/search-field'
import {
  traderMetricsQuery,
  tradersQuery,
} from '@/components/trader/trader-queries'
import { TradersFrame } from '@/components/trader/traders-frame'
import { TradersList } from '@/components/trader/traders-list'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import {
  configuredAccounts,
  configuredModels,
  useAccounts,
  useModelConfigs,
} from '@/hooks/use-connections'
import { t } from '@/i18n'
import type { TraderInfo, TraderMetrics } from '@/types'
import { fmtUsd } from '@/utils/trader-format'

const SEARCH_FROM = 8

// What the accounts hold, counting each account once however many traders share it: each
// trader stores the equity of the whole account, so adding them up would count it twice.
function accountEquity(
  traders: TraderInfo[],
  metrics: Record<string, TraderMetrics> | undefined
) {
  const latest = new Map<string, { equity: number; at: number }>()
  for (const trader of traders) {
    const row = metrics?.[trader.trader_id]
    if (row?.equity == null) continue
    const at = Number(row.equity_at) || 0
    const account = trader.account_id ?? trader.trader_id
    const known = latest.get(account)
    if (!known || at > known.at) latest.set(account, { equity: row.equity, at })
  }
  if (latest.size === 0) return null
  return [...latest.values()].reduce((sum, row) => sum + row.equity, 0)
}

// Every trader side by side: what they add up to, and a table to compare them by. A row
// opens that trader.
export function TradersOverviewPage() {
  const { language } = useLanguage()
  const { user, token } = useAuth()
  const label = (key: string) => t(`traderList.${key}`, language)
  const [search, setSearch] = useState('')
  const models = useModelConfigs()
  const accounts = useAccounts()
  const { data: traders, error } = useQuery({
    ...tradersQuery,
    enabled: Boolean(user && token),
  })
  const { data: metrics } = useQuery({
    ...traderMetricsQuery,
    enabled: Boolean(user && token),
  })
  const all = traders ?? []
  const equity = accountEquity(all, metrics)
  const closed = all.reduce(
    (sum, trader) => sum + (metrics?.[trader.trader_id]?.total_trades ?? 0),
    0
  )
  const running = all.filter((trader) => trader.is_running).length

  return (
    <TradersFrame language={language}>
      <Panel className="flex min-h-0 flex-col overflow-hidden lg:h-[max(440px,calc(100dvh-181px))]">
        {!traders ? (
          error ? (
            <PanelState role="alert">{label('loadFailed')}</PanelState>
          ) : (
            <PanelState role="status">{t('loading', language)}</PanelState>
          )
        ) : (
          <>
            <StatStrip
              stats={[
                {
                  name: label('runningOf'),
                  value: `${running} / ${all.length}`,
                },
                {
                  name: label('accountEquity'),
                  value: fmtUsd(equity ?? undefined),
                },
                { name: label('closedTrades'), value: closed },
              ]}
            />
            {all.length >= SEARCH_FROM && (
              <div className="shrink-0 border-b border-vergex-line p-2">
                <SearchField
                  label={label('search')}
                  value={search}
                  onChange={setSearch}
                  className="w-72 max-sm:w-full"
                />
              </div>
            )}
            <TradersList
              traders={traders}
              metrics={metrics}
              failed={false}
              models={models.data ?? []}
              accounts={accounts.data ?? []}
              modelCount={configuredModels(models.data ?? []).length}
              accountCount={configuredAccounts(accounts.data ?? []).length}
              search={search}
              language={language}
            />
          </>
        )}
      </Panel>
    </TradersFrame>
  )
}
