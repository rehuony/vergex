import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { EditableTitle, NameInput } from '@/components/common/name-editing'
import { Panel } from '@/components/common/panel'
import { SectionTabs } from '@/components/common/section-tabs'
import { ActionMenu, type RailAction } from '@/components/common/split-view'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  getAccountDisplayName,
  getTraderModelLabel,
} from '@/constants/model-providers'
import { t, type Language } from '@/i18n'
import type { Account, AgentModel, TraderInfo } from '@/types'
import { cn } from '@/utils/cn'
import { pollEvery } from '@/utils/poll'
import { parseScanMinutes } from '@/utils/trader-format'
import { DecisionFunnel } from './decision-funnel'
import { DecisionLog } from './decision-log'
import { EquityCurve } from './equity-curve'
import { GridRiskPanel } from './grid-risk-panel'
import { OpenPositions } from './open-positions'
import { CLOSES_PAGE_SIZE, RecentCloses } from './recent-closes'
import { TraderBreakdown } from './trader-breakdown'
import { TraderMetrics } from './trader-metrics'
import { TraderOrders } from './trader-orders'
import { TraderSettings } from './trader-settings'
import { TraderState } from './trader-state'
import { traderMetricsQuery } from './trader-queries'
import { useTraderData } from './use-trader-data'

export type TraderTab =
  | 'performance'
  | 'positions'
  | 'decisions'
  | 'orders'
  | 'settings'

export const readTraderTab = (value: string | null): TraderTab =>
  value === 'positions' ||
  value === 'decisions' ||
  value === 'orders' ||
  value === 'settings'
    ? value
    : 'performance'

