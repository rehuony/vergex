import { useCallback, useRef, useState } from 'react'
import useSWR from 'swr'
import { api } from '@/api'
import type { ActivityEvent } from '@/api/activity'
import type { Overview } from '@/api/overview'
import { AppChrome } from '@/components/common/app-chrome'
import { Page } from '@/components/common/page'
import { Panel, PanelState } from '@/components/common/panel'
import { AttentionList } from '@/components/dashboard/attention-list'
import { FirstRun } from '@/components/dashboard/first-run'
import { FlowEvents, type FlowKind } from '@/components/dashboard/flow-events'
import { FlowGroups } from '@/components/dashboard/flow-groups'
import type { LineKey } from '@/components/dashboard/flow-layout'
import {
  FlowScene,
  type FlowSceneHandle,
} from '@/components/dashboard/flow-scene'
import type { FlowSelection } from '@/components/dashboard/flow-text'
import { PositionsOverview } from '@/components/dashboard/positions-overview'
import { useActivity } from '@/components/dashboard/use-activity'
import { useAuth } from '@/contexts/auth-context'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import { pollEvery } from '@/utils/poll'

function clock(at: number) {
  return new Date(at).toLocaleTimeString('en-GB', { hour12: false })
}

// The home page: a read-only view of the whole system, built around where things
// are going right now. Everything that changes something lives on the page a row
// leads to.
export function DashboardPage() {
  const { user, token } = useAuth()
  const { language } = useLanguage()
  const label = useCallback(
    (key: string, params?: Record<string, string | number>) =>
      t(`dashboard.${key}`, language, params),
    [language]
  )
  const signedIn = Boolean(user && token)
  const { data: overview, error } = useSWR<Overview>(
    signedIn ? 'overview' : null,
    () => api.getOverview(true),
    pollEvery(20000)
  )
  const scene = useRef<FlowSceneHandle>(null)
  const play = useCallback((events: ActivityEvent[]) => {
    scene.current?.play(events)
  }, [])
  const activity = useActivity(signedIn, play)
  const [kind, setKind] = useState<FlowKind>('all')
  const [selection, setSelection] = useState<FlowSelection | null>(null)
  const [pointed, setPointed] = useState<LineKey | null>(null)

  const staleAt = activity.stale
    ? activity.at
    : error && overview
      ? new Date(overview.generated_at).getTime()
      : 0
  const staleText = staleAt ? label('stale', { time: clock(staleAt) }) : ''

  return (
    <AppChrome currentPage="dashboard" animateContent>
      <Page title={t('dashboardNav', language)}>
        {!overview ? (
          <Panel>
            {error ? (
              <PanelState role="alert">{label('loadFailed')}</PanelState>
            ) : (
              <PanelState role="status">{t('loading', language)}</PanelState>
            )}
          </Panel>
        ) : overview.models.length === 0 || overview.accounts.length === 0 ? (
          <FirstRun overview={overview} label={label} />
        ) : (
          <div className="flex flex-col gap-3">
            {/* What needs attention shares the first screen with the scene. */}
            <div className="flex min-w-0 flex-col gap-3 xl:h-[max(620px,calc(100dvh-108px))]">
              <AttentionList overview={overview} language={language} />
              <div className="grid min-h-0 min-w-0 flex-1 gap-3 xl:grid-cols-[minmax(0,1fr)_380px]">
                <Panel className="min-h-0 overflow-hidden">
                  <div className="hidden h-full md:block">
                    <FlowScene
                      ref={scene}
                      overview={overview}
                      totals={activity.totals}
                      kind={kind}
                      selection={selection}
                      onSelect={setSelection}
                      pointed={pointed}
                      staleText={staleText}
                      label={label}
                    />
                  </div>
                  <div className="md:hidden">
                    <FlowGroups
                      overview={overview}
                      totals={activity.totals}
                      selection={selection}
                      onSelect={setSelection}
                      staleText={staleText}
                      label={label}
                    />
                  </div>
                </Panel>
                <Panel className="min-h-0 overflow-hidden">
                  <FlowEvents
                    overview={overview}
                    events={activity.events}
                    loaded={activity.loaded}
                    stale={activity.stale}
                    kind={kind}
                    onKind={setKind}
                    selection={selection}
                    onPoint={setPointed}
                    label={label}
                  />
                </Panel>
              </div>
            </div>
            <PositionsOverview overview={overview} language={language} />
          </div>
        )}
      </Page>
    </AppChrome>
  )
}