// One trader, beside the list: how it stands and what can be done about it in a header
// that stays, and a tab of what it has done scrolling under that. It is mounted per
// trader, so a draft of its settings outlives moving between tabs.
export function TraderPanel({
  trader,
  tab,
  models,
  accounts,
  actions,
  renaming,
  stopping,
  language,
  onTab,
  onStartStop,
  onRenaming,
  onRename,
  onDirty,
}: {
  trader: TraderInfo
  tab: TraderTab
  models: AgentModel[]
  accounts: Account[]
  // Everything the list's menu has, for where the list is not shown.
  actions: RailAction[]
  renaming: boolean
  stopping: boolean
  language: Language
  onTab: (tab: TraderTab) => void
  onStartStop: () => void
  onRenaming: (renaming: boolean) => void
  onRename: (name: string) => void
  onDirty: (dirty: boolean) => void
}) {
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const id = trader.trader_id
  const running = Boolean(trader.is_running)
  const { status, account, positions } = useTraderData(id)
  // A draft of the settings outlives moving to another tab and back.
  const [settingsOpened, setSettingsOpened] = useState(false)
  if (tab === 'settings' && !settingsOpened) setSettingsOpened(true)

  const { data: storedMetrics } = useQuery(traderMetricsQuery)
  const { data: stats } = useQuery({
    queryKey: ['full-stats', id],
    queryFn: () => api.getFullStats(id, true),
    ...pollEvery(30000),
  })
  const { data: history, error: historyError } = useQuery({
    queryKey: ['pos-history', id],
    queryFn: () => api.getPositionHistory(id, CLOSES_PAGE_SIZE, true),
    ...pollEvery(60000),
  })
  const { data: config } = useQuery({
    queryKey: ['trader-config', id],
    queryFn: () => api.getTraderConfig(id, true),
    ...pollEvery(120000),
  })
  const { data: latest } = useQuery({
    queryKey: ['decisions-latest', id],
    queryFn: () => api.getLatestDecisions(id, 1, true),
    ...pollEvery(30000),
  })
  const latestDecision = latest?.[0]
  const scanMinutes =
    config?.scan_interval_minutes ||
    parseScanMinutes(status.data?.scan_interval)
  const nextCycleMs = latestDecision?.timestamp
    ? new Date(latestDecision.timestamp).getTime() + scanMinutes * 60_000
    : null

  const modelLabel = getTraderModelLabel(trader.agent_model, models)
  const accountLabel = getAccountDisplayName(trader.account_id, accounts)
  const kindLabel = t(
    trader.kind === 'grid' ? 'traderDialog.kindGrid' : 'traderDialog.kindAgent',
    language
  )
  const metaLink = 'hover:text-vergex-ink hover:underline'

  const body: Record<Exclude<TraderTab, 'settings'>, ReactNode> = {
    performance: (
      <>
        <TraderMetrics
          account={account.data}
          accountFailed={Boolean(account.error)}
          stored={storedMetrics?.[id]}
          stats={stats}
          positions={positions.data}
          running={status.data?.is_running}
          nextCycleMs={nextCycleMs}
          language={language}
        />
        <EquityCurve traderId={id} language={language} />
        {status.data?.kind === 'grid' && (
          <GridRiskPanel traderId={id} language={language} />
        )}
        <TraderBreakdown
          symbols={history?.symbol_stats}
          positions={history?.positions}
          language={language}
        />
      </>
    ),
    positions: (
      <>
        <OpenPositions
          traderId={id}
          positions={positions.data}
          failed={Boolean(positions.error)}
          language={language}
        />
        <RecentCloses
          traderId={id}
          trades={history?.positions}
          failed={Boolean(historyError)}
          language={language}
        />
      </>
    ),
    decisions: (
      <>
        <DecisionFunnel
          latest={latestDecision}
          positions={positions.data}
          language={language}
        />
        <DecisionLog traderId={id} language={language} />
      </>
    ),
    orders: (
      <TraderOrders
        traderId={id}
        traderName={trader.trader_name}
        language={language}
      />
    ),
  }

  return (
    <Panel
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
      title={
        <EditableTitle
          name={trader.trader_name}
          editing={renaming}
          renameLabel={t('traderList.rename', language)}
          onEdit={running ? undefined : () => onRenaming(true)}
          field={
            <NameInput
              value={trader.trader_name}
              label={t('traderList.traderName', language)}
              language={language}
              onSave={onRename}
              onCancel={() => onRenaming(false)}
              className="max-w-64 md:max-w-96"
            />
          }
        >
          <TraderState
            running={running}
            status={status.data}
            language={language}
          />
        </EditableTitle>
      }
      meta={
        <span
          className={cn('block truncate', renaming && 'invisible')}
          title={[modelLabel, accountLabel, kindLabel].join(' · ')}
        >
          <SettingsLink section="models" className={metaLink}>
            {modelLabel}
          </SettingsLink>
          {' · '}
          <SettingsLink section="accounts" className={metaLink}>
            {accountLabel}
          </SettingsLink>
          {' · '}
          {kindLabel}
        </span>
      }
      actions={
        <>
          <Button
            type="button"
            variant={running ? 'secondary' : 'primary'}
            size="sm"
            className="max-sm:h-10"
            disabled={stopping}
            onClick={onStartStop}
          >
            {running ? t('stop', language) : t('start', language)}
          </Button>
          <ActionMenu
            label={t('traderList.moreActions', language)}
            actions={actions}
            className="lg:hidden max-sm:size-10"
          />
        </>
      }
    >
      <SectionTabs<TraderTab>
        tabs={[
          { id: 'performance', label: label('performance') },
          { id: 'positions', label: label('positions') },
          { id: 'decisions', label: label('decisions') },
          { id: 'orders', label: label('orders') },
          { id: 'settings', label: label('settings') },
        ]}
        value={tab}
        onChange={onTab}
        label={label('tabs')}
        className="mb-0! shrink-0 px-2"
      />
      {tab !== 'settings' && (
        <ScrollArea
          key={tab}
          className="min-h-0 flex-1 rounded-b-2xl"
          viewportClassName="max-sm:overflow-x-auto"
        >
          {body[tab]}
        </ScrollArea>
      )}
      {settingsOpened && (
        <div
          hidden={tab !== 'settings'}
          className="flex min-h-0 flex-1 flex-col"
        >
          <TraderSettings
            traderId={id}
            running={running}
            language={language}
            onDirty={onDirty}
          />
        </div>
      )}
    </Panel>
  )
}
